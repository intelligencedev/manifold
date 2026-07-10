package agentd

import (
	"os"

	yaml "gopkg.in/yaml.v3"
)

func persistToConfigYAML(settings agentdSettings) error {
	settings = normalizeAgentdSettings(settings)
	path := findConfigYAMLPath()
	if err := ensureConfigParentDir(path); err != nil {
		return err
	}

	root := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(b, &root)
	}

	applyAgentdSettingsYAML(root, settings)

	b, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func applyAgentdSettingsYAML(root map[string]any, settings agentdSettings) {
	settings = normalizeAgentdSettings(settings)

	applyPrimaryLLMSettingsYAML(root, settings)
	applySummarySettingsYAML(root, settings)
	setNestedMapValue(root, []string{"requestInfoEnabled"}, settings.RequestInfoEnabled)
	applyPromptOverrideSettingsYAML(root, settings)
	applyEmbeddingSettingsYAML(root, settings)
	applyRerankSettingsYAML(root, settings)
	applyTimeoutSettingsYAML(root, settings)
	applyExecSettingsYAML(root, settings)
	applyObservabilitySettingsYAML(root, settings)
	applyLogSettingsYAML(root, settings)
	applyWebSettingsYAML(root, settings)
	applyDatabaseSettingsYAML(root, settings)
}


func applyPrimaryLLMSettingsYAML(root map[string]any, settings agentdSettings) {
	provider := firstNonEmptyTrimmed(settings.LLMProvider)
	if provider != "" {
		setNestedMapValue(root, []string{"llm_client", "provider"}, provider)
	}
	switch provider {
	case "anthropic":
		if settings.LLMAPIKey != "" {
			setNestedMapValue(root, []string{"llm_client", "anthropic", "apiKey"}, settings.LLMAPIKey)
		}
		if settings.LLMModel != "" {
			setNestedMapValue(root, []string{"llm_client", "anthropic", "model"}, settings.LLMModel)
		}
		if settings.LLMBaseURL != "" {
			setNestedMapValue(root, []string{"llm_client", "anthropic", "baseURL"}, settings.LLMBaseURL)
		}
	case "google":
		if settings.LLMAPIKey != "" {
			setNestedMapValue(root, []string{"llm_client", "google", "apiKey"}, settings.LLMAPIKey)
		}
		if settings.LLMModel != "" {
			setNestedMapValue(root, []string{"llm_client", "google", "model"}, settings.LLMModel)
		}
		if settings.LLMBaseURL != "" {
			setNestedMapValue(root, []string{"llm_client", "google", "baseURL"}, settings.LLMBaseURL)
		}
	default:
		if settings.LLMAPIKey != "" {
			setNestedMapValue(root, []string{"llm_client", "openai", "apiKey"}, settings.LLMAPIKey)
		}
		if settings.LLMModel != "" {
			setNestedMapValue(root, []string{"llm_client", "openai", "model"}, settings.LLMModel)
		}
		if settings.LLMBaseURL != "" {
			setNestedMapValue(root, []string{"llm_client", "openai", "baseURL"}, settings.LLMBaseURL)
		}
	}
	setNestedMapValue(root, []string{"memory", "enabled"}, settings.MemoryEnabled)
}

func applyPromptOverrideSettingsYAML(root map[string]any, settings agentdSettings) {
	setNestedMapValue(root, []string{"promptOverrides", "baseSystem"}, settings.PromptBaseSystem)
	setNestedMapValue(root, []string{"promptOverrides", "memoryInstructions"}, settings.PromptMemoryInstructions)
	setNestedMapValue(root, []string{"promptOverrides", "toolDiscoveryInstructions"}, settings.PromptToolDiscoveryInstructions)
	setNestedMapValue(root, []string{"promptOverrides", "skillDiscoveryInstructions"}, settings.PromptSkillDiscoveryInstructions)
}

