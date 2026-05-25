package agentd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	appcodeqa "manifold/internal/codeqa"
	codeqaservice "manifold/internal/codeqa/service"
	codeqastore "manifold/internal/codeqa/store"
	"manifold/internal/config"
	"manifold/internal/constitution"
	"manifold/internal/durable"
	"manifold/internal/embeddedpg"
	"manifold/internal/fleet"
	"manifold/internal/httpapi"
	llmpkg "manifold/internal/llm"
	openaillm "manifold/internal/llm/openai"
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/matrixgw"
	"manifold/internal/mcpclient"
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
	"manifold/internal/rag/embedder"
	ragreranker "manifold/internal/rag/reranker"
	ragservice "manifold/internal/rag/service"
	"manifold/internal/skills"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	agenttools "manifold/internal/tools/agents"
	"manifold/internal/tools/cli"
	codeevolvetool "manifold/internal/tools/codeevolve"
	codeqatool "manifold/internal/tools/codeqa"
	tooldiscovery "manifold/internal/tools/discovery"
	"manifold/internal/tools/filetool"
	"manifold/internal/tools/imagetool"
	inputrequesttool "manifold/internal/tools/inputrequest"
	"manifold/internal/tools/llmparallel"
	matrixroomtool "manifold/internal/tools/matrixroom"
	"manifold/internal/tools/multitool"
	"manifold/internal/tools/patchtool"
	pulsetool "manifold/internal/tools/pulse"
	ragtool "manifold/internal/tools/rag"
	"manifold/internal/tools/textsplitter"
	transittools "manifold/internal/tools/transit"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	"manifold/internal/tools/web"
	transitdomain "manifold/internal/transit"
	"manifold/internal/trust"
	"manifold/internal/workspaces"
)

