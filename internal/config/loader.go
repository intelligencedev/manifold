package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	yaml "gopkg.in/yaml.v3"
)

// Load reads configuration from environment variables (optionally .env).
func Load() (Config, error) {
	// Use Overload so .env values override existing OS environment variables.
	// This allows repository/local configuration to deterministically control
	// runtime behavior in development unless explicitly changed.
	_ = godotenv.Overload()

	cfg := Config{}
	// Defaults that are awkward to represent as zero-values.
	//
	// Tokenization:
	// - FallbackToHeuristic: default true
	// Skills:
	// - UseS3Loader: default true
	cfg.Tokenization.FallbackToHeuristic = true
	cfg.Skills.UseS3Loader = true
	// Allow overriding the agent system prompt via env var SYSTEM_PROMPT.
	cfg.SystemPrompt = strings.TrimSpace(os.Getenv("SYSTEM_PROMPT"))
	cfg.LLMClient.Provider = strings.TrimSpace(os.Getenv("LLM_PROVIDER"))
	// Read environment values first (no defaults here; we'll apply defaults later)
	if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
		cfg.OpenAI.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); v != "" {
		cfg.OpenAI.Model = v
	}
	// Allow overriding API base via env (useful for proxies/self-hosted gateways)
	if v := firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), strings.TrimSpace(os.Getenv("OPENAI_API_BASE_URL"))); v != "" {
		cfg.OpenAI.BaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_SUMMARY_URL")); v != "" {
		cfg.OpenAI.SummaryBaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_SUMMARY_MODEL")); v != "" {
		cfg.OpenAI.SummaryModel = v
	}
	// Allow selecting API surface ("completions" or "responses"). Defaults later.
	if v := strings.TrimSpace(os.Getenv("OPENAI_API")); v != "" {
		cfg.OpenAI.API = v
	}
	// Optional Anthropic provider env config
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); v != "" {
		cfg.LLMClient.Anthropic.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL")); v != "" {
		cfg.LLMClient.Anthropic.Model = v
	}
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")); v != "" {
		cfg.LLMClient.Anthropic.BaseURL = v
	}
	// Optional Google provider env config
	if v := strings.TrimSpace(os.Getenv("GOOGLE_LLM_API_KEY")); v != "" {
		cfg.LLMClient.Google.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("GOOGLE_LLM_MODEL")); v != "" {
		cfg.LLMClient.Google.Model = v
	}
	if v := strings.TrimSpace(os.Getenv("GOOGLE_LLM_BASE_URL")); v != "" {
		cfg.LLMClient.Google.BaseURL = v
	}
	cfg.Workdir = strings.TrimSpace(os.Getenv("WORKDIR"))
	cfg.LogPath = strings.TrimSpace(os.Getenv("LOG_PATH"))
	cfg.LogLevel = strings.TrimSpace(os.Getenv("LOG_LEVEL"))
	if v := strings.TrimSpace(os.Getenv("LOG_PAYLOADS")); v != "" {
		cfg.LogPayloads = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
		cfg.OpenAI.LogPayloads = cfg.LogPayloads
	}
	// Int env parsing without defaults; defaults applied after YAML
	if v := strings.TrimSpace(os.Getenv("MAX_COMMAND_SECONDS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Exec.MaxCommandSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("OUTPUT_TRUNCATE_BYTES")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.OutputTruncateByte = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_STEPS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.MaxSteps = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_TOOL_PARALLELISM")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.MaxToolParallelism = n
		}
	}
	// Agent / Stream / Workflow timeouts (seconds)
	if v := strings.TrimSpace(os.Getenv("AGENT_RUN_TIMEOUT_SECONDS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.AgentRunTimeoutSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("STREAM_RUN_TIMEOUT_SECONDS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.StreamRunTimeoutSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("WORKFLOW_TIMEOUT_SECONDS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.WorkflowTimeoutSeconds = n
		}
	}

	cfg.Obs.ServiceName = strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	cfg.Obs.ServiceVersion = strings.TrimSpace(os.Getenv("SERVICE_VERSION"))
	cfg.Obs.Environment = strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	cfg.Obs.OTLP = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	cfg.Obs.ClickHouse.DSN = strings.TrimSpace(os.Getenv("CLICKHOUSE_DSN"))
	cfg.Obs.ClickHouse.Database = strings.TrimSpace(os.Getenv("CLICKHOUSE_DATABASE"))
	cfg.Obs.ClickHouse.MetricsTable = strings.TrimSpace(os.Getenv("CLICKHOUSE_METRICS_TABLE"))
	cfg.Obs.ClickHouse.TracesTable = strings.TrimSpace(os.Getenv("CLICKHOUSE_TRACES_TABLE"))
	cfg.Obs.ClickHouse.LogsTable = strings.TrimSpace(os.Getenv("CLICKHOUSE_LOGS_TABLE"))
	cfg.Obs.ClickHouse.TimestampColumn = strings.TrimSpace(os.Getenv("CLICKHOUSE_TIMESTAMP_COLUMN"))
	cfg.Obs.ClickHouse.ValueColumn = strings.TrimSpace(os.Getenv("CLICKHOUSE_VALUE_COLUMN"))
	cfg.Obs.ClickHouse.ModelAttributeKey = strings.TrimSpace(os.Getenv("CLICKHOUSE_MODEL_ATTRIBUTE_KEY"))
	cfg.Obs.ClickHouse.PromptMetricName = strings.TrimSpace(os.Getenv("CLICKHOUSE_PROMPT_METRIC_NAME"))
	cfg.Obs.ClickHouse.CompletionMetricName = strings.TrimSpace(os.Getenv("CLICKHOUSE_COMPLETION_METRIC_NAME"))
	if v := strings.TrimSpace(os.Getenv("CLICKHOUSE_LOOKBACK_HOURS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Obs.ClickHouse.LookbackHours = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("CLICKHOUSE_TIMEOUT_SECONDS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Obs.ClickHouse.TimeoutSeconds = n
		}
	}

	cfg.Web.SearXNGURL = strings.TrimSpace(os.Getenv("SEARXNG_URL"))
	// Kafka defaults for orchestrator integration
	cfg.Kafka.Brokers = strings.TrimSpace(firstNonEmpty(os.Getenv("KAFKA_BROKERS"), os.Getenv("KAFKA_BOOTSTRAP_SERVERS")))
	cfg.Kafka.CommandsTopic = strings.TrimSpace(firstNonEmpty(os.Getenv("KAFKA_COMMANDS_TOPIC"), os.Getenv("KAFKA_COMMAND_TOPIC")))
	cfg.Kafka.ResponsesTopic = strings.TrimSpace(firstNonEmpty(os.Getenv("KAFKA_RESPONSES_TOPIC"), os.Getenv("KAFKA_RESPONSE_TOPIC")))
	// TTS defaults (optional)
	cfg.TTS.BaseURL = strings.TrimSpace(os.Getenv("TTS_BASE_URL"))
	cfg.TTS.Model = strings.TrimSpace(os.Getenv("TTS_MODEL"))
	cfg.TTS.Voice = strings.TrimSpace(os.Getenv("TTS_VOICE"))

	// Summary configuration via env (token-based only)
	if v := strings.TrimSpace(os.Getenv("SUMMARY_ENABLED")); v != "" {
		cfg.SummaryEnabled = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_CONTEXT_WINDOW_TOKENS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryContextWindowTokens = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_RESERVE_BUFFER_TOKENS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryReserveBufferTokens = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_MIN_KEEP_LAST_MESSAGES")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryMinKeepLastMessages = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_MAX_KEEP_LAST_MESSAGES")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryMaxKeepLastMessages = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_MAX_SUMMARY_CHUNK_TOKENS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryMaxSummaryChunkTokens = n
		}
	}

	// Global enableTools via env (overrides YAML)
	if v := strings.TrimSpace(os.Getenv("ENABLE_TOOLS")); v != "" {
		cfg.EnableTools = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	// Tool allow-list via env (overrides YAML). Comma-separated tool names.
	if v := strings.TrimSpace(os.Getenv("ALLOW_TOOLS")); v != "" {
		cfg.ToolAllowList = parseCommaSeparatedList(v)
	}

	// Database backends via environment variables
	// Optional default shared DSN (e.g., postgresql://...)
	cfg.Databases.DefaultDSN = firstNonEmpty(strings.TrimSpace(os.Getenv("DATABASE_URL")), strings.TrimSpace(os.Getenv("DB_URL")), strings.TrimSpace(os.Getenv("POSTGRES_DSN")))
	cfg.Databases.Search.Backend = strings.TrimSpace(os.Getenv("SEARCH_BACKEND"))
	cfg.Databases.Search.DSN = strings.TrimSpace(os.Getenv("SEARCH_DSN"))
	cfg.Databases.Search.Index = strings.TrimSpace(os.Getenv("SEARCH_INDEX"))
	cfg.Databases.Vector.Backend = strings.TrimSpace(os.Getenv("VECTOR_BACKEND"))
	cfg.Databases.Vector.DSN = strings.TrimSpace(os.Getenv("VECTOR_DSN"))
	cfg.Databases.Vector.Index = strings.TrimSpace(os.Getenv("VECTOR_INDEX"))
	if v := strings.TrimSpace(os.Getenv("VECTOR_DIMENSIONS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Databases.Vector.Dimensions = n
		}
	}
	cfg.Databases.Vector.Metric = strings.TrimSpace(os.Getenv("VECTOR_METRIC"))
	cfg.Databases.Graph.Backend = strings.TrimSpace(os.Getenv("GRAPH_BACKEND"))
	cfg.Databases.Graph.DSN = strings.TrimSpace(os.Getenv("GRAPH_DSN"))
	cfg.Databases.Chat.Backend = strings.TrimSpace(os.Getenv("CHAT_BACKEND"))
	cfg.Databases.Chat.DSN = strings.TrimSpace(os.Getenv("CHAT_DSN"))

	// Embedding service configuration via environment variables
	cfg.Embedding.BaseURL = strings.TrimSpace(os.Getenv("EMBED_BASE_URL"))
	cfg.Embedding.Model = strings.TrimSpace(os.Getenv("EMBED_MODEL"))
	cfg.Embedding.APIKey = strings.TrimSpace(os.Getenv("EMBED_API_KEY"))
	cfg.Embedding.APIHeader = strings.TrimSpace(os.Getenv("EMBED_API_HEADER"))
	// Optional: set EMBED_API_HEADERS as JSON string or comma-separated key:value pairs
	if v := strings.TrimSpace(os.Getenv("EMBED_API_HEADERS")); v != "" {
		// Try JSON first
		var m map[string]string
		if err := json.Unmarshal([]byte(v), &m); err == nil {
			cfg.Embedding.Headers = m
		} else {
			// Fallback: parse comma-separated key:value pairs
			m = make(map[string]string)
			parts := strings.Split(v, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				if strings.Contains(p, ":") {
					kv := strings.SplitN(p, ":", 2)
					m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				} else if strings.Contains(p, "=") {
					kv := strings.SplitN(p, "=", 2)
					m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				}
			}
			cfg.Embedding.Headers = m
		}
	}
	cfg.Embedding.Path = strings.TrimSpace(os.Getenv("EMBED_PATH"))
	if v := strings.TrimSpace(os.Getenv("EMBED_TIMEOUT")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Embedding.Timeout = n
		}
	}

	// Evolving Memory configuration via environment variables
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_ENABLED")); v != "" {
		cfg.EvolvingMemory.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_MAX_SIZE")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.EvolvingMemory.MaxSize = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_TOP_K")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.EvolvingMemory.TopK = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_WINDOW_SIZE")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.EvolvingMemory.WindowSize = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_ENABLE_RAG")); v != "" {
		cfg.EvolvingMemory.EnableRAG = strings.EqualFold(v, "true") || v == "1"
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_REMEM_ENABLED")); v != "" {
		cfg.EvolvingMemory.ReMemEnabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_MAX_INNER_STEPS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.EvolvingMemory.MaxInnerSteps = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_MODEL")); v != "" {
		cfg.EvolvingMemory.Model = v
	}
	// Smart pruning env vars
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_ENABLE_SMART_PRUNE")); v != "" {
		cfg.EvolvingMemory.EnableSmartPrune = strings.EqualFold(v, "true") || v == "1"
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_PRUNE_THRESHOLD")); v != "" {
		if f, err := parseFloat(v); err == nil {
			cfg.EvolvingMemory.PruneThreshold = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_RELEVANCE_DECAY")); v != "" {
		if f, err := parseFloat(v); err == nil {
			cfg.EvolvingMemory.RelevanceDecay = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("EVOLVING_MEMORY_MIN_RELEVANCE")); v != "" {
		if f, err := parseFloat(v); err == nil {
			cfg.EvolvingMemory.MinRelevance = f
		}
	}

	// Projects configuration via environment variables
	if v := strings.TrimSpace(os.Getenv("PROJECTS_BACKEND")); v != "" {
		cfg.Projects.Backend = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPT")); v != "" {
		cfg.Projects.Encrypt = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	// Encryption key provider configuration
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_PROVIDER")); v != "" {
		cfg.Projects.Encryption.Provider = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_FILE_KEYSTORE_PATH")); v != "" {
		cfg.Projects.Encryption.File.KeystorePath = v
	}
	// Vault Transit configuration
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_VAULT_ADDRESS")); v != "" {
		cfg.Projects.Encryption.Vault.Address = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_VAULT_TOKEN")); v != "" {
		cfg.Projects.Encryption.Vault.Token = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_VAULT_KEY_NAME")); v != "" {
		cfg.Projects.Encryption.Vault.KeyName = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_VAULT_MOUNT_PATH")); v != "" {
		cfg.Projects.Encryption.Vault.MountPath = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_VAULT_NAMESPACE")); v != "" {
		cfg.Projects.Encryption.Vault.Namespace = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_VAULT_TLS_SKIP_VERIFY")); v != "" {
		cfg.Projects.Encryption.Vault.TLSSkipVerify = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_VAULT_TIMEOUT_SECONDS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Projects.Encryption.Vault.TimeoutSeconds = n
		}
	}
	// AWS KMS configuration
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_AWSKMS_KEY_ID")); v != "" {
		cfg.Projects.Encryption.AWSKMS.KeyID = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_AWSKMS_REGION")); v != "" {
		cfg.Projects.Encryption.AWSKMS.Region = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_AWSKMS_ACCESS_KEY_ID")); v != "" {
		cfg.Projects.Encryption.AWSKMS.AccessKeyID = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_AWSKMS_SECRET_ACCESS_KEY")); v != "" {
		cfg.Projects.Encryption.AWSKMS.SecretAccessKey = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_ENCRYPTION_AWSKMS_ENDPOINT")); v != "" {
		cfg.Projects.Encryption.AWSKMS.Endpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_WORKSPACE_MODE")); v != "" {
		cfg.Projects.Workspace.Mode = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_WORKSPACE_ROOT")); v != "" {
		cfg.Projects.Workspace.Root = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_WORKSPACE_TTL_SECONDS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Projects.Workspace.TTLSeconds = n
		}
	}
	// S3/MinIO configuration for projects storage
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_ENDPOINT")); v != "" {
		cfg.Projects.S3.Endpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_REGION")); v != "" {
		cfg.Projects.S3.Region = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_BUCKET")); v != "" {
		cfg.Projects.S3.Bucket = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_PREFIX")); v != "" {
		cfg.Projects.S3.Prefix = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_ACCESS_KEY")); v != "" {
		cfg.Projects.S3.AccessKey = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_SECRET_KEY")); v != "" {
		cfg.Projects.S3.SecretKey = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_USE_PATH_STYLE")); v != "" {
		cfg.Projects.S3.UsePathStyle = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_TLS_INSECURE")); v != "" {
		cfg.Projects.S3.TLSInsecureSkipVerify = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_SSE_MODE")); v != "" {
		cfg.Projects.S3.SSE.Mode = v
	}
	if v := strings.TrimSpace(os.Getenv("PROJECTS_S3_SSE_KMS_KEY_ID")); v != "" {
		cfg.Projects.S3.SSE.KMSKeyID = v
	}

	// Optionally load specialist agents from YAML.
	if err := loadSpecialists(&cfg); err != nil {
		return Config{}, err
	}

	// Auth provider default is applied after YAML merge to allow config.yaml to override.

	// Apply defaults after merging YAML
	if cfg.OpenAI.Model == "" {
		cfg.OpenAI.Model = "gpt-4o-mini"
	}
	if cfg.OpenAI.SummaryModel == "" {
		cfg.OpenAI.SummaryModel = cfg.OpenAI.Model
	}
	if cfg.OpenAI.SummaryBaseURL == "" {
		cfg.OpenAI.SummaryBaseURL = cfg.OpenAI.BaseURL
	}
	// Default API surface to completions when absent or invalid
	if cfg.OpenAI.API == "" {
		cfg.OpenAI.API = "completions"
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider))
	if provider == "" {
		provider = "openai"
	}
	switch provider {
	case "openai", "anthropic", "google", "local":
		cfg.LLMClient.Provider = provider
	default:
		return Config{}, fmt.Errorf("llm provider must be one of openai, anthropic, google, or local (got %q)", provider)
	}
	if cfg.LLMClient.Provider == "local" {
		cfg.OpenAI.API = "completions"
	}
	if cfg.Obs.ServiceName == "" {
		cfg.Obs.ServiceName = "manifold"
	}
	if cfg.Obs.Environment == "" {
		cfg.Obs.Environment = "dev"
	}
	if cfg.Obs.ClickHouse.MetricsTable == "" {
		cfg.Obs.ClickHouse.MetricsTable = "metrics"
	}
	if cfg.Obs.ClickHouse.TracesTable == "" {
		cfg.Obs.ClickHouse.TracesTable = "traces"
	}
	if cfg.Obs.ClickHouse.LogsTable == "" {
		cfg.Obs.ClickHouse.LogsTable = "logs"
	}
	if cfg.Obs.ClickHouse.TimestampColumn == "" {
		cfg.Obs.ClickHouse.TimestampColumn = "TimeUnix"
	}
	if cfg.Obs.ClickHouse.ValueColumn == "" {
		cfg.Obs.ClickHouse.ValueColumn = "Value"
	}
	if cfg.Obs.ClickHouse.ModelAttributeKey == "" {
		cfg.Obs.ClickHouse.ModelAttributeKey = "llm.model"
	}
	if cfg.Obs.ClickHouse.PromptMetricName == "" {
		cfg.Obs.ClickHouse.PromptMetricName = "llm.prompt_tokens"
	}
	if cfg.Obs.ClickHouse.CompletionMetricName == "" {
		cfg.Obs.ClickHouse.CompletionMetricName = "llm.completion_tokens"
	}
	if cfg.Obs.ClickHouse.LookbackHours <= 0 {
		cfg.Obs.ClickHouse.LookbackHours = 24
	}
	if cfg.Obs.ClickHouse.TimeoutSeconds <= 0 {
		cfg.Obs.ClickHouse.TimeoutSeconds = 5
	}
	if cfg.Web.SearXNGURL == "" {
		cfg.Web.SearXNGURL = "http://localhost:8080"
	}
	if cfg.Kafka.Brokers == "" {
		cfg.Kafka.Brokers = "localhost:9092"
	}
	if cfg.Kafka.CommandsTopic == "" {
		cfg.Kafka.CommandsTopic = "dev.sio.orchestrator.commands"
	}
	if cfg.Kafka.ResponsesTopic == "" {
		cfg.Kafka.ResponsesTopic = "dev.sio.orchestrator.responses"
	}
	if cfg.Exec.MaxCommandSeconds == 0 {
		cfg.Exec.MaxCommandSeconds = 30
	}
	if cfg.OutputTruncateByte == 0 {
		cfg.OutputTruncateByte = 64 * 1024
	}
	if cfg.MaxSteps == 0 {
		cfg.MaxSteps = 8
	}
	// MaxToolParallelism: 0 means unbounded (default), 1 means sequential, >1 caps concurrency
	if cfg.MaxToolParallelism == 0 {
		cfg.MaxToolParallelism = 0 // Explicitly keep 0 as default (unbounded)
	}
	// Provide sensible large defaults only if explicitly needed; leave 0 = disabled
	if cfg.AgentRunTimeoutSeconds < 0 {
		cfg.AgentRunTimeoutSeconds = 0
	}
	if cfg.StreamRunTimeoutSeconds < 0 {
		cfg.StreamRunTimeoutSeconds = 0
	}
	if cfg.WorkflowTimeoutSeconds < 0 {
		cfg.WorkflowTimeoutSeconds = 0
	}

	// Apply tokenization defaults
	if cfg.Tokenization.CacheSize <= 0 {
		cfg.Tokenization.CacheSize = 1000
	}
	if cfg.Tokenization.CacheTTLSeconds <= 0 {
		cfg.Tokenization.CacheTTLSeconds = 3600 // 1 hour
	}
	// Tokenization.FallbackToHeuristic defaults to true at initialization.

	// Apply embedding defaults
	if cfg.Embedding.BaseURL == "" {
		cfg.Embedding.BaseURL = "https://api.openai.com"
	}
	if cfg.Embedding.Model == "" {
		cfg.Embedding.Model = "text-embedding-3-small"
	}
	if cfg.Embedding.APIHeader == "" {
		cfg.Embedding.APIHeader = "Authorization"
	}
	if cfg.Embedding.Path == "" {
		cfg.Embedding.Path = "/v1/embeddings"
	}
	if cfg.Embedding.Timeout == 0 {
		cfg.Embedding.Timeout = 30
	}

	// Apply database defaults. If a DefaultDSN is provided and the backend is
	// unspecified, prefer "auto" so the factory can attempt Postgres.
	if cfg.Databases.Search.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Search.Backend = "auto"
		} else {
			cfg.Databases.Search.Backend = "memory"
		}
	}
	if cfg.Databases.Vector.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Vector.Backend = "auto"
		} else {
			cfg.Databases.Vector.Backend = "memory"
		}
	}
	if cfg.Databases.Graph.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Graph.Backend = "auto"
		} else {
			cfg.Databases.Graph.Backend = "memory"
		}
	}
	if cfg.Databases.Chat.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Chat.Backend = "auto"
		} else {
			cfg.Databases.Chat.Backend = "memory"
		}
	}

	if strings.TrimSpace(cfg.Auth.Provider) == "" {
		cfg.Auth.Provider = "oidc"
	}

	// Apply projects defaults
	if cfg.Projects.Backend == "" {
		cfg.Projects.Backend = "filesystem"
	}
	if cfg.Projects.Workspace.Mode == "" {
		cfg.Projects.Workspace.Mode = "legacy"
	}
	if cfg.Projects.Workspace.TTLSeconds == 0 {
		cfg.Projects.Workspace.TTLSeconds = 86400 // 24 hours
	}
	if cfg.Projects.S3.Region == "" {
		cfg.Projects.S3.Region = "us-east-1"
	}
	if cfg.Projects.S3.Prefix == "" {
		cfg.Projects.S3.Prefix = "workspaces"
	}
	if cfg.Projects.S3.SSE.Mode == "" {
		cfg.Projects.S3.SSE.Mode = "none"
	}

	if cfg.OpenAI.APIKey == "" {
		return Config{}, errors.New("OPENAI_API_KEY is required for llm_client.openai (set in .env or environment)")
	}
	for i := range cfg.Specialists {
		if strings.TrimSpace(cfg.Specialists[i].Provider) == "" {
			cfg.Specialists[i].Provider = cfg.LLMClient.Provider
		}
	}
	if cfg.Workdir == "" {
		return Config{}, errors.New("WORKDIR is required (set in .env or environment)")
	}

	absWD, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve WORKDIR: %w", err)
	}
	info, err := os.Stat(absWD)
	if err != nil {
		return Config{}, fmt.Errorf("stat WORKDIR: %w", err)
	}
	if !info.IsDir() {
		return Config{}, fmt.Errorf("WORKDIR must be a directory: %s", absWD)
	}
	cfg.Workdir = absWD

	// Apply workspace root default after WORKDIR is resolved
	if cfg.Projects.Workspace.Root == "" {
		cfg.Projects.Workspace.Root = filepath.Join(absWD, "sandboxes")
	}

	// Keep LLMClient.OpenAI in sync with the effective OpenAI config.
	cfg.LLMClient.OpenAI = cfg.OpenAI

	// Parse blocklist
	blockStr := strings.TrimSpace(os.Getenv("BLOCK_BINARIES"))
	if blockStr != "" {
		// env overrides any YAML-defined list
		cfg.Exec.BlockBinaries = nil
		for _, p := range parseCommaSeparatedList(blockStr) {
			if strings.Contains(p, "/") || strings.Contains(p, "\\") {
				return Config{}, fmt.Errorf("BLOCK_BINARIES must contain bare binary names only (no paths): %q", p)
			}
			cfg.Exec.BlockBinaries = append(cfg.Exec.BlockBinaries, p)
		}
	}
	return cfg, nil
}

