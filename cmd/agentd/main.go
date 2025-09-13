package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/rs/zerolog/log"

	"github.com/joho/godotenv"

	"intelligence.dev/internal/agent"
	"intelligence.dev/internal/agent/prompts"
	"intelligence.dev/internal/config"
	llmpkg "intelligence.dev/internal/llm"
	openaillm "intelligence.dev/internal/llm/openai"
	"intelligence.dev/internal/mcpclient"
	"intelligence.dev/internal/observability"
	"intelligence.dev/internal/persistence/databases"
	"intelligence.dev/internal/specialists"
	"intelligence.dev/internal/tools"
	"intelligence.dev/internal/tools/cli"
	"intelligence.dev/internal/tools/db"
	"intelligence.dev/internal/tools/imagetool"
	llmtools "intelligence.dev/internal/tools/llmtool"
	specialists_tool "intelligence.dev/internal/tools/specialists"
	"intelligence.dev/internal/tools/tts"
	"intelligence.dev/internal/tools/web"
	"intelligence.dev/internal/warpp"
	"intelligence.dev/internal/webui"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

type sessionStore struct {
	histories map[string][]llmpkg.Message
	mu        sync.RWMutex
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		histories: make(map[string][]llmpkg.Message),
	}
}

func (s *sessionStore) getHistory(sessionID string) []llmpkg.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if history, ok := s.histories[sessionID]; ok {
		return history
	}
	return nil
}

func (s *sessionStore) addToHistory(sessionID string, userMsg, assistantMsg llmpkg.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.histories[sessionID] = append(s.histories[sessionID], userMsg, assistantMsg)
}

