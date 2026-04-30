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

func TestSummaryEndpointSupportsResponsesCompaction(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		api      string
		baseURL  string
		expect   bool
	}{
		{
			name:     "openai responses native endpoint",
			provider: "openai",
			api:      "responses",
			baseURL:  "https://api.openai.com/v1",
			expect:   true,
		},
		{
			name:     "local openai compatible endpoint",
			provider: "openai",
			api:      "responses",
			baseURL:  "http://192.168.1.46:32182/v1",
			expect:   false,
		},
		{
			name:     "non responses api",
			provider: "openai",
			api:      "completions",
			baseURL:  "https://api.openai.com/v1",
			expect:   false,
		},
		{
			name:     "non openai provider",
			provider: "anthropic",
			api:      "responses",
			baseURL:  "https://api.openai.com/v1",
			expect:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summaryEndpointSupportsResponsesCompaction(tt.provider, tt.api, tt.baseURL)
			if got != tt.expect {
				t.Fatalf("summaryEndpointSupportsResponsesCompaction() = %v, want %v", got, tt.expect)
			}
		})
	}
}
