package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/prompts"
	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/mcpclient"
	"manifold/internal/observability"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	"manifold/internal/tools/patchtool"
	"manifold/internal/tools/textsplitter"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"
)

const systemUserID int64 = 0

const (
	defaultRunTimeout = 2 * time.Minute
	mcpInitTimeout    = 20 * time.Second
)

func main() {
	// Load config first to populate defaults.
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

	if err := run(&cfg, *q, *maxSteps, *warppFlag, *specialist); err != nil {
		log.Fatal().Err(err).Msg("agent")
	}
}

func run(cfg *config.Config, query string, maxSteps int, warppEnabled bool, specialistName string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	observability.InitLogger(cfg.LogPath, cfg.LogLevel)
	log.Info().Msg("agent starting")
	baseCtx := context.Background()
	shutdown, err := observability.InitOTel(baseCtx, cfg.Obs)
	if err != nil {
		log.Warn().Err(err).Msg("otel init failed, continuing without observability")
		shutdown = nil
	} else {
		// Bridge zerolog to OTLP log exporter
		observability.EnableOTelLogging(cfg.Obs.ServiceName)
	}
	if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	// Configure global LLM payload logging/truncation before creating providers.
	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.OutputTruncateByte)

	// Initialize the specialists store and apply DB-backed overrides so the CLI
	// mirrors agentd behavior (specialists and orchestrator loaded from DB).
	var specPool *pgxpool.Pool
	if cfg.Databases.DefaultDSN != "" {
		p, err := databases.OpenPool(baseCtx, cfg.Databases.DefaultDSN)
		if err != nil {
			log.Warn().Err(err).Msg("open specialists db")
		} else {
			specPool = p
		}
	}
	if specPool != nil {
		defer specPool.Close()
	}
	specStore := databases.NewSpecialistsStore(specPool)
	if err := specStore.Init(baseCtx); err != nil {
		log.Warn().Err(err).Msg("init specialists store")
	}
	if err := specialists.SeedStore(baseCtx, specStore, systemUserID, cfg.Specialists); err != nil {
		log.Warn().Err(err).Msg("seed specialists")
	}
	specList, specListErr := specStore.List(baseCtx, systemUserID)
	if specListErr != nil {
		log.Warn().Err(specListErr).Msg("list specialists")
	}
	sp, ok, spErr := specStore.GetByName(baseCtx, systemUserID, specialists.OrchestratorName)
	if spErr != nil {
		log.Warn().Err(spErr).Msg("load orchestrator specialist")
	}
	if ok {
		specialists.ApplyOrchestratorConfig(cfg, sp)
		if strings.TrimSpace(cfg.SystemPrompt) == "" {
			cfg.SystemPrompt = specialists.DefaultOrchestratorPrompt
		}
	} else {
		// Ensure a safe default system prompt when no DB record exists.
		cfg.SystemPrompt = specialists.DefaultOrchestratorPrompt
	}

	httpClient := observability.NewHTTPClient(nil)
	// Inject global headers for the main agent if configured.
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}

	// Create the LLM provider after potential DB overrides.
	llm, err := llmproviders.Build(*cfg, httpClient)
	if err != nil {
		return fmt.Errorf("build llm provider: %w", err)
	}

	// Build specialists registry from DB (fallback to YAML) so the CLI resolves
	// the same set as agentd.
	var specReg *specialists.Registry
	if specListErr == nil {
		specReg = specialists.NewRegistry(cfg.LLMClient, specialists.ConfigsFromStore(specList), httpClient, nil)
	} else {
		specReg = specialists.NewRegistry(cfg.LLMClient, cfg.Specialists, httpClient, nil)
	}
	specReg.SetWorkdir(cfg.Workdir)

	// If a specialist was requested, route the query directly and exit.
	if strings.TrimSpace(specialistName) != "" {
		a, ok := specReg.Get(specialistName)
		if !ok {
			return fmt.Errorf("unknown specialist %q; available: %v", specialistName, specReg.Names())
		}
		log.Info().Str("specialist", specialistName).Msg("direct specialist invocation")
		ctx, cancel := context.WithTimeout(baseCtx, defaultRunTimeout)
		defer cancel()
		out, err := a.Inference(ctx, query, nil)
		if err != nil {
			return fmt.Errorf("specialist %q: %w", specialistName, err)
		}
		fmt.Println(out)
		return nil
	}

	registry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	mgr, err := databases.NewManager(baseCtx, cfg.Databases)
	if err != nil {
		return fmt.Errorf("init databases: %w", err)
	}
	defer mgr.Close()
	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	registry.Register(cli.NewTool(exec))               // provides run_cli
	registry.Register(web.NewTool(cfg.Web.SearXNGURL)) // provides web_search
	registry.Register(web.NewFetchTool(mgr.Search))    // provides web_fetch
	// Register patch application tool (unified diff).
	registry.Register(patchtool.New(cfg.Workdir)) // provides apply_patch
	// Register text splitting tool (RAG ingestion helpers).
	registry.Register(textsplitter.New()) // provides split_text
	registry.Register(utility.NewTextboxTool())
	// Register TTS tool.
	registry.Register(tts.New(*cfg, httpClient))

	// Register specialists tool for LLM-driven routing (prefer DB-backed registry to stay in sync with agentd).
	if specListErr == nil {
		specReg = specialists.NewRegistry(cfg.LLMClient, specialists.ConfigsFromStore(specList), httpClient, registry)
	} else {
		specReg = specialists.NewRegistry(cfg.LLMClient, cfg.Specialists, httpClient, registry)
	}
	specReg.SetWorkdir(cfg.Workdir)

	// If tools are globally disabled, use an empty registry.
	if !cfg.EnableTools {
		registry = tools.NewRegistry() // Empty registry
	} else if len(cfg.ToolAllowList) > 0 {
		// If a top-level tool allow-list is configured, expose only those tools
		// to the main orchestrator agent by wrapping the registry.
		registry = tools.NewFilteredRegistry(registry, cfg.ToolAllowList)
	}

	// Log which tools are exposed after filtering to diagnose missing registrations at runtime.
	{
		names := make([]string, 0, len(registry.Schemas()))
		for _, s := range registry.Schemas() {
			names = append(names, s.Name)
		}
		log.Info().Bool("enableTools", cfg.EnableTools).Strs("allowList", cfg.ToolAllowList).Strs("tools", names).Msg("tool_registry_contents")
	}

	// Connect to configured MCP servers and register their tools.
	mcpMgr := mcpclient.NewManager()
	defer mcpMgr.Close()
	ctxInit, cancelInit := context.WithTimeout(baseCtx, mcpInitTimeout)
	if err := mcpMgr.RegisterFromConfig(ctxInit, registry, cfg.MCP); err != nil {
		log.Warn().Err(err).Msg("mcp init")
	}
	cancelInit()

	// Run the WARPP workflow executor instead of the LLM loop when enabled.
	if warppEnabled {
		// Configure WARPP to source defaults from the database, not hard-coded values.
		warpp.SetDefaultStore(mgr.Warpp)
		wfreg, err := warpp.LoadFromStore(baseCtx, mgr.Warpp, systemUserID)
		if err != nil {
			return fmt.Errorf("load workflows: %w", err)
		}
		runner := &warpp.Runner{Workflows: wfreg, Tools: registry}
		ctx, cancel := context.WithTimeout(baseCtx, defaultRunTimeout)
		defer cancel()
		intent := runner.DetectIntent(ctx, query)
		wf, err := wfreg.Get(intent)
		if err != nil {
			return fmt.Errorf("workflow %q not found: %w", intent, err)
		}
		attrs := warpp.Attrs{"utter": query}
		wfStar, _, attrs, err := runner.Personalize(ctx, wf, attrs)
		if err != nil {
			return fmt.Errorf("personalize workflow: %w", err)
		}
		allow := map[string]bool{}
		for _, s := range wfStar.Steps {
			if s.Tool != nil {
				allow[s.Tool.Name] = true
			}
		}
		final, err := runner.Execute(ctx, wfStar, allow, attrs, nil)
		if err != nil {
			return fmt.Errorf("execute workflow: %w", err)
		}
		fmt.Println(final)
		return nil
	}

	// Call a specialist directly if a pre-dispatch route matches.
	if name := specialists.Route(cfg.SpecialistRoutes, query); name != "" {
		log.Info().Str("route", name).Msg("pre-dispatch specialist route matched")
		a, ok := specReg.Get(name)
		if !ok {
			log.Error().Str("route", name).Msg("specialist not found for route")
		} else {
			ctx, cancel := context.WithTimeout(baseCtx, defaultRunTimeout)
			defer cancel()
			out, err := a.Inference(ctx, query, nil)
			if err != nil {
				return fmt.Errorf("specialist pre-dispatch %q: %w", name, err)
			}
			fmt.Println(out)
			return nil
		}
	}

	systemPrompt := prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt)
	systemPrompt = specReg.AppendToSystemPrompt(systemPrompt)

	eng := agent.Engine{
		LLM:                        llm,
		Tools:                      registry,
		MaxSteps:                   maxSteps,
		System:                     systemPrompt,
		SummaryEnabled:             cfg.SummaryEnabled,
		SummaryReserveBufferTokens: cfg.SummaryReserveBufferTokens,
	}

	// Honor the configured run timeout; 0 disables the deadline.
	var ctx context.Context
	var cancel context.CancelFunc
	if cfg.AgentRunTimeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(baseCtx, time.Duration(cfg.AgentRunTimeoutSeconds)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(baseCtx)
	}
	defer cancel()

	final, err := eng.Run(ctx, query, nil)
	if err != nil {
		return err
	}
	fmt.Println(final)
	return nil
}