func main() {
	// Load environment from .env (or fallback to example.env) so local
	// development can run without exporting variables manually. Do this
	// before initializing the logger so LOG_PATH/LOG_LEVEL are respected.
	if err := godotenv.Load(".env"); err != nil {
		_ = godotenv.Load("example.env")
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Initialize logger next (after .env has been loaded)
	observability.InitLogger(cfg.LogPath, cfg.LogLevel)

	shutdown, err := observability.InitOTel(context.Background(), cfg.Obs)
	if err != nil {
		// don't abort startup for observability failures; log and continue
		log.Warn().Err(err).Msg("otel init failed, continuing without observability")
		shutdown = nil
	}
	if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	httpClient := observability.NewHTTPClient(nil)
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}
	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.OutputTruncateByte)
	llm := openaillm.New(cfg.OpenAI, httpClient)

	registry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	// Databases: construct backends and register tools
	mgr, err := databases.NewManager(context.Background(), cfg.Databases)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init databases")
	}
	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	registry.Register(cli.NewTool(exec))
	registry.Register(web.NewTool(cfg.Web.SearXNGURL))
	registry.Register(web.NewFetchTool())
	registry.Register(tts.New(cfg, httpClient))
	registry.Register(db.NewSearchIndexTool(mgr.Search))
	registry.Register(db.NewSearchQueryTool(mgr.Search))
	registry.Register(db.NewSearchRemoveTool(mgr.Search))
	registry.Register(db.NewVectorUpsertTool(mgr.Vector, cfg.Embedding))
	registry.Register(db.NewVectorQueryTool(mgr.Vector))
	registry.Register(db.NewVectorDeleteTool(mgr.Vector))
	registry.Register(db.NewGraphUpsertNodeTool(mgr.Graph))
	registry.Register(db.NewGraphUpsertEdgeTool(mgr.Graph))
	registry.Register(db.NewGraphNeighborsTool(mgr.Graph))
	registry.Register(db.NewGraphGetNodeTool(mgr.Graph))
	// Provider factory for base_url override in llm_transform
	newProv := func(baseURL string) llmpkg.Provider {
		c2 := cfg.OpenAI
		c2.BaseURL = baseURL
		return openaillm.New(c2, httpClient)
	}
	registry.Register(llmtools.NewTransform(llm, cfg.OpenAI.Model, newProv))
	registry.Register(imagetool.NewDescribeTool(llm, cfg.Workdir, cfg.OpenAI.Model, newProv))
	// Specialists tool for LLM-driven routing
	specReg := specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, httpClient, registry)
	registry.Register(specialists_tool.New(specReg))

	// If tools are globally disabled, use an empty registry
	if !cfg.EnableTools {
		registry = tools.NewRegistry() // Empty registry
	} else if len(cfg.ToolAllowList) > 0 {
		// If a top-level tool allow-list is configured, expose only those tools
		// to the main orchestrator agent by wrapping the registry.
		registry = tools.NewFilteredRegistry(registry, cfg.ToolAllowList)
	}

	// Debug: log which tools are exposed after any filtering so we can diagnose
	// missing tool registrations at runtime.
	{
		names := make([]string, 0, len(registry.Schemas()))
		for _, s := range registry.Schemas() {
			names = append(names, s.Name)
		}
		log.Info().Bool("enableTools", cfg.EnableTools).Strs("allowList", cfg.ToolAllowList).Strs("tools", names).Msg("tool_registry_contents")
	}

	// MCP: connect to configured servers and register their tools
	mcpMgr := mcpclient.NewManager()
	ctxInit, cancelInit := context.WithTimeout(context.Background(), 20*time.Second)
	_ = mcpMgr.RegisterFromConfig(ctxInit, registry, cfg.MCP)
	cancelInit()

	// WARPP runner for workflow execution
	wfreg, _ := warpp.LoadFromDir("configs/workflows")
	warppRunner := &warpp.Runner{Workflows: wfreg, Tools: registry}

	eng := &agent.Engine{
		LLM:              llm,
		Tools:            registry,
		MaxSteps:         cfg.MaxSteps,
		System:           prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt),
		Model:            cfg.OpenAI.Model,
		SummaryEnabled:   cfg.SummaryEnabled,
		SummaryThreshold: cfg.SummaryThreshold,
		SummaryKeepLast:  cfg.SummaryKeepLast,
	}

	// Initialize session store for conversation history
	sessions := newSessionStore()

	// Initialize Whisper model for speech-to-text (optional – if model file present)
	var whisperModel whisper.Model
	modelPath := "models/ggml-small.en.bin"
	if model, err := whisper.New(modelPath); err == nil {
		whisperModel = model
		log.Info().Str("model", modelPath).Msg("whisper model loaded")
	} else {
		log.Warn().Str("model", modelPath).Err(err).Msg("whisper model load failed; /stt disabled")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ready")
	})

	// Simple in-process metrics endpoint for web UI token graph.
	mux.HandleFunc("/api/metrics/tokens", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"timestamp": time.Now().Unix(), "models": llmpkg.TokenTotalsSnapshot()})
	})

	mux.HandleFunc("/agent/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Prompt    string `json:"prompt"`
			SessionID string `json:"session_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Use default session if not provided
		if req.SessionID == "" {
			req.SessionID = "default"
		}

		// Get conversation history for this session
		history := sessions.getHistory(req.SessionID)

		// Pre-dispatch routing: call a specialist directly if there's a match.
		if name := specialists.Route(cfg.SpecialistRoutes, req.Prompt); name != "" {
			log.Info().Str("route", name).Msg("pre-dispatch specialist route matched")
			a, ok := specReg.Get(name)
			if !ok {
				log.Error().Str("route", name).Msg("specialist not found for route")
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				out, err := a.Inference(ctx, req.Prompt, nil)
				if err != nil {
					log.Error().Err(err).Msg("specialist pre-dispatch")
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"result": out})
				return
			}
		}

		// WARPP mode: run the WARPP workflow executor instead of the LLM loop
		if r.URL.Query().Get("warpp") == "true" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			intent := warppRunner.DetectIntent(ctx, req.Prompt)
			wf, _ := wfreg.Get(intent)
			attrs := warpp.Attrs{"utter": req.Prompt}
			wfStar, _, attrs, err := warppRunner.Personalize(ctx, wf, attrs)
			if err != nil {
				log.Error().Err(err).Msg("personalize")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			allow := map[string]bool{}
			for _, s := range wfStar.Steps {
				if s.Tool != nil {
					allow[s.Tool.Name] = true
				}
			}
			final, err := warppRunner.Execute(ctx, wfStar, allow, attrs, nil)
			if err != nil {
				log.Error().Err(err).Msg("warpp")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"result": final})
			return
		}

		// If no OpenAI API key is configured, return a deterministic dev response
		// so the web UI can be exercised locally without external credentials.
		if cfg.OpenAI.APIKey == "" {
			// Support SSE if requested
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fl, _ := w.(http.Flusher)
				if b, err := json.Marshal("(dev) mock response: " + req.Prompt); err == nil {
					fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock response")
				}
				fl.Flush()
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock response: " + req.Prompt})
			return
		}

		// If client requested SSE, use streaming RunStream and proxy deltas/tool events
		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			// Wire up engine callbacks to write SSE events.
			// delta -> incremental assistant text
			eng.OnDelta = func(d string) {
				payload := map[string]string{"type": "delta", "data": d}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			// tool start -> announce tool invocation early (no result yet)
			eng.OnToolStart = func(name string, args []byte, toolID string) {
				payload := map[string]any{"type": "tool_start", "title": "Tool: " + name, "tool_id": toolID, "args": string(args)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			// tool result -> append results (and emit specialized tts_audio event for TTS playback)
			eng.OnTool = func(name string, args []byte, result []byte) {
				// Stream chunk event
				if name == "text_to_speech_chunk" {
					var meta map[string]any
					_ = json.Unmarshal(result, &meta)
					metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
					b, _ := json.Marshal(metaPayload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
					return
				}
				payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()

				if name == "text_to_speech" {
					var resp map[string]any
					if err := json.Unmarshal(result, &resp); err == nil {
						if fp, ok := resp["file_path"].(string); ok && fp != "" {
							trimmed := strings.TrimPrefix(fp, "./")
							trimmed = strings.TrimPrefix(trimmed, "/")
							url := "/audio/" + trimmed
							ap := map[string]any{"type": "tts_audio", "file_path": fp, "url": url}
							if bb, err2 := json.Marshal(ap); err2 == nil {
								fmt.Fprintf(w, "data: %s\n\n", bb)
								fl.Flush()
							}
						}
					}
				}
			}

			// Run streaming engine
			res, err := eng.RunStream(ctx, req.Prompt, nil)
			if err != nil {
				log.Error().Err(err).Msg("agent run error")
				if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
					fmt.Fprintf(w, "data: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "data: %q\n\n", "(error)")
				}
				fl.Flush()
				return
			}
			// send final event
			payload := map[string]string{"type": "final", "data": res}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			return
		}

		// Non-streaming path
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, err := eng.Run(ctx, req.Prompt, history)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})

		// Add to conversation history
		userMsg := llmpkg.Message{Role: "user", Content: req.Prompt}
		assistantMsg := llmpkg.Message{Role: "assistant", Content: result}
		sessions.addToHistory(req.SessionID, userMsg, assistantMsg)
	})

	// POST /api/prompt accepts {"prompt":"..."} and runs the agent (for web UI compatibility)
	mux.HandleFunc("/api/prompt", func(w http.ResponseWriter, r *http.Request) {
		// Basic CORS support
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// limit body to 64KB
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()

		var req struct {
			Prompt    string `json:"prompt"`
			SessionID string `json:"session_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("decode prompt: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Use default session if not provided
		if req.SessionID == "" {
			req.SessionID = "default"
		}

		// Get conversation history for this session
		history := sessions.getHistory(req.SessionID)

		// If no OpenAI API key is configured, return a deterministic dev response
		if cfg.OpenAI.APIKey == "" {
			// Support SSE if requested
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fl, _ := w.(http.Flusher)
				if b, err := json.Marshal("(dev) mock response: " + req.Prompt); err == nil {
					fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock response")
				}
				fl.Flush()
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock response: " + req.Prompt})
			return
		}

		// If client requested SSE, use streaming RunStream and proxy deltas/tool events
		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			// Wire up engine callbacks to write SSE events (duplicate path for /api/prompt).
			eng.OnDelta = func(d string) {
				payload := map[string]string{"type": "delta", "data": d}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnToolStart = func(name string, args []byte, toolID string) {
				payload := map[string]any{"type": "tool_start", "title": "Tool: " + name, "tool_id": toolID, "args": string(args)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnTool = func(name string, args []byte, result []byte) {
				if name == "text_to_speech_chunk" {
					var meta map[string]any
					_ = json.Unmarshal(result, &meta)
					metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
					b, _ := json.Marshal(metaPayload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
					return
				}
				payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
				if name == "text_to_speech" {
					var resp map[string]any
					if err := json.Unmarshal(result, &resp); err == nil {
						if fp, ok := resp["file_path"].(string); ok && fp != "" {
							trimmed := strings.TrimPrefix(fp, "./")
							trimmed = strings.TrimPrefix(trimmed, "/")
							url := "/audio/" + trimmed
							ap := map[string]any{"type": "tts_audio", "file_path": fp, "url": url}
							if bb, err2 := json.Marshal(ap); err2 == nil {
								fmt.Fprintf(w, "data: %s\n\n", bb)
								fl.Flush()
							}
						}
					}
				}
			}

			// Run streaming engine
			res, err := eng.RunStream(ctx, req.Prompt, nil)
			if err != nil {
				log.Error().Err(err).Msg("agent run error")
				if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
					fmt.Fprintf(w, "data: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "data: %q\n\n", "(error)")
				}
				fl.Flush()
				return
			}
			// send final event
			payload := map[string]string{"type": "final", "data": res}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			return
		}

		// Non-streaming path
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, err := eng.Run(ctx, req.Prompt, history)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
	})

	// Serve static files under /static/
	fs := http.FS(webui.Assets)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(fs)))

	// Serve assets (images, gifs) for avatar panel and future UI decoration
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	// Serve TTS audio files
	mux.HandleFunc("/audio/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Extract filename from path
		filename := strings.TrimPrefix(r.URL.Path, "/audio/")
		if filename == "" {
			http.Error(w, "file not specified", http.StatusBadRequest)
			return
		}
		// Serve the file from the working directory (where TTS saves files)
		http.ServeFile(w, r, filename)
	})

	// POST /stt accepts multipart/form-data with field "audio" (WAV 16kHz mono or stereo) and returns {text: "..."}
	mux.HandleFunc("/stt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if whisperModel == nil { // model failed to load
			http.Error(w, "whisper model unavailable", http.StatusServiceUnavailable)
			return
		}
		// 32MB max upload
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("audio")
		if err != nil {
			http.Error(w, "missing audio", http.StatusBadRequest)
			return
		}
		defer file.Close()
		// Read whole file (size bounded by ParseMultipartForm limit)
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		// Minimal WAV parsing (reuse logic similar to whisper-go example)
		if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
			http.Error(w, "unsupported audio (expect WAV)", http.StatusBadRequest)
			return
		}
		// Extract format fields
		channels := binary.LittleEndian.Uint16(data[22:24])
		sampleRate := binary.LittleEndian.Uint32(data[24:28])
		bitsPerSample := binary.LittleEndian.Uint16(data[34:36])
		// Find "data" chunk (simple scan)
		offset := 12
		var audioStart, audioLen int
		for offset+8 <= len(data) {
			chunkID := string(data[offset : offset+4])
			chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			if chunkID == "data" {
				audioStart = offset + 8
				audioLen = chunkSize
				break
			}
			offset += 8 + chunkSize
		}
		if audioLen == 0 || audioStart+audioLen > len(data) {
			http.Error(w, "invalid wav data", http.StatusBadRequest)
			return
		}
		raw := data[audioStart : audioStart+audioLen]
		var samples []float32
		switch bitsPerSample {
		case 16:
			for i := 0; i+1 < len(raw); i += 2 {
				sample := int16(binary.LittleEndian.Uint16(raw[i : i+2]))
				samples = append(samples, float32(sample)/32768.0)
			}
		case 32:
			for i := 0; i+3 < len(raw); i += 4 {
				bits := binary.LittleEndian.Uint32(raw[i : i+4])
				f := *(*float32)(unsafe.Pointer(&bits))
				samples = append(samples, f)
			}
		default:
			http.Error(w, "unsupported bit depth", http.StatusBadRequest)
			return
		}
		if channels == 2 { // stereo -> mono
			mono := make([]float32, len(samples)/2)
			for i := 0; i < len(mono); i++ {
				mono[i] = (samples[i*2] + samples[i*2+1]) / 2
			}
			samples = mono
		}
		if sampleRate != 16000 {
			log.Warn().Uint32("rate", sampleRate).Msg("non-16k audio provided; transcription may be degraded")
		}
		ctx, err := whisperModel.NewContext()
		if err != nil {
			http.Error(w, "ctx error", http.StatusInternalServerError)
			return
		}
		ctx.SetLanguage("en")
		if err := ctx.Process(samples, nil, nil, nil); err != nil {
			http.Error(w, "process error", http.StatusInternalServerError)
			return
		}
		var sb strings.Builder
		for {
			seg, err := ctx.NextSegment()
			if err != nil {
				break
			}
			sb.WriteString(seg.Text)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"text": strings.TrimSpace(sb.String())})
	})

	// Serve index on /
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		f, err := webui.Assets.Open("templates/index.html")
		if err != nil {
			log.Printf("open index: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := io.Copy(w, f); err != nil {
			log.Printf("copy index: %v", err)
		}
	})

	log.Info().Msg("agentd listening on :32180")
	if err := http.ListenAndServe(":32180", mux); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

func formatToolPayload(cmd string, args []string, res cli.ExecResult) string {
	var b strings.Builder
	if cmd != "" {
		b.WriteString(fmt.Sprintf("$ %s %s\n", cmd, strings.Join(args, " ")))
	}
	b.WriteString(fmt.Sprintf("exit %d | ok=%v | %dms\n", res.ExitCode, res.OK, res.Duration))
	if res.Truncated {
		b.WriteString("(output truncated)\n")
	}
	if strings.TrimSpace(res.Stdout) != "" {
		b.WriteString("\nstdout:\n")
		b.WriteString(res.Stdout)
	}
	if strings.TrimSpace(res.Stderr) != "" {
		b.WriteString("\nstderr:\n")
		b.WriteString(res.Stderr)
	}
	return b.String()
}
