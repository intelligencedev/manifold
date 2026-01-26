package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/auth"
	"manifold/internal/config"
	"manifold/internal/httpapi"
	llmpkg "manifold/internal/llm"
	openaillm "manifold/internal/llm/openai"
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/mcpclient"
	"manifold/internal/observability"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	persistdb "manifold/internal/persistence/databases"
	"manifold/internal/playground"
	"manifold/internal/playground/artifacts"
	"manifold/internal/playground/dataset"
	"manifold/internal/playground/eval"
	"manifold/internal/playground/experiment"
	"manifold/internal/playground/provider"
	playgroundregistry "manifold/internal/playground/registry"
	"manifold/internal/playground/worker"
	"manifold/internal/projects"
	"manifold/internal/rag/embedder"
	ragservice "manifold/internal/rag/service"
	"manifold/internal/skills"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	agenttools "manifold/internal/tools/agents"
	"manifold/internal/tools/cli"
	codeevolvetool "manifold/internal/tools/codeevolve"
	"manifold/internal/tools/filetool"
	"manifold/internal/tools/imagetool"
	"manifold/internal/tools/patchtool"
	ragtool "manifold/internal/tools/rag"
	"manifold/internal/tools/textsplitter"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	warpptool "manifold/internal/tools/warpptool"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"
	"manifold/internal/webui"
	"manifold/internal/workspaces"
)

const systemUserID int64 = 0

type app struct {
	cfg                *config.Config
	httpClient         *http.Client
	mgr                *databases.Manager
	llm                llmpkg.Provider
	baseToolRegistry   tools.Registry
	toolRegistry       tools.Registry
	specRegistry       *specialists.Registry
	specRegMu          sync.RWMutex
	userSpecRegs       map[int64]*specialists.Registry
	summaryLLM         llmpkg.Provider
	warppMu            sync.RWMutex
	warppRunner        *warpp.Runner
	warppRegistries    map[int64]*warpp.Registry
	warppStore         persist.WarppWorkflowStore
	evolvingMu         sync.RWMutex
	userEvolving       map[int64]map[string]*memory.EvolvingMemory
	evolvingCfg        memory.EvolvingMemoryConfig
	rememMaxInnerSteps int
	engine             *agent.Engine
	chatStore          persist.ChatStore
	chatMemory         *memory.Manager
	runs               *runStore
	playgroundHandler  http.Handler
	projectsService    projects.ProjectService
	workspaceManager   workspaces.WorkspaceManager
	whisperModel       whisper.Model
	authStore          *auth.Store
	authProvider       auth.Provider
	specStore          persist.SpecialistsStore
	groupStore         persist.SpecialistGroupsStore
	mcpStore           persist.MCPStore
	userPrefsStore     persist.UserPreferencesStore
	mcpManager         *mcpclient.Manager
	mcpPool            *mcpclient.MCPServerPool
	tokenMetrics       tokenMetricsProvider
	traceMetrics       *clickhouseTraceMetrics
	runMetrics         *clickhouseRunMetrics
	logMetrics         *clickhouseLogMetrics
}

type tokenMetricsProvider interface {
	TokenTotals(ctx context.Context, window time.Duration) ([]llmpkg.TokenTotal, time.Duration, error)
	TokenTotalsForUser(ctx context.Context, userID int64, window time.Duration) ([]llmpkg.TokenTotal, time.Duration, error)
	Source() string
}

// cloneEngine returns a shallow copy of the base orchestrator engine so that
// per-request callbacks (OnDelta/OnTool/etc) don't race across concurrent
// requests. Callers can safely mutate the returned engine without affecting
// other in-flight runs.
func (a *app) cloneEngine() *agent.Engine {
	if a.engine == nil {
		return nil
	}
	clone := *a.engine
	clone.OnAssistant = nil
	clone.OnDelta = nil
	clone.OnTool = nil
	clone.OnToolStart = nil
	return &clone
}

