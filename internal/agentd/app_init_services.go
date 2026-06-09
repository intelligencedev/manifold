package agentd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/constitution"
	"manifold/internal/durable"
	"manifold/internal/fleet"
	"manifold/internal/httpapi"
	llmpkg "manifold/internal/llm"
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/matrixgw"
	"manifold/internal/observability"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/playground"
	"manifold/internal/playground/artifacts"
	"manifold/internal/playground/dataset"
	"manifold/internal/playground/eval"
	"manifold/internal/playground/experiment"
	"manifold/internal/playground/provider"
	playgroundregistry "manifold/internal/playground/registry"
	"manifold/internal/playground/worker"
	"manifold/internal/projects"
	ragreranker "manifold/internal/rag/reranker"
	"manifold/internal/skills"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	agenttools "manifold/internal/tools/agents"
	tooldiscovery "manifold/internal/tools/discovery"
	"manifold/internal/trust"
	"manifold/internal/workspaces"
)

type appStartupDeps struct {
	httpClient      *http.Client
	llm             llmpkg.Provider
	summaryLLM      llmpkg.Provider
	summaryModel    string
	mgr             databases.Manager
	durableClient   *durable.Client
	durableRegistry *durable.Registry
	tooling         appTooling
	routing         appRouting
}

func buildAppStartup(ctx context.Context, cfg *config.Config) (appStartupDeps, error) {
	httpClient := observability.NewHTTPClient(nil)
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}
	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.LogRawPrompts, cfg.OutputTruncateByte)
	llm, err := llmproviders.Build(*cfg, httpClient)
	if err != nil {
		return appStartupDeps{}, fmt.Errorf("build llm provider: %w", err)
	}
	summaryLLM, summaryModel, err := buildSummaryLLM(cfg, httpClient)
	if err != nil {
		return appStartupDeps{}, fmt.Errorf("build summary llm provider: %w", err)
	}
	mgr, err := databases.NewManager(ctx, cfg.Databases)
	if err != nil {
		return appStartupDeps{}, fmt.Errorf("init databases: %w", err)
	}
	if err := initializeCommandPolicy(ctx, cfg, mgr.CommandPolicy); err != nil {
		return appStartupDeps{}, fmt.Errorf("init command policy: %w", err)
	}
	durableClient := durable.NewClient(mgr.Durable)
	durableRegistry := durable.NewRegistry()
	tooling, err := initAppTooling(ctx, cfg, httpClient, llm, mgr)
	if err != nil {
		return appStartupDeps{}, err
	}
	routing := initAppRouting(ctx, cfg, httpClient, mgr, tooling)
	return appStartupDeps{
		httpClient:      httpClient,
		llm:             llm,
		summaryLLM:      summaryLLM,
		summaryModel:    summaryModel,
		mgr:             mgr,
		durableClient:   durableClient,
		durableRegistry: durableRegistry,
		tooling:         tooling,
		routing:         routing,
	}, nil
}

func (deps appStartupDeps) shellDeps(cfg *config.Config) appShellDeps {
	return appShellDeps{
		cfg:              cfg,
		httpClient:       deps.httpClient,
		mgr:              deps.mgr,
		llm:              deps.llm,
		summaryLLM:       deps.summaryLLM,
		durableClient:    deps.durableClient,
		durableRegistry:  deps.durableRegistry,
		tooling:          deps.tooling,
		routing:          deps.routing,
		baseToolRegistry: deps.tooling.baseRegistry,
		toolRegistry:     deps.routing.toolRegistry,
		toolIndex:        deps.routing.toolIndex,
		specReg:          deps.routing.specRegistry,
	}
}

func (deps appStartupDeps) agentRuntimeDeps(ctx context.Context, cfg *config.Config) agentRuntimeDeps {
	return agentRuntimeDeps{
		ctx:          ctx,
		cfg:          cfg,
		llm:          deps.llm,
		summaryLLM:   deps.summaryLLM,
		summaryModel: deps.summaryModel,
		httpClient:   deps.httpClient,
		toolRegistry: deps.routing.toolRegistry,
		specReg:      deps.routing.specRegistry,
		wsMgr:        deps.routing.workspaceManager,
		mgr:          deps.mgr,
	}
}

