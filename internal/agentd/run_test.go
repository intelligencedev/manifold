package agentd

import (
	"testing"

	"manifold/internal/config"
)

func TestResolveEvolvingMemoryLLMUsesDedicatedLLMClient(t *testing.T) {
	cfg := &config.Config{
		LLMClient: config.LLMClientConfig{
			Provider: "openai",
			OpenAI: config.OpenAIConfig{
				APIKey:  "base-key",
				BaseURL: "https://api.openai.com/v1",
				Model:   "base-model",
			},
		},
		EvolvingMemory: config.EvolvingMemoryConfig{
			LLMClient: config.LLMClientConfig{
				Provider: "local",
				OpenAI: config.OpenAIConfig{
					BaseURL: "http://localhost:11434/v1",
					Model:   "memory-local-model",
				},
			},
		},
	}

	provider, model, providerName, err := resolveEvolvingMemoryLLM(cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("resolveEvolvingMemoryLLM() error: %v", err)
	}
	if provider == nil {
		t.Fatalf("expected provider to be constructed")
	}
	if providerName != "local" {
		t.Fatalf("expected provider local, got %q", providerName)
	}
	if model != "memory-local-model" {
		t.Fatalf("expected memory-local-model, got %q", model)
	}
}
