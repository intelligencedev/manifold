package agentd

import (
	"testing"

	"manifold/internal/config"
)

func TestApplyPrimaryLLMSettingsOpenRouter(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLMClient.Provider = "openai"

	err := applyPrimaryLLMSettings(cfg, agentdSettings{
		LLMProvider: "openrouter",
		LLMAPIKey:   "sk-or-settings",
		LLMModel:    "anthropic/claude-3.5-sonnet",
		// no base URL -> should default to the OpenRouter endpoint
	})
	if err != nil {
		t.Fatalf("applyPrimaryLLMSettings(openrouter) error: %v", err)
	}
	if cfg.LLMClient.Provider != "openrouter" {
		t.Errorf("provider = %q, want openrouter", cfg.LLMClient.Provider)
	}
	if cfg.LLMClient.OpenAI.APIKey != "sk-or-settings" {
		t.Errorf("OpenAI.APIKey = %q, want sk-or-settings", cfg.LLMClient.OpenAI.APIKey)
	}
	if cfg.LLMClient.OpenAI.Model != "anthropic/claude-3.5-sonnet" {
		t.Errorf("OpenAI.Model = %q", cfg.LLMClient.OpenAI.Model)
	}
	if cfg.LLMClient.OpenAI.BaseURL != "https://openrouter.ai/api/v1" || cfg.LLMClient.OpenAI.API != "responses" {
		t.Errorf("OpenAI config = %+v, want OpenRouter Responses endpoint", cfg.LLMClient.OpenAI)
	}
	if !config.HasPrimaryLLMCredentials(cfg) {
		t.Errorf("expected openrouter settings to be considered configured")
	}
}

func TestApplyPrimaryLLMSettingsYAMLOpenRouterBlock(t *testing.T) {
	root := map[string]any{}
	applyPrimaryLLMSettingsYAML(root, agentdSettings{
		LLMProvider: "openrouter",
		LLMAPIKey:   "sk-or",
		LLMModel:    "anthropic/claude-3.5-sonnet",
	})
	llm, _ := root["llm_client"].(map[string]any)
	openai, _ := llm["openai"].(map[string]any)
	if openai == nil || openai["apiKey"] != "sk-or" {
		t.Fatalf("expected openrouter creds under llm_client.openai, got: %#v", root)
	}
	if _, hasAnthropic := llm["anthropic"]; hasAnthropic {
		t.Fatalf("openrouter must not write an anthropic block: %#v", llm)
	}
}
