package agentd

import (
	"maps"
	"net/http"
	"strings"

	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
	llmproviders "manifold/internal/llm/providers"
)

func resolveLLMClientModel(cfg config.LLMClientConfig) string {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "anthropic":
		return strings.TrimSpace(cfg.Anthropic.Model)
	case "google":
		return strings.TrimSpace(cfg.Google.Model)
	default:
		return strings.TrimSpace(cfg.OpenAI.Model)
	}
}

func mergeStringMap(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	maps.Copy(out, base)
	maps.Copy(out, override)
	return out
}

func mergeAnyMap(base, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(override))
	maps.Copy(out, base)
	maps.Copy(out, override)
	return out
}

func hasLLMClientOverride(cfg config.LLMClientConfig) bool {
	if strings.TrimSpace(cfg.Provider) != "" {
		return true
	}
	if strings.TrimSpace(cfg.OpenAI.APIKey) != "" || strings.TrimSpace(cfg.OpenAI.Model) != "" || strings.TrimSpace(cfg.OpenAI.BaseURL) != "" || strings.TrimSpace(cfg.OpenAI.SummaryModel) != "" || strings.TrimSpace(cfg.OpenAI.SummaryBaseURL) != "" || strings.TrimSpace(cfg.OpenAI.API) != "" || len(cfg.OpenAI.ExtraHeaders) > 0 || len(cfg.OpenAI.ExtraParams) > 0 || cfg.OpenAI.LogPayloads {
		return true
	}
	if strings.TrimSpace(cfg.Anthropic.APIKey) != "" || strings.TrimSpace(cfg.Anthropic.Model) != "" || strings.TrimSpace(cfg.Anthropic.BaseURL) != "" || cfg.Anthropic.MaxTokens != 0 || len(cfg.Anthropic.ExtraParams) > 0 || cfg.Anthropic.PromptCache.Enabled || cfg.Anthropic.PromptCache.CacheSystem || cfg.Anthropic.PromptCache.CacheTools || cfg.Anthropic.PromptCache.CacheMessages {
		return true
	}
	if strings.TrimSpace(cfg.Google.APIKey) != "" || strings.TrimSpace(cfg.Google.Model) != "" || strings.TrimSpace(cfg.Google.BaseURL) != "" || cfg.Google.Timeout != 0 || len(cfg.Google.ExtraParams) > 0 {
		return true
	}
	return false
}

func mergeLLMClientConfig(base, override config.LLMClientConfig) config.LLMClientConfig {
	out := base
	if provider := strings.TrimSpace(override.Provider); provider != "" {
		out.Provider = strings.ToLower(provider)
	}
	if value := strings.TrimSpace(override.OpenAI.APIKey); value != "" {
		out.OpenAI.APIKey = value
	}
	if value := strings.TrimSpace(override.OpenAI.Model); value != "" {
		out.OpenAI.Model = value
	}
	if value := strings.TrimSpace(override.OpenAI.BaseURL); value != "" {
		out.OpenAI.BaseURL = value
	}
	if value := strings.TrimSpace(override.OpenAI.SummaryModel); value != "" {
		out.OpenAI.SummaryModel = value
	}
	if value := strings.TrimSpace(override.OpenAI.SummaryBaseURL); value != "" {
		out.OpenAI.SummaryBaseURL = value
	}
	if value := strings.TrimSpace(override.OpenAI.API); value != "" {
		out.OpenAI.API = value
	}
	if len(override.OpenAI.ExtraHeaders) > 0 {
		out.OpenAI.ExtraHeaders = mergeStringMap(out.OpenAI.ExtraHeaders, override.OpenAI.ExtraHeaders)
	}
	if len(override.OpenAI.ExtraParams) > 0 {
		out.OpenAI.ExtraParams = mergeAnyMap(out.OpenAI.ExtraParams, override.OpenAI.ExtraParams)
	}
	if override.OpenAI.LogPayloads {
		out.OpenAI.LogPayloads = true
	}

	if value := strings.TrimSpace(override.Anthropic.APIKey); value != "" {
		out.Anthropic.APIKey = value
	}
	if value := strings.TrimSpace(override.Anthropic.Model); value != "" {
		out.Anthropic.Model = value
	}
	if value := strings.TrimSpace(override.Anthropic.BaseURL); value != "" {
		out.Anthropic.BaseURL = value
	}
	if override.Anthropic.MaxTokens != 0 {
		out.Anthropic.MaxTokens = override.Anthropic.MaxTokens
	}
	if len(override.Anthropic.ExtraParams) > 0 {
		out.Anthropic.ExtraParams = mergeAnyMap(out.Anthropic.ExtraParams, override.Anthropic.ExtraParams)
	}
	if override.Anthropic.PromptCache.Enabled {
		out.Anthropic.PromptCache.Enabled = true
	}
	if override.Anthropic.PromptCache.CacheSystem {
		out.Anthropic.PromptCache.CacheSystem = true
	}
	if override.Anthropic.PromptCache.CacheTools {
		out.Anthropic.PromptCache.CacheTools = true
	}
	if override.Anthropic.PromptCache.CacheMessages {
		out.Anthropic.PromptCache.CacheMessages = true
	}

	if value := strings.TrimSpace(override.Google.APIKey); value != "" {
		out.Google.APIKey = value
	}
	if value := strings.TrimSpace(override.Google.Model); value != "" {
		out.Google.Model = value
	}
	if value := strings.TrimSpace(override.Google.BaseURL); value != "" {
		out.Google.BaseURL = value
	}
	if override.Google.Timeout != 0 {
		out.Google.Timeout = override.Google.Timeout
	}
	if len(override.Google.ExtraParams) > 0 {
		out.Google.ExtraParams = mergeAnyMap(out.Google.ExtraParams, override.Google.ExtraParams)
	}

	return out
}

