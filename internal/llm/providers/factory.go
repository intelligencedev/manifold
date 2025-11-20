package providers

import (
	"fmt"
	"net/http"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/llm/anthropic"
	"manifold/internal/llm/google"
	openaillm "manifold/internal/llm/openai"
)

// Build constructs an llm.Provider based on the configured provider name.
// - openai: uses the OpenAI client
// - local: uses the OpenAI client with completions API
// - anthropic/google: stub providers for future implementation
func Build(cfg config.Config, httpClient *http.Client) (llm.Provider, error) {
	switch cfg.LLMClient.Provider {
	case "", "openai":
		return openaillm.New(cfg.LLMClient.OpenAI, httpClient), nil
	case "local":
		oc := cfg.LLMClient.OpenAI
		oc.API = "completions"
		return openaillm.New(oc, httpClient), nil
	case "anthropic":
		return anthropic.New(cfg.LLMClient.Anthropic, httpClient), nil
	case "google":
		return google.New(cfg.LLMClient.Google, httpClient), nil
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", cfg.LLMClient.Provider)
	}
}
