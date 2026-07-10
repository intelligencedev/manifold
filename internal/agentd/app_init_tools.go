package agentd

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	decisionmem "manifold/internal/agent/memory/decision"
	appcodeqa "manifold/internal/codeqa"
	codeqaservice "manifold/internal/codeqa/service"
	codeqastore "manifold/internal/codeqa/store"
	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
	openaillm "manifold/internal/llm/openai"
	"manifold/internal/persistence/databases"
	"manifold/internal/rag/embedder"
	ragreranker "manifold/internal/rag/reranker"
	ragservice "manifold/internal/rag/service"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	codeevolvetool "manifold/internal/tools/codeevolve"
	codeqatool "manifold/internal/tools/codeqa"
	decisiontools "manifold/internal/tools/decision"
	"manifold/internal/tools/filetool"
	"manifold/internal/tools/imagetool"
	"manifold/internal/tools/llmparallel"
	matrixroomtool "manifold/internal/tools/matrixroom"
	"manifold/internal/tools/multitool"
	"manifold/internal/tools/patchtool"
	pulsetool "manifold/internal/tools/pulse"
	ragtool "manifold/internal/tools/rag"
	terminaltool "manifold/internal/tools/terminal"
	"manifold/internal/tools/textsplitter"
	transittools "manifold/internal/tools/transit"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	"manifold/internal/tools/web"
	transitdomain "manifold/internal/transit"
)

type appTooling struct {
	registry        tools.Registry
	baseRegistry    tools.Registry
	cliExecutor     *cli.ExecutorImpl
	terminalManager *terminaltool.Manager
	codeQAService   *codeqaservice.Service
	transitService  *transitdomain.Service
	ragService      *ragservice.Service
}

func initAppTooling(ctx context.Context, cfg *config.Config, httpClient *http.Client, llm llmpkg.Provider, mgr databases.Manager) (appTooling, error) {
	toolRegistry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	baseToolRegistry := toolRegistry
	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	terminalManager := terminaltool.NewManager(cfg.Exec, cfg.Workdir)
	codeQAService := initCodeQAService(ctx, cfg, exec, llm, mgr)

	registerBaseTools(baseToolOptions{
		cfg:              cfg,
		httpClient:       httpClient,
		mgr:              mgr,
		toolRegistry:     toolRegistry,
		baseToolRegistry: baseToolRegistry,
		exec:             exec,
		terminalManager:  terminalManager,
	})
	ragOpts, err := buildRAGOptions(ctx, cfg, httpClient, llm)
	if err != nil {
		return appTooling{}, err
	}
	registerRAGTools(toolRegistry, mgr, ragOpts)
	runtimeRAGService := ragservice.New(mgr, ragOpts...)
	if cfg.Magma.Enabled {
		runtimeRAGService.StartMagmaBackgroundWorkers(context.Background())
	}

	toolRegistry.Register(codeevolvetool.New(cfg, llm))
	toolRegistry.Register(codeqatool.NewJudge(cfg, codeQAService))
	toolRegistry.Register(codeqatool.NewRun(cfg, codeQAService))
	toolRegistry.Register(codeqatool.NewOptimize(cfg, codeQAService))
	transitSvc := initTransitService(cfg, mgr, runtimeRAGService, toolRegistry)
	registerDecisionTools(cfg, mgr, toolRegistry)
	registerImageTool(cfg, httpClient, llm, toolRegistry)

	return appTooling{
		registry:        toolRegistry,
		baseRegistry:    baseToolRegistry,
		cliExecutor:     exec,
		terminalManager: terminalManager,
		codeQAService:   codeQAService,
		transitService:  transitSvc,
		ragService:      runtimeRAGService,
	}, nil
}

func initCodeQAService(ctx context.Context, cfg *config.Config, exec cli.Executor, llm llmpkg.Provider, mgr databases.Manager) *codeqaservice.Service {
	codeQAOpts := appcodeqa.OptionsFromConfig(cfg.CodeQA, cfg.Workdir)
	codeQAStore := codeqastore.CodeQAStore(codeqastore.NewMemoryStore())
	if shouldUseSQLiteDefault(cfg, mgr) {
		sqliteStore := codeqastore.NewSQLiteStore(mgr.SQLite)
		if initErr := sqliteStore.Init(ctx); initErr != nil {
			log.Warn().Err(initErr).Msg("codeqa_sqlite_store_init_failed")
		} else {
			codeQAStore = sqliteStore
		}
	} else if strings.TrimSpace(cfg.Databases.DefaultDSN) != "" {
		if pool, poolErr := databases.OpenPool(ctx, cfg.Databases.DefaultDSN); poolErr != nil {
			log.Warn().Err(poolErr).Msg("codeqa_postgres_pool_open_failed")
		} else {
			pgStore := codeqastore.NewPostgresStore(pool)
			if initErr := pgStore.Init(ctx); initErr != nil {
				log.Warn().Err(initErr).Msg("codeqa_postgres_store_init_failed")
				pool.Close()
			} else {
				codeQAStore = pgStore
			}
		}
	}
	codeQARunner := appcodeqa.NewCLICommandRunner(exec, codeQAOpts.AllowedCommands)
	return codeqaservice.New(codeQAOpts, codeQARunner, llm, codeQAStore)
}

