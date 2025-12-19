package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	// Summary configuration via env
	if v := strings.TrimSpace(os.Getenv("SUMMARY_ENABLED")); v != "" {
		cfg.SummaryEnabled = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_THRESHOLD")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryThreshold = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_KEEP_LAST")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryKeepLast = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_MODE")); v != "" {
		cfg.SummaryMode = v
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_TARGET_UTILIZATION_PCT")); v != "" {
		if n, err := parseInt(v); err == nil {
			// Interpret as percentage 0-100.
			cfg.SummaryTargetUtilizationPct = float64(n) / 100.0
		}
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_MIN_KEEP_LAST_MESSAGES")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryMinKeepLastMessages = n
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
	// Keep LLMClient.OpenAI in sync with the effective OpenAI config.
	cfg.LLMClient.OpenAI = cfg.OpenAI

	// Parse blocklist
	blockStr := strings.TrimSpace(os.Getenv("BLOCK_BINARIES"))
	if blockStr != "" {
		// env overrides any YAML-defined list
		cfg.Exec.BlockBinaries = nil
		parts := strings.Split(blockStr, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
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
		SystemPrompt     string             `yaml:"systemPrompt"`
		Specialists      []SpecialistConfig `yaml:"specialists"`
		Routes           []SpecialistRoute  `yaml:"routes"`
		LLMClient        llmClientYAML      `yaml:"llm_client"`
		OpenAI           openAIYAML         `yaml:"openai"`
		Workdir          string             `yaml:"workdir"`
		OutputTrunc      int                `yaml:"outputTruncateBytes"`
		SummaryEnabled   bool               `yaml:"summaryEnabled"`
		SummaryThreshold int                `yaml:"summaryThreshold"`
		SummaryKeepLast  int                `yaml:"summaryKeepLast"`
		Exec             execYAML           `yaml:"exec"`
		Obs              obsYAML            `yaml:"obs"`
		Web              webYAML            `yaml:"web"`
		Databases        databasesYAML      `yaml:"databases"`
		MCP              mcpYAML            `yaml:"mcp"`
		Embedding        embeddingYAML      `yaml:"embedding"`
		EvolvingMemory   evolvingMemoryYAML `yaml:"evolvingMemory"`
		TTS              ttsYAML            `yaml:"tts"`
		EnableTools      *bool              `yaml:"enableTools"`
		// AllowTools is a top-level allow-list for tools exposed to the main agent.
		// If present, it should map into cfg.ToolAllowList.
		AllowTools []string `yaml:"allowTools"`
		Auth       authYAML `yaml:"auth"`
	}
	var w wrap
	// Expand ${VAR} with environment variables before parsing.
	data = []byte(os.ExpandEnv(string(data)))
	if err := yaml.Unmarshal(data, &w); err == nil {
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
		if cfg.Workdir == "" && strings.TrimSpace(w.Workdir) != "" {
			cfg.Workdir = strings.TrimSpace(w.Workdir)
		}
		// YAML-provided system prompt is applied only if env did not already
		// provide an override.
		if cfg.SystemPrompt == "" && strings.TrimSpace(w.SystemPrompt) != "" {
			cfg.SystemPrompt = w.SystemPrompt
		}

		if cfg.OutputTruncateByte == 0 && w.OutputTrunc > 0 {
			cfg.OutputTruncateByte = w.OutputTrunc
		}
		// Summary config from YAML if not provided via env
		if !cfg.SummaryEnabled && w.SummaryEnabled {
			cfg.SummaryEnabled = true
		}
		if cfg.SummaryThreshold == 0 && w.SummaryThreshold > 0 {
			cfg.SummaryThreshold = w.SummaryThreshold
		}
		if cfg.SummaryKeepLast == 0 && w.SummaryKeepLast > 0 {
			cfg.SummaryKeepLast = w.SummaryKeepLast
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
		// Top-level AllowTools mapping into cfg.ToolAllowList (env would override if implemented)
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
		return nil
	}
	// Fallback: try list at root
	var list []SpecialistConfig
	if err := yaml.Unmarshal(data, &list); err == nil && len(list) > 0 {
		cfg.Specialists = list
		return nil
	}
	return fmt.Errorf("%s: could not parse specialists configuration", chosen)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
