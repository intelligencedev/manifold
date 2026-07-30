package config

import "testing"

func TestSeedsOpenRouterDefaultsWhenEmpty(t *testing.T) {
	cfg := &Config{}
	cfg.LLMClient.Provider = "openrouter"

	applyLLMDefaults(cfg)

	if got := cfg.LLMClient.OpenAI.BaseURL; got != "https://openrouter.ai/api/v1" {
		t.Errorf("OpenAI.BaseURL = %q, want openrouter endpoint", got)
	}
	if got := cfg.LLMClient.OpenAI.API; got != "responses" {
		t.Errorf("OpenAI.API = %q, want responses", got)
	}
	if got := cfg.LLMClient.OpenAI.ExtraParams["max_output_tokens"]; got != 16384 {
		t.Errorf("OpenAI.ExtraParams[max_output_tokens] = %v, want 16384", got)
	}
	if _, ok := cfg.LLMClient.OpenAI.ExtraParams["reasoning"]; !ok {
		t.Errorf("expected openrouter reasoning param to be seeded; got %v", cfg.LLMClient.OpenAI.ExtraParams)
	}
}

func TestSeedsAnthropicDefaultsWhenEmpty(t *testing.T) {
	cfg := &Config{}
	cfg.LLMClient.Provider = "anthropic"

	applyLLMDefaults(cfg)

	if got := cfg.LLMClient.Anthropic.ExtraParams["max_tokens"]; got != 16384 {
		t.Errorf("Anthropic.ExtraParams[max_tokens] = %v, want 16384", got)
	}
	if _, ok := cfg.LLMClient.Anthropic.ExtraParams["thinking"]; !ok {
		t.Errorf("expected anthropic thinking param to be seeded; got %v", cfg.LLMClient.Anthropic.ExtraParams)
	}
}

func TestPreservesConfiguredExtraParams(t *testing.T) {
	cfg := &Config{}
	cfg.LLMClient.Provider = "anthropic"
	cfg.LLMClient.Anthropic.ExtraParams = map[string]any{"custom_key": true}

	applyLLMDefaults(cfg)

	if _, ok := cfg.LLMClient.Anthropic.ExtraParams["custom_key"]; !ok {
		t.Fatalf("existing ExtraParams were dropped: %v", cfg.LLMClient.Anthropic.ExtraParams)
	}
	if _, ok := cfg.LLMClient.Anthropic.ExtraParams["max_tokens"]; ok {
		t.Fatalf("defaults were merged into user-configured ExtraParams; want left untouched: %v", cfg.LLMClient.Anthropic.ExtraParams)
	}
}

func TestSeedsOpenAIDefaultsKeepsV1BaseURL(t *testing.T) {
	cfg := &Config{}
	cfg.LLMClient.Provider = "openai"

	applyLLMDefaults(cfg)

	if got := cfg.LLMClient.OpenAI.BaseURL; got != OpenAIAPIV1BaseURL {
		t.Errorf("OpenAI.BaseURL = %q, want %q", got, OpenAIAPIV1BaseURL)
	}
	if got := cfg.LLMClient.OpenAI.ExtraParams["reasoning_effort"]; got != "medium" {
		t.Errorf("OpenAI.ExtraParams[reasoning_effort] = %v, want medium", got)
	}
}