// cloneEngineForUser returns a shallow copy of the base engine with user-specific
// orchestrator settings applied (particularly the tool allowlist). This enables
// per-user orchestrator configurations.
func (a *app) cloneEngineForUser(ctx context.Context, userID int64, sessionID string) *agent.Engine {
	eng := a.cloneEngine()
	if eng == nil {
		return nil
	}

	// Ensure the specialists catalog in the system prompt is user-scoped.
	// The base engine prompt is composed with the system (user=0) specialists
	// registry; without this override, non-system users can see system specialists.
	//
	// Do this before applying any per-user orchestrator overlay so we can build a
	// base prompt with the correct catalog.
	if a.cfg.Auth.Enabled && userID != systemUserID {
		eng.System = a.composeSystemPromptForUser(ctx, userID)
	}

	// Attach session-scoped evolving memory/ReMem when configured.
	// This avoids using a shared system-level memory store across sessions.
	if a.evolvingCfg.LLM != nil {
		em := a.getOrCreateEvolvingMemoryForSession(userID, sessionID)
		if em != nil {
			eng.EvolvingMemory = em
			if a.engine != nil && a.engine.ReMemEnabled {
				eng.ReMemEnabled = true
				eng.ReMemController = memory.NewReMemController(memory.ReMemConfig{
					LLM:           a.evolvingCfg.LLM,
					Model:         a.evolvingCfg.Model,
					Memory:        em,
					MaxInnerSteps: a.rememMaxInnerSteps,
				})
			}
		}
	}

	// For system user or when auth is disabled, use the shared engine config
	if !a.cfg.Auth.Enabled || userID == systemUserID {
		return eng
	}

	// Look up user's orchestrator overlay
	sp, ok, err := a.specStore.GetByName(ctx, userID, specialists.OrchestratorName)
	if err != nil || !ok {
		// No per-user orchestrator config; use defaults
		return eng
	}

	// Apply user's LLM overrides (provider/model/extra params).
	llmCfg, provider := specialists.ApplyLLMClientOverride(a.cfg.LLMClient, sp)
	userCfg := *a.cfg
	userCfg.LLMClient = llmCfg
	if provider == "" || provider == "openai" || provider == "local" {
		userCfg.OpenAI = llmCfg.OpenAI
	}
	if userLLM, err := llmproviders.Build(userCfg, a.httpClient); err != nil {
		log.Warn().Err(err).Msg("failed to build per-user llm provider")
	} else {
		eng.LLM = userLLM
	}
	currentModel := strings.TrimSpace(sp.Model)
	if currentModel == "" {
		switch provider {
		case "anthropic":
			currentModel = strings.TrimSpace(llmCfg.Anthropic.Model)
		case "google":
			currentModel = strings.TrimSpace(llmCfg.Google.Model)
		default:
			currentModel = strings.TrimSpace(llmCfg.OpenAI.Model)
		}
	}
	if currentModel != "" {
		eng.Model = currentModel
	}

	// Apply user's tool configuration
	if !sp.EnableTools {
		eng.Tools = tools.NewRegistry() // empty registry
	} else if len(sp.AllowTools) > 0 {
		eng.Tools = tools.NewFilteredRegistry(a.baseToolRegistry, sp.AllowTools)
	} else {
		eng.Tools = a.baseToolRegistry // all tools
	}

	// Apply user's system prompt if set.
	// This should preserve the user-scoped specialists catalog.
	if sp.System != "" {
		eng.System = a.composeSystemPromptForUserWithOverride(ctx, userID, sp.System)
	}

	// Create a per-request delegator so ask_agent/agent_call uses the
	// user-specific specialists registry (including tool allowlists).
	reg := a.specRegistry
	if a.cfg.Auth.Enabled && userID != systemUserID {
		if userReg, err := a.specialistsRegistryForUser(ctx, userID); err == nil && userReg != nil {
			reg = userReg
		}
	}
	delegator := agenttools.NewDelegator(eng.Tools, reg, a.workspaceManager, a.cfg.MaxSteps)
	delegator.SetDefaultTimeout(a.cfg.AgentRunTimeoutSeconds)
	eng.Delegator = delegator

	return eng
}

func (a *app) getOrCreateEvolvingMemoryForSession(userID int64, sessionID string) *memory.EvolvingMemory {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	a.evolvingMu.RLock()
	if a.userEvolving != nil {
		if sessions := a.userEvolving[userID]; sessions != nil {
			if em := sessions[sessionID]; em != nil {
				a.evolvingMu.RUnlock()
				return em
			}
		}
	}
	a.evolvingMu.RUnlock()

	a.evolvingMu.Lock()
	defer a.evolvingMu.Unlock()
	if a.userEvolving == nil {
		a.userEvolving = make(map[int64]map[string]*memory.EvolvingMemory)
	}
	if a.userEvolving[userID] == nil {
		a.userEvolving[userID] = make(map[string]*memory.EvolvingMemory)
	}
	if em := a.userEvolving[userID][sessionID]; em != nil {
		return em
	}
	if a.evolvingCfg.LLM == nil {
		return nil
	}
	cfg := a.evolvingCfg
	cfg.UserID = userID
	cfg.SessionID = sessionID
	em := memory.NewEvolvingMemory(cfg)
	a.userEvolving[userID][sessionID] = em
	return em
}

