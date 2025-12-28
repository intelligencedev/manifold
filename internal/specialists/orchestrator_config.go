package specialists

import (
	"strings"

	"manifold/internal/config"
	"manifold/internal/persistence"
)

// DefaultOrchestratorPrompt is used when no persisted system prompt is available.
const DefaultOrchestratorPrompt = "You are a helpful assistant with access to tools and specialists to help you complete objectives."

// ApplyLLMClientOverride overlays persisted specialist fields onto an LLM client config.
// It returns the updated config and resolved provider name.
func ApplyLLMClientOverride(base config.LLMClientConfig, sp persistence.Specialist) (config.LLMClientConfig, string) {
	provider := strings.TrimSpace(sp.Provider)
	if provider == "" {
		provider = base.Provider
	}
	cfg := base
	cfg.Provider = provider

	switch provider {
	case "anthropic":
		if strings.TrimSpace(sp.BaseURL) != "" {
			cfg.Anthropic.BaseURL = strings.TrimSpace(sp.BaseURL)
		}
		if strings.TrimSpace(sp.APIKey) != "" {
			cfg.Anthropic.APIKey = strings.TrimSpace(sp.APIKey)
		}
		if strings.TrimSpace(sp.Model) != "" {
			cfg.Anthropic.Model = strings.TrimSpace(sp.Model)
		}
	case "google":
		if strings.TrimSpace(sp.BaseURL) != "" {
			cfg.Google.BaseURL = strings.TrimSpace(sp.BaseURL)
		}
		if strings.TrimSpace(sp.APIKey) != "" {
			cfg.Google.APIKey = strings.TrimSpace(sp.APIKey)
		}
		if strings.TrimSpace(sp.Model) != "" {
			cfg.Google.Model = strings.TrimSpace(sp.Model)
		}
	default:
		if strings.TrimSpace(sp.BaseURL) != "" {
			cfg.OpenAI.BaseURL = strings.TrimSpace(sp.BaseURL)
		}
		if strings.TrimSpace(sp.APIKey) != "" {
			cfg.OpenAI.APIKey = strings.TrimSpace(sp.APIKey)
		}
		if strings.TrimSpace(sp.Model) != "" {
			cfg.OpenAI.Model = strings.TrimSpace(sp.Model)
		}
		if sp.ExtraHeaders != nil {
			cfg.OpenAI.ExtraHeaders = sp.ExtraHeaders
		}
		if sp.ExtraParams != nil {
			cfg.OpenAI.ExtraParams = sp.ExtraParams
		}
	}

	return cfg, provider
}

// ApplyOrchestratorConfig overlays a persisted orchestrator specialist on the runtime config.
// It returns the resolved provider name.
func ApplyOrchestratorConfig(cfg *config.Config, sp persistence.Specialist) string {
	llmCfg, provider := ApplyLLMClientOverride(cfg.LLMClient, sp)
	cfg.LLMClient = llmCfg
	if provider == "" || provider == "openai" || provider == "local" {
		cfg.OpenAI = llmCfg.OpenAI
	}
	cfg.EnableTools = sp.EnableTools
	cfg.ToolAllowList = append([]string(nil), sp.AllowTools...)
	cfg.SystemPrompt = sp.System

	return provider
}
