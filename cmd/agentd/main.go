package main

import (
	"context"
	"encoding/json"
	"fmt"
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
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock response: " + req.Prompt})
			return
		}

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

	log.Info().Msg("agentd listening on :32180")
	if err := http.ListenAndServe(":32180", mux); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
