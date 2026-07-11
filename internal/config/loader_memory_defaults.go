package config

import "strings"

func applyHarnessDefaults(cfg *Config) {
	applyHarnessConfigDefaults(&cfg.Harness)
	for i := range cfg.Specialists {
		if cfg.Specialists[i].Harness != nil {
			applyHarnessConfigDefaults(cfg.Specialists[i].Harness)
		}
	}
}

func applyHarnessConfigDefaults(cfg *HarnessConfig) {
	if cfg == nil {
		return
	}
	if cfg.Mode == "" {
		cfg.Mode = "guarded_chat"
	}
	if cfg.MaxRetriesPerStep <= 0 {
		cfg.MaxRetriesPerStep = 3
	}
	if cfg.MaxToolErrors <= 0 {
		cfg.MaxToolErrors = 2
	}
	if len(cfg.TerminalTools) == 0 {
		cfg.TerminalTools = []string{"agent_response"}
	}
	if cfg.Compact.KeepRecentSteps <= 0 {
		cfg.Compact.KeepRecentSteps = 4
	}
	if len(cfg.Compact.PhaseThresholds) == 0 {
		cfg.Compact.PhaseThresholds = []float64{0.60, 0.75, 0.90}
	}
}

func applyBeliefMemoryDefaults(cfg *Config) {
	beliefs := &cfg.BeliefMemory
	if beliefs.Distillation.Mode == "" {
		if hasConfigLLMClientOverride(beliefs.LLMClient) {
			beliefs.Distillation.Mode = "llm"
		} else {
			beliefs.Distillation.Mode = "simple"
		}
	}
	if beliefs.Distillation.MaxCandidatesPerEpisode <= 0 {
		beliefs.Distillation.MaxCandidatesPerEpisode = 5
	}
	if beliefs.Distillation.MinCandidateConfidence == 0 {
		beliefs.Distillation.MinCandidateConfidence = 0.55
	}
	if beliefs.Distillation.AutoApplyMinConfidence == 0 {
		beliefs.Distillation.AutoApplyMinConfidence = 0.65
	}
	if beliefs.Retrieval.MinConfidence == 0 {
		beliefs.Retrieval.MinConfidence = 0.35
	}
	if beliefs.Retrieval.MaxTokensPerPrompt <= 0 {
		beliefs.Retrieval.MaxTokensPerPrompt = 700
	}
	if !beliefs.Retrieval.IncludeContradictions {
		beliefs.Retrieval.IncludeContradictions = true
	}
	if beliefs.Lifecycle.MinEvidenceForPromotion <= 0 {
		beliefs.Lifecycle.MinEvidenceForPromotion = 2
	}
	if beliefs.Lifecycle.StaleAfterDays <= 0 {
		beliefs.Lifecycle.StaleAfterDays = 30
	}
	if beliefs.Lifecycle.StaleConfidenceDecay == 0 {
		beliefs.Lifecycle.StaleConfidenceDecay = 0.90
	}
	if !beliefs.Enforcement.AutoEnable {
		beliefs.Enforcement.AutoEnable = true
	}
	if beliefs.Enforcement.SoftPolicyThreshold == 0 {
		beliefs.Enforcement.SoftPolicyThreshold = 0.85
	}
	if beliefs.Enforcement.HardConstraintThreshold == 0 {
		beliefs.Enforcement.HardConstraintThreshold = 0.95
	}
	if beliefs.Enforcement.HardConstraintMinEvidenceFor <= 0 {
		beliefs.Enforcement.HardConstraintMinEvidenceFor = 3
	}
}
func hasConfigLLMClientOverride(cfg LLMClientConfig) bool {
	if strings.TrimSpace(cfg.Provider) != "" {
		return true
	}
	if strings.TrimSpace(cfg.OpenAI.APIKey) != "" || strings.TrimSpace(cfg.OpenAI.Model) != "" || strings.TrimSpace(cfg.OpenAI.BaseURL) != "" || strings.TrimSpace(cfg.OpenAI.SummaryModel) != "" || strings.TrimSpace(cfg.OpenAI.SummaryBaseURL) != "" || strings.TrimSpace(cfg.OpenAI.API) != "" || len(cfg.OpenAI.ExtraHeaders) > 0 || len(cfg.OpenAI.ExtraParams) > 0 || cfg.OpenAI.LogPayloads {
		return true
	}
	if strings.TrimSpace(cfg.Anthropic.APIKey) != "" || strings.TrimSpace(cfg.Anthropic.Model) != "" || strings.TrimSpace(cfg.Anthropic.BaseURL) != "" || cfg.Anthropic.MaxTokens != 0 || len(cfg.Anthropic.ExtraParams) > 0 || cfg.Anthropic.PromptCache.Enabled || cfg.Anthropic.PromptCache.CacheSystem || cfg.Anthropic.PromptCache.CacheTools || cfg.Anthropic.PromptCache.CacheMessages {
		return true
	}
	if strings.TrimSpace(cfg.Google.APIKey) != "" || strings.TrimSpace(cfg.Google.Model) != "" || strings.TrimSpace(cfg.Google.BaseURL) != "" || cfg.Google.Timeout != 0 || googleContextCacheConfigured(cfg.Google.ContextCache) || len(cfg.Google.ExtraParams) > 0 {
		return true
	}
	return false
}

