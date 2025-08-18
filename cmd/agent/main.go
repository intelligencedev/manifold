package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"

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
	"singularityio/internal/tools/fs"
	llmtools "singularityio/internal/tools/llmtool"
	specialists_tool "singularityio/internal/tools/specialists"
	"singularityio/internal/tools/web"
	"singularityio/internal/warpp"
)

func main() {
	q := flag.String("q", "", "User request")
	maxSteps := flag.Int("max-steps", 8, "Max reasoning steps")
	warppFlag := flag.Bool("warpp", false, "Run WARPP workflow instead of LLM agent")
	specialist := flag.String("specialist", "", "Name of specialist agent to use (inference-only; no tool calls unless enabled)")
	flag.Parse()
	if *q == "" {
		fmt.Fprintln(os.Stderr, "usage: agent -q \"...\"")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config")
	}

	observability.InitLogger(cfg.LogPath, cfg.LogLevel)
	log.Info().Msg("agent starting")
	shutdown, _ := observability.InitOTel(context.Background(), cfg.Obs)
	defer func() { _ = shutdown(context.Background()) }()

	httpClient := observability.NewHTTPClient(nil)
	// Inject global headers for main agent if configured
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}
	llm := openaillm.New(cfg.OpenAI, httpClient)

	// If a specialist was requested, route the query directly and exit.
	if *specialist != "" {
		specReg := specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, httpClient, nil)
		a, ok := specReg.Get(*specialist)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown specialist %q. Available: %v\n", *specialist, specReg.Names())
			os.Exit(2)
		}
		log.Info().Str("specialist", *specialist).Msg("direct specialist invocation")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		out, err := a.Inference(ctx, *q, nil)
		if err != nil {
			log.Fatal().Err(err).Msg("specialist")
		}
		fmt.Println(out)
		return
	}

	registry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	// Databases: construct backends and register tools
	mgr, err := databases.NewManager(context.Background(), cfg.Databases)
	if err != nil {
		log.Fatal().Err(err).Msg("databases")
	}
	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	registry.Register(cli.NewTool(exec))                 // provides run_cli
	registry.Register(web.NewTool(cfg.Web.SearXNGURL))   // provides web_search
	registry.Register(web.NewFetchTool())                // provides web_fetch
	registry.Register(fs.NewWriteTool(cfg.Workdir))      // provides write_file
	registry.Register(fs.NewApplyPatchTool(cfg.Workdir)) // provides apply_patch
	registry.Register(fs.NewReadTool(cfg.Workdir))       // provides read_file

	// DB tools
	registry.Register(db.NewSearchIndexTool(mgr.Search))
	registry.Register(db.NewSearchQueryTool(mgr.Search))
	registry.Register(db.NewSearchRemoveTool(mgr.Search))
	registry.Register(db.NewVectorUpsertTool(mgr.Vector))
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
	registry.Register(llmtools.NewTransform(llm, cfg.OpenAI.Model, newProv)) // provides llm_transform
	// Specialists tool for LLM-driven routing
	specReg := specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, httpClient, registry)
	registry.Register(specialists_tool.New(specReg))

	// MCP: connect to configured servers and register their tools
	mcpMgr := mcpclient.NewManager()
	ctxInit, cancelInit := context.WithTimeout(context.Background(), 20*time.Second)
	_ = mcpMgr.RegisterFromConfig(ctxInit, registry, cfg.MCP)
	cancelInit()

	// WARPP mode: run the WARPP workflow executor instead of the LLM loop
	if *warppFlag {
		wfreg, _ := warpp.LoadFromDir("configs/workflows")
		runner := &warpp.Runner{Workflows: wfreg, Tools: registry}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		intent := runner.DetectIntent(ctx, *q)
		wf, _ := wfreg.Get(intent)
		attrs := warpp.Attrs{"utter": *q}
		wfStar, _, attrs, err := runner.Personalize(ctx, wf, attrs)
		if err != nil {
			log.Fatal().Err(err).Msg("personalize")
		}
		allow := map[string]bool{}
		for _, s := range wfStar.Steps {
			if s.Tool != nil {
				allow[s.Tool.Name] = true
			}
		}
		final, err := runner.Execute(ctx, wfStar, allow, attrs)
		if err != nil {
			log.Fatal().Err(err).Msg("warpp")
		}
		fmt.Println(final)
		return
	}

	// Pre-dispatch routing: call a specialist directly if there's a match.
	if name := specialists.Route(cfg.SpecialistRoutes, *q); name != "" {
		log.Info().Str("route", name).Msg("pre-dispatch specialist route matched")
		a, ok := specReg.Get(name)
		if !ok {
			log.Error().Str("route", name).Msg("specialist not found for route")
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			out, err := a.Inference(ctx, *q, nil)
			if err != nil {
				log.Fatal().Err(err).Msg("specialist pre-dispatch")
			}
			fmt.Println(out)
			return
		}
	}

	eng := agent.Engine{
		LLM:              llm,
		Tools:            registry,
		MaxSteps:         *maxSteps,
		System:           prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt),
		SummaryEnabled:   cfg.SummaryEnabled,
		SummaryThreshold: cfg.SummaryThreshold,
		SummaryKeepLast:  cfg.SummaryKeepLast,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	final, err := eng.Run(ctx, *q, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("agent")
	}
	fmt.Println(final)
}
