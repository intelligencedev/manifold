package agentd

import (
	"testing"

	"manifold/internal/config"
)

func TestShouldCheckEmbeddingReachability(t *testing.T) {
	t.Run("skips during onboarding when no primary LLM credentials", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.LLMClient.Provider = "openai" // default provider, but no API key yet
		if shouldCheckEmbeddingReachability(cfg) {
			t.Fatalf("expected reachability check to be skipped before onboarding")
		}
	})

	t.Run("runs once primary LLM credentials are configured", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.LLMClient.Provider = "openai"
		cfg.LLMClient.OpenAI.APIKey = "sk-configured"
		if !shouldCheckEmbeddingReachability(cfg) {
			t.Fatalf("expected reachability check to run for a configured install")
		}
	})

	t.Run("nil config skips", func(t *testing.T) {
		if shouldCheckEmbeddingReachability(nil) {
			t.Fatalf("expected nil config to skip the reachability check")
		}
	})
}