func shouldUseSQLiteDefault(cfg *config.Config, mgr databases.Manager) bool {
	if mgr.SQLite == nil || cfg == nil {
		return false
	}
	backend := strings.ToLower(strings.TrimSpace(cfg.Databases.Backend))
	return backend == "sqlite" || strings.TrimSpace(cfg.Databases.DefaultDSN) == ""
}

type baseToolOptions struct {
	cfg              *config.Config
	httpClient       *http.Client
	mgr              databases.Manager
	toolRegistry     tools.Registry
	baseToolRegistry tools.Registry
	exec             cli.Executor
	terminalManager  *terminaltool.Manager
}

func registerBaseTools(opts baseToolOptions) {
	opts.toolRegistry.Register(cli.NewTool(opts.exec))
	opts.toolRegistry.Register(terminaltool.NewStartTool(opts.terminalManager))
	opts.toolRegistry.Register(terminaltool.NewReadTool(opts.terminalManager))
	opts.toolRegistry.Register(terminaltool.NewWriteTool(opts.terminalManager))
	opts.toolRegistry.Register(terminaltool.NewStopTool(opts.terminalManager))
	opts.toolRegistry.Register(terminaltool.NewListTool(opts.terminalManager))
	opts.toolRegistry.Register(web.NewScreenshotTool())
	opts.toolRegistry.Register(web.NewFetchTool(opts.mgr.Search))
	opts.toolRegistry.Register(patchtool.New(opts.cfg.Workdir))
	allowedRoots := []string{opts.cfg.Workdir}
	opts.toolRegistry.Register(filetool.NewReadTool(allowedRoots, opts.cfg.OutputTruncateByte))
	opts.toolRegistry.Register(filetool.NewWriteTool(allowedRoots, 0))
	opts.toolRegistry.Register(filetool.NewPatchTool(allowedRoots, 0))
	opts.toolRegistry.Register(filetool.NewDeleteTool(allowedRoots))
	opts.toolRegistry.Register(textsplitter.New())
	opts.toolRegistry.Register(utility.NewTextboxTool())
	opts.toolRegistry.Register(utility.NewAgentResponseTool())
	opts.toolRegistry.Register(matrixroomtool.New())
	opts.toolRegistry.Register(pulsetool.New(opts.mgr.Pulse))
	opts.toolRegistry.Register(llmparallel.New(opts.httpClient, opts.cfg.OpenAI.BaseURL, opts.cfg.OpenAI.Model, opts.cfg.OpenAI.APIKey))
	opts.toolRegistry.Register(multitool.NewParallel(opts.baseToolRegistry, multitool.WithMaxParallel(opts.cfg.MaxToolParallelism)))
	opts.toolRegistry.Register(tts.New(*opts.cfg, opts.httpClient))
}

// shouldCheckEmbeddingReachability reports whether startup should fail-fast on an
// unreachable embedding endpoint. During onboarding (before any primary LLM
// credentials exist) the check is skipped so a fresh install can boot to /setup
// instead of crashing; once configured, the fail-fast health gate is restored.
func shouldCheckEmbeddingReachability(cfg *config.Config) bool {
	return config.HasPrimaryLLMCredentials(cfg)
}

