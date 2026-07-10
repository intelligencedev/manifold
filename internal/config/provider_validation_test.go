package config

import "testing"

func TestValidateProviderAcceptsKnownProviders(t *testing.T) {
	for _, p := range []string{"openai", "anthropic", "google", "local", "openrouter", "llamacpp"} {
		if err := validateProvider("llm_client.provider", p); err != nil {
			t.Errorf("validateProvider(%q) unexpected error: %v", p, err)
		}
	}
	if err := validateProvider("llm_client.provider", "bogus"); err == nil {
		t.Errorf("expected error for unknown provider")
	}
}