// loadSpecialists populates cfg.Specialists by reading a YAML file if present.
// The file path can be specified with SPECIALISTS_CONFIG. If not set, the
// loader will look for config.yaml or config.yml in the current working directory.
func loadSpecialists(cfg *Config) error {
	// Allow disabling via env
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SPECIALISTS_DISABLED")), "true") {
		return nil
	}
	providerFromEnv := strings.TrimSpace(os.Getenv("LLM_PROVIDER")) != ""
	openAIAPIKeyFromEnv := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != ""
	openAIModelFromEnv := strings.TrimSpace(os.Getenv("OPENAI_MODEL")) != ""
	openAIBaseURLFromEnv := firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), strings.TrimSpace(os.Getenv("OPENAI_API_BASE_URL"))) != ""
	openAISummaryURLFromEnv := strings.TrimSpace(os.Getenv("OPENAI_SUMMARY_URL")) != ""
	openAISummaryModelFromEnv := strings.TrimSpace(os.Getenv("OPENAI_SUMMARY_MODEL")) != ""
	openAIAPIChoiceFromEnv := strings.TrimSpace(os.Getenv("OPENAI_API")) != ""
	workdirFromEnv := strings.TrimSpace(os.Getenv("WORKDIR")) != ""
	logPathFromEnv := strings.TrimSpace(os.Getenv("LOG_PATH")) != ""
	logLevelFromEnv := strings.TrimSpace(os.Getenv("LOG_LEVEL")) != ""
	logPayloadsFromEnv := strings.TrimSpace(os.Getenv("LOG_PAYLOADS")) != ""
	maxStepsFromEnv := strings.TrimSpace(os.Getenv("MAX_STEPS")) != ""
	maxToolParallelismFromEnv := strings.TrimSpace(os.Getenv("MAX_TOOL_PARALLELISM")) != ""
	agentRunTimeoutFromEnv := strings.TrimSpace(os.Getenv("AGENT_RUN_TIMEOUT_SECONDS")) != ""
	streamRunTimeoutFromEnv := strings.TrimSpace(os.Getenv("STREAM_RUN_TIMEOUT_SECONDS")) != ""
	workflowTimeoutFromEnv := strings.TrimSpace(os.Getenv("WORKFLOW_TIMEOUT_SECONDS")) != ""
	summaryContextWindowTokensFromEnv := strings.TrimSpace(os.Getenv("SUMMARY_CONTEXT_WINDOW_TOKENS")) != ""
	summaryReserveBufferTokensFromEnv := strings.TrimSpace(os.Getenv("SUMMARY_RESERVE_BUFFER_TOKENS")) != ""
	summaryMinKeepLastFromEnv := strings.TrimSpace(os.Getenv("SUMMARY_MIN_KEEP_LAST_MESSAGES")) != ""
	summaryMaxKeepLastFromEnv := strings.TrimSpace(os.Getenv("SUMMARY_MAX_KEEP_LAST_MESSAGES")) != ""
	summaryMaxSummaryChunkTokensFromEnv := strings.TrimSpace(os.Getenv("SUMMARY_MAX_SUMMARY_CHUNK_TOKENS")) != ""
	outputTruncateBytesFromEnv := strings.TrimSpace(os.Getenv("OUTPUT_TRUNCATE_BYTES")) != ""
	anthropicAPIKeyFromEnv := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != ""
	anthropicModelFromEnv := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL")) != ""
	anthropicBaseURLFromEnv := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")) != ""
	googleAPIKeyFromEnv := strings.TrimSpace(os.Getenv("GOOGLE_LLM_API_KEY")) != ""
	googleModelFromEnv := strings.TrimSpace(os.Getenv("GOOGLE_LLM_MODEL")) != ""
	googleBaseURLFromEnv := strings.TrimSpace(os.Getenv("GOOGLE_LLM_BASE_URL")) != ""
	var paths []string
	if p := strings.TrimSpace(os.Getenv("SPECIALISTS_CONFIG")); p != "" {
		paths = append(paths, p)
	}
	// Default to local config file next to the running binary / current dir
	paths = append(paths, "config.yaml", "config.yml")
	var data []byte
	var chosen string
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			data = b
			chosen = p
			break
		}
		if os.IsNotExist(err) {
			continue
		}
		// Unexpected read error
		return fmt.Errorf("read %s: %w", p, err)
	}
	if len(data) == 0 {
		return nil // optional
	}
	// Two accepted shapes:
	//   specialists: [ {name: ..., ...}, ... ]
	// or directly a list: [ {name: ..., ...} ]
	type openAIYAML struct {
		APIKey         string            `yaml:"apiKey"`
		Model          string            `yaml:"model"`
		BaseURL        string            `yaml:"baseURL"`
		SummaryModel   string            `yaml:"summaryModel"`
		SummaryBaseURL string            `yaml:"summaryBaseURL"`
		API            string            `yaml:"api"`
		ExtraHeaders   map[string]string `yaml:"extraHeaders"`
		ExtraParams    map[string]any    `yaml:"extraParams"`
		LogPayloads    bool              `yaml:"logPayloads"`
	}
	type anthropicYAML struct {
		APIKey  string `yaml:"apiKey"`
		Model   string `yaml:"model"`
		BaseURL string `yaml:"baseURL"`
	}
	type googleYAML struct {
		APIKey  string `yaml:"apiKey"`
		Model   string `yaml:"model"`
		BaseURL string `yaml:"baseURL"`
	}
	type llmClientYAML struct {
		Provider  string        `yaml:"provider"`
		OpenAI    openAIYAML    `yaml:"openai"`
		Anthropic anthropicYAML `yaml:"anthropic"`
		Google    googleYAML    `yaml:"google"`
	}
	type execYAML struct {
		BlockBinaries     []string `yaml:"blockBinaries"`
		MaxCommandSeconds int      `yaml:"maxCommandSeconds"`
	}
	type kafkaYAML struct {
		Brokers        string `yaml:"brokers"`
		CommandsTopic  string `yaml:"commandsTopic"`
		ResponsesTopic string `yaml:"responsesTopic"`
	}
	type obsClickhouseYAML struct {
		DSN                  string `yaml:"dsn"`
		Database             string `yaml:"database"`
		MetricsTable         string `yaml:"metricsTable"`
		TracesTable          string `yaml:"tracesTable"`
		LogsTable            string `yaml:"logsTable"`
		TimestampColumn      string `yaml:"timestampColumn"`
		ValueColumn          string `yaml:"valueColumn"`
		ModelAttributeKey    string `yaml:"modelAttributeKey"`
		PromptMetricName     string `yaml:"promptMetricName"`
		CompletionMetricName string `yaml:"completionMetricName"`
		LookbackHours        int    `yaml:"lookbackHours"`
		TimeoutSeconds       int    `yaml:"timeoutSeconds"`
	}
	type obsYAML struct {
		ServiceName    string            `yaml:"serviceName"`
		ServiceVersion string            `yaml:"serviceVersion"`
		Environment    string            `yaml:"environment"`
		OTLP           string            `yaml:"otlp"`
		ClickHouse     obsClickhouseYAML `yaml:"clickhouse"`
	}
	type webYAML struct {
		SearXNGURL string `yaml:"searXNGURL"`
	}
	type dbSearchYAML struct {
		Backend string `yaml:"backend"`
		DSN     string `yaml:"dsn"`
		Index   string `yaml:"index"`
	}
	type dbVectorYAML struct {
		Backend    string `yaml:"backend"`
		DSN        string `yaml:"dsn"`
		Index      string `yaml:"index"`
		Dimensions int    `yaml:"dimensions"`
		Metric     string `yaml:"metric"`
	}
	type dbGraphYAML struct {
		Backend string `yaml:"backend"`
		DSN     string `yaml:"dsn"`
	}
	type dbChatYAML struct {
		Backend string `yaml:"backend"`
		DSN     string `yaml:"dsn"`
	}
	type databasesYAML struct {
		DefaultDSN string       `yaml:"defaultDSN"`
		Search     dbSearchYAML `yaml:"search"`
		Vector     dbVectorYAML `yaml:"vector"`
		Graph      dbGraphYAML  `yaml:"graph"`
		Chat       dbChatYAML   `yaml:"chat"`
	}
	type mcpServerYAML struct {
		Name             string            `yaml:"name"`
		Command          string            `yaml:"command"`
		Args             []string          `yaml:"args"`
		Env              map[string]string `yaml:"env"`
		KeepAliveSeconds int               `yaml:"keepAliveSeconds"`
		PathDependent    bool              `yaml:"pathDependent"`
		URL              string            `yaml:"url"`
		Headers          map[string]string `yaml:"headers"`
		BearerToken      string            `yaml:"bearerToken"`
		Origin           string            `yaml:"origin"`
		ProtocolVersion  string            `yaml:"protocolVersion"`
		HTTP             struct {
			TimeoutSeconds int    `yaml:"timeoutSeconds"`
			ProxyURL       string `yaml:"proxyURL"`
			TLS            struct {
				InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
				CAFile             string `yaml:"caFile"`
				CertFile           string `yaml:"certFile"`
				KeyFile            string `yaml:"keyFile"`
			} `yaml:"tls"`
		} `yaml:"http"`
	}
	type mcpYAML struct {
		Servers []mcpServerYAML `yaml:"servers"`
	}
	type embeddingYAML struct {
		BaseURL        string `yaml:"baseURL"`
		Model          string `yaml:"model"`
		APIKey         string `yaml:"apiKey"`
		APIHeader      string `yaml:"apiHeader"`
		Path           string `yaml:"path"`
		TimeoutSeconds int    `yaml:"timeoutSeconds"`
	}
	type evolvingMemoryYAML struct {
		Enabled          bool    `yaml:"enabled"`
		MaxSize          int     `yaml:"maxSize"`
		TopK             int     `yaml:"topK"`
		WindowSize       int     `yaml:"windowSize"`
		EnableRAG        bool    `yaml:"enableRAG"`
		ReMemEnabled     bool    `yaml:"reMemEnabled"`
		MaxInnerSteps    int     `yaml:"maxInnerSteps"`
		Model            string  `yaml:"model"`
		EnableSmartPrune bool    `yaml:"enableSmartPrune"`
		PruneThreshold   float64 `yaml:"pruneThreshold"`
		RelevanceDecay   float64 `yaml:"relevanceDecay"`
		MinRelevance     float64 `yaml:"minRelevance"`
	}
	type ttsYAML struct {
		BaseURL string `yaml:"baseURL"`
		Model   string `yaml:"model"`
		Voice   string `yaml:"voice"`
		Format  string `yaml:"format"`
	}
	type fileKeyProviderYAML struct {
		KeystorePath string `yaml:"keystorePath"`
	}
	type vaultKeyProviderYAML struct {
		Address        string `yaml:"address"`
		Token          string `yaml:"token"`
		KeyName        string `yaml:"keyName"`
		MountPath      string `yaml:"mountPath"`
		Namespace      string `yaml:"namespace"`
		TLSSkipVerify  bool   `yaml:"tlsSkipVerify"`
		TimeoutSeconds int    `yaml:"timeoutSeconds"`
	}
	type awsKMSKeyProviderYAML struct {
		KeyID           string `yaml:"keyID"`
		Region          string `yaml:"region"`
		AccessKeyID     string `yaml:"accessKeyID"`
		SecretAccessKey string `yaml:"secretAccessKey"`
		Endpoint        string `yaml:"endpoint"`
	}
	type encryptionYAML struct {
		Provider string                `yaml:"provider"`
		File     fileKeyProviderYAML   `yaml:"file"`
		Vault    vaultKeyProviderYAML  `yaml:"vault"`
		AWSKMS   awsKMSKeyProviderYAML `yaml:"awskms"`
	}
	type redisYAML struct {
		Enabled               bool   `yaml:"enabled"`
		Addr                  string `yaml:"addr"`
		Password              string `yaml:"password"`
		DB                    int    `yaml:"db"`
		TLSInsecureSkipVerify bool   `yaml:"tlsInsecureSkipVerify"`
	}
	type projectsKafkaYAML struct {
		Enabled bool   `yaml:"enabled"`
		Brokers string `yaml:"brokers"`
		Topic   string `yaml:"topic"`
	}
	type skillsYAML struct {
		RedisCacheTTLSeconds int   `yaml:"redisCacheTTLSeconds"`
		UseS3Loader          *bool `yaml:"useS3Loader"`
	}
	type tokenizationYAML struct {
		Enabled             bool  `yaml:"enabled"`
		CacheSize           int   `yaml:"cacheSize"`
		CacheTTLSeconds     int   `yaml:"cacheTTLSeconds"`
		FallbackToHeuristic *bool `yaml:"fallbackToHeuristic"`
	}
	type s3SSEConfigYAML struct {
		Mode     string `yaml:"mode"`
		KMSKeyID string `yaml:"kmsKeyID"`
	}
	type s3ConfigYAML struct {
		Endpoint              string          `yaml:"endpoint"`
		Region                string          `yaml:"region"`
		Bucket                string          `yaml:"bucket"`
		Prefix                string          `yaml:"prefix"`
		AccessKey             string          `yaml:"accessKey"`
		SecretKey             string          `yaml:"secretKey"`
		UsePathStyle          bool            `yaml:"usePathStyle"`
		TLSInsecureSkipVerify bool            `yaml:"tlsInsecureSkipVerify"`
		SSE                   s3SSEConfigYAML `yaml:"sse"`
	}
	type workspaceConfigYAML struct {
		Mode       string `yaml:"mode"`
		Root       string `yaml:"root"`
		TTLSeconds int    `yaml:"ttlSeconds"`
		CacheDir   string `yaml:"cacheDir"`
		TmpfsDir   string `yaml:"tmpfsDir"`
	}
	type projectsYAML struct {
		Backend    string              `yaml:"backend"`
		Encrypt    bool                `yaml:"encrypt"`
		Encryption encryptionYAML      `yaml:"encryption"`
		Workspace  workspaceConfigYAML `yaml:"workspace"`
		S3         s3ConfigYAML        `yaml:"s3"`
		Redis      redisYAML           `yaml:"redis"`
		Events     projectsKafkaYAML   `yaml:"events"`
	}
	type oauth2YAML struct {
		AuthURL             string   `yaml:"authURL"`
		TokenURL            string   `yaml:"tokenURL"`
		UserInfoURL         string   `yaml:"userInfoURL"`
		LogoutURL           string   `yaml:"logoutURL"`
		LogoutRedirectParam string   `yaml:"logoutRedirectParam"`
		Scopes              []string `yaml:"scopes"`
		ProviderName        string   `yaml:"providerName"`
		DefaultRoles        []string `yaml:"defaultRoles"`
		EmailField          string   `yaml:"emailField"`
		NameField           string   `yaml:"nameField"`
		PictureField        string   `yaml:"pictureField"`
		SubjectField        string   `yaml:"subjectField"`
		RolesField          string   `yaml:"rolesField"`
	}
	type authYAML struct {
		Enabled         bool       `yaml:"enabled"`
		Provider        string     `yaml:"provider"`
		IssuerURL       string     `yaml:"issuerURL"`
		ClientID        string     `yaml:"clientID"`
		ClientSecret    string     `yaml:"clientSecret"`
		RedirectURL     string     `yaml:"redirectURL"`
		AllowedDomains  []string   `yaml:"allowedDomains"`
		CookieName      string     `yaml:"cookieName"`
		CookieSecure    bool       `yaml:"cookieSecure"`
		CookieDomain    string     `yaml:"cookieDomain"`
		StateTTLSeconds int        `yaml:"stateTTLSeconds"`
		SessionTTLHours int        `yaml:"sessionTTLHours"`
		OAuth2          oauth2YAML `yaml:"oauth2"`
	}
	type wrap struct {
		// SystemPrompt is an optional top-level YAML field to override the
		// default system prompt used by the agent.
		SystemPrompt                 string             `yaml:"systemPrompt"`
		Specialists                  []SpecialistConfig `yaml:"specialists"`
		Routes                       []SpecialistRoute  `yaml:"routes"`
		LLMClient                    llmClientYAML      `yaml:"llm_client"`
		OpenAI                       openAIYAML         `yaml:"openai"`
		Workdir                      string             `yaml:"workdir"`
		LogPath                      string             `yaml:"logPath"`
		LogLevel                     string             `yaml:"logLevel"`
		LogPayloads                  *bool              `yaml:"logPayloads"`
		OutputTruncB                 int                `yaml:"outputTruncateBytes"`
		OutputTrunc                  int                `yaml:"outputTruncateByte"`
		SummaryEnabled               bool               `yaml:"summaryEnabled"`
		SummaryContextWindowTokens   int                `yaml:"summaryContextWindowTokens"`
		SummaryReserveBufferTokens   int                `yaml:"summaryReserveBufferTokens"`
		SummaryMinKeepLastMessages   int                `yaml:"summaryMinKeepLastMessages"`
		SummaryMaxKeepLastMessages   int                `yaml:"summaryMaxKeepLastMessages"`
		SummaryMaxSummaryChunkTokens int                `yaml:"summaryMaxSummaryChunkTokens"`
		MaxSteps                     int                `yaml:"maxSteps"`
		MaxToolParallelism           int                `yaml:"maxToolParallelism"`
		Exec                         execYAML           `yaml:"exec"`
		Kafka                        kafkaYAML          `yaml:"kafka"`
		Obs                          obsYAML            `yaml:"obs"`
		Web                          webYAML            `yaml:"web"`
		Databases                    databasesYAML      `yaml:"databases"`
		MCP                          mcpYAML            `yaml:"mcp"`
		Embedding                    embeddingYAML      `yaml:"embedding"`
		EvolvingMemory               evolvingMemoryYAML `yaml:"evolvingMemory"`
		TTS                          ttsYAML            `yaml:"tts"`
		Projects                     projectsYAML       `yaml:"projects"`
		Skills                       skillsYAML         `yaml:"skills"`
		Tokenization                 tokenizationYAML   `yaml:"tokenization"`
		EnableTools                  *bool              `yaml:"enableTools"`
		// AllowTools is a top-level allow-list for tools exposed to the main agent.
		// If present, it should map into cfg.ToolAllowList.
		AllowTools              []string `yaml:"allowTools"`
		Auth                    authYAML `yaml:"auth"`
		AgentRunTimeoutSeconds  int      `yaml:"agentRunTimeoutSeconds"`
		StreamRunTimeoutSeconds int      `yaml:"streamRunTimeoutSeconds"`
		WorkflowTimeoutSeconds  int      `yaml:"workflowTimeoutSeconds"`
	}
	var w wrap
	// Expand ${VAR} with environment variables before parsing.
	data = []byte(os.ExpandEnv(string(data)))
	if err := yaml.Unmarshal(data, &w); err != nil {
		// Fallback: try list at root
		var list []SpecialistConfig
		if err2 := yaml.Unmarshal(data, &list); err2 == nil {
			cfg.Specialists = list
			return nil
		}
		return fmt.Errorf("%s: could not parse specialists configuration: %w", chosen, err)
	} else {
		// Specialists and routes
		if len(w.Specialists) > 0 {
			cfg.Specialists = w.Specialists
		}
		if len(w.Routes) > 0 {
			cfg.SpecialistRoutes = w.Routes
		}
		for i := range cfg.Specialists {
			if strings.TrimSpace(cfg.Specialists[i].Provider) == "" {
				cfg.Specialists[i].Provider = cfg.LLMClient.Provider
			}
		}

		applyOpenAIExtras := func(src openAIYAML, override bool) {
			if len(src.ExtraHeaders) > 0 && (override || len(cfg.OpenAI.ExtraHeaders) == 0) {
				cfg.OpenAI.ExtraHeaders = src.ExtraHeaders
			}
			if len(src.ExtraParams) > 0 && (override || len(cfg.OpenAI.ExtraParams) == 0) {
				cfg.OpenAI.ExtraParams = src.ExtraParams
			}
			if !cfg.LogPayloads && src.LogPayloads {
				cfg.LogPayloads = true
				cfg.OpenAI.LogPayloads = true
			}
		}
		applyOpenAICore := func(src openAIYAML, allowOverride bool) {
			if !openAIAPIKeyFromEnv && strings.TrimSpace(src.APIKey) != "" {
				if allowOverride || cfg.OpenAI.APIKey == "" {
					cfg.OpenAI.APIKey = strings.TrimSpace(src.APIKey)
				}
			}
			if !openAIModelFromEnv && strings.TrimSpace(src.Model) != "" {
				if allowOverride || cfg.OpenAI.Model == "" {
					cfg.OpenAI.Model = strings.TrimSpace(src.Model)
				}
			}
			if !openAIBaseURLFromEnv && strings.TrimSpace(src.BaseURL) != "" {
				if allowOverride || cfg.OpenAI.BaseURL == "" {
					cfg.OpenAI.BaseURL = strings.TrimSpace(src.BaseURL)
				}
			}
			if !openAISummaryModelFromEnv && strings.TrimSpace(src.SummaryModel) != "" {
				if allowOverride || cfg.OpenAI.SummaryModel == "" {
					cfg.OpenAI.SummaryModel = strings.TrimSpace(src.SummaryModel)
				}
			}
			if !openAISummaryURLFromEnv && strings.TrimSpace(src.SummaryBaseURL) != "" {
				if allowOverride || cfg.OpenAI.SummaryBaseURL == "" {
					cfg.OpenAI.SummaryBaseURL = strings.TrimSpace(src.SummaryBaseURL)
				}
			}
			if !openAIAPIChoiceFromEnv && strings.TrimSpace(src.API) != "" {
				if allowOverride || cfg.OpenAI.API == "" {
					cfg.OpenAI.API = strings.TrimSpace(src.API)
				}
			}
		}

		// Legacy openai: only fill empty fields (env takes precedence).
		applyOpenAIExtras(w.OpenAI, false)
		applyOpenAICore(w.OpenAI, false)

		if strings.TrimSpace(w.LLMClient.Provider) != "" && !providerFromEnv {
			cfg.LLMClient.Provider = strings.TrimSpace(w.LLMClient.Provider)
		}
		applyOpenAIExtras(w.LLMClient.OpenAI, true)
		applyOpenAICore(w.LLMClient.OpenAI, true)
		if !anthropicAPIKeyFromEnv && strings.TrimSpace(w.LLMClient.Anthropic.APIKey) != "" {
			cfg.LLMClient.Anthropic.APIKey = strings.TrimSpace(w.LLMClient.Anthropic.APIKey)
		}
		if !anthropicModelFromEnv && strings.TrimSpace(w.LLMClient.Anthropic.Model) != "" {
			cfg.LLMClient.Anthropic.Model = strings.TrimSpace(w.LLMClient.Anthropic.Model)
		}
		if !anthropicBaseURLFromEnv && strings.TrimSpace(w.LLMClient.Anthropic.BaseURL) != "" {
			cfg.LLMClient.Anthropic.BaseURL = strings.TrimSpace(w.LLMClient.Anthropic.BaseURL)
		}
		if !googleAPIKeyFromEnv && strings.TrimSpace(w.LLMClient.Google.APIKey) != "" {
			cfg.LLMClient.Google.APIKey = strings.TrimSpace(w.LLMClient.Google.APIKey)
		}
		if !googleModelFromEnv && strings.TrimSpace(w.LLMClient.Google.Model) != "" {
			cfg.LLMClient.Google.Model = strings.TrimSpace(w.LLMClient.Google.Model)
		}
		if !googleBaseURLFromEnv && strings.TrimSpace(w.LLMClient.Google.BaseURL) != "" {
			cfg.LLMClient.Google.BaseURL = strings.TrimSpace(w.LLMClient.Google.BaseURL)
		}
		// Workdir and others only if empty
		if cfg.Workdir == "" && strings.TrimSpace(w.Workdir) != "" && !workdirFromEnv {
			cfg.Workdir = strings.TrimSpace(w.Workdir)
		}
		if cfg.LogPath == "" && strings.TrimSpace(w.LogPath) != "" && !logPathFromEnv {
			cfg.LogPath = strings.TrimSpace(w.LogPath)
		}
		if cfg.LogLevel == "" && strings.TrimSpace(w.LogLevel) != "" && !logLevelFromEnv {
			cfg.LogLevel = strings.TrimSpace(w.LogLevel)
		}
		if w.LogPayloads != nil && !logPayloadsFromEnv {
			cfg.LogPayloads = *w.LogPayloads
			cfg.OpenAI.LogPayloads = *w.LogPayloads
		}
		// YAML-provided system prompt is applied only if env did not already
		// provide an override.
		if cfg.SystemPrompt == "" && strings.TrimSpace(w.SystemPrompt) != "" {
			cfg.SystemPrompt = w.SystemPrompt
		}

		outputTruncate := w.OutputTruncB
		if outputTruncate == 0 {
			outputTruncate = w.OutputTrunc
		}
		if cfg.OutputTruncateByte == 0 && outputTruncate > 0 && !outputTruncateBytesFromEnv {
			cfg.OutputTruncateByte = outputTruncate
		}
		// Summary config from YAML if not provided via env
		if !cfg.SummaryEnabled && w.SummaryEnabled {
			cfg.SummaryEnabled = true
		}
		if cfg.SummaryContextWindowTokens == 0 && w.SummaryContextWindowTokens > 0 && !summaryContextWindowTokensFromEnv {
			cfg.SummaryContextWindowTokens = w.SummaryContextWindowTokens
		}
		if cfg.SummaryReserveBufferTokens == 0 && w.SummaryReserveBufferTokens > 0 && !summaryReserveBufferTokensFromEnv {
			cfg.SummaryReserveBufferTokens = w.SummaryReserveBufferTokens
		}
		if cfg.SummaryMinKeepLastMessages == 0 && w.SummaryMinKeepLastMessages > 0 && !summaryMinKeepLastFromEnv {
			cfg.SummaryMinKeepLastMessages = w.SummaryMinKeepLastMessages
		}
		if cfg.SummaryMaxKeepLastMessages == 0 && w.SummaryMaxKeepLastMessages > 0 && !summaryMaxKeepLastFromEnv {
			cfg.SummaryMaxKeepLastMessages = w.SummaryMaxKeepLastMessages
		}
		if cfg.SummaryMaxSummaryChunkTokens == 0 && w.SummaryMaxSummaryChunkTokens > 0 && !summaryMaxSummaryChunkTokensFromEnv {
			cfg.SummaryMaxSummaryChunkTokens = w.SummaryMaxSummaryChunkTokens
		}
		if cfg.MaxSteps == 0 && w.MaxSteps > 0 && !maxStepsFromEnv {
			cfg.MaxSteps = w.MaxSteps
		}
		if cfg.MaxToolParallelism == 0 && w.MaxToolParallelism != 0 && !maxToolParallelismFromEnv {
			cfg.MaxToolParallelism = w.MaxToolParallelism
		}
		if cfg.AgentRunTimeoutSeconds == 0 && w.AgentRunTimeoutSeconds != 0 && !agentRunTimeoutFromEnv {
			cfg.AgentRunTimeoutSeconds = w.AgentRunTimeoutSeconds
		}
		if cfg.StreamRunTimeoutSeconds == 0 && w.StreamRunTimeoutSeconds != 0 && !streamRunTimeoutFromEnv {
			cfg.StreamRunTimeoutSeconds = w.StreamRunTimeoutSeconds
		}
		if cfg.WorkflowTimeoutSeconds == 0 && w.WorkflowTimeoutSeconds != 0 && !workflowTimeoutFromEnv {
			cfg.WorkflowTimeoutSeconds = w.WorkflowTimeoutSeconds
		}
		if cfg.Kafka.Brokers == "" && strings.TrimSpace(w.Kafka.Brokers) != "" {
			cfg.Kafka.Brokers = strings.TrimSpace(w.Kafka.Brokers)
		}
		if cfg.Kafka.CommandsTopic == "" && strings.TrimSpace(w.Kafka.CommandsTopic) != "" {
			cfg.Kafka.CommandsTopic = strings.TrimSpace(w.Kafka.CommandsTopic)
		}
		if cfg.Kafka.ResponsesTopic == "" && strings.TrimSpace(w.Kafka.ResponsesTopic) != "" {
			cfg.Kafka.ResponsesTopic = strings.TrimSpace(w.Kafka.ResponsesTopic)
		}
		if cfg.Exec.MaxCommandSeconds == 0 && w.Exec.MaxCommandSeconds > 0 {
			cfg.Exec.MaxCommandSeconds = w.Exec.MaxCommandSeconds
		}
		if len(cfg.Exec.BlockBinaries) == 0 && len(w.Exec.BlockBinaries) > 0 {
			cfg.Exec.BlockBinaries = append([]string{}, w.Exec.BlockBinaries...)
		}
		if cfg.Obs.ServiceName == "" && w.Obs.ServiceName != "" {
			cfg.Obs.ServiceName = w.Obs.ServiceName
		}
		if cfg.Obs.ServiceVersion == "" && w.Obs.ServiceVersion != "" {
			cfg.Obs.ServiceVersion = w.Obs.ServiceVersion
		}
		if cfg.Obs.Environment == "" && w.Obs.Environment != "" {
			cfg.Obs.Environment = w.Obs.Environment
		}
		if cfg.Obs.OTLP == "" && w.Obs.OTLP != "" {
			cfg.Obs.OTLP = w.Obs.OTLP
		}
		if cfg.Obs.ClickHouse.DSN == "" && strings.TrimSpace(w.Obs.ClickHouse.DSN) != "" {
			cfg.Obs.ClickHouse.DSN = strings.TrimSpace(w.Obs.ClickHouse.DSN)
		}
		if cfg.Obs.ClickHouse.Database == "" && strings.TrimSpace(w.Obs.ClickHouse.Database) != "" {
			cfg.Obs.ClickHouse.Database = strings.TrimSpace(w.Obs.ClickHouse.Database)
		}
		if cfg.Obs.ClickHouse.MetricsTable == "" && strings.TrimSpace(w.Obs.ClickHouse.MetricsTable) != "" {
			cfg.Obs.ClickHouse.MetricsTable = strings.TrimSpace(w.Obs.ClickHouse.MetricsTable)
		}
		if cfg.Obs.ClickHouse.TracesTable == "" && strings.TrimSpace(w.Obs.ClickHouse.TracesTable) != "" {
			cfg.Obs.ClickHouse.TracesTable = strings.TrimSpace(w.Obs.ClickHouse.TracesTable)
		}
		if cfg.Obs.ClickHouse.LogsTable == "" && strings.TrimSpace(w.Obs.ClickHouse.LogsTable) != "" {
			cfg.Obs.ClickHouse.LogsTable = strings.TrimSpace(w.Obs.ClickHouse.LogsTable)
		}
		if cfg.Obs.ClickHouse.TimestampColumn == "" && strings.TrimSpace(w.Obs.ClickHouse.TimestampColumn) != "" {
			cfg.Obs.ClickHouse.TimestampColumn = strings.TrimSpace(w.Obs.ClickHouse.TimestampColumn)
		}
		if cfg.Obs.ClickHouse.ValueColumn == "" && strings.TrimSpace(w.Obs.ClickHouse.ValueColumn) != "" {
			cfg.Obs.ClickHouse.ValueColumn = strings.TrimSpace(w.Obs.ClickHouse.ValueColumn)
		}
		if cfg.Obs.ClickHouse.ModelAttributeKey == "" && strings.TrimSpace(w.Obs.ClickHouse.ModelAttributeKey) != "" {
			cfg.Obs.ClickHouse.ModelAttributeKey = strings.TrimSpace(w.Obs.ClickHouse.ModelAttributeKey)
		}
		if cfg.Obs.ClickHouse.PromptMetricName == "" && strings.TrimSpace(w.Obs.ClickHouse.PromptMetricName) != "" {
			cfg.Obs.ClickHouse.PromptMetricName = strings.TrimSpace(w.Obs.ClickHouse.PromptMetricName)
		}
		if cfg.Obs.ClickHouse.CompletionMetricName == "" && strings.TrimSpace(w.Obs.ClickHouse.CompletionMetricName) != "" {
			cfg.Obs.ClickHouse.CompletionMetricName = strings.TrimSpace(w.Obs.ClickHouse.CompletionMetricName)
		}
		if cfg.Obs.ClickHouse.LookbackHours == 0 && w.Obs.ClickHouse.LookbackHours > 0 {
			cfg.Obs.ClickHouse.LookbackHours = w.Obs.ClickHouse.LookbackHours
		}
		if cfg.Obs.ClickHouse.TimeoutSeconds == 0 && w.Obs.ClickHouse.TimeoutSeconds > 0 {
			cfg.Obs.ClickHouse.TimeoutSeconds = w.Obs.ClickHouse.TimeoutSeconds
		}
		if cfg.Web.SearXNGURL == "" && strings.TrimSpace(w.Web.SearXNGURL) != "" {
			cfg.Web.SearXNGURL = strings.TrimSpace(w.Web.SearXNGURL)
		}
		// Databases: only assign fields which are empty to allow env to override
		if cfg.Databases.DefaultDSN == "" && strings.TrimSpace(w.Databases.DefaultDSN) != "" {
			cfg.Databases.DefaultDSN = strings.TrimSpace(w.Databases.DefaultDSN)
		}
		if cfg.Databases.Search.Backend == "" && strings.TrimSpace(w.Databases.Search.Backend) != "" {
			cfg.Databases.Search.Backend = strings.TrimSpace(w.Databases.Search.Backend)
		}
		if cfg.Databases.Search.DSN == "" && strings.TrimSpace(w.Databases.Search.DSN) != "" {
			cfg.Databases.Search.DSN = strings.TrimSpace(w.Databases.Search.DSN)
		}
		if cfg.Databases.Search.Index == "" && strings.TrimSpace(w.Databases.Search.Index) != "" {
			cfg.Databases.Search.Index = strings.TrimSpace(w.Databases.Search.Index)
		}
		if cfg.Databases.Vector.Backend == "" && strings.TrimSpace(w.Databases.Vector.Backend) != "" {
			cfg.Databases.Vector.Backend = strings.TrimSpace(w.Databases.Vector.Backend)
		}
		if cfg.Databases.Vector.DSN == "" && strings.TrimSpace(w.Databases.Vector.DSN) != "" {
			cfg.Databases.Vector.DSN = strings.TrimSpace(w.Databases.Vector.DSN)
		}
		if cfg.Databases.Vector.Index == "" && strings.TrimSpace(w.Databases.Vector.Index) != "" {
			cfg.Databases.Vector.Index = strings.TrimSpace(w.Databases.Vector.Index)
		}
		if cfg.Databases.Vector.Dimensions == 0 && w.Databases.Vector.Dimensions > 0 {
			cfg.Databases.Vector.Dimensions = w.Databases.Vector.Dimensions
		}
		if cfg.Databases.Vector.Metric == "" && strings.TrimSpace(w.Databases.Vector.Metric) != "" {
			cfg.Databases.Vector.Metric = strings.TrimSpace(w.Databases.Vector.Metric)
		}
		if cfg.Databases.Graph.Backend == "" && strings.TrimSpace(w.Databases.Graph.Backend) != "" {
			cfg.Databases.Graph.Backend = strings.TrimSpace(w.Databases.Graph.Backend)
		}
		if cfg.Databases.Graph.DSN == "" && strings.TrimSpace(w.Databases.Graph.DSN) != "" {
			cfg.Databases.Graph.DSN = strings.TrimSpace(w.Databases.Graph.DSN)
		}
		if cfg.Databases.Chat.Backend == "" && strings.TrimSpace(w.Databases.Chat.Backend) != "" {
			cfg.Databases.Chat.Backend = strings.TrimSpace(w.Databases.Chat.Backend)
		}
		if cfg.Databases.Chat.DSN == "" && strings.TrimSpace(w.Databases.Chat.DSN) != "" {
			cfg.Databases.Chat.DSN = strings.TrimSpace(w.Databases.Chat.DSN)
		}
		// Embedding: only assign fields which are empty to allow env to override
		if cfg.Embedding.BaseURL == "" && strings.TrimSpace(w.Embedding.BaseURL) != "" {
			cfg.Embedding.BaseURL = strings.TrimSpace(w.Embedding.BaseURL)
		}
		if cfg.Embedding.Model == "" && strings.TrimSpace(w.Embedding.Model) != "" {
			cfg.Embedding.Model = strings.TrimSpace(w.Embedding.Model)
		}
		if cfg.Embedding.APIKey == "" && strings.TrimSpace(w.Embedding.APIKey) != "" {
			cfg.Embedding.APIKey = strings.TrimSpace(w.Embedding.APIKey)
		}
		if cfg.Embedding.APIHeader == "" && strings.TrimSpace(w.Embedding.APIHeader) != "" {
			cfg.Embedding.APIHeader = strings.TrimSpace(w.Embedding.APIHeader)
		}
		if cfg.Embedding.Path == "" && strings.TrimSpace(w.Embedding.Path) != "" {
			cfg.Embedding.Path = strings.TrimSpace(w.Embedding.Path)
		}
		if cfg.Embedding.Timeout == 0 && w.Embedding.TimeoutSeconds > 0 {
			cfg.Embedding.Timeout = w.Embedding.TimeoutSeconds
		}
		// EvolvingMemory: assign evolving memory config fields
		if w.EvolvingMemory.Enabled {
			cfg.EvolvingMemory.Enabled = true
		}
		if cfg.EvolvingMemory.MaxSize == 0 && w.EvolvingMemory.MaxSize > 0 {
			cfg.EvolvingMemory.MaxSize = w.EvolvingMemory.MaxSize
		}
		if cfg.EvolvingMemory.TopK == 0 && w.EvolvingMemory.TopK > 0 {
			cfg.EvolvingMemory.TopK = w.EvolvingMemory.TopK
		}
		if cfg.EvolvingMemory.WindowSize == 0 && w.EvolvingMemory.WindowSize > 0 {
			cfg.EvolvingMemory.WindowSize = w.EvolvingMemory.WindowSize
		}
		if w.EvolvingMemory.EnableRAG {
			cfg.EvolvingMemory.EnableRAG = true
		}
		if w.EvolvingMemory.ReMemEnabled {
			cfg.EvolvingMemory.ReMemEnabled = true
		}
		if cfg.EvolvingMemory.MaxInnerSteps == 0 && w.EvolvingMemory.MaxInnerSteps > 0 {
			cfg.EvolvingMemory.MaxInnerSteps = w.EvolvingMemory.MaxInnerSteps
		}
		if cfg.EvolvingMemory.Model == "" && strings.TrimSpace(w.EvolvingMemory.Model) != "" {
			cfg.EvolvingMemory.Model = strings.TrimSpace(w.EvolvingMemory.Model)
		}
		// Smart pruning config from YAML
		if w.EvolvingMemory.EnableSmartPrune {
			cfg.EvolvingMemory.EnableSmartPrune = true
		}
		if cfg.EvolvingMemory.PruneThreshold == 0 && w.EvolvingMemory.PruneThreshold > 0 {
			cfg.EvolvingMemory.PruneThreshold = w.EvolvingMemory.PruneThreshold
		}
		if cfg.EvolvingMemory.RelevanceDecay == 0 && w.EvolvingMemory.RelevanceDecay > 0 {
			cfg.EvolvingMemory.RelevanceDecay = w.EvolvingMemory.RelevanceDecay
		}
		if cfg.EvolvingMemory.MinRelevance == 0 && w.EvolvingMemory.MinRelevance > 0 {
			cfg.EvolvingMemory.MinRelevance = w.EvolvingMemory.MinRelevance
		}
		// MCP servers
		if len(w.MCP.Servers) > 0 {
			cfg.MCP.Servers = make([]MCPServerConfig, 0, len(w.MCP.Servers))
			for _, s := range w.MCP.Servers {
				mcps := MCPServerConfig{
					Name:             strings.TrimSpace(s.Name),
					Command:          strings.TrimSpace(s.Command),
					Args:             append([]string{}, s.Args...),
					Env:              s.Env,
					KeepAliveSeconds: s.KeepAliveSeconds,
					PathDependent:    s.PathDependent,
					URL:              strings.TrimSpace(s.URL),
					Headers:          s.Headers,
					BearerToken:      strings.TrimSpace(s.BearerToken),
					Origin:           strings.TrimSpace(s.Origin),
					ProtocolVersion:  strings.TrimSpace(s.ProtocolVersion),
				}
				mcps.HTTP.TimeoutSeconds = s.HTTP.TimeoutSeconds
				mcps.HTTP.ProxyURL = strings.TrimSpace(s.HTTP.ProxyURL)
				mcps.HTTP.TLS.InsecureSkipVerify = s.HTTP.TLS.InsecureSkipVerify
				mcps.HTTP.TLS.CAFile = strings.TrimSpace(s.HTTP.TLS.CAFile)
				mcps.HTTP.TLS.CertFile = strings.TrimSpace(s.HTTP.TLS.CertFile)
				mcps.HTTP.TLS.KeyFile = strings.TrimSpace(s.HTTP.TLS.KeyFile)
				cfg.MCP.Servers = append(cfg.MCP.Servers, mcps)
			}
		}
		// Global enableTools only if not set via env (env takes precedence)
		if !cfg.EnableTools && w.EnableTools != nil {
			cfg.EnableTools = *w.EnableTools
		}
		// Top-level AllowTools mapping into cfg.ToolAllowList (env takes precedence).
		if len(cfg.ToolAllowList) == 0 && len(w.AllowTools) > 0 {
			cfg.ToolAllowList = append([]string{}, w.AllowTools...)
		}
		// TTS defaults from YAML if not set by env
		if cfg.TTS.BaseURL == "" && strings.TrimSpace(w.TTS.BaseURL) != "" {
			cfg.TTS.BaseURL = strings.TrimSpace(w.TTS.BaseURL)
		}
		if cfg.TTS.Model == "" && strings.TrimSpace(w.TTS.Model) != "" {
			cfg.TTS.Model = strings.TrimSpace(w.TTS.Model)
		}
		if cfg.TTS.Voice == "" && strings.TrimSpace(w.TTS.Voice) != "" {
			cfg.TTS.Voice = strings.TrimSpace(w.TTS.Voice)
		}
		// Auth from YAML (env overrides not yet implemented)
		if w.Auth.Enabled {
			cfg.Auth.Enabled = true
		}
		// Always let YAML override provider (env override not implemented yet)
		if strings.TrimSpace(w.Auth.Provider) != "" {
			cfg.Auth.Provider = strings.TrimSpace(w.Auth.Provider)
		}
		if cfg.Auth.IssuerURL == "" && strings.TrimSpace(w.Auth.IssuerURL) != "" {
			cfg.Auth.IssuerURL = strings.TrimSpace(w.Auth.IssuerURL)
		}
		if cfg.Auth.ClientID == "" && strings.TrimSpace(w.Auth.ClientID) != "" {
			cfg.Auth.ClientID = strings.TrimSpace(w.Auth.ClientID)
		}
		if cfg.Auth.ClientSecret == "" && strings.TrimSpace(w.Auth.ClientSecret) != "" {
			cfg.Auth.ClientSecret = strings.TrimSpace(w.Auth.ClientSecret)
		}
		if cfg.Auth.RedirectURL == "" && strings.TrimSpace(w.Auth.RedirectURL) != "" {
			cfg.Auth.RedirectURL = strings.TrimSpace(w.Auth.RedirectURL)
		}
		if len(cfg.Auth.AllowedDomains) == 0 && len(w.Auth.AllowedDomains) > 0 {
			cfg.Auth.AllowedDomains = append([]string{}, w.Auth.AllowedDomains...)
		}
		if cfg.Auth.CookieName == "" && strings.TrimSpace(w.Auth.CookieName) != "" {
			cfg.Auth.CookieName = strings.TrimSpace(w.Auth.CookieName)
		}
		if !cfg.Auth.CookieSecure && w.Auth.CookieSecure {
			cfg.Auth.CookieSecure = true
		}
		if cfg.Auth.CookieDomain == "" && strings.TrimSpace(w.Auth.CookieDomain) != "" {
			cfg.Auth.CookieDomain = strings.TrimSpace(w.Auth.CookieDomain)
		}
		if cfg.Auth.StateTTLSeconds == 0 && w.Auth.StateTTLSeconds > 0 {
			cfg.Auth.StateTTLSeconds = w.Auth.StateTTLSeconds
		}
		if cfg.Auth.SessionTTLHours == 0 && w.Auth.SessionTTLHours > 0 {
			cfg.Auth.SessionTTLHours = w.Auth.SessionTTLHours
		}
		if cfg.Auth.OAuth2.AuthURL == "" && strings.TrimSpace(w.Auth.OAuth2.AuthURL) != "" {
			cfg.Auth.OAuth2.AuthURL = strings.TrimSpace(w.Auth.OAuth2.AuthURL)
		}
		if cfg.Auth.OAuth2.TokenURL == "" && strings.TrimSpace(w.Auth.OAuth2.TokenURL) != "" {
			cfg.Auth.OAuth2.TokenURL = strings.TrimSpace(w.Auth.OAuth2.TokenURL)
		}
		if cfg.Auth.OAuth2.UserInfoURL == "" && strings.TrimSpace(w.Auth.OAuth2.UserInfoURL) != "" {
			cfg.Auth.OAuth2.UserInfoURL = strings.TrimSpace(w.Auth.OAuth2.UserInfoURL)
		}
		if cfg.Auth.OAuth2.LogoutURL == "" && strings.TrimSpace(w.Auth.OAuth2.LogoutURL) != "" {
			cfg.Auth.OAuth2.LogoutURL = strings.TrimSpace(w.Auth.OAuth2.LogoutURL)
		}
		if cfg.Auth.OAuth2.LogoutRedirectParam == "" && strings.TrimSpace(w.Auth.OAuth2.LogoutRedirectParam) != "" {
			cfg.Auth.OAuth2.LogoutRedirectParam = strings.TrimSpace(w.Auth.OAuth2.LogoutRedirectParam)
		}
		if len(cfg.Auth.OAuth2.Scopes) == 0 && len(w.Auth.OAuth2.Scopes) > 0 {
			cfg.Auth.OAuth2.Scopes = append([]string{}, w.Auth.OAuth2.Scopes...)
		}
		if cfg.Auth.OAuth2.ProviderName == "" && strings.TrimSpace(w.Auth.OAuth2.ProviderName) != "" {
			cfg.Auth.OAuth2.ProviderName = strings.TrimSpace(w.Auth.OAuth2.ProviderName)
		}
		if len(cfg.Auth.OAuth2.DefaultRoles) == 0 && len(w.Auth.OAuth2.DefaultRoles) > 0 {
			cfg.Auth.OAuth2.DefaultRoles = append([]string{}, w.Auth.OAuth2.DefaultRoles...)
		}
		if cfg.Auth.OAuth2.EmailField == "" && strings.TrimSpace(w.Auth.OAuth2.EmailField) != "" {
			cfg.Auth.OAuth2.EmailField = strings.TrimSpace(w.Auth.OAuth2.EmailField)
		}
		if cfg.Auth.OAuth2.NameField == "" && strings.TrimSpace(w.Auth.OAuth2.NameField) != "" {
			cfg.Auth.OAuth2.NameField = strings.TrimSpace(w.Auth.OAuth2.NameField)
		}
		if cfg.Auth.OAuth2.PictureField == "" && strings.TrimSpace(w.Auth.OAuth2.PictureField) != "" {
			cfg.Auth.OAuth2.PictureField = strings.TrimSpace(w.Auth.OAuth2.PictureField)
		}
		if cfg.Auth.OAuth2.SubjectField == "" && strings.TrimSpace(w.Auth.OAuth2.SubjectField) != "" {
			cfg.Auth.OAuth2.SubjectField = strings.TrimSpace(w.Auth.OAuth2.SubjectField)
		}
		if cfg.Auth.OAuth2.RolesField == "" && strings.TrimSpace(w.Auth.OAuth2.RolesField) != "" {
			cfg.Auth.OAuth2.RolesField = strings.TrimSpace(w.Auth.OAuth2.RolesField)
		}
		// Projects: merge YAML config if env not set
		if cfg.Projects.Backend == "" && strings.TrimSpace(w.Projects.Backend) != "" {
			cfg.Projects.Backend = strings.TrimSpace(w.Projects.Backend)
		}
		if w.Projects.Encrypt {
			cfg.Projects.Encrypt = true
		}
		if cfg.Projects.Encryption.Provider == "" && strings.TrimSpace(w.Projects.Encryption.Provider) != "" {
			cfg.Projects.Encryption.Provider = strings.TrimSpace(w.Projects.Encryption.Provider)
		}
		if cfg.Projects.Encryption.File.KeystorePath == "" && strings.TrimSpace(w.Projects.Encryption.File.KeystorePath) != "" {
			cfg.Projects.Encryption.File.KeystorePath = strings.TrimSpace(w.Projects.Encryption.File.KeystorePath)
		}
		if cfg.Projects.Encryption.Vault.Address == "" && strings.TrimSpace(w.Projects.Encryption.Vault.Address) != "" {
			cfg.Projects.Encryption.Vault.Address = strings.TrimSpace(w.Projects.Encryption.Vault.Address)
		}
		if cfg.Projects.Encryption.Vault.Token == "" && strings.TrimSpace(w.Projects.Encryption.Vault.Token) != "" {
			cfg.Projects.Encryption.Vault.Token = strings.TrimSpace(w.Projects.Encryption.Vault.Token)
		}
		if cfg.Projects.Encryption.Vault.KeyName == "" && strings.TrimSpace(w.Projects.Encryption.Vault.KeyName) != "" {
			cfg.Projects.Encryption.Vault.KeyName = strings.TrimSpace(w.Projects.Encryption.Vault.KeyName)
		}
		if cfg.Projects.Encryption.Vault.MountPath == "" && strings.TrimSpace(w.Projects.Encryption.Vault.MountPath) != "" {
			cfg.Projects.Encryption.Vault.MountPath = strings.TrimSpace(w.Projects.Encryption.Vault.MountPath)
		}
		if cfg.Projects.Encryption.Vault.Namespace == "" && strings.TrimSpace(w.Projects.Encryption.Vault.Namespace) != "" {
			cfg.Projects.Encryption.Vault.Namespace = strings.TrimSpace(w.Projects.Encryption.Vault.Namespace)
		}
		if !cfg.Projects.Encryption.Vault.TLSSkipVerify && w.Projects.Encryption.Vault.TLSSkipVerify {
			cfg.Projects.Encryption.Vault.TLSSkipVerify = true
		}
		if cfg.Projects.Encryption.Vault.TimeoutSeconds == 0 && w.Projects.Encryption.Vault.TimeoutSeconds > 0 {
			cfg.Projects.Encryption.Vault.TimeoutSeconds = w.Projects.Encryption.Vault.TimeoutSeconds
		}
		if cfg.Projects.Encryption.AWSKMS.KeyID == "" && strings.TrimSpace(w.Projects.Encryption.AWSKMS.KeyID) != "" {
			cfg.Projects.Encryption.AWSKMS.KeyID = strings.TrimSpace(w.Projects.Encryption.AWSKMS.KeyID)
		}
		if cfg.Projects.Encryption.AWSKMS.Region == "" && strings.TrimSpace(w.Projects.Encryption.AWSKMS.Region) != "" {
			cfg.Projects.Encryption.AWSKMS.Region = strings.TrimSpace(w.Projects.Encryption.AWSKMS.Region)
		}
		if cfg.Projects.Encryption.AWSKMS.AccessKeyID == "" && strings.TrimSpace(w.Projects.Encryption.AWSKMS.AccessKeyID) != "" {
			cfg.Projects.Encryption.AWSKMS.AccessKeyID = strings.TrimSpace(w.Projects.Encryption.AWSKMS.AccessKeyID)
		}
		if cfg.Projects.Encryption.AWSKMS.SecretAccessKey == "" && strings.TrimSpace(w.Projects.Encryption.AWSKMS.SecretAccessKey) != "" {
			cfg.Projects.Encryption.AWSKMS.SecretAccessKey = strings.TrimSpace(w.Projects.Encryption.AWSKMS.SecretAccessKey)
		}
		if cfg.Projects.Encryption.AWSKMS.Endpoint == "" && strings.TrimSpace(w.Projects.Encryption.AWSKMS.Endpoint) != "" {
			cfg.Projects.Encryption.AWSKMS.Endpoint = strings.TrimSpace(w.Projects.Encryption.AWSKMS.Endpoint)
		}
		if cfg.Projects.Workspace.Mode == "" && strings.TrimSpace(w.Projects.Workspace.Mode) != "" {
			cfg.Projects.Workspace.Mode = strings.TrimSpace(w.Projects.Workspace.Mode)
		}
		if cfg.Projects.Workspace.Root == "" && strings.TrimSpace(w.Projects.Workspace.Root) != "" {
			cfg.Projects.Workspace.Root = strings.TrimSpace(w.Projects.Workspace.Root)
		}
		if cfg.Projects.Workspace.TTLSeconds == 0 && w.Projects.Workspace.TTLSeconds > 0 {
			cfg.Projects.Workspace.TTLSeconds = w.Projects.Workspace.TTLSeconds
		}
		if cfg.Projects.Workspace.CacheDir == "" && strings.TrimSpace(w.Projects.Workspace.CacheDir) != "" {
			cfg.Projects.Workspace.CacheDir = strings.TrimSpace(w.Projects.Workspace.CacheDir)
		}
		if cfg.Projects.Workspace.TmpfsDir == "" && strings.TrimSpace(w.Projects.Workspace.TmpfsDir) != "" {
			cfg.Projects.Workspace.TmpfsDir = strings.TrimSpace(w.Projects.Workspace.TmpfsDir)
		}
		if cfg.Projects.S3.Endpoint == "" && strings.TrimSpace(w.Projects.S3.Endpoint) != "" {
			cfg.Projects.S3.Endpoint = strings.TrimSpace(w.Projects.S3.Endpoint)
		}
		if cfg.Projects.S3.Region == "" && strings.TrimSpace(w.Projects.S3.Region) != "" {
			cfg.Projects.S3.Region = strings.TrimSpace(w.Projects.S3.Region)
		}
		if cfg.Projects.S3.Bucket == "" && strings.TrimSpace(w.Projects.S3.Bucket) != "" {
			cfg.Projects.S3.Bucket = strings.TrimSpace(w.Projects.S3.Bucket)
		}
		if cfg.Projects.S3.Prefix == "" && strings.TrimSpace(w.Projects.S3.Prefix) != "" {
			cfg.Projects.S3.Prefix = strings.TrimSpace(w.Projects.S3.Prefix)
		}
		if cfg.Projects.S3.AccessKey == "" && strings.TrimSpace(w.Projects.S3.AccessKey) != "" {
			cfg.Projects.S3.AccessKey = strings.TrimSpace(w.Projects.S3.AccessKey)
		}
		if cfg.Projects.S3.SecretKey == "" && strings.TrimSpace(w.Projects.S3.SecretKey) != "" {
			cfg.Projects.S3.SecretKey = strings.TrimSpace(w.Projects.S3.SecretKey)
		}
		if w.Projects.S3.UsePathStyle {
			cfg.Projects.S3.UsePathStyle = true
		}
		if w.Projects.S3.TLSInsecureSkipVerify {
			cfg.Projects.S3.TLSInsecureSkipVerify = true
		}
		if cfg.Projects.S3.SSE.Mode == "" && strings.TrimSpace(w.Projects.S3.SSE.Mode) != "" {
			cfg.Projects.S3.SSE.Mode = strings.TrimSpace(w.Projects.S3.SSE.Mode)
		}
		if cfg.Projects.S3.SSE.KMSKeyID == "" && strings.TrimSpace(w.Projects.S3.SSE.KMSKeyID) != "" {
			cfg.Projects.S3.SSE.KMSKeyID = strings.TrimSpace(w.Projects.S3.SSE.KMSKeyID)
		}
		if !cfg.Projects.Redis.Enabled && w.Projects.Redis.Enabled {
			cfg.Projects.Redis.Enabled = true
		}
		if cfg.Projects.Redis.Addr == "" && strings.TrimSpace(w.Projects.Redis.Addr) != "" {
			cfg.Projects.Redis.Addr = strings.TrimSpace(w.Projects.Redis.Addr)
		}
		if cfg.Projects.Redis.Password == "" && strings.TrimSpace(w.Projects.Redis.Password) != "" {
			cfg.Projects.Redis.Password = strings.TrimSpace(w.Projects.Redis.Password)
		}
		if cfg.Projects.Redis.DB == 0 && w.Projects.Redis.DB != 0 {
			cfg.Projects.Redis.DB = w.Projects.Redis.DB
		}
		if !cfg.Projects.Redis.TLSInsecureSkipVerify && w.Projects.Redis.TLSInsecureSkipVerify {
			cfg.Projects.Redis.TLSInsecureSkipVerify = true
		}
		if !cfg.Projects.Events.Enabled && w.Projects.Events.Enabled {
			cfg.Projects.Events.Enabled = true
		}
		if cfg.Projects.Events.Brokers == "" && strings.TrimSpace(w.Projects.Events.Brokers) != "" {
			cfg.Projects.Events.Brokers = strings.TrimSpace(w.Projects.Events.Brokers)
		}
		if cfg.Projects.Events.Topic == "" && strings.TrimSpace(w.Projects.Events.Topic) != "" {
			cfg.Projects.Events.Topic = strings.TrimSpace(w.Projects.Events.Topic)
		}
		if cfg.Skills.RedisCacheTTLSeconds == 0 && w.Skills.RedisCacheTTLSeconds > 0 {
			cfg.Skills.RedisCacheTTLSeconds = w.Skills.RedisCacheTTLSeconds
		}
		if w.Skills.UseS3Loader != nil {
			cfg.Skills.UseS3Loader = *w.Skills.UseS3Loader
		}
		if !cfg.Tokenization.Enabled && w.Tokenization.Enabled {
			cfg.Tokenization.Enabled = true
		}
		if cfg.Tokenization.CacheSize == 0 && w.Tokenization.CacheSize > 0 {
			cfg.Tokenization.CacheSize = w.Tokenization.CacheSize
		}
		if cfg.Tokenization.CacheTTLSeconds == 0 && w.Tokenization.CacheTTLSeconds > 0 {
			cfg.Tokenization.CacheTTLSeconds = w.Tokenization.CacheTTLSeconds
		}
		if w.Tokenization.FallbackToHeuristic != nil {
			cfg.Tokenization.FallbackToHeuristic = *w.Tokenization.FallbackToHeuristic
		}
		return nil
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseCommaSeparatedList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func intFromEnv(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := parseInt(v); err == nil {
			return n
		}
	}
	return def
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}