type appShellDeps struct {
	cfg              *config.Config
	httpClient       *http.Client
	mgr              databases.Manager
	llm              llmpkg.Provider
	summaryLLM       llmpkg.Provider
	durableClient    *durable.Client
	durableRegistry  *durable.Registry
	tooling          appTooling
	routing          appRouting
	baseToolRegistry tools.Registry
	toolRegistry     tools.Registry
	toolIndex        *tooldiscovery.ToolIndex
	specReg          *specialists.Registry
}

func newAppShell(deps appShellDeps) *app {
	a := &app{
		cfg:                deps.cfg,
		httpClient:         deps.httpClient,
		mgr:                &deps.mgr,
		llm:                deps.llm,
		summaryLLM:         deps.summaryLLM,
		durableStore:       deps.mgr.Durable,
		durableClient:      deps.durableClient,
		durableRegistry:    deps.durableRegistry,
		baseToolRegistry:   deps.baseToolRegistry,
		toolRegistry:       deps.toolRegistry,
		toolIndex:          deps.toolIndex,
		specRegistry:       deps.specReg,
		userSpecRegs:       map[int64]*specialists.Registry{systemUserID: deps.specReg},
		runs:               newRunStore(),
		inputRequests:      newInputRequestBroker(),
		flowV2:             newFlowV2Runtime(deps.mgr.FlowV2, deps.durableClient),
		codeQARuntime:      newCodeQARuntime(),
		codeQAService:      deps.tooling.codeQAService,
		cliExecutor:        deps.tooling.cliExecutor,
		terminalManager:    deps.tooling.terminalManager,
		evolvingSessionTTL: defaultEvolvingSessionTTL,
		mcpStore:           deps.mgr.MCP,
		userPrefsStore:     deps.mgr.UserPreferences,
		commandPolicyStore: deps.mgr.CommandPolicy,
		mcpManager:         deps.routing.mcpManager,
		mcpPool:            deps.routing.mcpPool,
		workspaceManager:   deps.routing.workspaceManager,
		transitService:     deps.tooling.transitService,
		ragService:         deps.tooling.ragService,
		fleetBus:           fleet.NewBus(512),
	}
	if a.mcpPool != nil {
		a.mcpPool.SetToolsChangedCallback(a.refreshToolDiscoveryIndex)
	}
	a.registerDurableHandlers()
	a.durableWorker = durable.NewWorker(deps.mgr.Durable, deps.durableClient, deps.durableRegistry, durable.WorkerOptions{
		WorkerID:     fmt.Sprintf("agentd-%d", os.Getpid()),
		Lease:        5 * time.Minute,
		PollInterval: 500 * time.Millisecond,
		QueueConcurrency: map[string]int{
			durableChatQueue: durableChatWorkerConcurrency,
		},
	})
	return a
}

func (a *app) initPersistentServices(ctx context.Context, cfg *config.Config) error {
	trustStore := trust.NewStore(optionalPool(ctx, cfg.Databases.DefaultDSN))
	a.trustService = trust.NewService(trustStore)
	if err := a.trustService.Init(ctx); err != nil {
		return fmt.Errorf("init trust service: %w", err)
	}
	constitutionStore := constitution.NewStore(optionalPool(ctx, cfg.Databases.DefaultDSN))
	a.constitutionSvc = constitution.NewService(constitutionStore)
	if err := a.constitutionSvc.Init(ctx); err != nil {
		return fmt.Errorf("init constitution service: %w", err)
	}
	return a.initMatrixGateway(cfg)
}

func (a *app) initMatrixGateway(cfg *config.Config) error {
	var err error
	a.matrixGateway, err = matrixgw.New(cfg.Matrix)
	if err != nil {
		return fmt.Errorf("init matrix gateway: %w", err)
	}
	a.matrixGateway.SetHandler(matrixgw.MessageHandlerFunc(a.handleMatrixMessage))
	a.matrixGateway.SetOutboundRecorder(func(runCtx context.Context, message matrixgw.OutboundMessage) error {
		if a.matrixMessageStore == nil {
			return nil
		}
		_, err := a.matrixMessageStore.Append(runCtx, persist.MatrixMessage{
			RoomID:        message.RoomID,
			Direction:     "outbound",
			Target:        message.Target,
			Body:          message.Body,
			FormattedBody: message.FormattedBody,
			MsgType:       message.MsgType,
			MediaURL:      message.MediaURL,
			MediaMIME:     message.MediaMIME,
			MediaSize:     message.MediaSize,
			CreatedAt:     time.Now().UTC(),
		}, matrixMessageRetention(cfg.Matrix, message.RoomID))
		return err
	})
	return nil
}

