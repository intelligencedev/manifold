package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/joho/godotenv"

	"singularityio/internal/agent"
	"singularityio/internal/agent/prompts"
	"singularityio/internal/config"
	llmpkg "singularityio/internal/llm"
	openaillm "singularityio/internal/llm/openai"
	"singularityio/internal/observability"
	"singularityio/internal/tools"
	"singularityio/internal/tools/cli"
	"singularityio/internal/tools/tts"
	"singularityio/internal/tools/web"
)

//go:embed templates/*
var assets embed.FS

func main() {
	// Load environment from .env (or fallback to example.env) so local
	// development can run without exporting variables manually. Do this
	// before initializing the logger so LOG_PATH/LOG_LEVEL are respected.
	if err := godotenv.Load(".env"); err != nil {
		_ = godotenv.Load("example.env")
	}

	// Initialize logger next (after .env has been loaded)
	observability.InitLogger("sio.log", "trace")

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		log.Fatal().Err(err).Msg("failed to load config")
	}

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
	// mgr, err := databases.NewManager(context.Background(), cfg.Databases)
	// if err != nil {
	// 	log.Fatal().Err(err).Msg("failed to init databases")
	// }
	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	registry.Register(cli.NewTool(exec))
	registry.Register(web.NewTool(cfg.Web.SearXNGURL))
	registry.Register(web.NewFetchTool())
	registry.Register(tts.New(cfg, httpClient))
	// registry.Register(db.NewSearchIndexTool(mgr.Search))
	// registry.Register(db.NewSearchQueryTool(mgr.Search))
	// registry.Register(db.NewSearchRemoveTool(mgr.Search))
	// registry.Register(db.NewVectorUpsertTool(mgr.Vector, cfg.Embedding))
	// registry.Register(db.NewVectorQueryTool(mgr.Vector))
	// registry.Register(db.NewVectorDeleteTool(mgr.Vector))

	eng := &agent.Engine{
		LLM:              llm,
		Tools:            registry,
		MaxSteps:         8,
		System:           prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt),
		SummaryEnabled:   cfg.SummaryEnabled,
		SummaryThreshold: cfg.SummaryThreshold,
		SummaryKeepLast:  cfg.SummaryKeepLast,
	}

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
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
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
		result, err := eng.Run(ctx, req.Prompt, nil)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
	})

	// Serve static files under /static/
	fs := http.FS(assets)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(fs)))

	// Serve index on /
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		f, err := assets.Open("templates/index.html")
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