func applySummarySettingsYAML(root map[string]any, settings agentdSettings) {
	setNestedMapValue(root, []string{"summaryEnabled"}, settings.SummaryEnabled)
	setNestedMapValue(root, []string{"summary", "enabled"}, settings.SummaryEnabled)
	setNestedMapValue(root, []string{"summaryContextWindowTokens"}, settings.SummaryContextWindowTokens)
	setNestedMapValue(root, []string{"summary", "contextWindowTokens"}, settings.SummaryContextWindowTokens)
	setNestedMapValue(root, []string{"summaryPlainTextContextWindowTokens"}, settings.SummaryPlainTextContextWindowTokens)
	setNestedMapValue(root, []string{"summary", "plainTextContextWindowTokens"}, settings.SummaryPlainTextContextWindowTokens)
	setNestedMapValue(root, []string{"summaryReserveBufferTokens"}, settings.SummaryReserveBufferTokens)
	setNestedMapValue(root, []string{"summary", "reserveBufferTokens"}, settings.SummaryReserveBufferTokens)
	setNestedMapValue(root, []string{"summaryMinKeepLastMessages"}, settings.SummaryMinKeepLastMessages)
	setNestedMapValue(root, []string{"summary", "minKeepLastMessages"}, settings.SummaryMinKeepLastMessages)
	setNestedMapValue(root, []string{"summaryMaxKeepLastMessages"}, settings.SummaryMaxKeepLastMessages)
	setNestedMapValue(root, []string{"summary", "maxKeepLastMessages"}, settings.SummaryMaxKeepLastMessages)
	setNestedMapValue(root, []string{"summaryMaxSummaryChunkTokens"}, settings.SummaryMaxSummaryChunkTokens)
	setNestedMapValue(root, []string{"summary", "maxSummaryChunkTokens"}, settings.SummaryMaxSummaryChunkTokens)
	setNestedMapValue(root, []string{"summary", "callTimeoutSeconds"}, settings.SummaryCallTimeoutSeconds)

	if settings.SummaryProvider != "" {
		setNestedMapValue(root, []string{"summary", "llm_client", "provider"}, settings.SummaryProvider)
	}
	summaryModel := firstNonEmptyTrimmed(settings.SummaryModel, settings.OpenAISummaryModel)
	if summaryModel != "" {
		setNestedMapValue(root, []string{"summary", "llm_client", "openai", "model"}, summaryModel)
	}
	summaryURL := firstNonEmptyTrimmed(settings.SummaryURL, settings.OpenAISummaryURL)
	if summaryURL != "" {
		setNestedMapValue(root, []string{"summary", "llm_client", "openai", "baseURL"}, summaryURL)
	}
}

func applyEmbeddingSettingsYAML(root map[string]any, settings agentdSettings) {
	if settings.EmbedBaseURL != "" {
		setNestedMapValue(root, []string{"embedding", "baseURL"}, settings.EmbedBaseURL)
	}
	if settings.EmbedModel != "" {
		setNestedMapValue(root, []string{"embedding", "model"}, settings.EmbedModel)
	}
	if settings.EmbedAPIKey != "" {
		setNestedMapValue(root, []string{"embedding", "apiKey"}, settings.EmbedAPIKey)
	}
	if settings.EmbedAPIHeader != "" {
		setNestedMapValue(root, []string{"embedding", "apiHeader"}, settings.EmbedAPIHeader)
	}
	if settings.EmbedPath != "" {
		setNestedMapValue(root, []string{"embedding", "path"}, settings.EmbedPath)
	}
	if len(settings.EmbedAPIHeaders) > 0 {
		setNestedMapValue(root, []string{"embedding", "headers"}, settings.EmbedAPIHeaders)
	}
	if settings.EmbedInstructionMode != "" {
		setNestedMapValue(root, []string{"embedding", "instructions", "mode"}, settings.EmbedInstructionMode)
	}
	if settings.EmbedInstructionFormat != "" {
		setNestedMapValue(root, []string{"embedding", "instructions", "format"}, settings.EmbedInstructionFormat)
	}
	setNestedMapValue(root, []string{"embedding", "instructions", "defaultQuery"}, settings.EmbedDefaultQueryInstruction)
	setNestedMapValue(root, []string{"embedding", "instructions", "ragQuery"}, settings.EmbedRAGQueryInstruction)
	setNestedMapValue(root, []string{"embedding", "instructions", "evolvingMemoryQuery"}, settings.EmbedEvolvingMemoryQueryInstruction)
	setNestedMapValue(root, []string{"embedding", "instructions", "transitQuery"}, settings.EmbedTransitQueryInstruction)
}