// Run initialises the agentd server and starts the HTTP listener.
func Run() {
	if err := loadEnv(); err != nil {
		log.Debug().Err(err).Msg("no .env loaded")
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		log.Fatal().Err(err).Msg("failed to load config")
	}

	observability.InitLogger(cfg.LogPath, cfg.LogLevel)

	shutdown, err := observability.InitOTel(context.Background(), cfg.Obs)
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

	ctx := context.Background()
	a, err := newApp(ctx, &cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("initialization failed")
	}

	mux := newRouter(a)
	if err := a.registerFrontend(mux); err != nil {
		log.Error().Err(err).Msg("frontend registration failed")
	}

	root := a.wrapWithMiddleware(mux)

	log.Info().Msg("agentd listening on :32180")
	if err := http.ListenAndServe(":32180", root); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

func loadEnv() error {
	if err := godotenv.Load(".env"); err != nil {
		return godotenv.Load("example.env")
	}
	return nil
}

func newApp(ctx context.Context, cfg *config.Config) (*app, error) {
	httpClient := observability.NewHTTPClient(nil)
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}

	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.OutputTruncateByte)
	llm, err := llmproviders.Build(*cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("build llm provider: %w", err)
	}
	summaryCfg := cfg.OpenAI
	summaryCfg.Model = cfg.OpenAI.SummaryModel
	summaryCfg.BaseURL = cfg.OpenAI.SummaryBaseURL
	summaryLLM := openaillm.New(summaryCfg, httpClient)

	toolRegistry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	baseToolRegistry := toolRegistry

	mgr, err := databases.NewManager(ctx, cfg.Databases)
	if err != nil {
		return nil, fmt.Errorf("init databases: %w", err)
	}

	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	toolRegistry.Register(cli.NewTool(exec))
	toolRegistry.Register(web.NewTool(cfg.Web.SearXNGURL))
	toolRegistry.Register(web.NewScreenshotTool())
	toolRegistry.Register(web.NewFetchTool(mgr.Search))
	toolRegistry.Register(patchtool.New(cfg.Workdir))
	allowedRoots := []string{cfg.Workdir}
	toolRegistry.Register(filetool.NewReadTool(allowedRoots, cfg.OutputTruncateByte))
	toolRegistry.Register(filetool.NewWriteTool(allowedRoots, 0))
	toolRegistry.Register(filetool.NewPatchTool(allowedRoots, 0))
	toolRegistry.Register(filetool.NewDeleteTool(allowedRoots))
	toolRegistry.Register(textsplitter.New())
	toolRegistry.Register(utility.NewTextboxTool())
	toolRegistry.Register(tts.New(*cfg, httpClient))

	// Register RAG tools backed by the internal rag service.
	// Create a real embedder using the configured embedding service.
	emb := embedder.NewClient(cfg.Embedding, cfg.Databases.Vector.Dimensions)
	if err := emb.Ping(ctx); err != nil {
		return nil, fmt.Errorf("embedding service reachability check failed: %w", err)
	}
	toolRegistry.Register(ragtool.NewIngestTool(mgr, ragservice.WithEmbedder(emb)))
	toolRegistry.Register(ragtool.NewRetrieveTool(mgr, ragservice.WithEmbedder(emb)))

	// Register the AlphaEvolve-inspired code evolution tool.
	toolRegistry.Register(codeevolvetool.New(cfg, llm))

	newProv := func(baseURL string) llmpkg.Provider {
		switch cfg.LLMClient.Provider {
		case "", "openai", "local":
			cfgCopy := cfg.LLMClient.OpenAI
			cfgCopy.BaseURL = baseURL
			return openaillm.New(cfgCopy, httpClient)
		default:
			return llm
		}
	}
	toolRegistry.Register(imagetool.NewDescribeTool(llm, cfg.Workdir, cfg.OpenAI.Model, newProv))

	// Initialize workspace manager (local filesystem only).
	wsMgr := workspaces.NewManager(cfg)
	log.Info().Str("mode", wsMgr.Mode()).Msg("workspace_manager_initialized")

	specReg := specialists.NewRegistry(cfg.LLMClient, cfg.Specialists, httpClient, toolRegistry)
	specReg.SetWorkdir(cfg.Workdir)

	// Register specialist routing tools.
	agentCallTool := agenttools.NewAgentCallTool(toolRegistry, specReg, wsMgr)
	agentCallTool.SetDefaultTimeoutSeconds(cfg.AgentRunTimeoutSeconds)
	toolRegistry.Register(agentCallTool)
	toolRegistry.Register(agenttools.NewAskAgentTool(httpClient, "http://127.0.0.1:32180", cfg.AgentRunTimeoutSeconds))

	if !cfg.EnableTools {
		toolRegistry = tools.NewRegistry()
	} else if len(cfg.ToolAllowList) > 0 {
		allowList := make([]string, 0, len(cfg.ToolAllowList))
		for _, name := range cfg.ToolAllowList {
			allowList = append(allowList, name)
		}
		toolRegistry = tools.NewFilteredRegistry(baseToolRegistry, allowList)
	}

	{
		names := make([]string, 0, len(toolRegistry.Schemas()))
		for _, s := range toolRegistry.Schemas() {
			names = append(names, s.Name)
		}
		log.Info().Bool("enableTools", cfg.EnableTools).Strs("allowList", cfg.ToolAllowList).Strs("tools", names).Msg("tool_registry_contents")
	}

	mcpMgr := mcpclient.NewManager()
	ctxInit, cancelInit := context.WithTimeout(ctx, 20*time.Second)
	_ = mcpMgr.RegisterFromConfig(ctxInit, baseToolRegistry, cfg.MCP)

	requiresPerUserMCP := false
	if cfg.Auth.Enabled {
		for _, srv := range cfg.MCP.Servers {
			if srv.PathDependent {
				requiresPerUserMCP = true
				break
			}
		}
	}

	// Load MCP servers from the system user store.
	if mgr.MCP != nil {
		if servers, err := mgr.MCP.List(ctxInit, systemUserID); err == nil {
			// When auth is enabled we must NOT start path-dependent servers as shared singletons,
			// since they require a real project workspace path. Those are managed by MCPServerPool.
			pathDependentNames := map[string]bool{}
			for _, s := range cfg.MCP.Servers {
				if s.PathDependent {
					pathDependentNames[s.Name] = true
				}
			}

			for _, s := range servers {
				if s.Disabled {
					continue
				}
				cfgSrv := convertToConfig(s)
				// Skip persisted servers that are path-dependent in current config, or that still
				// contain {{PROJECT_DIR}} placeholders (older records won't have PathDependent set).
				if requiresPerUserMCP {
					if pathDependentNames[s.Name] {
						log.Debug().Str("server", s.Name).Msg("skipping_path_dependent_mcp_server_from_db")
						continue
					}
					isPlaceholder := false
					for _, arg := range cfgSrv.Args {
						if strings.Contains(arg, "{{PROJECT_DIR}}") {
							isPlaceholder = true
							break
						}
					}
					if !isPlaceholder {
						for _, v := range cfgSrv.Env {
							if strings.Contains(v, "{{PROJECT_DIR}}") {
								isPlaceholder = true
								break
							}
						}
					}
					if isPlaceholder {
						log.Debug().Str("server", s.Name).Msg("skipping_placeholder_mcp_server_from_db")
						continue
					}
				}
				_ = mcpMgr.RegisterOne(ctxInit, baseToolRegistry, cfgSrv)
			}
		} else {
			log.Warn().Err(err).Msg("failed to load mcp servers from db")
		}
	}
	cancelInit()

	// Create MCP Server Pool for managing shared and per-user MCP sessions
	mcpPool := mcpclient.NewMCPServerPool(cfg, wsMgr, mgr.UserPreferences)
	mcpPool.SetToolRegistry(baseToolRegistry)

	// Wire workspace checkout callback to initialize MCP sessions on checkout
	workspaces.SetCheckoutCallback(mcpPool.OnWorkspaceCheckout)

	// Register non-path-dependent servers to the pool (shared)
	// Path-dependent servers are registered per-user on project switch when auth is enabled
	ctxPool, cancelPool := context.WithTimeout(ctx, 20*time.Second)
	if err := mcpPool.RegisterFromConfig(ctxPool, baseToolRegistry); err != nil {
		log.Warn().Err(err).Msg("mcp_pool_registration_failed")
	}

	// Discover and register tools from path-dependent MCP servers for UI display
	// This temporarily starts servers with a temp directory just to enumerate tools
	if mcpPool.RequiresPerUserMCP() {
		mcpPool.RegisterPathDependentToolsForDiscovery(ctxPool, baseToolRegistry)
	}
	cancelPool()

	// Start idle session reaper for per-user MCP sessions (15 min check interval, 1 hour max idle)
	if mcpPool.RequiresPerUserMCP() {
		mcpPool.StartReaper(ctx, baseToolRegistry, 15*time.Minute, 1*time.Hour)
	}

	app := &app{
		cfg:              cfg,
		httpClient:       httpClient,
		mgr:              &mgr,
		llm:              llm,
		summaryLLM:       summaryLLM,
		baseToolRegistry: baseToolRegistry,
		toolRegistry:     toolRegistry,
		specRegistry:     specReg,
		userSpecRegs:     map[int64]*specialists.Registry{systemUserID: specReg},
		runs:             newRunStore(),
		mcpStore:         mgr.MCP,
		userPrefsStore:   mgr.UserPreferences,
		mcpManager:       mcpMgr,
		mcpPool:          mcpPool,
		workspaceManager: wsMgr,
	}

	systemPrompt := app.composeSystemPrompt()

	// Register WARPP workflows as callable tools for the runtime.
	if err := app.initWarpp(ctx, toolRegistry); err != nil {
		return nil, err
	}

	// Detect an approximate context window for the main model so summarization
	// auto-mode can size history appropriately.
	ctxSize, _ := llmpkg.ContextSize(cfg.OpenAI.Model)
	app.engine = &agent.Engine{
		LLM:                          llm,
		Tools:                        toolRegistry,
		MaxSteps:                     cfg.MaxSteps,
		MaxToolParallelism:           cfg.MaxToolParallelism,
		System:                       systemPrompt,
		Model:                        cfg.OpenAI.Model,
		ContextWindowTokens:          ctxSize,
		SummaryEnabled:               cfg.SummaryEnabled,
		SummaryReserveBufferTokens:   cfg.SummaryReserveBufferTokens,
		SummaryMinKeepLastMessages:   cfg.SummaryMinKeepLastMessages,
		SummaryMaxSummaryChunkTokens: cfg.SummaryMaxSummaryChunkTokens,
	}
	app.engine.AttachTokenizer(llm, nil)

	delegator := agenttools.NewDelegator(toolRegistry, specReg, wsMgr, cfg.MaxSteps)
	delegator.SetDefaultTimeout(cfg.AgentRunTimeoutSeconds)
	app.engine.Delegator = delegator

	// Initialize evolving memory if enabled
	if cfg.EvolvingMemory.Enabled {
		// Prefer OpenAI summary client for evolving memory if available,
		// to avoid provider/model mismatches (e.g., Google v1beta vs OpenAI models).
		memLLM := llm
		memModel := cfg.EvolvingMemory.Model
		if summaryLLM != nil && strings.TrimSpace(cfg.OpenAI.SummaryModel) != "" {
			memLLM = summaryLLM
			memModel = cfg.OpenAI.SummaryModel
		}

		var evStore memory.EvolvingMemoryStore
		if mgr.EvolvingMemory != nil {
			if s, ok := mgr.EvolvingMemory.(memory.EvolvingMemoryStore); ok {
				evStore = s
			}
		}

		app.evolvingCfg = memory.EvolvingMemoryConfig{
			EmbeddingConfig:  cfg.Embedding,
			LLM:              memLLM,
			Model:            memModel,
			MaxSize:          cfg.EvolvingMemory.MaxSize,
			TopK:             cfg.EvolvingMemory.TopK,
			WindowSize:       cfg.EvolvingMemory.WindowSize,
			EnableRAG:        cfg.EvolvingMemory.EnableRAG,
			EnableSmartPrune: cfg.EvolvingMemory.EnableSmartPrune,
			PruneThreshold:   cfg.EvolvingMemory.PruneThreshold,
			RelevanceDecay:   cfg.EvolvingMemory.RelevanceDecay,
			MinRelevance:     cfg.EvolvingMemory.MinRelevance,
			Store:            evStore,
			UserID:           systemUserID,
		}
		app.rememMaxInnerSteps = cfg.EvolvingMemory.MaxInnerSteps

		app.engine.EvolvingMemory = memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
			EmbeddingConfig:  cfg.Embedding,
			LLM:              memLLM,
			Model:            memModel,
			MaxSize:          cfg.EvolvingMemory.MaxSize,
			TopK:             cfg.EvolvingMemory.TopK,
			WindowSize:       cfg.EvolvingMemory.WindowSize,
			EnableRAG:        cfg.EvolvingMemory.EnableRAG,
			EnableSmartPrune: cfg.EvolvingMemory.EnableSmartPrune,
			PruneThreshold:   cfg.EvolvingMemory.PruneThreshold,
			RelevanceDecay:   cfg.EvolvingMemory.RelevanceDecay,
			MinRelevance:     cfg.EvolvingMemory.MinRelevance,
			Store:            evStore,
			UserID:           systemUserID,
		})
		log.Info().
			Bool("enabled", true).
			Int("maxSize", cfg.EvolvingMemory.MaxSize).
			Int("topK", cfg.EvolvingMemory.TopK).
			Bool("rag", cfg.EvolvingMemory.EnableRAG).
			Bool("smartPrune", cfg.EvolvingMemory.EnableSmartPrune).
			Msg("evolving_memory_initialized")

		// Initialize ReMem controller if enabled
		if cfg.EvolvingMemory.ReMemEnabled {
			app.engine.ReMemEnabled = true
			app.engine.ReMemController = memory.NewReMemController(memory.ReMemConfig{
				LLM:           memLLM,
				Model:         memModel,
				Memory:        app.engine.EvolvingMemory,
				MaxInnerSteps: cfg.EvolvingMemory.MaxInnerSteps,
			})
			log.Info().
				Bool("remem_enabled", true).
				Int("maxInnerSteps", cfg.EvolvingMemory.MaxInnerSteps).
				Msg("remem_controller_initialized")
		}
	}

	app.chatStore = mgr.Chat
	if app.chatStore == nil {
		return nil, fmt.Errorf("chat store not initialized")
	}
	// Derive a context window for chat-memory budgeting.
	// Even if the underlying model supports very large context windows (e.g. GPT-5),
	// we intentionally cap the default budgeting window so the orchestrator doesn't
	// receive an excessively long raw transcript every turn.
	summaryCtxSize, _ := llmpkg.ContextSize(cfg.OpenAI.SummaryModel)
	if cfg.SummaryContextWindowTokens > 0 {
		summaryCtxSize = cfg.SummaryContextWindowTokens
	} else {
		const defaultSummaryContextWindowCap = 32_000
		if summaryCtxSize <= 0 || summaryCtxSize > defaultSummaryContextWindowCap {
			summaryCtxSize = defaultSummaryContextWindowCap
		}
	}
	useResponsesCompaction := (cfg.LLMClient.Provider == "" || cfg.LLMClient.Provider == "openai") &&
		strings.EqualFold(cfg.OpenAI.API, "responses")
	app.chatMemory = memory.NewManager(app.chatStore, summaryLLM, memory.Config{
		Enabled:                cfg.SummaryEnabled,
		ReserveBufferTokens:    cfg.SummaryReserveBufferTokens,
		MinKeepLastMessages:    cfg.SummaryMinKeepLastMessages,
		MaxKeepLastMessages:    cfg.SummaryMaxKeepLastMessages,
		MaxSummaryChunkTokens:  cfg.SummaryMaxSummaryChunkTokens,
		ContextWindowTokens:    summaryCtxSize,
		SummaryModel:           cfg.OpenAI.SummaryModel,
		UseResponsesCompaction: useResponsesCompaction,
	})

	if mgr.Playground == nil {
		return nil, fmt.Errorf("playground store not initialized; set databases.defaultDSN or chat DSN")
	}
	artifactDir := filepath.Join(cfg.Workdir, "playground-artifacts")
	artifactStore := artifacts.NewFilesystemStore(artifactDir)
	playgroundRegistry := playgroundregistry.New(mgr.Playground)
	playgroundDataset := dataset.NewService(mgr.Playground)
	playgroundRepo := experiment.NewRepository()
	playgroundPlanner := experiment.NewPlanner(experiment.PlannerConfig{MaxRowsPerShard: 32, MaxVariantsPerShard: 4})
	playgroundProvider := provider.NewLLMAdapter(llm, cfg.OpenAI.Model)
	playgroundWorker := worker.NewWorker(playgroundProvider, artifactStore)
	playgroundEvals := eval.NewRunner(eval.NewRegistry(), playgroundProvider)
	playgroundService := playground.NewService(playground.Config{MaxConcurrentShards: 4}, playgroundRegistry, playgroundDataset, playgroundRepo, playgroundPlanner, playgroundWorker, playgroundEvals, mgr.Playground)
	app.playgroundHandler = httpapi.NewServer(playgroundService)

	// Filesystem backend only.
	fsService := projects.NewService(cfg.Workdir)
	app.projectsService = fsService
	log.Info().Str("workdir", cfg.Workdir).Msg("projects_filesystem_backend_initialized")

	// Initialize skills cache service (local only).
	if err := skills.InitCacheService(skills.CacheServiceConfig{}); err != nil {
		log.Warn().Err(err).Msg("skills_cache_service_init_failed")
	} else {
		log.Info().Msg("skills_cache_service_initialized")
	}
	// Register skills invalidator with workspaces package to break import cycle
	workspaces.SetSkillsInvalidator(skills.InvalidateCacheForProject)

	app.whisperModel = app.loadWhisperModel("models/ggml-small.en.bin")

	if err := app.initAuth(ctx); err != nil {
		return nil, err
	}

	if err := app.initSpecialists(ctx); err != nil {
		return nil, err
	}

	// Ensure ClickHouse tables exist before initializing metrics providers.
	if err := ensureClickHouseTables(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("failed to ensure clickhouse tables")
	}

	if tm, err := newClickHouseTokenMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse metrics disabled")
	} else if tm != nil {
		app.tokenMetrics = tm
	}

	if chTraces, err := newClickHouseTraceMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse trace queries disabled")
	} else if chTraces != nil {
		app.traceMetrics = chTraces
		app.runMetrics = newClickHouseRunMetrics(chTraces)
	}

	if chLogs, err := newClickHouseLogMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse log queries disabled")
	} else if chLogs != nil {
		app.logMetrics = chLogs
	}

	_ = mcpMgr // ensure lifetime; manager currently long-lived

	// Refresh OAuth tokens for cached remote MCP servers and register them.
	// This runs after app is fully initialized so we have access to httpClient
	// and other dependencies needed for OAuth token refresh.
	if mgr.MCP != nil {
		ctxRefresh, cancelRefresh := context.WithTimeout(ctx, 30*time.Second)
		if err := app.RefreshMCPServersOnStartup(ctxRefresh, systemUserID); err != nil {
			log.Warn().Err(err).Msg("mcp_oauth_refresh_on_startup_failed")
		}
		cancelRefresh()
	}

	return app, nil
}