func (a *app) initEvolvingSessionJanitor(ctx context.Context, cfg *config.Config) {
	janitorInterval := defaultEvolvingJanitorInterval
	if cfg.EvolvingMemory.SessionTTLMinutes > 0 {
		a.evolvingSessionTTL = time.Duration(cfg.EvolvingMemory.SessionTTLMinutes) * time.Minute
	}
	if cfg.EvolvingMemory.JanitorIntervalMinutes > 0 {
		janitorInterval = time.Duration(cfg.EvolvingMemory.JanitorIntervalMinutes) * time.Minute
	}
	a.startEvolvingSessionJanitor(ctx, janitorInterval)
}

type agentRuntimeDeps struct {
	ctx          context.Context
	cfg          *config.Config
	llm          llmpkg.Provider
	summaryLLM   llmpkg.Provider
	summaryModel string
	httpClient   *http.Client
	toolRegistry tools.Registry
	specReg      *specialists.Registry
	wsMgr        workspaces.WorkspaceManager
	mgr          databases.Manager
}

func (a *app) initAgentRuntime(deps agentRuntimeDeps) error {
	a.initEngine(deps.cfg, deps.llm, deps.toolRegistry)
	if err := a.initBeliefMemory(deps.cfg, deps.llm, deps.httpClient); err != nil {
		return err
	}
	a.initDelegator(deps.cfg, deps.toolRegistry, deps.specReg, deps.wsMgr)
	return a.initEvolvingMemory(evolvingMemoryDeps{
		cfg:          deps.cfg,
		llm:          deps.llm,
		summaryLLM:   deps.summaryLLM,
		summaryModel: deps.summaryModel,
		httpClient:   deps.httpClient,
		mgr:          deps.mgr,
	})
}

func (a *app) initEngine(cfg *config.Config, llm llmpkg.Provider, toolRegistry tools.Registry) {
	ctxSize, _ := llmpkg.ContextSize(cfg.OpenAI.Model)
	a.engine = &agent.Engine{
		LLM:                          llm,
		Tools:                        toolRegistry,
		MaxSteps:                     cfg.MaxSteps,
		MaxToolParallelism:           cfg.MaxToolParallelism,
		System:                       a.composeSystemPrompt(),
		UserPromptContext:            a.composeUserPromptContext(),
		Model:                        cfg.OpenAI.Model,
		ContextWindowTokens:          ctxSize,
		SummaryEnabled:               cfg.SummaryEnabled,
		SummaryReserveBufferTokens:   cfg.SummaryReserveBufferTokens,
		SummaryMinKeepLastMessages:   cfg.SummaryMinKeepLastMessages,
		SummaryMaxSummaryChunkTokens: cfg.SummaryMaxSummaryChunkTokens,
		HarnessEnabled:               cfg.Harness.Enabled,
		HarnessConfig:                harnessRunConfig(cfg.Harness),
	}
	a.engine.AttachTokenizer(llm, nil)
}

func (a *app) initBeliefMemory(cfg *config.Config, llm llmpkg.Provider, httpClient *http.Client) error {
	if !cfg.BeliefMemory.Enabled || !cfg.BeliefMemory.EnableDistillation || cfg.BeliefMemory.Distillation.Mode != "llm" {
		return nil
	}
	beliefLLM, beliefModel, beliefProvider, err := resolveBeliefMemoryLLM(cfg, llm, httpClient)
	if err != nil {
		return fmt.Errorf("build belief memory llm provider: %w", err)
	}
	a.beliefLLM = beliefLLM
	a.beliefModel = beliefModel
	log.Info().Bool("enabled", true).Str("provider", beliefProvider).Str("model", beliefModel).Msg("belief_memory_llm_initialized")
	return nil
}

func (a *app) initDelegator(cfg *config.Config, toolRegistry tools.Registry, specReg *specialists.Registry, wsMgr workspaces.WorkspaceManager) {
	delegator := agenttools.NewDelegator(toolRegistry, specReg, wsMgr, cfg.MaxSteps)
	delegator.SetDefaultTimeout(cfg.AgentRunTimeoutSeconds)
	delegator.SetTeamDelegator(a)
	a.engine.Delegator = delegator
	a.engine.TeamDelegator = a
}

