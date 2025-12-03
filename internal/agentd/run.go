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
	"manifold/internal/specialists"
	"manifold/internal/tools"
	agenttools "manifold/internal/tools/agents"
	"manifold/internal/tools/cli"
	codeevolvetool "manifold/internal/tools/codeevolve"
	"manifold/internal/tools/imagetool"
	kafkatools "manifold/internal/tools/kafka"
	"manifold/internal/tools/multitool"
	"manifold/internal/tools/patchtool"
	ragtool "manifold/internal/tools/rag"
	"manifold/internal/tools/textsplitter"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	warpptool "manifold/internal/tools/warpptool"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"
	"manifold/internal/webui"
)

const systemUserID int64 = 0

type app struct {
	cfg               *config.Config
	httpClient        *http.Client
	mgr               *databases.Manager
	llm               llmpkg.Provider
	baseToolRegistry  tools.Registry
	toolRegistry      tools.Registry
	specRegistry      *specialists.Registry
	specRegMu         sync.RWMutex
	userSpecRegs      map[int64]*specialists.Registry
	summaryLLM        llmpkg.Provider
	warppMu           sync.RWMutex
	warppRunner       *warpp.Runner
	warppRegistries   map[int64]*warpp.Registry
	warppStore        persist.WarppWorkflowStore
	engine            *agent.Engine
	chatStore         persist.ChatStore
	chatMemory        *memory.Manager
	runs              *runStore
	playgroundHandler http.Handler
	projectsService   *projects.Service
	whisperModel      whisper.Model
	authStore         *auth.Store
	authProvider      auth.Provider
	specStore         persist.SpecialistsStore
	mcpStore          persist.MCPStore
	mcpManager        *mcpclient.Manager
	tokenMetrics      tokenMetricsProvider
}

type tokenMetricsProvider interface {
	TokenTotals(ctx context.Context, window time.Duration) ([]llmpkg.TokenTotal, time.Duration, error)
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
	toolRegistry.Register(textsplitter.New())
	toolRegistry.Register(utility.NewTextboxTool())
	toolRegistry.Register(tts.New(*cfg, httpClient))

	// Kafka tool for publishing messages
	if cfg.Kafka.Brokers != "" {
		if producer, err := kafkatools.NewProducerFromBrokers(cfg.Kafka.Brokers); err == nil {
			// NewSendMessageTool will auto-detect orchestrator commands topics by pattern matching
			toolRegistry.Register(kafkatools.NewSendMessageTool(producer))
		} else {
			log.Warn().Err(err).Msg("kafka tool registration failed, continuing without kafka support")
		}
	}

	// RAG tools backed by internal/rag Service
	// Create a real embedder using the configured embedding service
	emb := embedder.NewClient(cfg.Embedding, cfg.Databases.Vector.Dimensions)
	toolRegistry.Register(ragtool.NewIngestTool(mgr, ragservice.WithEmbedder(emb)))
	toolRegistry.Register(ragtool.NewRetrieveTool(mgr, ragservice.WithEmbedder(emb)))

	// AlphaEvolve-inspired code evolution tool
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

	specReg := specialists.NewRegistry(cfg.LLMClient, cfg.Specialists, httpClient, toolRegistry)

	// Phase 1: register simple team tools
	agentCallTool := agenttools.NewAgentCallTool(toolRegistry, specReg, cfg.Workdir)
	agentCallTool.SetDefaultTimeoutSeconds(cfg.AgentRunTimeoutSeconds)
	toolRegistry.Register(agentCallTool)
	toolRegistry.Register(agenttools.NewAskAgentTool(httpClient, "http://127.0.0.1:32180", cfg.AgentRunTimeoutSeconds))

	parallelTool := multitool.NewParallel(toolRegistry)
	toolRegistry.Register(parallelTool)

	if !cfg.EnableTools {
		toolRegistry = tools.NewRegistry()
	} else if len(cfg.ToolAllowList) > 0 {
		allowList := make([]string, 0, len(cfg.ToolAllowList))
		for _, name := range cfg.ToolAllowList {
			if name == "multi_tool_use.parallel" {
				name = multitool.ToolName
			}
			allowList = append(allowList, name)
		}
		toolRegistry = tools.NewFilteredRegistry(baseToolRegistry, allowList)
	}

	parallelTool.SetRegistry(toolRegistry)

	{
		names := make([]string, 0, len(toolRegistry.Schemas()))
		for _, s := range toolRegistry.Schemas() {
			names = append(names, s.Name)
		}
		log.Info().Bool("enableTools", cfg.EnableTools).Strs("allowList", cfg.ToolAllowList).Strs("tools", names).Msg("tool_registry_contents")
	}

	mcpMgr := mcpclient.NewManager()
	ctxInit, cancelInit := context.WithTimeout(ctx, 20*time.Second)
	_ = mcpMgr.RegisterFromConfig(ctxInit, toolRegistry, cfg.MCP)

	// Load from DB (System User)
	if mgr.MCP != nil {
		if servers, err := mgr.MCP.List(ctxInit, systemUserID); err == nil {
			for _, s := range servers {
				if s.Disabled {
					continue
				}
				_ = mcpMgr.RegisterOne(ctxInit, toolRegistry, convertToConfig(s))
			}
		} else {
			log.Warn().Err(err).Msg("failed to load mcp servers from db")
		}
	}
	cancelInit()

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
		mcpManager:       mcpMgr,
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
		System:                       systemPrompt,
		Model:                        cfg.OpenAI.Model,
		ContextWindowTokens:          ctxSize,
		SummaryEnabled:               cfg.SummaryEnabled,
		SummaryThreshold:             cfg.SummaryThreshold,
		SummaryKeepLast:              cfg.SummaryKeepLast,
		SummaryMode:                  cfg.SummaryMode,
		SummaryTargetUtilizationPct:  cfg.SummaryTargetUtilizationPct,
		SummaryMinKeepLastMessages:   cfg.SummaryMinKeepLastMessages,
		SummaryMaxSummaryChunkTokens: cfg.SummaryMaxSummaryChunkTokens,
	}

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

		app.engine.EvolvingMemory = memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
			EmbeddingConfig: cfg.Embedding,
			LLM:             memLLM,
			Model:           memModel,
			MaxSize:         cfg.EvolvingMemory.MaxSize,
			TopK:            cfg.EvolvingMemory.TopK,
			WindowSize:      cfg.EvolvingMemory.WindowSize,
			EnableRAG:       cfg.EvolvingMemory.EnableRAG,
			Store:           evStore,
			UserID:          systemUserID,
		})
		log.Info().
			Bool("enabled", true).
			Int("maxSize", cfg.EvolvingMemory.MaxSize).
			Int("topK", cfg.EvolvingMemory.TopK).
			Bool("rag", cfg.EvolvingMemory.EnableRAG).
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
	// Derive a context window for the summary model (may differ from main model).
	summaryCtxSize, _ := llmpkg.ContextSize(cfg.OpenAI.SummaryModel)
	app.chatMemory = memory.NewManager(app.chatStore, summaryLLM, memory.Config{
		Enabled:               cfg.SummaryEnabled,
		Mode:                  memory.MemoryMode(cfg.SummaryMode),
		Threshold:             cfg.SummaryThreshold,
		KeepLast:              cfg.SummaryKeepLast,
		TargetUtilizationPct:  cfg.SummaryTargetUtilizationPct,
		MinKeepLastMessages:   cfg.SummaryMinKeepLastMessages,
		MaxSummaryChunkTokens: cfg.SummaryMaxSummaryChunkTokens,
		ContextWindowTokens:   summaryCtxSize,
		SummaryModel:          cfg.OpenAI.SummaryModel,
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

	app.projectsService = projects.NewService(cfg.Workdir)
	if cfg.Projects.Encrypt {
		if err := app.projectsService.EnableEncryption(true); err != nil {
			return nil, fmt.Errorf("enable project encryption failed: %w", err)
		}
	}

	app.whisperModel = app.loadWhisperModel("models/ggml-small.en.bin")

	if err := app.initAuth(ctx); err != nil {
		return nil, err
	}

	if err := app.initSpecialists(ctx); err != nil {
		return nil, err
	}

	if tm, err := newClickHouseTokenMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse metrics disabled")
	} else if tm != nil {
		app.tokenMetrics = tm
	}

	_ = mcpMgr // ensure lifetime; manager currently long-lived

	return app, nil
}

