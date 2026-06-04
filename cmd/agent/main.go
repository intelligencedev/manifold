package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/prompts"
	"manifold/internal/config"
	"manifold/internal/embeddedpg"
	llmpkg "manifold/internal/llm"
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/mcpclient"
	"manifold/internal/observability"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	"manifold/internal/tools/patchtool"
	"manifold/internal/tools/textsplitter"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	"manifold/internal/tools/web"
)

const systemUserID int64 = 0

const (
	defaultRunTimeout = 2 * time.Minute
	mcpInitTimeout    = 20 * time.Second
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config")
	}

	q := flag.String("q", "", "User request")
	maxSteps := flag.Int("max-steps", cfg.MaxSteps, "Max reasoning steps")
	specialist := flag.String("specialist", "", "Name of specialist agent to use (inference-only; no tool calls unless enabled)")
	flag.Parse()
	if *q == "" {
		fmt.Fprintln(os.Stderr, "usage: agent -q \"...\"")
		os.Exit(2)
	}

	if err := run(&cfg, *q, *maxSteps, *specialist); err != nil {
		log.Fatal().Err(err).Msg("agent")
	}
}

func run(cfg *config.Config, query string, maxSteps int, specialistName string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	baseCtx := context.Background()
	shutdown := initObservability(baseCtx, cfg)
	if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}
	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.LogRawPrompts, cfg.OutputTruncateByte)

	embeddedRuntime, err := startEmbeddedPostgres(cfg)
	if err != nil {
		return err
	}
	if embeddedRuntime != nil {
		defer stopEmbeddedPostgres(embeddedRuntime)
	}

	specs, err := loadCLISpecialists(baseCtx, cfg)
	if err != nil {
		return err
	}
	defer specs.Close()

	httpClient := newCLIHTTPClient(cfg)
	llm, err := llmproviders.Build(*cfg, httpClient)
	if err != nil {
		return fmt.Errorf("build llm provider: %w", err)
	}

	specReg := newCLIRegistry(cfg, specs, httpClient, nil)

	if strings.TrimSpace(specialistName) != "" {
		return runDirectSpecialist(baseCtx, specReg, specialistName, query, "specialist")
	}

	registry, mgr, err := buildCLIToolRegistry(baseCtx, cfg, httpClient)
	if err != nil {
		return err
	}
	defer mgr.Close()
	specReg = newCLIRegistry(cfg, specs, httpClient, registry)
	registry = tools.ApplyTopLevelPolicy(registry, cfg.EnableTools, cfg.ToolAllowList)

	log.Info().Bool("enableTools", cfg.EnableTools).Strs("allowList", cfg.ToolAllowList).Strs("tools", tools.SchemaNames(registry)).Msg("tool_registry_contents")

	closeMCP := registerCLIMCP(baseCtx, registry, cfg)
	defer closeMCP()

	if name := specialists.Route(cfg.SpecialistRoutes, query); name != "" {
		if err := runDirectSpecialist(baseCtx, specReg, name, query, "specialist pre-dispatch"); err == nil {
			return nil
		} else if !strings.Contains(err.Error(), "unknown specialist") {
			return err
		}
	}

	return runOrchestrator(baseCtx, orchestratorRunRequest{
		Config:   cfg,
		Registry: specReg,
		LLM:      llm,
		Tools:    registry,
		Query:    query,
		MaxSteps: maxSteps,
	})
}

func initObservability(ctx context.Context, cfg *config.Config) func(context.Context) error {
	observability.InitLogger(cfg.LogPath, cfg.LogLevel)
	log.Info().Msg("agent starting")
	shutdown, err := observability.InitOTel(ctx, cfg.Obs)
	if err != nil {
		log.Warn().Err(err).Msg("otel init failed, continuing without observability")
		return nil
	}
	observability.EnableOTelLogging(cfg.Obs.ServiceName)
	return shutdown
}

func startEmbeddedPostgres(cfg *config.Config) (*embeddedpg.Runtime, error) {
	cfg.Databases.EmbeddedDiagnosticLogPath = cfg.LogPath
	embeddedRuntime, err := embeddedpg.Start(&cfg.Databases)
	if err != nil {
		return nil, fmt.Errorf("start embedded postgres: %w", err)
	}
	return embeddedRuntime, nil
}

func stopEmbeddedPostgres(runtime *embeddedpg.Runtime) {
	if stopErr := runtime.Stop(); stopErr != nil {
		log.Warn().Err(stopErr).Msg("stop embedded postgres")
	}
}

func newCLIHTTPClient(cfg *config.Config) *http.Client {
	httpClient := observability.NewHTTPClient(nil)
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}
	return httpClient
}

type cliSpecialists struct {
	pool *pgxpool.Pool
	list []persistence.Specialist
	err  error
}

