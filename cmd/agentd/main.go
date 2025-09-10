package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/joho/godotenv"

	"singularityio/internal/agent"
	"singularityio/internal/agent/prompts"
	"singularityio/internal/config"
	llmpkg "singularityio/internal/llm"
	openaillm "singularityio/internal/llm/openai"
	"singularityio/internal/mcpclient"
	"singularityio/internal/observability"
	"singularityio/internal/persistence/databases"
	"singularityio/internal/specialists"
	"singularityio/internal/tools"
	"singularityio/internal/tools/cli"
	"singularityio/internal/tools/db"
	"singularityio/internal/tools/imagetool"
	llmtools "singularityio/internal/tools/llmtool"
	specialists_tool "singularityio/internal/tools/specialists"
	"singularityio/internal/tools/tts"
	"singularityio/internal/tools/web"
	"singularityio/internal/warpp"
	"singularityio/internal/webui"
	// "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
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
		SummaryEnabled:   cfg.SummaryEnabled,
		SummaryThreshold: cfg.SummaryThreshold,
		SummaryKeepLast:  cfg.SummaryKeepLast,
	}

	// Initialize session store for conversation history
	sessions := newSessionStore()

	// Initialize Whisper model for speech-to-text
	// var whisperModel whisper.Model
	// var whisperCtx whisper.Context
	// modelPath := "models/ggml-small.en.bin"
	// if model, err := whisper.New(modelPath); err == nil {
	// 	whisperModel = model
	// 	if ctx, err := model.NewContext(); err == nil {
	// 		whisperCtx = ctx
	// 		ctx.SetLanguage("en")
	// 	} else {
	// 		log.Warn().Err(err).Msg("Whisper context creation failed")
	// 	}
	// } else {
	// 	log.Warn().Err(err).Msg("Whisper model load failed")
	// }

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ready")
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

			// Wire up engine callbacks to write SSE events
			eng.OnDelta = func(d string) {
				// send delta event
				payload := map[string]string{"type": "delta", "data": d}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnTool = func(name string, args []byte, result []byte) {
				// send tool event
				payload := map[string]string{"type": "tool", "title": "Tool: " + name, "data": string(result)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
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

			// Wire up engine callbacks to write SSE events
			eng.OnDelta = func(d string) {
				// send delta event
				payload := map[string]string{"type": "delta", "data": d}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnTool = func(name string, args []byte, result []byte) {
				// send tool event
				payload := map[string]string{"type": "tool", "title": "Tool: " + name, "data": string(result)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
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

	// POST /stt accepts audio file uploads and returns transcribed text
	// mux.HandleFunc("/stt", func(w http.ResponseWriter, r *http.Request) {
	// 	if r.Method != http.MethodPost {
	// 		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	// 		return
	// 	}

	// 	if whisperModel == nil || whisperCtx == nil {
	// 		http.Error(w, "Whisper not available", http.StatusServiceUnavailable)
	// 		return
	// 	}

	// 	// Parse multipart form
	// 	err := r.ParseMultipartForm(32 << 20) // 32MB max
	// 	if err != nil {
	// 		http.Error(w, "failed to parse form", http.StatusBadRequest)
	// 		return
	// 	}

	// 	file, _, err := r.FormFile("audio")
	// 	if err != nil {
	// 		http.Error(w, "failed to get audio file", http.StatusBadRequest)
	// 		return
	// 	}
	// 	defer file.Close()

	// 	// Read audio data
	// 	audioData, err := io.ReadAll(file)
	// 	if err != nil {
	// 		http.Error(w, "failed to read audio data", http.StatusInternalServerError)
	// 		return
	// 	}

	// 	// Process with Whisper
	// 	// For simplicity, assume 16kHz WAV format and convert to float samples
	// 	// In a real implementation, you'd need proper audio format handling
	// 	samples := make([]float32, len(audioData)/2)
	// 	for i := 0; i < len(samples); i++ {
	// 		sample := int16(audioData[i*2]) | int16(audioData[i*2+1])<<8
	// 		samples[i] = float32(sample) / 32768.0
	// 	}

	// 	err = whisperCtx.Process(samples, nil, nil, nil)
	// 	if err != nil {
	// 		http.Error(w, "Whisper processing failed", http.StatusInternalServerError)
	// 		return
	// 	}

	// 	// Extract text
	// 	var text strings.Builder
	// 	for {
	// 		segment, err := whisperCtx.NextSegment()
	// 		if err != nil {
	// 			break
	// 		}
	// 		text.WriteString(segment.Text)
	// 	}

	// 	transcribedText := strings.TrimSpace(text.String())
	// 	w.Header().Set("Content-Type", "application/json")
	// 	json.NewEncoder(w).Encode(map[string]string{"text": transcribedText})
	// })

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
