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
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/mcpclient"
	"manifold/internal/observability"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	llmtools "manifold/internal/tools/llmtool"
	"manifold/internal/tools/patchtool"
	specialists_tool "manifold/internal/tools/specialists"
	"manifold/internal/tools/textsplitter"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"

	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const systemUserID int64 = 0

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

	// Initialize specialists store and apply DB-backed overrides so the CLI
	// mirrors agentd behavior (specialists and orchestrator loaded from DB).
	var specStore persist.SpecialistsStore
	{
		var pg *pgxpool.Pool
		if cfg.Databases.DefaultDSN != "" {
			if p, err := databasesTestPool(context.Background(), cfg.Databases.DefaultDSN); err == nil {
				pg = p
			}
		}
		specStore = databases.NewSpecialistsStore(pg)
		_ = specStore.Init(context.Background())
		// Seed from YAML only if the store is empty or missing entries
		if list, err := specStore.List(context.Background(), systemUserID); err == nil {
			existing := map[string]bool{}
			for _, s := range list {
				existing[s.Name] = true
			}
			for _, sc := range cfg.Specialists {
				if sc.Name == "" {
					continue
				}
				if existing[sc.Name] {
					continue
				}
				_, _ = specStore.Upsert(context.Background(), systemUserID, persist.Specialist{
					Name: sc.Name, Description: sc.Description, BaseURL: sc.BaseURL, APIKey: sc.APIKey, Model: sc.Model,
					EnableTools: sc.EnableTools, Paused: sc.Paused, AllowTools: sc.AllowTools,
					ReasoningEffort: sc.ReasoningEffort, System: sc.System,
					ExtraHeaders: sc.ExtraHeaders, ExtraParams: sc.ExtraParams,
				})
			}
		}
		// Load orchestrator configuration from the database (if present)
		if sp, ok, _ := specStore.GetByName(context.Background(), systemUserID, "orchestrator"); ok {
			cfg.OpenAI.BaseURL = sp.BaseURL
			cfg.OpenAI.APIKey = sp.APIKey
			if strings.TrimSpace(sp.Model) != "" {
				cfg.OpenAI.Model = sp.Model
			}
			cfg.EnableTools = sp.EnableTools
			cfg.ToolAllowList = append([]string(nil), sp.AllowTools...)
			if strings.TrimSpace(sp.System) != "" {
				cfg.SystemPrompt = sp.System
			} else {
				cfg.SystemPrompt = "You are a helpful assistant with access to tools and specialists to help you complete objectives."
			}
			if sp.ExtraHeaders != nil {
				cfg.OpenAI.ExtraHeaders = sp.ExtraHeaders
			}
			if sp.ExtraParams != nil {
				cfg.OpenAI.ExtraParams = sp.ExtraParams
			}
		} else {
			// Ensure a safe default system prompt when no DB record exists
			cfg.SystemPrompt = "You are a helpful assistant with access to tools and specialists to help you complete objectives."
		}
	}

	// Create LLM provider after potential DB overrides
	llm, err := llmproviders.Build(cfg, httpClient)
	if err != nil {
		log.Fatal().Err(err).Msg("build llm provider")
	}

	// Build specialists registry from DB (fallback to YAML) so CLI resolves
	// the same set as agentd.
	var specReg *specialists.Registry
	if list, err := specStore.List(context.Background(), systemUserID); err == nil {
		specReg = specialists.NewRegistry(cfg.OpenAI, specialistsFromStore(list), httpClient, nil)
	} else {
		specReg = specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, httpClient, nil)
	}

	// If a specialist was requested, route the query directly and exit.
	if *specialist != "" {
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

	// Provider factory for base_url override in llm_transform
	newProv := func(baseURL string) llmpkg.Provider {
		switch cfg.LLMClient.Provider {
		case "", "openai", "local":
			c2 := cfg.LLMClient.OpenAI
			c2.BaseURL = baseURL
			return openaillm.New(c2, httpClient)
		default:
			return llm
		}
	}
	registry.Register(llmtools.NewTransform(llm, cfg.OpenAI.Model, newProv)) // provides llm_transform
	// Specialists tool for LLM-driven routing (prefer DB-backed registry to stay in sync with agentd)
	if list, err := specStore.List(context.Background(), systemUserID); err == nil {
		specReg = specialists.NewRegistry(cfg.OpenAI, specialistsFromStore(list), httpClient, registry)
	} else {
		specReg = specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, httpClient, registry)
	}
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
		// Configure WARPP to source defaults from the database, not hard-coded values.
		warpp.SetDefaultStore(mgr.Warpp)
		wfreg, _ := warpp.LoadFromStore(context.Background(), mgr.Warpp, systemUserID)
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

// databasesTestPool mirrors the lightweight helper in agentd to open a pgx pool
// for feature stores (e.g., specialists). It pings with a short timeout.
func databasesTestPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(cctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// specialistsFromStore converts persisted specialists to config structs
func specialistsFromStore(list []persist.Specialist) []config.SpecialistConfig {
	out := make([]config.SpecialistConfig, 0, len(list))
	for _, s := range list {
		// Skip the orchestrator; it is the main agent, not a specialist tool
		if strings.EqualFold(strings.TrimSpace(s.Name), "orchestrator") {
			continue
		}
		out = append(out, config.SpecialistConfig{
			Name: s.Name, BaseURL: s.BaseURL, APIKey: s.APIKey, Model: s.Model,
			EnableTools: s.EnableTools, Paused: s.Paused, AllowTools: s.AllowTools,
			ReasoningEffort: s.ReasoningEffort, System: s.System,
			ExtraHeaders: s.ExtraHeaders, ExtraParams: s.ExtraParams,
		})
	}
	return out
}
