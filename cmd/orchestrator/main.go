package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/segmentio/kafka-go"

	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
	"manifold/internal/mcpclient"
	"manifold/internal/observability"
	"manifold/internal/persistence/databases"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	kafkatools "manifold/internal/tools/kafka"
	"manifold/internal/tools/patchtool"
	"manifold/internal/tools/tts"
	warpptool "manifold/internal/tools/warpptool"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"

	"manifold/internal/orchestrator"
)

const systemUserID int64 = 0

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
	// Load application config early so Kafka topics/brokers can come from config.yaml/.env
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Parse brokers from config (comma-separated)
	brokersCSV := strings.TrimSpace(cfg.Kafka.Brokers)
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

	groupID := getenv("KAFKA_GROUP_ID", "manifold-orchestrator")
	commandsTopic := getenv("KAFKA_COMMANDS_TOPIC", cfg.Kafka.CommandsTopic)
	responsesTopic := getenv("KAFKA_RESPONSES_TOPIC", cfg.Kafka.ResponsesTopic)
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
	// Use previously loaded cfg to construct tools and logging.
	observability.InitLogger(cfg.LogPath, cfg.LogLevel)
	shutdown, _ := observability.InitOTel(context.Background(), cfg.Obs)
	defer func() { _ = shutdown(context.Background()) }()

	// Tuned HTTP transport for concurrency and observability
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 7 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   7 * time.Second,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		MaxConnsPerHost:       200,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}

	httpClient := observability.NewHTTPClient(&http.Client{Transport: tr})
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}
	// Configure global llm payload logging/truncation
	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.OutputTruncateByte)

	registry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	// Databases: construct backends and register tools
	mgr, err := databases.NewManager(context.Background(), cfg.Databases)
	if err != nil {
		log.Fatal().Err(err).Msg("databases init failed")
	}

	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	registry.Register(cli.NewTool(exec))               // provides run_cli
	registry.Register(web.NewTool(cfg.Web.SearXNGURL)) // provides web_search
	registry.Register(web.NewFetchTool(mgr.Search))    // provides web_fetch
	registry.Register(patchtool.New(cfg.Workdir))      // provides apply_patch
	// TTS tool
	registry.Register(tts.New(cfg, httpClient))

	// Kafka tool (if brokers configured)
	if cfg.Kafka.Brokers != "" {
		if producer, err := kafkatools.NewProducerFromBrokers(cfg.Kafka.Brokers); err == nil {
			// Pass commandsTopic so kafka_send_message can intelligently format messages for orchestrator
			registry.Register(kafkatools.NewSendMessageToolWithOrchestratorTopic(producer, commandsTopic)) // provides kafka_send_message
		} else {
			observability.LoggerWithTrace(context.Background()).Warn().Err(err).Msg("kafka_tool_init_failed")
		}
	}

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

	// Configure WARPP to source defaults from the database, not hard-coded values.
	warpp.SetDefaultStore(mgr.Warpp)
	wfreg, _ := warpp.LoadFromStore(context.Background(), mgr.Warpp, systemUserID)
	warppRunner := &warpp.Runner{Workflows: wfreg, Tools: registry}
	// Register WARPP workflows as tools (warpp_<intent>) so they can be invoked directly
	warpptool.RegisterAll(registry, warppRunner)
	// adapter to satisfy orchestrator.Runner
	runner := orchestrator.NewWarppAdapter(warppRunner)

	// Handle SIGINT/SIGTERM for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Verify Kafka connectivity and ensure topics exist before starting consumers.
	ctxAdmin, cancelAdmin := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAdmin()
	if err := orchestrator.CheckBrokers(ctxAdmin, brokers, 3*time.Second); err != nil {
		log.Fatal().Err(err).Msg("failed to reach Kafka brokers")
	}

	// Ensure commands, responses, and DLQ topics exist (create if missing).
	cmdCfg := kafka.TopicConfig{Topic: commandsTopic, NumPartitions: 1, ReplicationFactor: 1}
	respCfg := kafka.TopicConfig{Topic: responsesTopic, NumPartitions: 1, ReplicationFactor: 1}
	dlqCfg := kafka.TopicConfig{Topic: responsesTopic + ".dlq", NumPartitions: 1, ReplicationFactor: 1}
	if err := orchestrator.EnsureTopics(ctxAdmin, brokers, []kafka.TopicConfig{cmdCfg, respCfg, dlqCfg}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure Kafka topics")
	}

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