func newApp(ctx context.Context, cfg *config.Config) (*app, error) {
	embeddedRuntime, err := embeddedpg.Start(&cfg.Databases)
	if err != nil {
		return nil, fmt.Errorf("start embedded postgres: %w", err)
	}
	defer func() {
		if err != nil && embeddedRuntime != nil {
			_ = embeddedRuntime.Stop()
		}
	}()

	httpClient := observability.NewHTTPClient(nil)
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}

	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.LogRawPrompts, cfg.OutputTruncateByte)
	llm, err := llmproviders.Build(*cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("build llm provider: %w", err)
	}
	summaryLLM, summaryModel, err := buildSummaryLLM(cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("build summary llm provider: %w", err)
	}

	toolRegistry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	baseToolRegistry := toolRegistry

	mgr, err := databases.NewManager(ctx, cfg.Databases)
	if err != nil {
		return nil, fmt.Errorf("init databases: %w", err)
	}
	durableClient := durable.NewClient(mgr.Durable)
	durableRegistry := durable.NewRegistry()

	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	codeQAOpts := appcodeqa.OptionsFromConfig(cfg.CodeQA, cfg.Workdir)
	codeQAStore := codeqastore.CodeQAStore(codeqastore.NewMemoryStore())
	if strings.TrimSpace(cfg.Databases.DefaultDSN) != "" {
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
	codeQAService := codeqaservice.New(codeQAOpts, codeQARunner, llm, codeQAStore)
	toolRegistry.Register(cli.NewTool(exec))
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
	toolRegistry.Register(utility.NewAgentResponseTool())
	toolRegistry.Register(inputrequesttool.New())
	toolRegistry.Register(matrixroomtool.New())
	toolRegistry.Register(pulsetool.New(mgr.Pulse))
	toolRegistry.Register(llmparallel.New(httpClient, cfg.OpenAI.BaseURL, cfg.OpenAI.Model, cfg.OpenAI.APIKey))
	toolRegistry.Register(multitool.NewParallel(baseToolRegistry, multitool.WithMaxParallel(cfg.MaxToolParallelism)))
	toolRegistry.Register(tts.New(*cfg, httpClient))

	// Register RAG tools backed by the internal rag service.
	// Create a real embedder using the configured embedding service.
	emb := embedder.NewClient(cfg.Embedding, cfg.Databases.Vector.Dimensions)
	if err := emb.Ping(ctx); err != nil {
		return nil, fmt.Errorf("embedding service reachability check failed: %w", err)
	}
	ragOpts := []ragservice.Option{
		ragservice.WithEmbedder(emb),
		ragservice.WithEmbeddingConfig(cfg.Embedding),
	}
	if cfg.Reranking.Enabled {
		rr := ragreranker.NewClient(cfg.Reranking)
		if err := rr.Ping(ctx); err != nil {
			return nil, err
		}
		ragOpts = append(ragOpts, ragservice.WithReranker(rr))
	}
	toolRegistry.Register(ragtool.NewIngestTool(mgr, ragOpts...))
	toolRegistry.Register(ragtool.NewRetrieveTool(mgr, ragOpts...))
	// Reuse a single shared RAG service for runtime use (belief router, etc.).
	runtimeRAGService := ragservice.New(mgr, ragOpts...)

	// Register the AlphaEvolve-inspired code evolution tool.
	toolRegistry.Register(codeevolvetool.New(cfg, llm))
	toolRegistry.Register(codeqatool.NewJudge(cfg, codeQAService))
	toolRegistry.Register(codeqatool.NewRun(cfg, codeQAService))
	toolRegistry.Register(codeqatool.NewOptimize(cfg, codeQAService))

	var transitSvc *transitdomain.Service
	if cfg.Transit.Enabled && mgr.Transit != nil {
		transitSvc = transitdomain.NewService(transitdomain.ServiceConfig{
			Store:              mgr.Transit,
			Search:             searchIndexerAdapter{search: mgr.Search},
			Vector:             vectorIndexerAdapter{vector: mgr.Vector},
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
	}

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
	toolRegistry.Register(imagetool.NewDescribeTool(llm, cfg.Workdir, cfg.ImageTool.Model, cfg.ImageTool.BaseURL, newProv))

	// Initialize workspace manager (local filesystem only).
	wsMgr := workspaces.NewManager(cfg)
	log.Info().Str("mode", wsMgr.Mode()).Msg("workspace_manager_initialized")

	specReg := specialists.NewRegistryWithWorkdir(cfg.LLMClient, cfg.Specialists, httpClient, toolRegistry, cfg.Workdir)

	// Register specialist routing tools.
	agentCallTool := agenttools.NewAgentCallTool(toolRegistry, specReg, wsMgr)
	agentCallTool.SetDefaultTimeoutSeconds(cfg.AgentRunTimeoutSeconds)
	toolRegistry.Register(agentCallTool)
	toolRegistry.Register(agenttools.NewAskAgentTool(httpClient, "http://127.0.0.1:32180", cfg.AgentRunTimeoutSeconds))
	// Team delegation uses 0 timeout (no default timeout) because team tasks are
	// long-running multi-agent workflows. The team's internal agent runs have their
	// own timeout management via the parent context.
	toolRegistry.Register(agenttools.NewDelegateToTeamTool(httpClient, "http://127.0.0.1:32180", 0))

	mcpMgr := mcpclient.NewManager()
	ctxInit, cancelInit := context.WithTimeout(ctx, 30*time.Second)
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
				if s.URL != "" {
					log.Debug().Str("server", s.Name).Msg("skipping_remote_db_mcp_server_during_init")
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
	ctxPool, cancelPool := context.WithTimeout(ctx, 30*time.Second)
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

	toolIndex := tooldiscovery.NewToolIndex(baseToolRegistry.Schemas())
	if cfg.AutoDiscover && cfg.EnableTools {
		toolRegistry = tooldiscovery.NewDiscoverableRegistry(baseToolRegistry, toolIndex, cfg.ToolAllowList, cfg.MaxDiscoveredTools)
	} else {
		toolRegistry = tools.ApplyTopLevelPolicy(baseToolRegistry, cfg.EnableTools, cfg.ToolAllowList)
	}
	specReg.SetToolDiscovery(toolIndex, cfg.AutoDiscover, cfg.MaxDiscoveredTools)

	log.Info().Bool("enableTools", cfg.EnableTools).Bool("autoDiscover", cfg.AutoDiscover).Strs("allowList", cfg.ToolAllowList).Strs("tools", tools.SchemaNames(toolRegistry)).Msg("tool_registry_contents")

	app := &app{
		cfg:                cfg,
		httpClient:         httpClient,
		mgr:                &mgr,
		embeddedRuntime:    embeddedRuntime,
		llm:                llm,
		summaryLLM:         summaryLLM,
		durableStore:       mgr.Durable,
		durableClient:      durableClient,
		durableRegistry:    durableRegistry,
		baseToolRegistry:   baseToolRegistry,
		toolRegistry:       toolRegistry,
		toolIndex:          toolIndex,
		specRegistry:       specReg,
		userSpecRegs:       map[int64]*specialists.Registry{systemUserID: specReg},
		runs:               newRunStore(),
		inputRequests:      newInputRequestBroker(),
		flowV2:             newFlowV2Runtime(mgr.FlowV2, durableClient),
		codeQARuntime:      newCodeQARuntime(),
		codeQAService:      codeQAService,
		evolvingSessionTTL: defaultEvolvingSessionTTL,
		mcpStore:           mgr.MCP,
		userPrefsStore:     mgr.UserPreferences,
		mcpManager:         mcpMgr,
		mcpPool:            mcpPool,
		workspaceManager:   wsMgr,
		transitService:     transitSvc,
		ragService:         runtimeRAGService,
		fleetBus:           fleet.NewBus(512),
	}
	app.registerDurableHandlers()
	app.durableWorker = durable.NewWorker(mgr.Durable, durableClient, durableRegistry, durable.WorkerOptions{
		WorkerID:     fmt.Sprintf("agentd-%d", os.Getpid()),
		Lease:        5 * time.Minute,
		PollInterval: 500 * time.Millisecond,
	})
	trustStore := trust.NewStore(optionalPool(ctx, cfg.Databases.DefaultDSN))
	app.trustService = trust.NewService(trustStore)
	if err := app.trustService.Init(ctx); err != nil {
		return nil, fmt.Errorf("init trust service: %w", err)
	}
	constitutionStore := constitution.NewStore(optionalPool(ctx, cfg.Databases.DefaultDSN))
	app.constitutionSvc = constitution.NewService(constitutionStore)
	if err := app.constitutionSvc.Init(ctx); err != nil {
		return nil, fmt.Errorf("init constitution service: %w", err)
	}
	app.matrixGateway, err = matrixgw.New(cfg.Matrix)
	if err != nil {
		return nil, fmt.Errorf("init matrix gateway: %w", err)
	}
	app.matrixGateway.SetHandler(matrixgw.MessageHandlerFunc(app.handleMatrixMessage))
	app.matrixGateway.SetOutboundRecorder(func(runCtx context.Context, message matrixgw.OutboundMessage) error {
		if app.matrixMessageStore == nil {
			return nil
		}
		_, err := app.matrixMessageStore.Append(runCtx, persist.MatrixMessage{
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
	janitorInterval := defaultEvolvingJanitorInterval
	if cfg.EvolvingMemory.SessionTTLMinutes > 0 {
		app.evolvingSessionTTL = time.Duration(cfg.EvolvingMemory.SessionTTLMinutes) * time.Minute
	}
	if cfg.EvolvingMemory.JanitorIntervalMinutes > 0 {
		janitorInterval = time.Duration(cfg.EvolvingMemory.JanitorIntervalMinutes) * time.Minute
	}
	app.startEvolvingSessionJanitor(ctx, janitorInterval)

	systemPrompt := app.composeSystemPrompt()

	// Detect an approximate context window for the main model so summarization
	// auto-mode can size history appropriately.
	ctxSize, _ := llmpkg.ContextSize(cfg.OpenAI.Model)
	app.engine = &agent.Engine{
		LLM:                          llm,
		Tools:                        toolRegistry,
		MaxSteps:                     cfg.MaxSteps,
		MaxToolParallelism:           cfg.MaxToolParallelism,
		System:                       systemPrompt,
		UserPromptContext:            app.composeUserPromptContext(),
		Model:                        cfg.OpenAI.Model,
		ContextWindowTokens:          ctxSize,
		SummaryEnabled:               cfg.SummaryEnabled,
		SummaryReserveBufferTokens:   cfg.SummaryReserveBufferTokens,
		SummaryMinKeepLastMessages:   cfg.SummaryMinKeepLastMessages,
		SummaryMaxSummaryChunkTokens: cfg.SummaryMaxSummaryChunkTokens,
		HarnessEnabled:               cfg.Harness.Enabled,
		HarnessConfig:                harnessRunConfig(cfg.Harness),
	}
	app.engine.AttachTokenizer(llm, nil)

	if cfg.BeliefMemory.Enabled && cfg.BeliefMemory.EnableDistillation && cfg.BeliefMemory.Distillation.Mode == "llm" {
		beliefLLM, beliefModel, beliefProvider, err := resolveBeliefMemoryLLM(cfg, llm, httpClient)
		if err != nil {
			return nil, fmt.Errorf("build belief memory llm provider: %w", err)
		}
		app.beliefLLM = beliefLLM
		app.beliefModel = beliefModel
		log.Info().
			Bool("enabled", true).
			Str("provider", beliefProvider).
			Str("model", beliefModel).
			Msg("belief_memory_llm_initialized")
	}

	delegator := agenttools.NewDelegator(toolRegistry, specReg, wsMgr, cfg.MaxSteps)
	delegator.SetDefaultTimeout(cfg.AgentRunTimeoutSeconds)
	delegator.SetTeamDelegator(app)
	app.engine.Delegator = delegator
	app.engine.TeamDelegator = app

	// Initialize evolving memory if enabled
	if cfg.EvolvingMemory.Enabled {
		memLLM, memModel, memProvider, err := resolveEvolvingMemoryLLM(cfg, llm, summaryLLM, summaryModel, httpClient)
		if err != nil {
			return nil, fmt.Errorf("build evolving memory llm provider: %w", err)
		}

		var evStore memory.EvolvingMemoryStore
		if mgr.EvolvingMemory != nil {
			evStore = mgr.EvolvingMemory
		}

		storeJanitorInterval := time.Duration(cfg.EvolvingMemory.StoreJanitorIntervalMinutes) * time.Minute
		app.evolvingCfg = memory.EvolvingMemoryConfig{
			EmbeddingConfig:          cfg.Embedding,
			LLM:                      memLLM,
			Model:                    memModel,
			PersistDebounce:          defaultEvolvingPersistDebounce,
			MaxSize:                  cfg.EvolvingMemory.MaxSize,
			TopK:                     cfg.EvolvingMemory.TopK,
			WindowSize:               cfg.EvolvingMemory.WindowSize,
			EnableRAG:                cfg.EvolvingMemory.EnableRAG,
			EnableSmartPrune:         cfg.EvolvingMemory.EnableSmartPrune,
			PruneThreshold:           cfg.EvolvingMemory.PruneThreshold,
			RelevanceDecay:           cfg.EvolvingMemory.RelevanceDecay,
			MinRelevance:             cfg.EvolvingMemory.MinRelevance,
			PruneQualityFloor:        cfg.EvolvingMemory.PruneQualityFloor,
			PromotionAccessThreshold: cfg.EvolvingMemory.PromotionAccessThreshold,
			JanitorInterval:          storeJanitorInterval,
			Metrics:                  memory.NewMemoryMetrics(),
			Store:                    evStore,
			UserID:                   systemUserID,
		}
		if cfg.EvolvingMemory.PersistDebounceMs > 0 {
			app.evolvingCfg.PersistDebounce = time.Duration(cfg.EvolvingMemory.PersistDebounceMs) * time.Millisecond
		}
		app.rememMaxInnerSteps = cfg.EvolvingMemory.MaxInnerSteps

		app.engine.EvolvingMemory = memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
			EmbeddingConfig:          cfg.Embedding,
			LLM:                      memLLM,
			Model:                    memModel,
			PersistDebounce:          app.evolvingCfg.PersistDebounce,
			MaxSize:                  cfg.EvolvingMemory.MaxSize,
			TopK:                     cfg.EvolvingMemory.TopK,
			WindowSize:               cfg.EvolvingMemory.WindowSize,
			EnableRAG:                cfg.EvolvingMemory.EnableRAG,
			EnableSmartPrune:         cfg.EvolvingMemory.EnableSmartPrune,
			PruneThreshold:           cfg.EvolvingMemory.PruneThreshold,
			RelevanceDecay:           cfg.EvolvingMemory.RelevanceDecay,
			MinRelevance:             cfg.EvolvingMemory.MinRelevance,
			PruneQualityFloor:        cfg.EvolvingMemory.PruneQualityFloor,
			PromotionAccessThreshold: cfg.EvolvingMemory.PromotionAccessThreshold,
			JanitorInterval:          storeJanitorInterval,
			Metrics:                  app.evolvingCfg.Metrics,
			Store:                    evStore,
			UserID:                   systemUserID,
		})
		log.Info().
			Bool("enabled", true).
			Str("provider", memProvider).
			Str("model", memModel).
			Dur("persistDebounce", app.evolvingCfg.PersistDebounce).
			Dur("sessionTTL", app.evolvingSessionTTL).
			Dur("janitorInterval", janitorInterval).
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
	app.matrixMessageStore = mgr.MatrixMessages
	app.activityStore = mgr.SpecialistActivity
	if app.activityStore == nil {
		return nil, fmt.Errorf("specialist activity store not initialized")
	}
	// Derive a context window for chat-memory budgeting.
	// Even if the underlying model supports very large context windows (e.g. GPT-5),
	// we intentionally cap the default budgeting window so the orchestrator doesn't
	// receive an excessively long raw transcript every turn.
	summaryCtxSize, _ := llmpkg.ContextSize(summaryModel)
	if cfg.Summary.ContextWindowTokens > 0 {
		summaryCtxSize = cfg.Summary.ContextWindowTokens
	} else {
		const defaultSummaryContextWindowCap = 32_000
		if summaryCtxSize <= 0 || summaryCtxSize > defaultSummaryContextWindowCap {
			summaryCtxSize = defaultSummaryContextWindowCap
		}
	}
	app.chatMemory = memory.NewManager(app.chatStore, summaryLLM, memory.Config{
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
	playgroundRunner := newPlaygroundSpecialistRunner(app, worker.NewProviderRunner(playgroundProvider))
	playgroundWorker := worker.NewWorkerWithRunner(playgroundRunner, artifactStore)
	playgroundEvals := eval.NewRunner(eval.NewRegistry(), playgroundProvider)
	playgroundService := playground.NewService(playground.Config{MaxConcurrentShards: 4, SpecialistValidator: playgroundRunner}, playgroundRegistry, playgroundDataset, playgroundRepo, playgroundPlanner, playgroundWorker, playgroundEvals, mgr.Playground)
	app.playgroundHandler = httpapi.NewServer(playgroundService)

	// Filesystem backend only.
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

	fsService := projects.NewService(cfg.Workdir, defaultSkillsDir)
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

	if err := app.initAuth(ctx); err != nil {
		return nil, err
	}

	if err := app.initSpecialists(ctx); err != nil {
		return nil, err
	}

	if cfg.Obs.Local.IsEnabled() {
		metricsWindow := time.Duration(cfg.Obs.Local.MetricsWindowMinutes) * time.Minute
		memory.ConfigureLocalTelemetry(memory.LocalTelemetryConfig{
			Enabled:   true,
			MaxEvents: cfg.Obs.Local.MaxLogs,
			Retention: metricsWindow,
		})
		processLogs := newProcessLogMetrics(cfg.Obs.Local.MaxLogs)
		observability.AddLogWriter(processLogs)
		app.tokenMetrics = append(app.tokenMetrics, processTokenMetrics{})
		app.memoryMetrics = append(app.memoryMetrics, processMemoryMetrics{})
		app.traceMetrics = append(app.traceMetrics, processTraceMetrics{})
		app.logMetrics = append(app.logMetrics, processLogs)
	} else {
		memory.ConfigureLocalTelemetry(memory.LocalTelemetryConfig{Enabled: false})
	}

	// Ensure ClickHouse tables exist before initializing metrics providers.
	if strings.TrimSpace(cfg.Obs.ClickHouse.DSN) == "" {
		log.Warn().Msg("clickhouse dashboard queries disabled: CLICKHOUSE_DSN is not set")
	}

	if err := ensureClickHouseTables(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("failed to ensure clickhouse tables")
	}

	if tm, err := newClickHouseTokenMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse metrics disabled")
	} else if tm != nil {
		app.tokenMetrics = append([]tokenMetricsProvider{tm}, app.tokenMetrics...)
	}

	if mm, err := newClickHouseMemoryMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse memory metrics disabled")
	} else if mm != nil {
		app.memoryMetrics = append([]memoryMetricsProvider{mm}, app.memoryMetrics...)
	}

	if chTraces, err := newClickHouseTraceMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse trace queries disabled")
	} else if chTraces != nil {
		app.traceMetrics = append([]traceMetricsProvider{chTraces}, app.traceMetrics...)
		app.runMetrics = newClickHouseRunMetrics(chTraces)
	}

	if chLogs, err := newClickHouseLogMetrics(ctx, cfg.Obs.ClickHouse); err != nil {
		log.Warn().Err(err).Msg("clickhouse log queries disabled")
	} else if chLogs != nil {
		app.logMetrics = append([]logMetricsProvider{chLogs}, app.logMetrics...)
	}

	_ = mcpMgr // ensure lifetime; manager currently long-lived

	// Refresh OAuth tokens for cached remote MCP servers and register them.
	// This runs after app is fully initialized so we have access to httpClient
	// and other dependencies needed for OAuth token refresh.
	if mgr.MCP != nil {
		ctxRefresh, cancelRefresh := context.WithTimeout(ctx, 30*time.Second)
		pendingAuthIDs, err := app.RefreshMCPServersOnStartup(ctxRefresh, systemUserID)
		if err != nil {
			log.Warn().Err(err).Msg("mcp_oauth_refresh_on_startup_failed")
		} else if !cfg.Auth.Enabled {
			app.startupMCPOAuthIDs = pendingAuthIDs
		}
		cancelRefresh()
	}
	app.syncWarppTools(context.Background())

	if err := app.matrixGateway.Start(ctx); err != nil {
		return nil, fmt.Errorf("start matrix gateway: %w", err)
	}
	app.pulseRuntime = newPulseRuntime(app, mgr.Pulse)
	if err := app.pulseRuntime.Start(ctx); err != nil {
		return nil, fmt.Errorf("start matrix pulse runtime: %w", err)
	}
	app.durableWorker.Start(ctx)

	return app, nil
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