func llmClientHasExplicitModel(cfg config.LLMClientConfig, providerName string) bool {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "anthropic":
		return strings.TrimSpace(cfg.Anthropic.Model) != ""
	case "google":
		return strings.TrimSpace(cfg.Google.Model) != ""
	default:
		return strings.TrimSpace(cfg.OpenAI.Model) != ""
	}
}

func resolveEvolvingMemoryLLM(cfg *config.Config, mainLLM llmpkg.Provider, summaryLLM llmpkg.Provider, summaryModel string, httpClient *http.Client) (llmpkg.Provider, string, string, error) {
	if hasLLMClientOverride(cfg.EvolvingMemory.LLMClient) {
		llmCfg := mergeLLMClientConfig(cfg.LLMClient, cfg.EvolvingMemory.LLMClient)
		if providerName := strings.ToLower(strings.TrimSpace(cfg.EvolvingMemory.Provider)); providerName != "" && strings.TrimSpace(cfg.EvolvingMemory.LLMClient.Provider) == "" {
			llmCfg.Provider = providerName
		}
		if model := strings.TrimSpace(cfg.EvolvingMemory.Model); model != "" && !llmClientHasExplicitModel(cfg.EvolvingMemory.LLMClient, llmCfg.Provider) {
			switch strings.ToLower(strings.TrimSpace(llmCfg.Provider)) {
			case "anthropic":
				llmCfg.Anthropic.Model = model
			case "google":
				llmCfg.Google.Model = model
			default:
				llmCfg.OpenAI.Model = model
			}
		}

		provider, err := llmproviders.BuildFromLLMClientConfig(llmCfg, httpClient)
		if err != nil {
			return nil, "", "", err
		}
		return provider, resolveLLMClientModel(llmCfg), strings.ToLower(strings.TrimSpace(llmCfg.Provider)), nil
	}

	providerName := strings.ToLower(strings.TrimSpace(cfg.EvolvingMemory.Provider))
	if providerName != "" {
		llmCfg := cfg.LLMClient
		llmCfg.Provider = providerName
		if model := strings.TrimSpace(cfg.EvolvingMemory.Model); model != "" {
			switch providerName {
			case "anthropic":
				llmCfg.Anthropic.Model = model
			case "google":
				llmCfg.Google.Model = model
			default:
				llmCfg.OpenAI.Model = model
			}
		}

		provider, err := llmproviders.BuildFromLLMClientConfig(llmCfg, httpClient)
		if err != nil {
			return nil, "", "", err
		}
		return provider, resolveLLMClientModel(llmCfg), providerName, nil
	}

	memModel := strings.TrimSpace(cfg.EvolvingMemory.Model)
	if memModel != "" {
		return mainLLM, memModel, strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider)), nil
	}
	if summaryLLM != nil && strings.TrimSpace(summaryModel) != "" {
		return summaryLLM, strings.TrimSpace(summaryModel), "summary", nil
	}
	return mainLLM, resolveLLMClientModel(cfg.LLMClient), strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider)), nil
}

func resolveBeliefMemoryLLM(cfg *config.Config, mainLLM llmpkg.Provider, httpClient *http.Client) (llmpkg.Provider, string, string, error) {
	if cfg == nil {
		return nil, "", "", nil
	}
	if hasLLMClientOverride(cfg.BeliefMemory.LLMClient) {
		llmCfg := mergeLLMClientConfig(cfg.LLMClient, cfg.BeliefMemory.LLMClient)
		provider, err := llmproviders.BuildFromLLMClientConfig(llmCfg, httpClient)
		if err != nil {
			return nil, "", "", err
		}
		return provider, resolveLLMClientModel(llmCfg), strings.ToLower(strings.TrimSpace(llmCfg.Provider)), nil
	}
	if mainLLM != nil {
		return mainLLM, resolveLLMClientModel(cfg.LLMClient), strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider)), nil
	}
	provider, err := llmproviders.BuildFromLLMClientConfig(cfg.LLMClient, httpClient)
	if err != nil {
		return nil, "", "", err
	}
	return provider, resolveLLMClientModel(cfg.LLMClient), strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider)), nil
}

func buildSummaryLLM(cfg *config.Config, httpClient *http.Client) (llmpkg.Provider, string, error) {
	if !cfg.Summary.Enabled {
		return nil, "", nil
	}
	provider, err := llmproviders.BuildFromLLMClientConfig(cfg.Summary.LLMClient, httpClient)
	if err != nil {
		return nil, "", err
	}
	return provider, resolveLLMClientModel(cfg.Summary.LLMClient), nil
}