type evolvingMemoryDeps struct {
	cfg          *config.Config
	llm          llmpkg.Provider
	summaryLLM   llmpkg.Provider
	summaryModel string
	httpClient   *http.Client
	mgr          databases.Manager
}

func (a *app) initEvolvingMemory(deps evolvingMemoryDeps) error {
	cfg := deps.cfg
	if !cfg.EvolvingMemory.Enabled {
		return nil
	}
	memLLM, memModel, memProvider, err := resolveEvolvingMemoryLLM(cfg, deps.llm, deps.summaryLLM, deps.summaryModel, deps.httpClient)
	if err != nil {
		return fmt.Errorf("build evolving memory llm provider: %w", err)
	}
	evStore := memory.EvolvingMemoryStore(nil)
	if deps.mgr.EvolvingMemory != nil {
		evStore = deps.mgr.EvolvingMemory
	}
	magmaSink := a.evolvingMagmaSink(cfg)
	storeJanitorInterval := time.Duration(cfg.EvolvingMemory.StoreJanitorIntervalMinutes) * time.Minute
	a.evolvingCfg = a.evolvingMemoryConfig(cfg, memLLM, memModel, evStore, magmaSink, storeJanitorInterval)
	a.rememMaxInnerSteps = cfg.EvolvingMemory.MaxInnerSteps
	a.engine.EvolvingMemory = memory.NewEvolvingMemory(a.evolvingMemoryConfig(cfg, memLLM, memModel, evStore, magmaSink, storeJanitorInterval))
	a.logEvolvingMemoryInitialized(cfg, memProvider, memModel)
	a.initReMemController(cfg, memLLM, memModel)
	return nil
}

func (a *app) evolvingMagmaSink(cfg *config.Config) memory.EvolvingMemoryMagmaSink {
	if cfg.Magma.Enabled && a.ragService != nil && a.ragService.MagmaService() != nil {
		return evolvingMagmaSink{service: a.ragService.MagmaService(), workerCount: cfg.Magma.Consolidation.WorkerCount}
	}
	return nil
}

func (a *app) evolvingMemoryConfig(cfg *config.Config, memLLM llmpkg.Provider, memModel string, evStore memory.EvolvingMemoryStore, magmaSink memory.EvolvingMemoryMagmaSink, storeJanitorInterval time.Duration) memory.EvolvingMemoryConfig {
	persistDebounce := defaultEvolvingPersistDebounce
	if cfg.EvolvingMemory.PersistDebounceMs > 0 {
		persistDebounce = time.Duration(cfg.EvolvingMemory.PersistDebounceMs) * time.Millisecond
	}
	var reranker memory.EvolvingMemoryReranker
	if cfg.Reranking.Enabled {
		reranker = evolvingMemoryRAGReranker{reranker: ragreranker.NewClient(cfg.Reranking)}
	}
	return memory.EvolvingMemoryConfig{
		EmbeddingConfig:              cfg.Embedding,
		LLM:                          memLLM,
		Model:                        memModel,
		PersistDebounce:              persistDebounce,
		MaxSize:                      cfg.EvolvingMemory.MaxSize,
		TopK:                         cfg.EvolvingMemory.TopK,
		WindowSize:                   cfg.EvolvingMemory.WindowSize,
		EnableRAG:                    cfg.EvolvingMemory.EnableRAG,
		RetrievalSimilarityThreshold: cfg.EvolvingMemory.RetrievalSimilarityThreshold,
		EnableSmartPrune:             cfg.EvolvingMemory.EnableSmartPrune,
		PruneThreshold:               cfg.EvolvingMemory.PruneThreshold,
		RelevanceDecay:               cfg.EvolvingMemory.RelevanceDecay,
		MinRelevance:                 cfg.EvolvingMemory.MinRelevance,
		Reranker:                     reranker,
		PruneQualityFloor:            cfg.EvolvingMemory.PruneQualityFloor,
		PromotionAccessThreshold:     cfg.EvolvingMemory.PromotionAccessThreshold,
		JanitorInterval:              storeJanitorInterval,
		Metrics:                      memory.NewMemoryMetrics(),
		Store:                        evStore,
		UserID:                       systemUserID,
		MagmaSink:                    magmaSink,
	}
}

