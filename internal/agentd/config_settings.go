package agentd

import (
	"fmt"
	"strings"

	"manifold/internal/config"
)

func currentAgentdSettings(cfg *config.Config) agentdSettings {
	settings := currentSummaryAgentdSettings(cfg)
	projectPrimaryLLMAgentdSettings(&settings, cfg)
	projectPromptAgentdSettings(&settings, cfg)
	projectEmbeddingAgentdSettings(&settings, cfg)
	projectRerankAgentdSettings(&settings, cfg)
	projectRuntimeAgentdSettings(&settings, cfg)
	projectOpsAgentdSettings(&settings, cfg)
	projectDatabaseAgentdSettings(&settings, cfg)
	return settings
}

func projectPrimaryLLMAgentdSettings(settings *agentdSettings, cfg *config.Config) {
	settings.LLMProvider = strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider))
	settings.LLMModel = resolveLLMClientModel(cfg.LLMClient)
	switch settings.LLMProvider {
	case "anthropic":
		settings.LLMAPIKey = cfg.LLMClient.Anthropic.APIKey
		settings.LLMBaseURL = cfg.LLMClient.Anthropic.BaseURL
	case "google":
		settings.LLMAPIKey = cfg.LLMClient.Google.APIKey
		settings.LLMBaseURL = cfg.LLMClient.Google.BaseURL
	default:
		settings.LLMAPIKey = cfg.LLMClient.OpenAI.APIKey
		settings.LLMBaseURL = cfg.LLMClient.OpenAI.BaseURL
	}
	settings.MemoryEnabled = cfg.Memory.Enabled
}

func currentSummaryAgentdSettings(cfg *config.Config) agentdSettings {
	contextWindow := firstPositiveInt(cfg.Summary.ContextWindowTokens, cfg.SummaryContextWindowTokens)
	reserveTokens := firstPositiveInt(cfg.Summary.ReserveBufferTokens, cfg.SummaryReserveBufferTokens)
	return agentdSettings{
		OpenAISummaryModel:                  cfg.Summary.LLMClient.OpenAI.Model,
		OpenAISummaryURL:                    cfg.Summary.LLMClient.OpenAI.BaseURL,
		SummaryProvider:                     cfg.Summary.LLMClient.Provider,
		SummaryModel:                        resolveLLMClientModel(cfg.Summary.LLMClient),
		SummaryURL:                          cfg.Summary.LLMClient.OpenAI.BaseURL,
		SummaryEnabled:                      cfg.Summary.Enabled || cfg.SummaryEnabled,
		SummaryContextWindowTokens:          contextWindow,
		SummaryPlainTextContextWindowTokens: firstPositiveInt(cfg.Summary.PlainTextContextWindowTokens, cfg.SummaryPlainTextContextWindowTokens),
		SummaryReserveBufferTokens:          reserveTokens,
		SummaryMinKeepLastMessages:          firstPositiveInt(cfg.Summary.MinKeepLastMessages, cfg.SummaryMinKeepLastMessages),
		SummaryMaxKeepLastMessages:          firstPositiveInt(cfg.Summary.MaxKeepLastMessages, cfg.SummaryMaxKeepLastMessages),
		SummaryMaxSummaryChunkTokens:        firstPositiveInt(cfg.Summary.MaxSummaryChunkTokens, cfg.SummaryMaxSummaryChunkTokens),
		SummaryCallTimeoutSeconds:           cfg.Summary.CallTimeoutSeconds,
		SummaryTokenBudget:                  effectiveSummaryTokenBudget(contextWindow, reserveTokens),
		RequestInfoEnabled:                  config.RequestInfoEnabled(cfg.RequestInfoEnabled),
	}
}

