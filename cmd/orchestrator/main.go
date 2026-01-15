package main

import (
	"context"
	"fmt"
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

const mcpInitTimeout = 20 * time.Second

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
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("orchestrator")
	}
}

func run() error {
	// Load application config early so Kafka topics/brokers can come from config.yaml/.env
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	observability.InitLogger(cfg.LogPath, cfg.LogLevel)

	baseCtx := context.Background()

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
		return fmt.Errorf("no Kafka brokers configured")
	}

	groupID := getenv("KAFKA_GROUP_ID", "manifold-orchestrator")
	commandsTopic := getenv("KAFKA_COMMANDS_TOPIC", cfg.Kafka.CommandsTopic)
	responsesTopic := getenv("KAFKA_RESPONSES_TOPIC", cfg.Kafka.ResponsesTopic)
	redisAddr := getenv("DEDUPE_REDIS_ADDR", "localhost:6379")
	workerCount := getenvInt("WORKER_COUNT", 4)
	workflowTimeout := getenvDuration("DEFAULT_WORKFLOW_TIMEOUT", 10*time.Minute)
	// Use the same duration as the dedupe TTL by default.
	dedupeTTL := workflowTimeout

	log.Info().
		Strs("brokers", brokers).
		Str("groupID", groupID).
		Str("commandsTopic", commandsTopic).
		Str("responsesTopic", responsesTopic).
		Int("workers", workerCount).
		Dur("workflowTimeout", workflowTimeout).
		Msg("starting orchestrator Kafka adapter")

	// Initialize Redis-based deduplication store.
	dedupe, err := orchestrator.NewRedisDedupeStore(redisAddr)
	if err != nil {
		return fmt.Errorf("init redis dedupe store: %w", err)
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

	shutdown, err := observability.InitOTel(baseCtx, cfg.Obs)
	if err != nil {
		log.Warn().Err(err).Msg("otel init failed, continuing without observability")
		shutdown = nil
	}
	if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

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
	mgr, err := databases.NewManager(baseCtx, cfg.Databases)
	if err != nil {
		return fmt.Errorf("init databases: %w", err)
	}
	defer mgr.Close()

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
	defer mcpMgr.Close()

	ctxInit, cancelInit := context.WithTimeout(baseCtx, mcpInitTimeout)
	if err := mcpMgr.RegisterFromConfig(ctxInit, registry, cfg.MCP); err != nil {
		log.Warn().Err(err).Msg("mcp init")
	}
	cancelInit()

	// Configure WARPP to source defaults from the database, not hard-coded values.
	warpp.SetDefaultStore(mgr.Warpp)
	wfreg, err := warpp.LoadFromStore(baseCtx, mgr.Warpp, systemUserID)
	if err != nil {
		return fmt.Errorf("load workflows: %w", err)
	}
	warppRunner := &warpp.Runner{Workflows: wfreg, Tools: registry}
	// Register WARPP workflows as tools (warpp_<intent>) so they can be invoked directly
	warpptool.RegisterAll(registry, warppRunner)
	// adapter to satisfy orchestrator.Runner
	runner := orchestrator.NewWarppAdapter(warppRunner)

	// Handle SIGINT/SIGTERM for graceful shutdown.
	ctx, cancel := signal.NotifyContext(baseCtx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Verify Kafka connectivity and ensure topics exist before starting consumers.
	ctxAdmin, cancelAdmin := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancelAdmin()
	if err := orchestrator.CheckBrokers(ctxAdmin, brokers, 3*time.Second); err != nil {
		return fmt.Errorf("reach kafka brokers: %w", err)
	}

	// Ensure commands, responses, and DLQ topics exist (create if missing).
	cmdCfg := kafka.TopicConfig{Topic: commandsTopic, NumPartitions: 1, ReplicationFactor: 1}
	respCfg := kafka.TopicConfig{Topic: responsesTopic, NumPartitions: 1, ReplicationFactor: 1}
	dlqCfg := kafka.TopicConfig{Topic: responsesTopic + ".dlq", NumPartitions: 1, ReplicationFactor: 1}
	if err := orchestrator.EnsureTopics(ctxAdmin, brokers, []kafka.TopicConfig{cmdCfg, respCfg, dlqCfg}); err != nil {
		return fmt.Errorf("ensure kafka topics: %w", err)
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
		return fmt.Errorf("kafka consumer terminated: %w", err)
	}

	log.Info().Msg("orchestrator Kafka adapter stopped")
	return nil
}
