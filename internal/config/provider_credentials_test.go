package config

import "testing"

func TestProviderBackend(t *testing.T) {
	cases := map[string]string{
		"openai":     "openai",
		"local":      "openai",
		"llamacpp":   "openai",
		"openrouter": "anthropic",
		"anthropic":  "anthropic",
		"google":     "google",
		"":           "openai",
		"unknown":    "openai",
	}
	for provider, want := range cases {
		if got := ProviderBackend(provider); got != want {
			t.Errorf("ProviderBackend(%q) = %q, want %q", provider, got, want)
		}
	}
}

func TestLLMClientConfigActiveAccessors(t *testing.T) {
	// OpenRouter (anthropic-backed) reads/writes the Anthropic sub-config.
	c := LLMClientConfig{Provider: "openrouter"}
	c.Anthropic.Model = "anthropic/claude-3.5-sonnet"
	c.Anthropic.APIKey = "sk-or"
	c.Anthropic.BaseURL = "https://openrouter.ai/api"
	// The OpenAI sub-config must be ignored for openrouter.
	c.OpenAI.Model = "gpt-should-not-be-used"

	if got := c.ActiveModel(); got != "anthropic/claude-3.5-sonnet" {
		t.Errorf("ActiveModel = %q", got)
	}
	if got := c.ActiveAPIKey(); got != "sk-or" {
		t.Errorf("ActiveAPIKey = %q", got)
	}
	if got := c.ActiveBaseURL(); got != "https://openrouter.ai/api" {
		t.Errorf("ActiveBaseURL = %q", got)
	}
}

func TestLLMClientConfigSetActive(t *testing.T) {
	c := &LLMClientConfig{Provider: "openrouter"}
	c.SetActiveModel("m")
	c.SetActiveAPIKey("k")
	c.SetActiveBaseURL("u")
	if c.Anthropic.Model != "m" || c.Anthropic.APIKey != "k" || c.Anthropic.BaseURL != "u" {
		t.Fatalf("openrouter setters did not target Anthropic sub-config: %+v", c.Anthropic)
	}
	if c.OpenAI.Model != "" {
		t.Fatalf("openrouter setter leaked into OpenAI sub-config")
	}

	o := &LLMClientConfig{Provider: "llamacpp"}
	o.SetActiveModel("lm")
	if o.OpenAI.Model != "lm" {
		t.Fatalf("llamacpp setter should target OpenAI sub-config, got %q", o.OpenAI.Model)
	}
}