func projectPromptAgentdSettings(settings *agentdSettings, cfg *config.Config) {
	settings.PromptBaseSystem = cfg.PromptOverrides.BaseSystem
	settings.PromptMemoryInstructions = cfg.PromptOverrides.MemoryInstructions
	settings.PromptToolDiscoveryInstructions = cfg.PromptOverrides.ToolDiscoveryInstructions
	settings.PromptSkillDiscoveryInstructions = cfg.PromptOverrides.SkillDiscoveryInstructions
}

func projectEmbeddingAgentdSettings(settings *agentdSettings, cfg *config.Config) {
	settings.EmbedBaseURL = cfg.Embedding.BaseURL
	settings.EmbedModel = cfg.Embedding.Model
	settings.EmbedAPIKey = cfg.Embedding.APIKey
	settings.EmbedAPIHeader = cfg.Embedding.APIHeader
	settings.EmbedAPIHeaders = cfg.Embedding.Headers
	settings.EmbedPath = cfg.Embedding.Path
	settings.EmbedInstructionMode = cfg.Embedding.Instructions.Mode
	settings.EmbedInstructionFormat = cfg.Embedding.Instructions.Format
	settings.EmbedDefaultQueryInstruction = cfg.Embedding.Instructions.DefaultQuery
	settings.EmbedRAGQueryInstruction = cfg.Embedding.Instructions.RAGQuery
	settings.EmbedEvolvingMemoryQueryInstruction = cfg.Embedding.Instructions.EvolvingMemoryQuery
	settings.EmbedTransitQueryInstruction = cfg.Embedding.Instructions.TransitQuery
}

func projectRerankAgentdSettings(settings *agentdSettings, cfg *config.Config) {
	settings.RerankEnabled = cfg.Reranking.Enabled
	settings.RerankBaseURL = cfg.Reranking.BaseURL
	settings.RerankModel = cfg.Reranking.Model
	settings.RerankInstruction = cfg.Reranking.Instruction
	settings.RerankAPIKey = cfg.Reranking.APIKey
	settings.RerankAPIHeader = cfg.Reranking.APIHeader
	settings.RerankAPIHeaders = cfg.Reranking.Headers
	settings.RerankPath = cfg.Reranking.Path
}

func projectRuntimeAgentdSettings(settings *agentdSettings, cfg *config.Config) {
	settings.AgentRunTimeoutSeconds = cfg.AgentRunTimeoutSeconds
	settings.StreamRunTimeoutSeconds = cfg.StreamRunTimeoutSeconds
	settings.WorkflowTimeoutSeconds = cfg.WorkflowTimeoutSeconds
	settings.BlockBinaries = strings.Join(cfg.Exec.BlockBinaries, ",")
	settings.CommandRules = append([]config.ExecCommandRule(nil), cfg.Exec.CommandRules...)
	settings.SandboxEnabled = cfg.Exec.Sandbox.Enabled
	settings.SandboxFailIfUnavailable = cfg.Exec.Sandbox.FailIfUnavailable
	settings.SandboxNetworkEnabled = cfg.Exec.Sandbox.Network.Enabled
	settings.SandboxNetworkAllowedDomains = append([]string(nil), cfg.Exec.Sandbox.Network.AllowedDomains...)
	settings.MaxCommandSeconds = cfg.Exec.MaxCommandSeconds
	settings.OutputTruncateBytes = cfg.OutputTruncateByte
	settings.MaxTerminalSessions = cfg.Exec.MaxTerminalSessions
	settings.MaxTerminalRuntimeSeconds = cfg.Exec.MaxTerminalRuntimeSeconds
	settings.TerminalIdleTTLSeconds = cfg.Exec.TerminalIdleTTLSeconds
	settings.TerminalOutputBufferBytes = cfg.Exec.TerminalOutputBufferBytes
}