func applyRerankSettingsYAML(root map[string]any, settings agentdSettings) {
	setNestedMapValue(root, []string{"reranking", "enabled"}, settings.RerankEnabled)
	if settings.RerankBaseURL != "" {
		setNestedMapValue(root, []string{"reranking", "baseURL"}, settings.RerankBaseURL)
	}
	if settings.RerankModel != "" {
		setNestedMapValue(root, []string{"reranking", "model"}, settings.RerankModel)
	}
	setNestedMapValue(root, []string{"reranking", "instruction"}, settings.RerankInstruction)
	if settings.RerankAPIKey != "" {
		setNestedMapValue(root, []string{"reranking", "apiKey"}, settings.RerankAPIKey)
	}
	if settings.RerankAPIHeader != "" {
		setNestedMapValue(root, []string{"reranking", "apiHeader"}, settings.RerankAPIHeader)
	}
	if settings.RerankPath != "" {
		setNestedMapValue(root, []string{"reranking", "path"}, settings.RerankPath)
	}
	if len(settings.RerankAPIHeaders) > 0 {
		setNestedMapValue(root, []string{"reranking", "headers"}, settings.RerankAPIHeaders)
	}
}

func applyTimeoutSettingsYAML(root map[string]any, settings agentdSettings) {
	if settings.AgentRunTimeoutSeconds != 0 {
		setNestedMapValue(root, []string{"agentRunTimeoutSeconds"}, settings.AgentRunTimeoutSeconds)
	}
	if settings.StreamRunTimeoutSeconds != 0 {
		setNestedMapValue(root, []string{"streamRunTimeoutSeconds"}, settings.StreamRunTimeoutSeconds)
	}
	if settings.WorkflowTimeoutSeconds != 0 {
		setNestedMapValue(root, []string{"workflowTimeoutSeconds"}, settings.WorkflowTimeoutSeconds)
	}
}

func applyExecSettingsYAML(root map[string]any, settings agentdSettings) {
	if settings.BlockBinaries != "" {
		parts, err := parseBlockBinaries(settings.BlockBinaries)
		if err == nil {
			setNestedMapValue(root, []string{"exec", "blockBinaries"}, parts)
		}
	}
	if settings.SandboxEnabled != nil {
		setNestedMapValue(root, []string{"exec", "sandbox", "enabled"}, *settings.SandboxEnabled)
	}
	if settings.SandboxFailIfUnavailable != nil {
		setNestedMapValue(root, []string{"exec", "sandbox", "failIfUnavailable"}, *settings.SandboxFailIfUnavailable)
	}
	if settings.SandboxNetworkEnabled != nil {
		setNestedMapValue(root, []string{"exec", "sandbox", "network", "enabled"}, *settings.SandboxNetworkEnabled)
	}
	if settings.SandboxNetworkAllowedDomains != nil {
		setNestedMapValue(root, []string{"exec", "sandbox", "network", "allowedDomains"}, settings.SandboxNetworkAllowedDomains)
	}
	if settings.MaxCommandSeconds != 0 {
		setNestedMapValue(root, []string{"exec", "maxCommandSeconds"}, settings.MaxCommandSeconds)
	}
	if settings.OutputTruncateBytes != 0 {
		setNestedMapValue(root, []string{"outputTruncateBytes"}, settings.OutputTruncateBytes)
	}
	if settings.MaxTerminalSessions != 0 {
		setNestedMapValue(root, []string{"exec", "maxTerminalSessions"}, settings.MaxTerminalSessions)
	}
	if settings.MaxTerminalRuntimeSeconds != 0 {
		setNestedMapValue(root, []string{"exec", "maxTerminalRuntimeSeconds"}, settings.MaxTerminalRuntimeSeconds)
	}
	if settings.TerminalIdleTTLSeconds != 0 {
		setNestedMapValue(root, []string{"exec", "terminalIdleTTLSeconds"}, settings.TerminalIdleTTLSeconds)
	}
	if settings.TerminalOutputBufferBytes != 0 {
		setNestedMapValue(root, []string{"exec", "terminalOutputBufferBytes"}, settings.TerminalOutputBufferBytes)
	}
}