func googleContextCacheConfigured(cfg GoogleContextCacheConfig) bool {
	return cfg.Enabled ||
		cfg.AutoCreate ||
		cfg.TTLSeconds != 0 ||
		cfg.CacheSystem ||
		cfg.CacheTools ||
		strings.TrimSpace(cfg.CachedContent) != "" ||
		strings.TrimSpace(cfg.DisplayName) != ""
}

func applySummaryDefaults(cfg *Config) {
	syncSummarySettingsDefaults(cfg)
	applySummaryOpenAIDefaults(cfg)
	applySummaryAnthropicDefaults(cfg)
	applySummaryGoogleDefaults(cfg)
	syncSummaryModelAliases(cfg)
}

func syncSummarySettingsDefaults(cfg *Config) {
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
}

func applySummaryOpenAIDefaults(cfg *Config) {
	if cfg.Summary.LLMClient.Provider == "" {
		cfg.Summary.LLMClient.Provider = cfg.LLMClient.Provider
	}
	if cfg.Summary.LLMClient.OpenAI.APIKey == "" {
		cfg.Summary.LLMClient.OpenAI.APIKey = cfg.LLMClient.OpenAI.APIKey
	}
	if cfg.Summary.LLMClient.OpenAI.Model == "" {
		cfg.Summary.LLMClient.OpenAI.Model = firstNonEmpty(cfg.LLMClient.OpenAI.SummaryModel, cfg.LLMClient.OpenAI.Model)
	}
	if cfg.Summary.LLMClient.OpenAI.BaseURL == "" {
		cfg.Summary.LLMClient.OpenAI.BaseURL = firstNonEmpty(cfg.LLMClient.OpenAI.SummaryBaseURL, cfg.LLMClient.OpenAI.BaseURL)
	}
	if cfg.Summary.LLMClient.OpenAI.API == "" {
		cfg.Summary.LLMClient.OpenAI.API = cfg.LLMClient.OpenAI.API
		if cfg.Summary.LLMClient.OpenAI.API == "" {
			cfg.Summary.LLMClient.OpenAI.API = "responses"
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
}

func applySummaryAnthropicDefaults(cfg *Config) {
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
}

func applySummaryGoogleDefaults(cfg *Config) {
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
	if !googleContextCacheConfigured(cfg.Summary.LLMClient.Google.ContextCache) && googleContextCacheConfigured(cfg.LLMClient.Google.ContextCache) {
		cfg.Summary.LLMClient.Google.ContextCache = cfg.LLMClient.Google.ContextCache
	}
	if len(cfg.Summary.LLMClient.Google.ExtraParams) == 0 && len(cfg.LLMClient.Google.ExtraParams) > 0 {
		cfg.Summary.LLMClient.Google.ExtraParams = cfg.LLMClient.Google.ExtraParams
	}
}

func syncSummaryModelAliases(cfg *Config) {
	cfg.LLMClient.OpenAI.SummaryModel = cfg.Summary.LLMClient.OpenAI.Model
	cfg.LLMClient.OpenAI.SummaryBaseURL = cfg.Summary.LLMClient.OpenAI.BaseURL
}
