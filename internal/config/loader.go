package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	yaml "gopkg.in/yaml.v3"
)

// Load reads configuration from YAML files, with optional ${VAR} expansion from
// the current environment or a loaded .env file.
func Load() (Config, error) {
	_ = godotenv.Overload()

	cfg := Config{}
	cfg.Tokenization.FallbackToHeuristic = true

	configPath, err := findRequiredFile("config.yaml", "config.yml")
	if err != nil {
		return Config{}, err
	}
	if err := loadMainConfig(configPath, &cfg); err != nil {
		return Config{}, err
	}
	if err := loadExternalConfigs(&cfg); err != nil {
		return Config{}, err
	}

	mergeOpenAIConfig(&cfg.LLMClient.OpenAI, cfg.OpenAI)
	applyDefaults(&cfg)
	applyDerivedConfig(&cfg)
	if err := validateConfig(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func loadMainConfig(path string, cfg *Config) error {
	data, err := readExpandedYAML(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("%s: could not parse configuration: %w", path, err)
	}

	var aliases struct {
		OutputTruncateByte int `yaml:"outputTruncateByte"`
	}
	if err := yaml.Unmarshal(data, &aliases); err != nil {
		return fmt.Errorf("%s: could not parse configuration aliases: %w", path, err)
	}
	if cfg.OutputTruncateByte == 0 && aliases.OutputTruncateByte > 0 {
		cfg.OutputTruncateByte = aliases.OutputTruncateByte
	}

	return nil
}

func loadExternalConfigs(cfg *Config) error {
	if err := loadSpecialistsFile(cfg); err != nil {
		return err
	}
	if err := loadMCPFile(cfg); err != nil {
		return err
	}
	return nil
}

func loadSpecialistsFile(cfg *Config) error {
	path, found, err := findOptionalConfigFile(os.Getenv("SPECIALISTS_CONFIG"), "specialists.yaml", "specialists.yml")
	if err != nil || !found {
		return err
	}

	data, err := readExpandedYAML(path)
	if err != nil {
		return err
	}

	type specialistFile struct {
		Specialists []SpecialistConfig `yaml:"specialists"`
		Routes      []SpecialistRoute  `yaml:"routes"`
	}

	var wrapped specialistFile
	if err := yaml.Unmarshal(data, &wrapped); err == nil && (len(wrapped.Specialists) > 0 || len(wrapped.Routes) > 0) {
		if len(wrapped.Specialists) > 0 {
			cfg.Specialists = wrapped.Specialists
		}
		if len(wrapped.Routes) > 0 {
			cfg.SpecialistRoutes = wrapped.Routes
		}
		return nil
	}

	var list []SpecialistConfig
	if err := yaml.Unmarshal(data, &list); err == nil {
		cfg.Specialists = list
		return nil
	}

	return fmt.Errorf("%s: could not parse specialists configuration", path)
}

func loadMCPFile(cfg *Config) error {
	path, found, err := findOptionalConfigFile(os.Getenv("MCP_CONFIG"), "mcp.yaml", "mcp.yml")
	if err != nil || !found {
		return err
	}

	data, err := readExpandedYAML(path)
	if err != nil {
		return err
	}

	var wrapped struct {
		Servers []MCPServerConfig `yaml:"servers"`
		MCP     MCPConfig         `yaml:"mcp"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return fmt.Errorf("%s: could not parse MCP configuration: %w", path, err)
	}

	if len(wrapped.Servers) > 0 {
		cfg.MCP.Servers = wrapped.Servers
		return nil
	}
	if len(wrapped.MCP.Servers) > 0 {
		cfg.MCP = wrapped.MCP
	}
	return nil
}

func readExpandedYAML(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return []byte(os.ExpandEnv(string(data))), nil
}

func findRequiredFile(paths ...string) (string, error) {
	path, found, err := findFirstFile(paths...)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("no configuration file found; expected one of: %s", strings.Join(paths, ", "))
	}
	return path, nil
}

func findOptionalConfigFile(override string, defaults ...string) (string, bool, error) {
	candidates := defaults
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		candidates = append([]string{trimmed}, defaults...)
	}
	return findFirstFile(candidates...)
}

func findFirstFile(paths ...string) (string, bool, error) {
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		info, err := os.Stat(trimmed)
		if err == nil {
			if info.IsDir() {
				return "", false, fmt.Errorf("configuration path must be a file: %s", trimmed)
			}
			return trimmed, true, nil
		}
		if os.IsNotExist(err) {
			continue
		}
		return "", false, fmt.Errorf("stat %s: %w", trimmed, err)
	}
	return "", false, nil
}

func applyDefaults(cfg *Config) {
	if cfg.LLMClient.Provider == "" {
		cfg.LLMClient.Provider = "openai"
	}
	if cfg.LLMClient.OpenAI.BaseURL == "" {
		cfg.LLMClient.OpenAI.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.LLMClient.OpenAI.Model == "" {
		cfg.LLMClient.OpenAI.Model = "gpt-4o-mini"
	}
	if cfg.LLMClient.OpenAI.API == "" {
		cfg.LLMClient.OpenAI.API = "completions"
	}
	applySummaryDefaults(cfg)
	if cfg.Obs.ServiceName == "" {
		cfg.Obs.ServiceName = "manifold"
	}
	if cfg.Obs.Environment == "" {
		cfg.Obs.Environment = "dev"
	}
	if cfg.Obs.Local.MetricsWindowMinutes <= 0 {
		cfg.Obs.Local.MetricsWindowMinutes = 60
	}
	if cfg.Obs.Local.MetricsBucketSeconds <= 0 {
		cfg.Obs.Local.MetricsBucketSeconds = 30
	}
	if cfg.Obs.Local.MaxLogs <= 0 {
		cfg.Obs.Local.MaxLogs = 5000
	}
	if cfg.Obs.Local.MaxTraces <= 0 {
		cfg.Obs.Local.MaxTraces = 1000
	}
	if cfg.Obs.Local.MaxSpansPerTrace <= 0 {
		cfg.Obs.Local.MaxSpansPerTrace = 256
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
	if cfg.Exec.MaxCommandSeconds <= 0 {
		cfg.Exec.MaxCommandSeconds = 30
	}
	if cfg.OutputTruncateByte <= 0 {
		cfg.OutputTruncateByte = 64 * 1024
	}
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 8
	}
	if cfg.MaxDiscoveredTools <= 0 {
		cfg.MaxDiscoveredTools = 20
	}
	if cfg.CodeQA.ArtifactDir == "" {
		cfg.CodeQA.ArtifactDir = "codeqa-artifacts"
	}
	if cfg.CodeQA.MaxConcurrentRuns <= 0 {
		cfg.CodeQA.MaxConcurrentRuns = 1
	}
	if cfg.CodeQA.MaxGateParallelism <= 0 {
		cfg.CodeQA.MaxGateParallelism = 2
	}
	if cfg.CodeQA.MaxJudgeParallelism <= 0 {
		cfg.CodeQA.MaxJudgeParallelism = 2
	}
	if cfg.CodeQA.DefaultMaxDiffBytes <= 0 {
		cfg.CodeQA.DefaultMaxDiffBytes = 128 * 1024
	}
	if cfg.CodeQA.DefaultMaxChangedFiles <= 0 {
		cfg.CodeQA.DefaultMaxChangedFiles = 12
	}
	if cfg.CodeQA.AcceptThreshold == 0 {
		cfg.CodeQA.AcceptThreshold = 0.10
	}
	if cfg.CodeQA.MinConfidence == 0 {
		cfg.CodeQA.MinConfidence = 0.70
	}
	if cfg.BeliefMemory.MaxBeliefsPerPrompt <= 0 {
		cfg.BeliefMemory.MaxBeliefsPerPrompt = 5
	}
	if cfg.BeliefMemory.MaxEvidencePerBelief <= 0 {
		cfg.BeliefMemory.MaxEvidencePerBelief = 3
	}
	if cfg.BeliefMemory.DefaultConfidence == 0 {
		cfg.BeliefMemory.DefaultConfidence = 0.50
	}
	if cfg.BeliefMemory.PromotionThreshold == 0 {
		cfg.BeliefMemory.PromotionThreshold = 0.80
	}
	if cfg.BeliefMemory.MaxRAGEvidencePerPrompt <= 0 {
		cfg.BeliefMemory.MaxRAGEvidencePerPrompt = 3
	}
	if cfg.BeliefMemory.RAGRetrievalK <= 0 {
		cfg.BeliefMemory.RAGRetrievalK = 8
	}
	if cfg.BeliefMemory.RAGMinScore < 0 {
		cfg.BeliefMemory.RAGMinScore = 0
	}
	if strings.TrimSpace(cfg.CodeQA.JudgeModel) == "" {
		cfg.CodeQA.JudgeModel = cfg.LLMClient.OpenAI.Model
	}
	if strings.TrimSpace(cfg.CodeQA.ProposerModel) == "" {
		cfg.CodeQA.ProposerModel = cfg.CodeQA.JudgeModel
	}
	if len(cfg.CodeQA.AllowedCommands) == 0 {
		cfg.CodeQA.AllowedCommands = []string{"go", "gofmt", "ruff", "pytest", "python", "prettier", "eslint", "tsc", "npm", "npx", "stylelint", "html-validate", "cargo", "rustfmt"}
	}
	if len(cfg.CodeQA.HighRiskGlobs) == 0 {
		cfg.CodeQA.HighRiskGlobs = []string{"**/auth/**", "**/migrations/**", "**/*crypto*", "**/deploy/**", "**/.github/**"}
	}
	if len(cfg.CodeQA.ForbiddenGlobs) == 0 {
		cfg.CodeQA.ForbiddenGlobs = []string{"**/*.pem", "**/*.key", "**/.env*", "**/node_modules/**", "**/dist/**"}
	}
	if cfg.AgentRunTimeoutSeconds < 0 {
		cfg.AgentRunTimeoutSeconds = 0
	}
	if cfg.StreamRunTimeoutSeconds < 0 {
		cfg.StreamRunTimeoutSeconds = 0
	}
	if cfg.WorkflowTimeoutSeconds < 0 {
		cfg.WorkflowTimeoutSeconds = 0
	}
	if cfg.Tokenization.CacheSize <= 0 {
		cfg.Tokenization.CacheSize = 1000
	}
	if cfg.Tokenization.CacheTTLSeconds <= 0 {
		cfg.Tokenization.CacheTTLSeconds = 3600
	}
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
	if cfg.Embedding.Timeout <= 0 {
		cfg.Embedding.Timeout = 30
	}
	if cfg.Embedding.Instructions.Mode == "" {
		cfg.Embedding.Instructions.Mode = "auto"
	} else {
		cfg.Embedding.Instructions.Mode = strings.ToLower(strings.TrimSpace(cfg.Embedding.Instructions.Mode))
	}
	if cfg.Embedding.Instructions.Format == "" {
		cfg.Embedding.Instructions.Format = "qwen"
	} else {
		cfg.Embedding.Instructions.Format = strings.ToLower(strings.TrimSpace(cfg.Embedding.Instructions.Format))
	}
	for i := range cfg.MCP.Servers {
		if cfg.MCP.Servers[i].HTTP.TimeoutSeconds <= 0 {
			cfg.MCP.Servers[i].HTTP.TimeoutSeconds = 30
		}
	}
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
	if cfg.Transit.DefaultSearchLimit <= 0 {
		cfg.Transit.DefaultSearchLimit = 10
	}
	if cfg.Transit.DefaultListLimit <= 0 {
		cfg.Transit.DefaultListLimit = 100
	}
	if cfg.Transit.MaxBatchSize <= 0 {
		cfg.Transit.MaxBatchSize = 100
	}
}

func applyDerivedConfig(cfg *Config) {
	cfg.LLMClient.Provider = strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider))
	cfg.Summary.LLMClient.Provider = strings.ToLower(strings.TrimSpace(cfg.Summary.LLMClient.Provider))
	cfg.EvolvingMemory.Provider = strings.ToLower(strings.TrimSpace(cfg.EvolvingMemory.Provider))
	cfg.EvolvingMemory.LLMClient.Provider = strings.ToLower(strings.TrimSpace(cfg.EvolvingMemory.LLMClient.Provider))

	if cfg.LLMClient.Provider == "local" {
		cfg.LLMClient.OpenAI.API = "completions"
	}
	if cfg.Summary.LLMClient.Provider == "local" {
		cfg.Summary.LLMClient.OpenAI.API = "completions"
	}
	cfg.OpenAI = cfg.LLMClient.OpenAI

	for i := range cfg.Specialists {
		if strings.TrimSpace(cfg.Specialists[i].Provider) == "" {
			cfg.Specialists[i].Provider = cfg.LLMClient.Provider
		}
	}
}

func validateConfig(cfg *Config) error {
	if err := validateProvider("llm_client.provider", cfg.LLMClient.Provider); err != nil {
		return err
	}
	if cfg.EvolvingMemory.Provider != "" {
		if err := validateProvider("evolvingMemory.provider", cfg.EvolvingMemory.Provider); err != nil {
			return err
		}
	}
	if cfg.EvolvingMemory.LLMClient.Provider != "" {
		if err := validateProvider("evolvingMemory.llmClient.provider", cfg.EvolvingMemory.LLMClient.Provider); err != nil {
			return err
		}
	}
	if cfg.Summary.Enabled {
		if err := validateProvider("summary.llm_client.provider", cfg.Summary.LLMClient.Provider); err != nil {
			return err
		}
	}

	switch cfg.LLMClient.Provider {
	case "openai":
		if strings.TrimSpace(cfg.LLMClient.OpenAI.APIKey) == "" {
			return errors.New("llm_client.openai.apiKey is required")
		}
	case "anthropic":
		if strings.TrimSpace(cfg.LLMClient.Anthropic.APIKey) == "" {
			return errors.New("llm_client.anthropic.apiKey is required")
		}
	case "google":
		if strings.TrimSpace(cfg.LLMClient.Google.APIKey) == "" {
			return errors.New("llm_client.google.apiKey is required")
		}
	}

	if strings.TrimSpace(cfg.Workdir) == "" {
		return errors.New("workdir is required")
	}
	absWD, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return fmt.Errorf("resolve workdir: %w", err)
	}
	info, err := os.Stat(absWD)
	if err != nil {
		return fmt.Errorf("stat workdir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workdir must be a directory: %s", absWD)
	}
	cfg.Workdir = absWD

	for _, binary := range cfg.Exec.BlockBinaries {
		if strings.Contains(binary, "/") || strings.Contains(binary, "\\") {
			return fmt.Errorf("exec.blockBinaries must contain bare binary names only (no paths): %q", binary)
		}
	}
	if cfg.BeliefMemory.DefaultConfidence < 0 || cfg.BeliefMemory.DefaultConfidence > 1 {
		return fmt.Errorf("beliefMemory.defaultConfidence must be between 0 and 1 (got %g)", cfg.BeliefMemory.DefaultConfidence)
	}
	if cfg.BeliefMemory.PromotionThreshold < 0 || cfg.BeliefMemory.PromotionThreshold > 1 {
		return fmt.Errorf("beliefMemory.promotionThreshold must be between 0 and 1 (got %g)", cfg.BeliefMemory.PromotionThreshold)
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Embedding.Instructions.Mode)) {
	case "", "auto", "enabled", "disabled":
	default:
		return fmt.Errorf("embedding.instructions.mode must be one of auto, enabled, or disabled (got %q)", cfg.Embedding.Instructions.Mode)
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Embedding.Instructions.Format)) {
	case "", "qwen":
	default:
		return fmt.Errorf("embedding.instructions.format must be qwen (got %q)", cfg.Embedding.Instructions.Format)
	}

	return nil
}

func validateProvider(path, provider string) error {
	switch provider {
	case "openai", "anthropic", "google", "local":
		return nil
	default:
		return fmt.Errorf("%s must be one of openai, anthropic, google, or local (got %q)", path, provider)
	}
}

func mergeOpenAIConfig(dst *OpenAIConfig, src OpenAIConfig) {
	if dst.APIKey == "" {
		dst.APIKey = src.APIKey
	}
	if dst.Model == "" {
		dst.Model = src.Model
	}
	if dst.BaseURL == "" {
		dst.BaseURL = src.BaseURL
	}
	if dst.SummaryModel == "" {
		dst.SummaryModel = src.SummaryModel
	}
	if dst.SummaryBaseURL == "" {
		dst.SummaryBaseURL = src.SummaryBaseURL
	}
	if dst.API == "" {
		dst.API = src.API
	}
	if len(dst.ExtraHeaders) == 0 && len(src.ExtraHeaders) > 0 {
		dst.ExtraHeaders = src.ExtraHeaders
	}
	if len(dst.ExtraParams) == 0 && len(src.ExtraParams) > 0 {
		dst.ExtraParams = src.ExtraParams
	}
	if !dst.LogPayloads && src.LogPayloads {
		dst.LogPayloads = true
	}
}

func applySummaryDefaults(cfg *Config) {
	if cfg.Summary.Enabled || cfg.SummaryEnabled {
		cfg.Summary.Enabled = true
		cfg.SummaryEnabled = true
	}
	if cfg.Summary.ContextWindowTokens == 0 {
		cfg.Summary.ContextWindowTokens = cfg.SummaryContextWindowTokens
	}
	if cfg.SummaryContextWindowTokens == 0 {
		cfg.SummaryContextWindowTokens = cfg.Summary.ContextWindowTokens
	}
	if cfg.Summary.PlainTextContextWindowTokens == 0 {
		cfg.Summary.PlainTextContextWindowTokens = cfg.SummaryPlainTextContextWindowTokens
	}
	if cfg.SummaryPlainTextContextWindowTokens == 0 {
		cfg.SummaryPlainTextContextWindowTokens = cfg.Summary.PlainTextContextWindowTokens
	}
	if cfg.Summary.ReserveBufferTokens == 0 {
		cfg.Summary.ReserveBufferTokens = cfg.SummaryReserveBufferTokens
	}
	if cfg.SummaryReserveBufferTokens == 0 {
		cfg.SummaryReserveBufferTokens = cfg.Summary.ReserveBufferTokens
	}
	if cfg.Summary.MinKeepLastMessages == 0 {
		cfg.Summary.MinKeepLastMessages = cfg.SummaryMinKeepLastMessages
	}
	if cfg.SummaryMinKeepLastMessages == 0 {
		cfg.SummaryMinKeepLastMessages = cfg.Summary.MinKeepLastMessages
	}
	if cfg.Summary.MaxKeepLastMessages == 0 {
		cfg.Summary.MaxKeepLastMessages = cfg.SummaryMaxKeepLastMessages
	}
	if cfg.SummaryMaxKeepLastMessages == 0 {
		cfg.SummaryMaxKeepLastMessages = cfg.Summary.MaxKeepLastMessages
	}
	if cfg.Summary.MaxSummaryChunkTokens == 0 {
		cfg.Summary.MaxSummaryChunkTokens = cfg.SummaryMaxSummaryChunkTokens
	}
	if cfg.SummaryMaxSummaryChunkTokens == 0 {
		cfg.SummaryMaxSummaryChunkTokens = cfg.Summary.MaxSummaryChunkTokens
	}

	if cfg.Summary.LLMClient.Provider == "" {
		cfg.Summary.LLMClient.Provider = cfg.LLMClient.Provider
	}
	if cfg.Summary.LLMClient.OpenAI.APIKey == "" {
		cfg.Summary.LLMClient.OpenAI.APIKey = cfg.LLMClient.OpenAI.APIKey
	}
	if cfg.Summary.LLMClient.OpenAI.Model == "" {
		cfg.Summary.LLMClient.OpenAI.Model = firstNonEmpty(cfg.LLMClient.OpenAI.SummaryModel, cfg.OpenAI.SummaryModel, cfg.LLMClient.OpenAI.Model)
	}
	if cfg.Summary.LLMClient.OpenAI.BaseURL == "" {
		cfg.Summary.LLMClient.OpenAI.BaseURL = firstNonEmpty(cfg.LLMClient.OpenAI.SummaryBaseURL, cfg.OpenAI.SummaryBaseURL, cfg.LLMClient.OpenAI.BaseURL)
	}
	if cfg.Summary.LLMClient.OpenAI.API == "" {
		cfg.Summary.LLMClient.OpenAI.API = cfg.LLMClient.OpenAI.API
		if cfg.Summary.LLMClient.OpenAI.API == "" {
			cfg.Summary.LLMClient.OpenAI.API = "completions"
		}
	}
	if len(cfg.Summary.LLMClient.OpenAI.ExtraHeaders) == 0 && len(cfg.LLMClient.OpenAI.ExtraHeaders) > 0 {
		cfg.Summary.LLMClient.OpenAI.ExtraHeaders = cfg.LLMClient.OpenAI.ExtraHeaders
	}
	if len(cfg.Summary.LLMClient.OpenAI.ExtraParams) == 0 && len(cfg.LLMClient.OpenAI.ExtraParams) > 0 {
		cfg.Summary.LLMClient.OpenAI.ExtraParams = cfg.LLMClient.OpenAI.ExtraParams
	}
	if !cfg.Summary.LLMClient.OpenAI.LogPayloads && cfg.LLMClient.OpenAI.LogPayloads {
		cfg.Summary.LLMClient.OpenAI.LogPayloads = true
	}
	if cfg.Summary.LLMClient.Anthropic.APIKey == "" {
		cfg.Summary.LLMClient.Anthropic.APIKey = cfg.LLMClient.Anthropic.APIKey
	}
	if cfg.Summary.LLMClient.Anthropic.Model == "" {
		cfg.Summary.LLMClient.Anthropic.Model = cfg.LLMClient.Anthropic.Model
	}
	if cfg.Summary.LLMClient.Anthropic.BaseURL == "" {
		cfg.Summary.LLMClient.Anthropic.BaseURL = cfg.LLMClient.Anthropic.BaseURL
	}
	if cfg.Summary.LLMClient.Anthropic.MaxTokens == 0 {
		cfg.Summary.LLMClient.Anthropic.MaxTokens = cfg.LLMClient.Anthropic.MaxTokens
	}
	if len(cfg.Summary.LLMClient.Anthropic.ExtraParams) == 0 && len(cfg.LLMClient.Anthropic.ExtraParams) > 0 {
		cfg.Summary.LLMClient.Anthropic.ExtraParams = cfg.LLMClient.Anthropic.ExtraParams
	}
	if cfg.Summary.LLMClient.Google.APIKey == "" {
		cfg.Summary.LLMClient.Google.APIKey = cfg.LLMClient.Google.APIKey
	}
	if cfg.Summary.LLMClient.Google.Model == "" {
		cfg.Summary.LLMClient.Google.Model = cfg.LLMClient.Google.Model
	}
	if cfg.Summary.LLMClient.Google.BaseURL == "" {
		cfg.Summary.LLMClient.Google.BaseURL = cfg.LLMClient.Google.BaseURL
	}
	if cfg.Summary.LLMClient.Google.Timeout == 0 {
		cfg.Summary.LLMClient.Google.Timeout = cfg.LLMClient.Google.Timeout
	}
	if len(cfg.Summary.LLMClient.Google.ExtraParams) == 0 && len(cfg.LLMClient.Google.ExtraParams) > 0 {
		cfg.Summary.LLMClient.Google.ExtraParams = cfg.LLMClient.Google.ExtraParams
	}

	cfg.LLMClient.OpenAI.SummaryModel = cfg.Summary.LLMClient.OpenAI.Model
	cfg.LLMClient.OpenAI.SummaryBaseURL = cfg.Summary.LLMClient.OpenAI.BaseURL
	cfg.OpenAI.SummaryModel = cfg.Summary.LLMClient.OpenAI.Model
	cfg.OpenAI.SummaryBaseURL = cfg.Summary.LLMClient.OpenAI.BaseURL
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

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}
