package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/segmentio/kafka-go"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/llm/openai"
	"manifold/internal/mcpclient"
	"manifold/internal/observability"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	"manifold/internal/tools/db"
	llmtools "manifold/internal/tools/llmtool"
	"manifold/internal/tools/patchtool"
	specialists_tool "manifold/internal/tools/specialists"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"

	"manifold/internal/orchestrator"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func main() {
	// Configuration from environment with sensible defaults.
	brokersCSV := getenv("KAFKA_BROKERS", "localhost:9092")
	brokers := make([]string, 0)
	for _, b := range strings.Split(brokersCSV, ",") {
		b = strings.TrimSpace(b)
		if b != "" {
			brokers = append(brokers, b)
		}
	}
	if len(brokers) == 0 {
		log.Fatal().Msg("no Kafka brokers configured")
	}

	groupID := getenv("KAFKA_GROUP_ID", "sio-orchestrator")
	commandsTopic := getenv("KAFKA_COMMANDS_TOPIC", "dev.sio.orchestrator.commands")
	responsesTopic := getenv("KAFKA_RESPONSES_TOPIC", "dev.sio.orchestrator.responses")
	redisAddr := getenv("DEDUPE_REDIS_ADDR", "localhost:6379")
	workerCount := getenvInt("WORKER_COUNT", 4)
	workflowTimeout := getenvDuration("DEFAULT_WORKFLOW_TIMEOUT", 10*time.Minute)
	// Use the same duration as the dedupe TTL by default.
	dedupeTTL := workflowTimeout

	log.Info().Msgf("starting orchestrator Kafka adapter: brokers=%v groupID=%s commandsTopic=%s responsesTopic=%s workers=%d workflowTimeout=%s",
		brokers, groupID, commandsTopic, responsesTopic, workerCount, workflowTimeout)

	// Initialize Redis-based deduplication store.
	dedupe, err := orchestrator.NewRedisDedupeStore(redisAddr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize Redis dedupe store")
	}
	defer func() {
		if cerr := dedupe.Close(); cerr != nil {
			log.Error().Err(cerr).Msg("error closing Redis client")
		}
	}()

	// Kafka producer for responses. Leave Topic empty so individual messages
	// can set their own Topic (the handler publishes to replyTopic or DLQ
	// topics per-message). Setting Topic on both Writer and Message is
	// rejected by the kafka-go client.
	producer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  brokers,
		Balancer: &kafka.LeastBytes{},
	})
	defer func() {
		if err := producer.Close(); err != nil {
			log.Error().Err(err).Msg("error closing Kafka producer")
		}
	}()

	// Build WARPP runner backed by the in-process tool registry.
	// Load application config to construct tools similar to the agent binary.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}
	observability.InitLogger(cfg.LogPath, cfg.LogLevel)
	shutdown, _ := observability.InitOTel(context.Background(), cfg.Obs)
	defer func() { _ = shutdown(context.Background()) }()

	// HTTP client with observability hooks
	httpClient := observability.NewHTTPClient(nil)
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}
	// Configure global llm payload logging/truncation
	llm.ConfigureLogging(cfg.LogPayloads, cfg.OutputTruncateByte)
	llmProv := openai.New(cfg.OpenAI, httpClient)

	registry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	// Databases: construct backends and register tools
	mgr, err := databases.NewManager(context.Background(), cfg.Databases)
	if err != nil {
		log.Fatal().Err(err).Msg("databases init failed")
	}

	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	registry.Register(cli.NewTool(exec))               // provides run_cli
	registry.Register(web.NewTool(cfg.Web.SearXNGURL)) // provides web_search
	registry.Register(web.NewFetchTool())              // provides web_fetch
	registry.Register(patchtool.New(cfg.Workdir))      // provides apply_patch
	// TTS tool
	registry.Register(tts.New(cfg, httpClient))

	// DB tools
	registry.Register(db.NewSearchIndexTool(mgr.Search))
	registry.Register(db.NewSearchQueryTool(mgr.Search))
	registry.Register(db.NewSearchRemoveTool(mgr.Search))
	registry.Register(db.NewVectorUpsertTool(mgr.Vector, cfg.Embedding))
	registry.Register(db.NewVectorQueryTool(mgr.Vector))
	registry.Register(db.NewVectorDeleteTool(mgr.Vector))
	registry.Register(db.NewGraphUpsertNodeTool(mgr.Graph))
	registry.Register(db.NewGraphUpsertEdgeTool(mgr.Graph))
	registry.Register(db.NewGraphNeighborsTool(mgr.Graph))
	registry.Register(db.NewGraphGetNodeTool(mgr.Graph))
	// Provider factory for base_url override in llm_transform
	newProv := func(baseURL string) llm.Provider {
		c2 := cfg.OpenAI
		c2.BaseURL = baseURL
		return openai.New(c2, httpClient)
	}
	registry.Register(llmtools.NewTransform(llmProv, cfg.OpenAI.Model, newProv)) // provides llm_transform
	// Specialists tool for LLM-driven routing
	specReg := specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, httpClient, registry)
	registry.Register(specialists_tool.New(specReg))

	// If tools are globally disabled, use an empty registry
	if !cfg.EnableTools {
		registry = tools.NewRegistry() // Empty registry
	} else if len(cfg.ToolAllowList) > 0 {
		// Apply top-level tool allow-list if configured.
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

	wfreg, _ := warpp.LoadFromStore(context.Background(), mgr.Warpp)
	warppRunner := &warpp.Runner{Workflows: wfreg, Tools: registry}
	// adapter to satisfy orchestrator.Runner
	runner := orchestrator.NewWarppAdapter(warppRunner)

	// Handle SIGINT/SIGTERM for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start the consumer with a default reader config.
	if err := orchestrator.StartKafkaConsumer(
		ctx,
		brokers,
		groupID,
		commandsTopic,
		nil, // use default reader config
		producer,
		runner,
		dedupe,
		workerCount,
		responsesTopic,
		dedupeTTL,
		workflowTimeout,
	); err != nil {
		// Exit on error as requested.
		log.Fatal().Err(err).Msg("kafka consumer terminated with error")
	}

	log.Info().Msg("orchestrator Kafka adapter stopped")
}