func (a *app) logEvolvingMemoryInitialized(cfg *config.Config, memProvider, memModel string) {
	log.Info().
		Bool("enabled", true).
		Str("provider", memProvider).
		Str("model", memModel).
		Dur("persistDebounce", a.evolvingCfg.PersistDebounce).
		Dur("sessionTTL", a.evolvingSessionTTL).
		Dur("janitorInterval", time.Duration(cfg.EvolvingMemory.JanitorIntervalMinutes)*time.Minute).
		Int("maxSize", cfg.EvolvingMemory.MaxSize).
		Int("topK", cfg.EvolvingMemory.TopK).
		Float64("retrievalSimilarityThreshold", cfg.EvolvingMemory.RetrievalSimilarityThreshold).
		Bool("rag", cfg.EvolvingMemory.EnableRAG).
		Bool("rerank", cfg.Reranking.Enabled).
		Bool("smartPrune", cfg.EvolvingMemory.EnableSmartPrune).
		Msg("evolving_memory_initialized")
}

func (a *app) initReMemController(cfg *config.Config, memLLM llmpkg.Provider, memModel string) {
	if !cfg.EvolvingMemory.ReMemEnabled {
		return
	}
	a.engine.ReMemEnabled = true
	a.engine.ReMemController = memory.NewReMemController(memory.ReMemConfig{
		LLM:           memLLM,
		Model:         memModel,
		Memory:        a.engine.EvolvingMemory,
		MaxInnerSteps: cfg.EvolvingMemory.MaxInnerSteps,
	})
	log.Info().Bool("remem_enabled", true).Int("maxInnerSteps", cfg.EvolvingMemory.MaxInnerSteps).Msg("remem_controller_initialized")
}

func (a *app) initChatMemory(cfg *config.Config, mgr databases.Manager, summaryLLM llmpkg.Provider, summaryModel string) error {
	a.chatStore = mgr.Chat
	if a.chatStore == nil {
		return fmt.Errorf("chat store not initialized")
	}
	a.matrixMessageStore = mgr.MatrixMessages
	a.activityStore = mgr.SpecialistActivity
	if a.activityStore == nil {
		return fmt.Errorf("specialist activity store not initialized")
	}

	summaryCtxSize, _ := llmpkg.ContextSize(summaryModel)
	if cfg.Summary.ContextWindowTokens > 0 {
		summaryCtxSize = cfg.Summary.ContextWindowTokens
	} else {
		const defaultSummaryContextWindowCap = 32_000
		if summaryCtxSize <= 0 || summaryCtxSize > defaultSummaryContextWindowCap {
			summaryCtxSize = defaultSummaryContextWindowCap
		}
	}
	a.chatMemory = memory.NewManager(a.chatStore, summaryLLM, memory.Config{
		Enabled:                      cfg.Summary.Enabled,
		CallTimeoutSeconds:           cfg.Summary.CallTimeoutSeconds,
		ReserveBufferTokens:          cfg.Summary.ReserveBufferTokens,
		MinKeepLastMessages:          cfg.Summary.MinKeepLastMessages,
		MaxKeepLastMessages:          cfg.Summary.MaxKeepLastMessages,
		MaxSummaryChunkTokens:        cfg.Summary.MaxSummaryChunkTokens,
		PlainTextContextWindowTokens: cfg.Summary.PlainTextContextWindowTokens,
		ContextWindowTokens:          summaryCtxSize,
		SummaryModel:                 summaryModel,
	})
	return nil
}

func (a *app) initPlaygroundServices(cfg *config.Config, mgr databases.Manager, llm llmpkg.Provider) error {
	if mgr.Playground == nil {
		return fmt.Errorf("playground store not initialized; set databases.defaultDSN or chat DSN")
	}
	artifactDir := filepath.Join(cfg.Workdir, "playground-artifacts")
	artifactStore := artifacts.NewFilesystemStore(artifactDir)
	playgroundRegistry := playgroundregistry.New(mgr.Playground)
	playgroundDataset := dataset.NewService(mgr.Playground)
	playgroundRepo := experiment.NewRepository()
	playgroundPlanner := experiment.NewPlanner(experiment.PlannerConfig{MaxRowsPerShard: 32, MaxVariantsPerShard: 4})
	playgroundProvider := provider.NewLLMAdapter(llm, cfg.OpenAI.Model)
	playgroundRunner := newPlaygroundSpecialistRunner(a, worker.NewProviderRunner(playgroundProvider))
	playgroundWorker := worker.NewWorkerWithRunner(playgroundRunner, artifactStore)
	playgroundEvals := eval.NewRunner(eval.NewRegistry(), playgroundProvider)
	playgroundService := playground.NewService(playground.Config{MaxConcurrentShards: 4, SpecialistValidator: playgroundRunner}, playground.Dependencies{
		Registry:    playgroundRegistry,
		Datasets:    playgroundDataset,
		Experiments: playgroundRepo,
		Planner:     playgroundPlanner,
		Workers:     playgroundWorker,
		Evals:       playgroundEvals,
		Store:       mgr.Playground,
	})
	a.playgroundHandler = httpapi.NewServer(playgroundService)
	return nil
}

