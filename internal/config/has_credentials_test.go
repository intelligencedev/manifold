package config

import "testing"

func TestHasPrimaryLLMCredentialsByBackend(t *testing.T) {
	t.Run("openrouter uses the openai key", func(t *testing.T) {
		cfg := &Config{}
		cfg.LLMClient.Provider = "openrouter"
		cfg.LLMClient.OpenAI.APIKey = "sk-or-test"
		if !HasPrimaryLLMCredentials(cfg) {
			t.Fatalf("expected openrouter with an OpenAI-sub-config key to be configured")
		}
	})

	t.Run("openrouter without a key is not configured", func(t *testing.T) {
		cfg := &Config{}
		cfg.LLMClient.Provider = "openrouter"
		if HasPrimaryLLMCredentials(cfg) {
			t.Fatalf("expected openrouter without a key to be unconfigured")
		}
	})

	t.Run("llamacpp needs no key", func(t *testing.T) {
		cfg := &Config{}
		cfg.LLMClient.Provider = "llamacpp"
		if !HasPrimaryLLMCredentials(cfg) {
			t.Fatalf("expected llamacpp (local server) to be considered configured without a key")
		}
	})

	t.Run("openai still uses the openai key", func(t *testing.T) {
		cfg := &Config{}
		cfg.LLMClient.Provider = "openai"
		if HasPrimaryLLMCredentials(cfg) {
			t.Fatalf("expected openai without a key to be unconfigured")
		}
		cfg.LLMClient.OpenAI.APIKey = "sk-test"
		if !HasPrimaryLLMCredentials(cfg) {
			t.Fatalf("expected openai with a key to be configured")
		}
	})
}