func projectOpsAgentdSettings(settings *agentdSettings, cfg *config.Config) {
	settings.OTELServiceName = cfg.Obs.ServiceName
	settings.ServiceVersion = cfg.Obs.ServiceVersion
	settings.Environment = cfg.Obs.Environment
	settings.OTLPEndpoint = cfg.Obs.OTLP
	settings.LogPath = cfg.LogPath
	settings.LogLevel = cfg.LogLevel
	settings.LogPayloads = cfg.LogPayloads
	settings.LogRawPrompts = cfg.LogRawPrompts
	settings.SearXNGURL = cfg.Web.SearXNGURL
	settings.WebSearXNGURL = cfg.Web.SearXNGURL
}

func projectDatabaseAgentdSettings(settings *agentdSettings, cfg *config.Config) {
	settings.DatabaseURL = cfg.Databases.DefaultDSN
	settings.DBURL = cfg.Databases.DefaultDSN
	settings.PostgresDSN = cfg.Databases.DefaultDSN
	settings.SearchBackend = cfg.Databases.Search.Backend
	settings.SearchDSN = cfg.Databases.Search.DSN
	settings.SearchIndex = cfg.Databases.Search.Index
	settings.VectorBackend = cfg.Databases.Vector.Backend
	settings.VectorDSN = cfg.Databases.Vector.DSN
	settings.VectorIndex = cfg.Databases.Vector.Index
	settings.VectorDims = cfg.Databases.Vector.Dimensions
	settings.VectorMetric = cfg.Databases.Vector.Metric
	settings.GraphBackend = cfg.Databases.Graph.Backend
	settings.GraphDSN = cfg.Databases.Graph.DSN
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func effectiveSummaryTokenBudget(contextWindow, reserveTokens int) int {
	if contextWindow <= 0 {
		contextWindow = 32_000
	}
	if reserveTokens <= 0 {
		reserveTokens = 25_000
	}
	budget := contextWindow - reserveTokens
	if budget <= 0 {
		return contextWindow / 2
	}
	return budget
}

func normalizeAgentdSettings(settings agentdSettings) agentdSettings {
	webURL := firstNonEmptyTrimmed(settings.WebSearXNGURL, settings.SearXNGURL)
	if webURL != "" {
		settings.WebSearXNGURL = webURL
		settings.SearXNGURL = webURL
	}

	defaultDSN := firstNonEmptyTrimmed(settings.PostgresDSN, settings.DBURL, settings.DatabaseURL)
	if defaultDSN != "" {
		settings.PostgresDSN = defaultDSN
		settings.DBURL = defaultDSN
		settings.DatabaseURL = defaultDSN
	}

	settings.BlockBinaries = strings.TrimSpace(settings.BlockBinaries)
	if strings.TrimSpace(settings.EmbedInstructionMode) == "" {
		settings.EmbedInstructionMode = "auto"
	} else {
		settings.EmbedInstructionMode = strings.ToLower(strings.TrimSpace(settings.EmbedInstructionMode))
	}
	if strings.TrimSpace(settings.EmbedInstructionFormat) == "" {
		settings.EmbedInstructionFormat = "qwen"
	} else {
		settings.EmbedInstructionFormat = strings.ToLower(strings.TrimSpace(settings.EmbedInstructionFormat))
	}
	if strings.TrimSpace(settings.RerankAPIHeader) == "" {
		settings.RerankAPIHeader = "Authorization"
	}
	if strings.TrimSpace(settings.RerankPath) == "" {
		settings.RerankPath = "/v1/rerank"
	}

	return settings
}

func applySummaryModel(cfg *config.Config, model string) {
	providerName := strings.ToLower(strings.TrimSpace(cfg.Summary.LLMClient.Provider))
	switch providerName {
	case "anthropic":
		cfg.Summary.LLMClient.Anthropic.Model = model
	case "google":
		cfg.Summary.LLMClient.Google.Model = model
	default:
		cfg.Summary.LLMClient.OpenAI.Model = model
	}
}

func applyAgentdSettings(cfg *config.Config, settings agentdSettings) error {
	settings = normalizeAgentdSettings(settings)
	if err := validateEmbeddingInstructionSettings(settings); err != nil {
		return err
	}
	if settings.RerankEnabled && strings.TrimSpace(settings.RerankBaseURL) == "" {
		return fmt.Errorf("rerankBaseUrl is required when rerankEnabled is true")
	}

	if err := applyPrimaryLLMSettings(cfg, settings); err != nil {
		return err
	}
	applySummarySettings(cfg, settings)
	applyRequestInfoSettings(cfg, settings)
	applyPromptOverrideSettings(cfg, settings)
	applyEmbeddingSettings(cfg, settings)
	applyRerankSettings(cfg, settings)
	applyTimeoutSettings(cfg, settings)
	if err := applyExecSettings(cfg, settings); err != nil {
		return err
	}
	applyObservabilitySettings(cfg, settings)
	applyLogSettings(cfg, settings)
	applyWebSettings(cfg, settings)
	applyDatabaseSettings(cfg, settings)
	return nil
}

func applyRequestInfoSettings(cfg *config.Config, settings agentdSettings) {
	cfg.RequestInfoEnabled = boolPtr(settings.RequestInfoEnabled)
}

func applyPromptOverrideSettings(cfg *config.Config, settings agentdSettings) {
	cfg.PromptOverrides.BaseSystem = settings.PromptBaseSystem
	cfg.PromptOverrides.MemoryInstructions = settings.PromptMemoryInstructions
	cfg.PromptOverrides.ToolDiscoveryInstructions = settings.PromptToolDiscoveryInstructions
	cfg.PromptOverrides.SkillDiscoveryInstructions = settings.PromptSkillDiscoveryInstructions
}

func applySummarySettings(cfg *config.Config, settings agentdSettings) {
	if settings.SummaryProvider != "" {
		cfg.Summary.LLMClient.Provider = settings.SummaryProvider
	}
	if settings.SummaryModel != "" {
		applySummaryModel(cfg, settings.SummaryModel)
	} else if settings.OpenAISummaryModel != "" {
		applySummaryModel(cfg, settings.OpenAISummaryModel)
	}
	if settings.SummaryURL != "" {
		cfg.Summary.LLMClient.OpenAI.BaseURL = settings.SummaryURL
	} else if settings.OpenAISummaryURL != "" {
		cfg.Summary.LLMClient.OpenAI.BaseURL = settings.OpenAISummaryURL
	}
	cfg.SummaryEnabled = settings.SummaryEnabled
	cfg.Summary.Enabled = settings.SummaryEnabled
	cfg.SummaryContextWindowTokens = settings.SummaryContextWindowTokens
	cfg.Summary.ContextWindowTokens = settings.SummaryContextWindowTokens
	cfg.SummaryPlainTextContextWindowTokens = settings.SummaryPlainTextContextWindowTokens
	cfg.Summary.PlainTextContextWindowTokens = settings.SummaryPlainTextContextWindowTokens
	cfg.SummaryReserveBufferTokens = settings.SummaryReserveBufferTokens
	cfg.Summary.ReserveBufferTokens = settings.SummaryReserveBufferTokens
	cfg.SummaryMinKeepLastMessages = settings.SummaryMinKeepLastMessages
	cfg.Summary.MinKeepLastMessages = settings.SummaryMinKeepLastMessages
	cfg.SummaryMaxKeepLastMessages = settings.SummaryMaxKeepLastMessages
	cfg.Summary.MaxKeepLastMessages = settings.SummaryMaxKeepLastMessages
	cfg.SummaryMaxSummaryChunkTokens = settings.SummaryMaxSummaryChunkTokens
	cfg.Summary.MaxSummaryChunkTokens = settings.SummaryMaxSummaryChunkTokens
	cfg.Summary.CallTimeoutSeconds = settings.SummaryCallTimeoutSeconds
}

func applyEmbeddingSettings(cfg *config.Config, settings agentdSettings) {
	if settings.EmbedBaseURL != "" {
		cfg.Embedding.BaseURL = settings.EmbedBaseURL
	}
	if settings.EmbedModel != "" {
		cfg.Embedding.Model = settings.EmbedModel
	}
	if settings.EmbedAPIKey != "" {
		cfg.Embedding.APIKey = settings.EmbedAPIKey
	}
	if settings.EmbedAPIHeader != "" {
		cfg.Embedding.APIHeader = settings.EmbedAPIHeader
	}
	if settings.EmbedAPIHeaders != nil {
		cfg.Embedding.Headers = settings.EmbedAPIHeaders
	}
	if settings.EmbedPath != "" {
		cfg.Embedding.Path = settings.EmbedPath
	}
	if settings.EmbedInstructionMode != "" {
		cfg.Embedding.Instructions.Mode = settings.EmbedInstructionMode
	}
	if settings.EmbedInstructionFormat != "" {
		cfg.Embedding.Instructions.Format = settings.EmbedInstructionFormat
	}
	cfg.Embedding.Instructions.DefaultQuery = settings.EmbedDefaultQueryInstruction
	cfg.Embedding.Instructions.RAGQuery = settings.EmbedRAGQueryInstruction
	cfg.Embedding.Instructions.EvolvingMemoryQuery = settings.EmbedEvolvingMemoryQueryInstruction
	cfg.Embedding.Instructions.TransitQuery = settings.EmbedTransitQueryInstruction
}

func applyRerankSettings(cfg *config.Config, settings agentdSettings) {
	cfg.Reranking.Enabled = settings.RerankEnabled
	if settings.RerankBaseURL != "" {
		cfg.Reranking.BaseURL = settings.RerankBaseURL
	}
	if settings.RerankModel != "" {
		cfg.Reranking.Model = settings.RerankModel
	}
	cfg.Reranking.Instruction = settings.RerankInstruction
	if settings.RerankAPIKey != "" {
		cfg.Reranking.APIKey = settings.RerankAPIKey
	}
	if settings.RerankAPIHeader != "" {
		cfg.Reranking.APIHeader = settings.RerankAPIHeader
	}
	if settings.RerankAPIHeaders != nil {
		cfg.Reranking.Headers = settings.RerankAPIHeaders
	}
	if settings.RerankPath != "" {
		cfg.Reranking.Path = settings.RerankPath
	}
}

func applyTimeoutSettings(cfg *config.Config, settings agentdSettings) {
	if settings.AgentRunTimeoutSeconds != 0 {
		cfg.AgentRunTimeoutSeconds = settings.AgentRunTimeoutSeconds
	}
	if settings.StreamRunTimeoutSeconds != 0 {
		cfg.StreamRunTimeoutSeconds = settings.StreamRunTimeoutSeconds
	}
	if settings.WorkflowTimeoutSeconds != 0 {
		cfg.WorkflowTimeoutSeconds = settings.WorkflowTimeoutSeconds
	}
}

func applyExecSettings(cfg *config.Config, settings agentdSettings) error {
	if settings.BlockBinaries != "" {
		binaries, err := parseBlockBinaries(settings.BlockBinaries)
		if err != nil {
			return err
		}
		cfg.Exec.BlockBinaries = binaries
	}
	if settings.CommandRules != nil {
		if err := validateCommandRulesSettings(settings.CommandRules); err != nil {
			return err
		}
		cfg.Exec.CommandRules = append([]config.ExecCommandRule(nil), settings.CommandRules...)
	}
	if settings.SandboxEnabled != nil {
		cfg.Exec.Sandbox.Enabled = boolPtr(*settings.SandboxEnabled)
	}
	if settings.SandboxFailIfUnavailable != nil {
		cfg.Exec.Sandbox.FailIfUnavailable = boolPtr(*settings.SandboxFailIfUnavailable)
	}
	if settings.SandboxNetworkEnabled != nil {
		cfg.Exec.Sandbox.Network.Enabled = boolPtr(*settings.SandboxNetworkEnabled)
	}
	if settings.SandboxNetworkAllowedDomains != nil {
		cfg.Exec.Sandbox.Network.AllowedDomains = append([]string(nil), settings.SandboxNetworkAllowedDomains...)
	}
	if settings.MaxCommandSeconds != 0 {
		cfg.Exec.MaxCommandSeconds = settings.MaxCommandSeconds
	}
	if settings.OutputTruncateBytes != 0 {
		cfg.OutputTruncateByte = settings.OutputTruncateBytes
	}
	if settings.MaxTerminalSessions != 0 {
		cfg.Exec.MaxTerminalSessions = settings.MaxTerminalSessions
	}
	if settings.MaxTerminalRuntimeSeconds != 0 {
		cfg.Exec.MaxTerminalRuntimeSeconds = settings.MaxTerminalRuntimeSeconds
	}
	if settings.TerminalIdleTTLSeconds != 0 {
		cfg.Exec.TerminalIdleTTLSeconds = settings.TerminalIdleTTLSeconds
	}
	if settings.TerminalOutputBufferBytes != 0 {
		cfg.Exec.TerminalOutputBufferBytes = settings.TerminalOutputBufferBytes
	}
	return nil
}

func applyObservabilitySettings(cfg *config.Config, settings agentdSettings) {
	if settings.OTELServiceName != "" {
		cfg.Obs.ServiceName = settings.OTELServiceName
	}
	if settings.ServiceVersion != "" {
		cfg.Obs.ServiceVersion = settings.ServiceVersion
	}
	if settings.Environment != "" {
		cfg.Obs.Environment = settings.Environment
	}
	if settings.OTLPEndpoint != "" {
		cfg.Obs.OTLP = settings.OTLPEndpoint
	}
}

func applyLogSettings(cfg *config.Config, settings agentdSettings) {
	if settings.LogPath != "" {
		cfg.LogPath = settings.LogPath
	}
	if settings.LogLevel != "" {
		cfg.LogLevel = settings.LogLevel
	}
	cfg.LogPayloads = settings.LogPayloads
	cfg.LogRawPrompts = settings.LogRawPrompts
}

func applyWebSettings(cfg *config.Config, settings agentdSettings) {
	if settings.WebSearXNGURL != "" {
		cfg.Web.SearXNGURL = settings.WebSearXNGURL
	}
}

func applyDatabaseSettings(cfg *config.Config, settings agentdSettings) {
	if settings.PostgresDSN != "" {
		cfg.Databases.DefaultDSN = settings.PostgresDSN
	}
	if settings.SearchBackend != "" {
		cfg.Databases.Search.Backend = settings.SearchBackend
	}
	if settings.SearchDSN != "" {
		cfg.Databases.Search.DSN = settings.SearchDSN
	}
	if settings.SearchIndex != "" {
		cfg.Databases.Search.Index = settings.SearchIndex
	}
	if settings.VectorBackend != "" {
		cfg.Databases.Vector.Backend = settings.VectorBackend
	}
	if settings.VectorDSN != "" {
		cfg.Databases.Vector.DSN = settings.VectorDSN
	}
	if settings.VectorIndex != "" {
		cfg.Databases.Vector.Index = settings.VectorIndex
	}
	if settings.VectorDims != 0 {
		cfg.Databases.Vector.Dimensions = settings.VectorDims
	}
	if settings.VectorMetric != "" {
		cfg.Databases.Vector.Metric = settings.VectorMetric
	}
	if settings.GraphBackend != "" {
		cfg.Databases.Graph.Backend = settings.GraphBackend
	}
	if settings.GraphDSN != "" {
		cfg.Databases.Graph.DSN = settings.GraphDSN
	}
}

func validateEmbeddingInstructionSettings(settings agentdSettings) error {
	switch strings.ToLower(strings.TrimSpace(settings.EmbedInstructionMode)) {
	case "auto", "enabled", "disabled":
	default:
		return fmt.Errorf("embedInstructionMode must be one of auto, enabled, or disabled (got %q)", settings.EmbedInstructionMode)
	}
	switch strings.ToLower(strings.TrimSpace(settings.EmbedInstructionFormat)) {
	case "qwen":
	default:
		return fmt.Errorf("embedInstructionFormat must be qwen (got %q)", settings.EmbedInstructionFormat)
	}
	return nil
}

func parseBlockBinaries(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") || strings.Contains(part, "\\") {
			return nil, fmt.Errorf("blockBinaries must be bare binary names (no paths): %q", part)
		}
		out = append(out, part)
	}
	return out, nil
}