func buildRAGOptions(ctx context.Context, cfg *config.Config, httpClient *http.Client, llm llmpkg.Provider) ([]ragservice.Option, error) {
	emb := embedder.NewClient(cfg.Embedding, cfg.Databases.Vector.Dimensions)
	if shouldCheckEmbeddingReachability(cfg) {
		if err := emb.Ping(ctx); err != nil {
			return nil, fmt.Errorf("embedding service reachability check failed: %w", err)
		}
	} else {
		log.Info().Msg("embedding reachability check skipped: no primary LLM credentials (onboarding)")
	}
	magmaCfg := cfg.Magma
	magmaLLM := llm
	if cfg.Magma.Enabled {
		provider, model, providerName, err := resolveMagmaMemoryLLM(cfg, llm, httpClient)
		if err != nil {
			return nil, fmt.Errorf("build magma memory llm provider: %w", err)
		}
		if provider != nil {
			magmaLLM = provider
		}
		if model != "" {
			magmaCfg.Consolidation.Model = model
		}
		log.Info().Bool("enabled", true).Str("provider", providerName).Str("model", model).Msg("magma_memory_llm_initialized")
	}
	ragOpts := []ragservice.Option{
		ragservice.WithEmbedder(emb),
		ragservice.WithEmbeddingConfig(cfg.Embedding),
		ragservice.WithMagmaConfig(magmaCfg),
		ragservice.WithMagmaLLM(magmaLLM),
	}
	if cfg.Reranking.Enabled {
		rr := ragreranker.NewClient(cfg.Reranking)
		if err := rr.Ping(ctx); err != nil {
			return nil, err
		}
		ragOpts = append(ragOpts, ragservice.WithReranker(rr))
	}
	return ragOpts, nil
}

func registerRAGTools(toolRegistry tools.Registry, mgr databases.Manager, ragOpts []ragservice.Option) {
	toolRegistry.Register(ragtool.NewIngestTool(mgr, ragOpts...))
	toolRegistry.Register(ragtool.NewRetrieveTool(mgr, ragOpts...))
	toolRegistry.Register(ragtool.NewMagmaLifecycleTool(mgr, ragOpts...))
}

func initTransitService(cfg *config.Config, mgr databases.Manager, runtimeRAGService *ragservice.Service, toolRegistry tools.Registry) *transitdomain.Service {
	if !cfg.Transit.Enabled || mgr.Transit == nil {
		return nil
	}
	var transitMagma transitdomain.MagmaSink
	if cfg.Magma.Enabled && runtimeRAGService != nil && runtimeRAGService.MagmaService() != nil {
		transitMagma = transitMagmaSink{service: runtimeRAGService.MagmaService(), workerCount: cfg.Magma.Consolidation.WorkerCount}
	}
	transitSvc := transitdomain.NewService(transitdomain.ServiceConfig{
		Store:              mgr.Transit,
		Search:             searchIndexerAdapter{search: mgr.Search},
		Vector:             vectorIndexerAdapter{vector: mgr.Vector},
		MagmaSink:          transitMagma,
		EmbeddingConfig:    cfg.Embedding,
		DefaultSearchLimit: cfg.Transit.DefaultSearchLimit,
		DefaultListLimit:   cfg.Transit.DefaultListLimit,
		MaxBatchSize:       cfg.Transit.MaxBatchSize,
		EnableVectorSearch: cfg.Transit.EnableVectorSearch,
	})
	toolRegistry.Register(transittools.NewCreateTool(transitSvc))
	toolRegistry.Register(transittools.NewGetTool(transitSvc))
	toolRegistry.Register(transittools.NewUpdateTool(transitSvc))
	toolRegistry.Register(transittools.NewDeleteTool(transitSvc))
	toolRegistry.Register(transittools.NewSearchTool(transitSvc))
	toolRegistry.Register(transittools.NewDiscoverTool(transitSvc))
	toolRegistry.Register(transittools.NewListKeysTool(transitSvc))
	toolRegistry.Register(transittools.NewListRecentTool(transitSvc))
	return transitSvc
}

func registerDecisionTools(cfg *config.Config, mgr databases.Manager, toolRegistry tools.Registry) {
	if cfg == nil || !cfg.Archaeology.Enabled || mgr.Decision == nil {
		return
	}
	service := &decisionmem.Service{Store: mgr.Decision, Belief: mgr.Belief}
	toolRegistry.Register(decisiontools.NewSearchTool(mgr.Decision))
	toolRegistry.Register(decisiontools.NewReconstructTool(service, mgr.Belief, mgr.Artifact))
	toolRegistry.Register(decisiontools.NewRecordTool(service))
	toolRegistry.Register(decisiontools.NewReviewTool(service))
}

func registerImageTool(cfg *config.Config, httpClient *http.Client, llm llmpkg.Provider, toolRegistry tools.Registry) {
	newProv := func(baseURL string) llmpkg.Provider {
		cfgCopy := cfg.LLMClient.OpenAI
		cfgCopy.BaseURL = baseURL
		return openaillm.New(cfgCopy, httpClient)
	}
	toolRegistry.Register(imagetool.NewDescribeTool(llm, cfg.Workdir, cfg.ImageTool.Model, cfg.ImageTool.BaseURL, newProv))
}
