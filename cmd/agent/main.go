package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/prompts"
	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
	openaillm "manifold/internal/llm/openai"
	"manifold/internal/mcpclient"
	"manifold/internal/observability"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	"manifold/internal/tools/db"
	llmtools "manifold/internal/tools/llmtool"
	"manifold/internal/tools/patchtool"
	"manifold/internal/tools/textsplitter"
	specialists_tool "manifold/internal/tools/specialists"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"
)

func main() {
	// Load config first to get defaults
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config")
	}

	q := flag.String("q", "", "User request")
	maxSteps := flag.Int("max-steps", cfg.MaxSteps, "Max reasoning steps")
	warppFlag := flag.Bool("warpp", false, "Run WARPP workflow instead of LLM agent")
	specialist := flag.String("specialist", "", "Name of specialist agent to use (inference-only; no tool calls unless enabled)")
	flag.Parse()
	if *q == "" {
		fmt.Fprintln(os.Stderr, "usage: agent -q \"...\"")
		os.Exit(2)
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
	// Configure global llm payload logging/truncation before creating providers
	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.OutputTruncateByte)
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
	registry.Register(cli.NewTool(exec))               // provides run_cli
	registry.Register(web.NewTool(cfg.Web.SearXNGURL)) // provides web_search
	registry.Register(web.NewFetchTool(mgr.Search))    // provides web_fetch
	// Patch application tool (unified diff)
	registry.Register(patchtool.New(cfg.Workdir)) // provides apply_patch
	// Text splitting tool (RAG ingestion helpers)
	registry.Register(textsplitter.New()) // provides split_text
	registry.Register(utility.NewTextboxTool())
	// TTS tool
	registry.Register(tts.New(cfg, httpClient))

	// DB tools
	registry.Register(db.NewSearchIndexTool(mgr.Search))
	registry.Register(db.NewSearchQueryTool(mgr.Search))
	registry.Register(db.NewSearchRemoveTool(mgr.Search))
	registry.Register(db.NewVectorUpsertTool(mgr.Vector, cfg.Embedding))
	registry.Register(db.NewVectorQueryTool(mgr.Vector))
	registry.Register(db.NewVectorDeleteTool(mgr.Vector))
	// Orchestration DB tools
	registry.Register(db.NewHybridQueryTool(mgr.Search, mgr.Vector, cfg.Embedding))
	registry.Register(db.NewIndexDocumentTool(mgr.Search, mgr.Vector, cfg.Embedding))
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

	// WARPP mode: run the WARPP workflow executor instead of the LLM loop
	if *warppFlag {
		wfreg, _ := warpp.LoadFromStore(context.Background(), mgr.Warpp)
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
		final, err := runner.Execute(ctx, wfStar, allow, attrs, nil)
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

	// Global agent run context: honor configurable timeout; 0 => no deadline.
	var ctx context.Context
	var cancel context.CancelFunc
	if cfg.AgentRunTimeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(cfg.AgentRunTimeoutSeconds)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	final, err := eng.Run(ctx, *q, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("agent")
	}
	fmt.Println(final)
}
