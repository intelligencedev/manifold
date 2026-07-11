package providers

import (
	"testing"

	"manifold/internal/config"
	openaillm "manifold/internal/llm/openai"
)

func TestBuildLlamaCppUsesOpenAIClient(t *testing.T) {
	cfg := config.LLMClientConfig{Provider: "llamacpp"}
	cfg.OpenAI.Model = "test-model"

	p, err := BuildFromLLMClientConfig(cfg, nil)
	if err != nil {
		t.Fatalf("BuildFromLLMClientConfig(llamacpp) error: %v", err)
	}
	if _, ok := p.(*openaillm.Client); !ok {
		t.Fatalf("expected *openai.Client for llamacpp, got %T", p)
	}
}

func TestBuildOpenRouterUsesOpenAIResponsesClient(t *testing.T) {
	cfg := config.LLMClientConfig{Provider: "openrouter"}
	cfg.OpenAI.Model = "openai/gpt-5"
	cfg.OpenAI.BaseURL = "https://openrouter.ai/api/v1"

	p, err := BuildFromLLMClientConfig(cfg, nil)
	if err != nil {
		t.Fatalf("BuildFromLLMClientConfig(openrouter) error: %v", err)
	}
	if _, ok := p.(*openaillm.Client); !ok {
		t.Fatalf("expected *openai.Client for openrouter, got %T", p)
	}
}

func TestBuildUnknownProviderStillErrors(t *testing.T) {
	if _, err := BuildFromLLMClientConfig(config.LLMClientConfig{Provider: "nope"}, nil); err == nil {
		t.Fatalf("expected error for unknown provider")
	}
}