func (a *app) initProjectServices(cfg *config.Config) {
	defaultSkillsDir := ""
	if cwd, err := os.Getwd(); err != nil {
		log.Warn().Err(err).Msg("unable_to_resolve_cwd_for_default_skills")
	} else {
		skillsPath := filepath.Join(cwd, "skills")
		if fi, err := os.Stat(skillsPath); err == nil && fi.IsDir() {
			defaultSkillsDir = skillsPath
			log.Info().Str("skillsPath", defaultSkillsDir).Msg("default_skills_source_enabled")
		} else if err == nil {
			log.Warn().Str("skillsPath", skillsPath).Msg("default_skills_source_not_directory")
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Warn().Err(err).Str("skillsPath", skillsPath).Msg("default_skills_source_check_failed")
		}
	}

	a.projectsService = projects.NewService(cfg.Workdir, defaultSkillsDir)
	log.Info().Str("workdir", cfg.Workdir).Msg("projects_filesystem_backend_initialized")

	if err := skills.InitCacheService(skills.CacheServiceConfig{}); err != nil {
		log.Warn().Err(err).Msg("skills_cache_service_init_failed")
	} else {
		log.Info().Msg("skills_cache_service_initialized")
	}
	workspaces.SetSkillsInvalidator(skills.InvalidateCacheForProject)
}

func (a *app) initMetrics(ctx context.Context, cfg *config.Config) {
	if cfg.Obs.Local.IsEnabled() {
		metricsWindow := time.Duration(cfg.Obs.Local.MetricsWindowMinutes) * time.Minute
		memory.ConfigureLocalTelemetry(memory.LocalTelemetryConfig{
			Enabled:   true,
			MaxEvents: cfg.Obs.Local.MaxLogs,
			Retention: metricsWindow,
		})
		processLogs := newProcessLogMetrics(cfg.Obs.Local.MaxLogs)
		observability.AddLogWriter(processLogs)
		a.tokenMetrics = append(a.tokenMetrics, processTokenMetrics{})
		a.memoryMetrics = append(a.memoryMetrics, processMemoryMetrics{})
		a.traceMetrics = append(a.traceMetrics, processTraceMetrics{})
		a.logMetrics = append(a.logMetrics, processLogs)
	} else {
		memory.ConfigureLocalTelemetry(memory.LocalTelemetryConfig{Enabled: false})
	}

	if strings.TrimSpace(cfg.Obs.ClickHouse.DSN) == "" {
		log.Warn().Msg("clickhouse dashboard queries disabled: CLICKHOUSE_DSN is not set")
	}

	if err := ensureClickHouseTables(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("failed to ensure clickhouse tables")
	}

	if tm, err := newClickHouseTokenMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse metrics disabled")
	} else if tm != nil {
		a.tokenMetrics = append([]tokenMetricsProvider{tm}, a.tokenMetrics...)
	}

	if mm, err := newClickHouseMemoryMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse memory metrics disabled")
	} else if mm != nil {
		a.memoryMetrics = append([]memoryMetricsProvider{mm}, a.memoryMetrics...)
	}

	if chTraces, err := newClickHouseTraceMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse trace queries disabled")
	} else if chTraces != nil {
		a.traceMetrics = append([]traceMetricsProvider{chTraces}, a.traceMetrics...)
		a.runMetrics = newClickHouseRunMetrics(chTraces)
	}

	if chLogs, err := newClickHouseLogMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse log queries disabled")
	} else if chLogs != nil {
		a.logMetrics = append([]logMetricsProvider{chLogs}, a.logMetrics...)
	}
}

func optionalPool(ctx context.Context, dsn string) *pgxpool.Pool {
	if strings.TrimSpace(dsn) == "" {
		return nil
	}
	pool, err := databases.OpenPool(ctx, dsn)
	if err != nil {
		return nil
	}
	return pool
}
