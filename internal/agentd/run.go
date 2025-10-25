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
	"manifold/internal/agent/prompts"
	"manifold/internal/auth"
	"manifold/internal/config"
	"manifold/internal/httpapi"
	llmpkg "manifold/internal/llm"
	openaillm "manifold/internal/llm/openai"
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
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	"manifold/internal/tools/db"
	"manifold/internal/tools/imagetool"
	kafkatools "manifold/internal/tools/kafka"
	llmtools "manifold/internal/tools/llmtool"
	"manifold/internal/tools/multitool"
	"manifold/internal/tools/patchtool"
	specialiststool "manifold/internal/tools/specialists"
	"manifold/internal/tools/textsplitter"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"
	"manifold/internal/webui"
)

const systemUserID int64 = 0

type app struct {
	cfg               *config.Config
	httpClient        *http.Client
	mgr               *databases.Manager
	llm               *openaillm.Client
	baseToolRegistry  tools.Registry
	toolRegistry      tools.Registry
	specRegistry      *specialists.Registry
	specRegMu         sync.RWMutex
	userSpecRegs      map[int64]*specialists.Registry
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
	oidcAuth          *auth.OIDC
	specStore         persist.SpecialistsStore
	tokenMetrics      tokenMetricsProvider
}

type tokenMetricsProvider interface {
	TokenTotals(ctx context.Context) ([]llmpkg.TokenTotal, time.Duration, error)
	Source() string
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
	llm := openaillm.New(cfg.OpenAI, httpClient)
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

	toolRegistry.Register(db.NewSearchIndexTool(mgr.Search))
	toolRegistry.Register(db.NewSearchQueryTool(mgr.Search))
	toolRegistry.Register(db.NewSearchRemoveTool(mgr.Search))
	toolRegistry.Register(db.NewVectorUpsertTool(mgr.Vector, cfg.Embedding))
	toolRegistry.Register(db.NewVectorQueryTool(mgr.Vector))
	toolRegistry.Register(db.NewVectorDeleteTool(mgr.Vector))
	toolRegistry.Register(db.NewHybridQueryTool(mgr.Search, mgr.Vector, cfg.Embedding))
	toolRegistry.Register(db.NewIndexDocumentTool(mgr.Search, mgr.Vector, cfg.Embedding))
	toolRegistry.Register(db.NewGraphUpsertNodeTool(mgr.Graph))
	toolRegistry.Register(db.NewGraphUpsertEdgeTool(mgr.Graph))
	toolRegistry.Register(db.NewGraphNeighborsTool(mgr.Graph))
	toolRegistry.Register(db.NewGraphGetNodeTool(mgr.Graph))

	newProv := func(baseURL string) llmpkg.Provider {
		cfgCopy := cfg.OpenAI
		cfgCopy.BaseURL = baseURL
		return openaillm.New(cfgCopy, httpClient)
	}
	toolRegistry.Register(llmtools.NewTransform(llm, cfg.OpenAI.Model, newProv))
	toolRegistry.Register(imagetool.NewDescribeTool(llm, cfg.Workdir, cfg.OpenAI.Model, newProv))

	specReg := specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, httpClient, toolRegistry)
	toolRegistry.Register(specialiststool.New(specReg))

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
	cancelInit()

	app := &app{
		cfg:              cfg,
		httpClient:       httpClient,
		mgr:              &mgr,
		llm:              llm,
		baseToolRegistry: baseToolRegistry,
		toolRegistry:     toolRegistry,
		specRegistry:     specReg,
		userSpecRegs:     map[int64]*specialists.Registry{systemUserID: specReg},
		runs:             newRunStore(),
	}

	if err := app.initWarpp(ctx, toolRegistry); err != nil {
		return nil, err
	}

	app.engine = &agent.Engine{
		LLM:              llm,
		Tools:            toolRegistry,
		MaxSteps:         cfg.MaxSteps,
		System:           prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt),
		Model:            cfg.OpenAI.Model,
		SummaryEnabled:   cfg.SummaryEnabled,
		SummaryThreshold: cfg.SummaryThreshold,
		SummaryKeepLast:  cfg.SummaryKeepLast,
	}

	app.chatStore = mgr.Chat
	if app.chatStore == nil {
		return nil, fmt.Errorf("chat store not initialized")
	}
	app.chatMemory = memory.NewManager(app.chatStore, summaryLLM, memory.Config{
		Enabled:      cfg.SummaryEnabled,
		Threshold:    cfg.SummaryThreshold,
		KeepLast:     cfg.SummaryKeepLast,
		SummaryModel: cfg.OpenAI.SummaryModel,
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

	if err := app.initSpecialists(ctx, llm); err != nil {
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

	issuer := a.cfg.Auth.IssuerURL
	clientID := a.cfg.Auth.ClientID
	clientSecret := a.cfg.Auth.ClientSecret
	redirectURL := a.cfg.Auth.RedirectURL
	oidcAuth, err := auth.NewOIDC(ctx, issuer, clientID, clientSecret, redirectURL, a.authStore, a.cfg.Auth.CookieName, a.cfg.Auth.AllowedDomains, a.cfg.Auth.StateTTLSeconds, a.cfg.Auth.CookieSecure)
	if err != nil {
		return fmt.Errorf("oidc init failed: %w", err)
	}
	a.oidcAuth = oidcAuth
	return nil
}

func (a *app) initSpecialists(ctx context.Context, llm *openaillm.Client) error {
	var pg *pgxpool.Pool
	if a.cfg.Databases.DefaultDSN != "" {
		if p, err := databasesTestPool(ctx, a.cfg.Databases.DefaultDSN); err == nil {
			pg = p
		}
	}
	specStore := databases.NewSpecialistsStore(pg)
	_ = specStore.Init(ctx)

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
				Name: sc.Name, Description: sc.Description, BaseURL: sc.BaseURL, APIKey: sc.APIKey, Model: sc.Model,
				EnableTools: sc.EnableTools, Paused: sc.Paused, AllowTools: sc.AllowTools,
				ReasoningEffort: sc.ReasoningEffort, System: sc.System,
				ExtraHeaders: sc.ExtraHeaders, ExtraParams: sc.ExtraParams,
			})
		}
	}

	if list, err := specStore.List(ctx, systemUserID); err == nil {
		a.specRegistry.ReplaceFromConfigs(a.cfg.OpenAI, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
	}

	if sp, ok, _ := specStore.GetByName(ctx, systemUserID, "orchestrator"); ok {
		a.cfg.OpenAI.BaseURL = sp.BaseURL
		a.cfg.OpenAI.APIKey = sp.APIKey
		if strings.TrimSpace(sp.Model) != "" {
			a.cfg.OpenAI.Model = sp.Model
		}
		a.cfg.EnableTools = sp.EnableTools
		a.cfg.ToolAllowList = append([]string(nil), sp.AllowTools...)
		if strings.TrimSpace(sp.System) != "" {
			a.cfg.SystemPrompt = sp.System
		} else {
			a.cfg.SystemPrompt = "You are a helpful assistant with access to tools and specialists to help you complete objectives."
		}
		if sp.ExtraHeaders != nil {
			a.cfg.OpenAI.ExtraHeaders = sp.ExtraHeaders
		}
		if sp.ExtraParams != nil {
			a.cfg.OpenAI.ExtraParams = sp.ExtraParams
		}
		llm = openaillm.New(a.cfg.OpenAI, a.httpClient)
		a.llm = llm
		a.engine.LLM = llm
		a.engine.Model = a.cfg.OpenAI.Model
		a.engine.System = prompts.DefaultSystemPrompt(a.cfg.Workdir, a.cfg.SystemPrompt)
		if !a.cfg.EnableTools {
			a.toolRegistry = tools.NewRegistry()
		} else if len(a.cfg.ToolAllowList) > 0 {
			a.toolRegistry = tools.NewFilteredRegistry(a.baseToolRegistry, a.cfg.ToolAllowList)
		} else {
			a.toolRegistry = a.baseToolRegistry
		}
		a.engine.Tools = a.toolRegistry
		a.warppMu.Lock()
		a.warppRunner.Tools = a.toolRegistry
		a.warppMu.Unlock()
		names := make([]string, 0, len(a.toolRegistry.Schemas()))
		for _, s := range a.toolRegistry.Schemas() {
			names = append(names, s.Name)
		}
		log.Info().Bool("enableTools", a.cfg.EnableTools).Strs("allowList", a.cfg.ToolAllowList).Strs("tools", names).Msg("tool_registry_contents_loaded_from_db")
	} else {
		a.cfg.SystemPrompt = "You are a helpful assistant with access to tools and specialists to help you complete objectives."
		a.engine.System = prompts.DefaultSystemPrompt(a.cfg.Workdir, a.cfg.SystemPrompt)
	}

	a.specStore = specStore
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
