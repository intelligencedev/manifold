package providers

import (
	"fmt"
	"net/http"
	"strings"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/llm/anthropic"
	"manifold/internal/llm/google"
	openaillm "manifold/internal/llm/openai"
)

// Build constructs an llm.Provider based on the configured provider name.
// - openai: uses the OpenAI client
// - local: uses the OpenAI client with completions API
// - anthropic/google: providers backed by vendor SDKs
func Build(cfg config.Config, httpClient *http.Client) (llm.Provider, error) {
	return BuildFromLLMClientConfig(cfg.LLMClient, httpClient)
}

// BuildFromLLMClientConfig constructs an llm.Provider from an LLM client config.
func BuildFromLLMClientConfig(cfg config.LLMClientConfig, httpClient *http.Client) (llm.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "openai":
		return openaillm.New(cfg.OpenAI, httpClient), nil
	case "local":
		oc := cfg.OpenAI
		oc.API = "completions"
		return openaillm.New(oc, httpClient), nil
	case "anthropic":
		return anthropic.New(cfg.Anthropic, httpClient), nil
	case "google":
		g, err := google.New(cfg.Google, httpClient)
		if err != nil {
			return nil, err
		}
		return g, nil
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", cfg.Provider)
	}
}