func applyObservabilitySettingsYAML(root map[string]any, settings agentdSettings) {
	if settings.OTELServiceName != "" {
		setNestedMapValue(root, []string{"obs", "serviceName"}, settings.OTELServiceName)
	}
	if settings.ServiceVersion != "" {
		setNestedMapValue(root, []string{"obs", "serviceVersion"}, settings.ServiceVersion)
	}
	if settings.Environment != "" {
		setNestedMapValue(root, []string{"obs", "environment"}, settings.Environment)
	}
	if settings.OTLPEndpoint != "" {
		setNestedMapValue(root, []string{"obs", "otlp"}, settings.OTLPEndpoint)
	}
}

func applyLogSettingsYAML(root map[string]any, settings agentdSettings) {
	setNestedMapValue(root, []string{"logPayloads"}, settings.LogPayloads)
	setNestedMapValue(root, []string{"logRawPrompts"}, settings.LogRawPrompts)
	if settings.LogPath != "" {
		setNestedMapValue(root, []string{"logPath"}, settings.LogPath)
	}
	if settings.LogLevel != "" {
		setNestedMapValue(root, []string{"logLevel"}, settings.LogLevel)
	}
}

func applyWebSettingsYAML(root map[string]any, settings agentdSettings) {
	if settings.WebSearXNGURL != "" {
		setNestedMapValue(root, []string{"web", "searXNGURL"}, settings.WebSearXNGURL)
	}
}

func applyDatabaseSettingsYAML(root map[string]any, settings agentdSettings) {
	if settings.PostgresDSN != "" {
		setNestedMapValue(root, []string{"databases", "defaultDSN"}, settings.PostgresDSN)
	}
	if settings.SearchBackend != "" {
		setNestedMapValue(root, []string{"databases", "search", "backend"}, settings.SearchBackend)
	}
	if settings.SearchDSN != "" {
		setNestedMapValue(root, []string{"databases", "search", "dsn"}, settings.SearchDSN)
	}
	if settings.SearchIndex != "" {
		setNestedMapValue(root, []string{"databases", "search", "index"}, settings.SearchIndex)
	}
	if settings.VectorBackend != "" {
		setNestedMapValue(root, []string{"databases", "vector", "backend"}, settings.VectorBackend)
	}
	if settings.VectorDSN != "" {
		setNestedMapValue(root, []string{"databases", "vector", "dsn"}, settings.VectorDSN)
	}
	if settings.VectorIndex != "" {
		setNestedMapValue(root, []string{"databases", "vector", "index"}, settings.VectorIndex)
	}
	if settings.VectorDims != 0 {
		setNestedMapValue(root, []string{"databases", "vector", "dimensions"}, settings.VectorDims)
	}
	if settings.VectorMetric != "" {
		setNestedMapValue(root, []string{"databases", "vector", "metric"}, settings.VectorMetric)
	}
	if settings.GraphBackend != "" {
		setNestedMapValue(root, []string{"databases", "graph", "backend"}, settings.GraphBackend)
	}
	if settings.GraphDSN != "" {
		setNestedMapValue(root, []string{"databases", "graph", "dsn"}, settings.GraphDSN)
	}
}