func (a *app) initWarpp(ctx context.Context, toolRegistry tools.Registry) error {
	const workflowDir = "configs/workflows"
	var wfreg *warpp.Registry
	var wfStore persist.WarppWorkflowStore

	if a.cfg.Databases.DefaultDSN != "" {
		if p, errPool := databases.OpenPool(ctx, a.cfg.Databases.DefaultDSN); errPool == nil {
			wfStore = persistdb.NewPostgresWarppStore(p)
		}
	}
	if wfStore == nil {
		wfStore = persistdb.NewPostgresWarppStore(nil)
	}
	_ = wfStore.Init(ctx)

	warpp.SetDefaultStore(wfStore)

	if list, err := wfStore.ListWorkflows(ctx, systemUserID); err == nil && len(list) > 0 {
		wfreg = &warpp.Registry{}
		for _, pw := range list {
			b, _ := json.Marshal(pw)
			var w warpp.Workflow
			if err := json.Unmarshal(b, &w); err == nil {
				wfreg.Upsert(w, "")
			}
		}
	} else {
		wfreg, _ = warpp.LoadFromDir(workflowDir)
		for _, w := range wfreg.All() {
			b, _ := json.Marshal(w)
			var pw persist.WarppWorkflow
			if err := json.Unmarshal(b, &pw); err == nil {
				_, _ = wfStore.Upsert(ctx, systemUserID, pw)
			}
		}
	}

	a.warppRegistries = map[int64]*warpp.Registry{systemUserID: wfreg}
	a.warppRunner = &warpp.Runner{Workflows: wfreg, Tools: toolRegistry}
	a.warppStore = wfStore
	// Register WARPP workflows as tools (warpp_<intent>) so they can be invoked directly.
	warpptool.RegisterAll(toolRegistry, a.warppRunner)
	return nil
}