func (a *app) initWarpp(ctx context.Context, toolRegistry tools.Registry) error {
	const workflowDir = "configs/workflows"
	var wfreg *warpp.Registry
	var wfStore persist.WarppWorkflowStore

	if a.cfg.Databases.DefaultDSN != "" {
		if p, errPool := databasesTestPool(ctx, a.cfg.Databases.DefaultDSN); errPool == nil {
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
	// Register WARPP workflows as tools (warpp_<intent>) so they can be invoked directly
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
	pool, err := databasesTestPool(ctx, dsn)
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
		if p, err := databasesTestPool(ctx, a.cfg.Databases.DefaultDSN); err == nil {
			pg = p
		}
	}
	specStore := databases.NewSpecialistsStore(pg)
	_ = specStore.Init(ctx)
	a.specStore = specStore

	if list, err := specStore.List(ctx, systemUserID); err == nil {
		existing := map[string]bool{}
		for _, s := range list {
			existing[s.Name] = true
		}
		for _, sc := range a.cfg.Specialists {
			if sc.Name == "" || existing[sc.Name] {
				continue
			}
			_, _ = specStore.Upsert(ctx, systemUserID, persist.Specialist{
				Name: sc.Name, Provider: sc.Provider, Description: sc.Description, BaseURL: sc.BaseURL, APIKey: sc.APIKey, Model: sc.Model,
				EnableTools: sc.EnableTools, Paused: sc.Paused, AllowTools: sc.AllowTools,
				ReasoningEffort: sc.ReasoningEffort, System: sc.System,
				ExtraHeaders: sc.ExtraHeaders, ExtraParams: sc.ExtraParams,
			})
		}
	}

	if list, err := specStore.List(ctx, systemUserID); err == nil {
		a.specRegistry.ReplaceFromConfigs(a.cfg.LLMClient, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
	}
	a.refreshEngineSystemPrompt()

	if sp, ok, _ := specStore.GetByName(ctx, systemUserID, "orchestrator"); ok {
		if err := a.applyOrchestratorUpdate(ctx, sp); err != nil {
			log.Warn().Err(err).Msg("failed to apply orchestrator overlay")
		}
	} else {
		a.cfg.SystemPrompt = "You are a helpful assistant with access to tools and specialists to help you complete objectives."
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
