package agentd

import (
	"testing"

	"manifold/internal/config"
)

// Even when the active provider is openai, every provider's card must expose its
// own static default params/baseURL so the frontend can populate the Advanced tab.
func TestProviderDefaultsReturnsStaticPerProvider(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLMClient.Provider = "openai"
	cfg.LLMClient.OpenAI.ExtraParams = config.DefaultProviderExtraParams("openai")
	a := &app{cfg: cfg}

	t.Run("openrouter (non-active, anthropic-backed)", func(t *testing.T) {
		_, baseURL, _, _, params := a.providerDefaults("openrouter")
		if baseURL != "https://openrouter.ai/api" {
			t.Errorf("baseURL = %q, want openrouter anthropic endpoint", baseURL)
		}
		if params["max_tokens"] != 16384 {
			t.Errorf("params[max_tokens] = %v, want 16384; got %v", params["max_tokens"], params)
		}
	})

	t.Run("openrouter active reflects Anthropic sub-config", func(t *testing.T) {
		orCfg := &config.Config{}
		orCfg.LLMClient.Provider = "openrouter"
		orCfg.LLMClient.Anthropic.APIKey = "sk-or-live"
		orCfg.LLMClient.Anthropic.BaseURL = "https://openrouter.ai/api"
		orApp := &app{cfg: orCfg}
		_, baseURL, apiKey, _, _ := orApp.providerDefaults("openrouter")
		if apiKey != "sk-or-live" {
			t.Errorf("apiKey = %q, want sk-or-live", apiKey)
		}
		if baseURL != "https://openrouter.ai/api" {
			t.Errorf("baseURL = %q, want openrouter endpoint", baseURL)
		}
		// The anthropic card must NOT show openrouter's live creds.
		_, _, anthKey, _, _ := orApp.providerDefaults("anthropic")
		if anthKey == "sk-or-live" {
			t.Errorf("anthropic card leaked openrouter creds")
		}
	})

	t.Run("llamacpp (non-active, blank endpoint)", func(t *testing.T) {
		_, baseURL, _, _, params := a.providerDefaults("llamacpp")
		if baseURL != "" {
			t.Errorf("baseURL = %q, want blank", baseURL)
		}
		if params["cache_prompt"] != true {
			t.Errorf("params[cache_prompt] = %v, want true", params["cache_prompt"])
		}
	})

	t.Run("anthropic (non-active)", func(t *testing.T) {
		_, _, _, _, params := a.providerDefaults("anthropic")
		if params["max_tokens"] != 16384 {
			t.Errorf("anthropic params[max_tokens] = %v, want 16384", params["max_tokens"])
		}
	})

	t.Run("openai (active) reflects config params", func(t *testing.T) {
		_, _, _, _, params := a.providerDefaults("openai")
		if params["reasoning_effort"] != "medium" {
			t.Errorf("openai params[reasoning_effort] = %v, want medium", params["reasoning_effort"])
		}
	})
}