func (a *app) loadWhisperModel(modelPath string) whisper.Model {
	model, err := whisper.New(modelPath)
	if err == nil {
		log.Info().Str("model", modelPath).Msg("whisper model loaded")
		return model
	}
	log.Warn().Str("model", modelPath).Err(err).Msg("whisper model load failed; /stt disabled")
	return nil
}

func (a *app) initAuth(ctx context.Context) error {
	if !a.cfg.Auth.Enabled {
		return nil
	}

	dsn := a.cfg.Databases.DefaultDSN
	if dsn == "" {
		return fmt.Errorf("auth enabled but databases.defaultDSN is empty")
	}
	pool, err := databases.OpenPool(ctx, dsn)
	if err != nil {
		return fmt.Errorf("auth db connect failed: %w", err)
	}
	a.authStore = auth.NewStore(pool, a.cfg.Auth.SessionTTLHours)
	if err := a.authStore.InitSchema(ctx); err != nil {
		return fmt.Errorf("auth schema init failed: %w", err)
	}
	_ = a.authStore.EnsureDefaultRoles(ctx)

	providerName := strings.ToLower(strings.TrimSpace(a.cfg.Auth.Provider))
	if providerName == "" {
		providerName = "oidc"
	}
	switch providerName {
	case "oidc":
		if strings.TrimSpace(a.cfg.Auth.IssuerURL) == "" {
			return fmt.Errorf("auth.provider=oidc requires issuerURL")
		}
		if strings.TrimSpace(a.cfg.Auth.ClientID) == "" || strings.TrimSpace(a.cfg.Auth.ClientSecret) == "" {
			return fmt.Errorf("auth.provider=oidc requires clientID and clientSecret")
		}
		oidcAuth, err := auth.NewOIDC(
			ctx,
			a.cfg.Auth.IssuerURL,
			a.cfg.Auth.ClientID,
			a.cfg.Auth.ClientSecret,
			a.cfg.Auth.RedirectURL,
			a.authStore,
			a.cfg.Auth.CookieName,
			a.cfg.Auth.AllowedDomains,
			a.cfg.Auth.StateTTLSeconds,
			a.cfg.Auth.CookieSecure,
		)
		if err != nil {
			return fmt.Errorf("oidc init failed: %w", err)
		}
		a.authProvider = oidcAuth
	case "oauth2":
		opts := auth.OAuth2Options{
			ClientID:            a.cfg.Auth.ClientID,
			ClientSecret:        a.cfg.Auth.ClientSecret,
			RedirectURL:         a.cfg.Auth.RedirectURL,
			AuthURL:             a.cfg.Auth.OAuth2.AuthURL,
			TokenURL:            a.cfg.Auth.OAuth2.TokenURL,
			UserInfoURL:         a.cfg.Auth.OAuth2.UserInfoURL,
			LogoutURL:           a.cfg.Auth.OAuth2.LogoutURL,
			LogoutRedirectParam: a.cfg.Auth.OAuth2.LogoutRedirectParam,
			Scopes:              a.cfg.Auth.OAuth2.Scopes,
			ProviderName:        a.cfg.Auth.OAuth2.ProviderName,
			DefaultRoles:        a.cfg.Auth.OAuth2.DefaultRoles,
			EmailField:          a.cfg.Auth.OAuth2.EmailField,
			NameField:           a.cfg.Auth.OAuth2.NameField,
			PictureField:        a.cfg.Auth.OAuth2.PictureField,
			SubjectField:        a.cfg.Auth.OAuth2.SubjectField,
			RolesField:          a.cfg.Auth.OAuth2.RolesField,
			CookieName:          a.cfg.Auth.CookieName,
			AllowedDomains:      a.cfg.Auth.AllowedDomains,
			StateTTLSeconds:     a.cfg.Auth.StateTTLSeconds,
			TempCookieSecure:    a.cfg.Auth.CookieSecure,
			HTTPClient:          a.httpClient,
			DisablePKCE:         a.cfg.Auth.OAuth2.DisablePKCE,
		}
		oauthProvider, err := auth.NewOAuth2(ctx, a.authStore, opts)
		if err != nil {
			return fmt.Errorf("oauth2 init failed: %w", err)
		}
		a.authProvider = oauthProvider
	default:
		return fmt.Errorf("unsupported auth provider: %s", a.cfg.Auth.Provider)
	}
	return nil
}

