package agentd

import (
	"testing"

	"manifold/internal/config"
)

func TestResolveLLMClientModelByBackend(t *testing.T) {
	t.Run("openrouter uses the openai model", func(t *testing.T) {
		cfg := config.LLMClientConfig{Provider: "openrouter"}
		cfg.OpenAI.Model = "anthropic/claude-3.5-sonnet"
		if got := resolveLLMClientModel(cfg); got != "anthropic/claude-3.5-sonnet" {
			t.Fatalf("resolveLLMClientModel(openrouter) = %q, want OpenAI-sub-config model", got)
		}
	})

	t.Run("llamacpp uses the openai model", func(t *testing.T) {
		cfg := config.LLMClientConfig{Provider: "llamacpp"}
		cfg.OpenAI.Model = "local-model"
		if got := resolveLLMClientModel(cfg); got != "local-model" {
			t.Fatalf("resolveLLMClientModel(llamacpp) = %q, want local-model", got)
		}
	})

	t.Run("openai unchanged", func(t *testing.T) {
		cfg := config.LLMClientConfig{Provider: "openai"}
		cfg.OpenAI.Model = "gpt-5-mini"
		if got := resolveLLMClientModel(cfg); got != "gpt-5-mini" {
			t.Fatalf("resolveLLMClientModel(openai) = %q, want gpt-5-mini", got)
		}
	})
}