func (s cliSpecialists) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func loadCLISpecialists(ctx context.Context, cfg *config.Config) (cliSpecialists, error) {
	specs := cliSpecialists{}
	if cfg.Databases.DefaultDSN != "" {
		pool, err := databases.OpenPool(ctx, cfg.Databases.DefaultDSN)
		if err != nil {
			log.Warn().Err(err).Msg("open specialists db")
		} else {
			specs.pool = pool
		}
	}
	specStore := databases.NewSpecialistsStore(specs.pool)
	if err := specStore.Init(ctx); err != nil {
		log.Warn().Err(err).Msg("init specialists store")
	}
	if err := specialists.SeedStore(ctx, specStore, systemUserID, cfg.Specialists); err != nil {
		log.Warn().Err(err).Msg("seed specialists")
	}
	specs.list, specs.err = specStore.List(ctx, systemUserID)
	if specs.err != nil {
		log.Warn().Err(specs.err).Msg("list specialists")
	}
	sp, ok, err := specStore.GetByName(ctx, systemUserID, specialists.OrchestratorName)
	if err != nil {
		log.Warn().Err(err).Msg("load orchestrator specialist")
	}
	if ok {
		specialists.ApplyOrchestratorConfig(cfg, sp)
		if strings.TrimSpace(cfg.SystemPrompt) == "" {
			cfg.SystemPrompt = specialists.DefaultOrchestratorPrompt
		}
	} else {
		cfg.SystemPrompt = specialists.DefaultOrchestratorPrompt
	}
	return specs, nil
}

func newCLIRegistry(cfg *config.Config, specs cliSpecialists, httpClient *http.Client, registry tools.Registry) *specialists.Registry {
	return specialists.NewRegistryFromStore(specialists.StoreRegistryRequest{
		Base:       cfg.LLMClient,
		Defaults:   cfg.Specialists,
		List:       specs.list,
		Err:        specs.err,
		HTTPClient: httpClient,
		Tools:      registry,
		Workdir:    cfg.Workdir,
	})
}

func runDirectSpecialist(ctx context.Context, reg *specialists.Registry, name, query, label string) error {
	specialist, ok := reg.Get(name)
	if !ok {
		return fmt.Errorf("unknown specialist %q; available: %v", name, reg.Names())
	}
	log.Info().Str("specialist", name).Msg(label + " invocation")
	runCtx, cancel := context.WithTimeout(ctx, defaultRunTimeout)
	defer cancel()
	out, err := specialist.Inference(runCtx, query, nil)
	if err != nil {
		return fmt.Errorf("%s %q: %w", label, name, err)
	}
	fmt.Println(out)
	return nil
}

func registerCLIMCP(ctx context.Context, registry tools.Registry, cfg *config.Config) func() {
	mcpMgr := mcpclient.NewManager()
	ctxInit, cancelInit := context.WithTimeout(ctx, mcpInitTimeout)
	if err := mcpMgr.RegisterFromConfig(ctxInit, registry, cfg.MCP); err != nil {
		log.Warn().Err(err).Msg("mcp init")
	}
	cancelInit()
	return mcpMgr.Close
}

type orchestratorRunRequest struct {
	Config   *config.Config
	Registry *specialists.Registry
	LLM      llmpkg.Provider
	Tools    tools.Registry
	Query    string
	MaxSteps int
}

func runOrchestrator(ctx context.Context, req orchestratorRunRequest) error {
	cfg := req.Config
	systemPrompt := prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt)
	systemPrompt = req.Registry.AppendToSystemPrompt(systemPrompt)
	eng := agent.Engine{
		LLM:                        req.LLM,
		Tools:                      req.Tools,
		MaxSteps:                   req.MaxSteps,
		System:                     systemPrompt,
		SummaryEnabled:             cfg.SummaryEnabled,
		SummaryReserveBufferTokens: cfg.SummaryReserveBufferTokens,
	}
	runCtx, cancel := cliRunContext(ctx, cfg)
	defer cancel()

	final, err := eng.Run(runCtx, req.Query, nil)
	if err != nil {
		return err
	}
	fmt.Println(final)
	return nil
}

func cliRunContext(ctx context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	if cfg.AgentRunTimeoutSeconds > 0 {
		return context.WithTimeout(ctx, time.Duration(cfg.AgentRunTimeoutSeconds)*time.Second)
	}
	return context.WithCancel(ctx)
}

func buildCLIToolRegistry(ctx context.Context, cfg *config.Config, httpClient *http.Client) (tools.Registry, databases.Manager, error) {
	registry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	mgr, err := databases.NewManager(ctx, cfg.Databases)
	if err != nil {
		return nil, databases.Manager{}, fmt.Errorf("init databases: %w", err)
	}
	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	registry.Register(cli.NewTool(exec))
	registry.Register(web.NewFetchTool(mgr.Search))
	registry.Register(patchtool.New(cfg.Workdir))
	registry.Register(textsplitter.New())
	registry.Register(utility.NewTextboxTool())
	registry.Register(tts.New(*cfg, httpClient))
	return registry, mgr, nil
}
