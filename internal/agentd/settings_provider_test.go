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
	if cfg.LLMClient.Anthropic.APIKey != "sk-or-settings" {
		t.Errorf("Anthropic.APIKey = %q, want sk-or-settings", cfg.LLMClient.Anthropic.APIKey)
	}
	if cfg.LLMClient.Anthropic.Model != "anthropic/claude-3.5-sonnet" {
		t.Errorf("Anthropic.Model = %q", cfg.LLMClient.Anthropic.Model)
	}
	if cfg.LLMClient.Anthropic.BaseURL != "https://openrouter.ai/api" {
		t.Errorf("Anthropic.BaseURL = %q, want openrouter endpoint", cfg.LLMClient.Anthropic.BaseURL)
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
	anth, _ := llm["anthropic"].(map[string]any)
	if anth == nil || anth["apiKey"] != "sk-or" {
		t.Fatalf("expected openrouter creds under llm_client.anthropic, got: %#v", root)
	}
	if _, hasOpenAI := llm["openai"]; hasOpenAI {
		t.Fatalf("openrouter must not write an openai block: %#v", llm)
	}
}
