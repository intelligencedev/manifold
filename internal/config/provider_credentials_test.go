package config

import "testing"

func TestProviderBackend(t *testing.T) {
	cases := map[string]string{
		"openai":     "openai",
		"local":      "openai",
		"llamacpp":   "openai",
		"openrouter": "openai",
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
	// OpenRouter reads/writes the OpenAI sub-config used by Responses.
	c := LLMClientConfig{Provider: "openrouter"}
	c.OpenAI.Model = "anthropic/claude-3.5-sonnet"
	c.OpenAI.APIKey = "sk-or"
	c.OpenAI.BaseURL = "https://openrouter.ai/api/v1"
	c.Anthropic.Model = "should-not-be-used"

	if got := c.ActiveModel(); got != "anthropic/claude-3.5-sonnet" {
		t.Errorf("ActiveModel = %q", got)
	}
	if got := c.ActiveAPIKey(); got != "sk-or" {
		t.Errorf("ActiveAPIKey = %q", got)
	}
	if got := c.ActiveBaseURL(); got != "https://openrouter.ai/api/v1" {
		t.Errorf("ActiveBaseURL = %q", got)
	}
}

func TestLLMClientConfigSetActive(t *testing.T) {
	c := &LLMClientConfig{Provider: "openrouter"}
	c.SetActiveModel("m")
	c.SetActiveAPIKey("k")
	c.SetActiveBaseURL("u")
	if c.OpenAI.Model != "m" || c.OpenAI.APIKey != "k" || c.OpenAI.BaseURL != "u" {
		t.Fatalf("openrouter setters did not target OpenAI sub-config: %+v", c.OpenAI)
	}
	if c.Anthropic.Model != "" {
		t.Fatalf("openrouter setter leaked into Anthropic sub-config")
	}

	o := &LLMClientConfig{Provider: "llamacpp"}
	o.SetActiveModel("lm")
	if o.OpenAI.Model != "lm" {
		t.Fatalf("llamacpp setter should target OpenAI sub-config, got %q", o.OpenAI.Model)
	}
}