func (a *app) initSpecialists(ctx context.Context) error {
	var pg *pgxpool.Pool
	if a.cfg.Databases.DefaultDSN != "" {
		if p, err := databases.OpenPool(ctx, a.cfg.Databases.DefaultDSN); err == nil {
			pg = p
		}
	}
	specStore := databases.NewSpecialistsStore(pg)
	_ = specStore.Init(ctx)
	a.specStore = specStore
	groupStore := databases.NewSpecialistGroupsStore(pg)
	_ = groupStore.Init(ctx)
	a.groupStore = groupStore

	if err := specialists.SeedStore(ctx, specStore, systemUserID, a.cfg.Specialists); err != nil {
		log.Warn().Err(err).Msg("seed specialists")
	}

	if list, err := specStore.List(ctx, systemUserID); err == nil {
		a.specRegistry.ReplaceFromConfigs(a.cfg.LLMClient, specialists.ConfigsFromStore(list), a.httpClient, a.baseToolRegistry)
	}
	a.refreshEngineSystemPrompt()

	if sp, ok, _ := specStore.GetByName(ctx, systemUserID, specialists.OrchestratorName); ok {
		if err := a.applyOrchestratorUpdate(ctx, sp); err != nil {
			log.Warn().Err(err).Msg("failed to apply orchestrator overlay")
		}
	} else {
		a.cfg.SystemPrompt = specialists.DefaultOrchestratorPrompt
		a.refreshEngineSystemPrompt()
	}

	return nil
}

func (a *app) wrapWithMiddleware(handler http.Handler) http.Handler {
	if a.cfg.Auth.Enabled && a.authStore != nil {
		return auth.Middleware(a.authStore, a.cfg.Auth.CookieName, false)(handler)
	}
	return handler
}

func (a *app) registerFrontend(mux *http.ServeMux) error {
	frontendProxy := os.Getenv("FRONTEND_DEV_PROXY")
	opts := webui.Options{DevProxy: frontendProxy}
	if a.cfg.Auth.Enabled {
		opts.AuthGate = func(r *http.Request) bool {
			_, ok := auth.CurrentUser(r.Context())
			return ok
		}
		opts.UnauthedRedirect = "/auth/login"
	}
	if err := webui.RegisterFrontend(mux, opts); err != nil {
		return err
	}
	if frontendProxy != "" {
		log.Info().Str("url", frontendProxy).Msg("frontend dev proxy enabled")
	}
	return nil
}