func validateCommandRulesSettings(rules []config.ExecCommandRule) error {
	for i, rule := range rules {
		switch strings.ToLower(strings.TrimSpace(rule.Decision)) {
		case "allow", "ask", "deny":
		default:
			return fmt.Errorf("commandRules[%d].decision must be allow, ask, or deny", i)
		}
		if len(rule.Pattern) == 0 {
			return fmt.Errorf("commandRules[%d].pattern must contain at least one token", i)
		}
		if strings.TrimSpace(rule.Pattern[0]) == "" || strings.Contains(rule.Pattern[0], "/") || strings.Contains(rule.Pattern[0], "\\") {
			return fmt.Errorf("commandRules[%d].pattern[0] must be a bare binary name", i)
		}
		for j, token := range rule.Pattern {
			if strings.TrimSpace(token) == "" {
				return fmt.Errorf("commandRules[%d].pattern[%d] must not be empty", i, j)
			}
		}
	}
	return nil
}

func setNestedMapValue(root map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	current := root
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}


func applyPrimaryLLMSettings(cfg *config.Config, settings agentdSettings) error {
	provider := strings.ToLower(strings.TrimSpace(settings.LLMProvider))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider))
	}
	if provider == "" {
		provider = "openai"
	}
	switch provider {
	case "openai", "anthropic", "google", "local":
	default:
		return fmt.Errorf("llmProvider must be one of openai, anthropic, google, or local (got %q)", settings.LLMProvider)
	}
	cfg.LLMClient.Provider = provider
	apiKey := strings.TrimSpace(settings.LLMAPIKey)
	model := strings.TrimSpace(settings.LLMModel)
	baseURL := strings.TrimSpace(settings.LLMBaseURL)
	switch provider {
	case "anthropic":
		if apiKey != "" {
			cfg.LLMClient.Anthropic.APIKey = apiKey
		}
		if model != "" {
			cfg.LLMClient.Anthropic.Model = model
		}
		if baseURL != "" {
			cfg.LLMClient.Anthropic.BaseURL = baseURL
		}
	case "google":
		if apiKey != "" {
			cfg.LLMClient.Google.APIKey = apiKey
		}
		if model != "" {
			cfg.LLMClient.Google.Model = model
		}
		if baseURL != "" {
			cfg.LLMClient.Google.BaseURL = baseURL
		}
	default:
		if apiKey != "" {
			cfg.LLMClient.OpenAI.APIKey = apiKey
		}
		if model != "" {
			cfg.LLMClient.OpenAI.Model = model
		}
		if baseURL != "" {
			cfg.LLMClient.OpenAI.BaseURL = baseURL
		}
		if provider == "local" {
			cfg.LLMClient.OpenAI.API = "completions"
		}
	}
	cfg.OpenAI = cfg.LLMClient.OpenAI

	cfg.Memory.Enabled = settings.MemoryEnabled
	cfg.MemoryConfigured = true
	cfg.EvolvingMemory.Enabled = settings.MemoryEnabled
	cfg.BeliefMemory.Enabled = settings.MemoryEnabled
	cfg.Magma.Enabled = settings.MemoryEnabled
	return nil
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
