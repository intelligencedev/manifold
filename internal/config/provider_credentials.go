package config

import "strings"

// ProviderBackend returns the client backend ("openai", "anthropic", or
// "google") that serves the given provider. Unknown or empty providers default
// to "openai". This is the single place that maps a provider identifier to the
// LLM client and config sub-block that back it, so OpenAI-compatible variants
// (local, llamacpp) resolve to "openai" and OpenRouter resolves to "anthropic".
func ProviderBackend(provider string) string {
	if pd, ok := ProviderDefaults(provider); ok && pd.Backend != "" {
		return pd.Backend
	}
	return "openai"
}

// Backend returns the client backend for this config's provider.
func (c LLMClientConfig) Backend() string { return ProviderBackend(c.Provider) }

// ActiveModel returns the model from the sub-config backing the active provider.
func (c LLMClientConfig) ActiveModel() string {
	switch c.Backend() {
	case "anthropic":
		return strings.TrimSpace(c.Anthropic.Model)
	case "google":
		return strings.TrimSpace(c.Google.Model)
	default:
		return strings.TrimSpace(c.OpenAI.Model)
	}
}

// ActiveAPIKey returns the API key from the sub-config backing the active provider.
func (c LLMClientConfig) ActiveAPIKey() string {
	switch c.Backend() {
	case "anthropic":
		return strings.TrimSpace(c.Anthropic.APIKey)
	case "google":
		return strings.TrimSpace(c.Google.APIKey)
	default:
		return strings.TrimSpace(c.OpenAI.APIKey)
	}
}

// ActiveBaseURL returns the base URL from the sub-config backing the active provider.
func (c LLMClientConfig) ActiveBaseURL() string {
	switch c.Backend() {
	case "anthropic":
		return strings.TrimSpace(c.Anthropic.BaseURL)
	case "google":
		return strings.TrimSpace(c.Google.BaseURL)
	default:
		return strings.TrimSpace(c.OpenAI.BaseURL)
	}
}

// SetActiveModel writes model to the sub-config backing the active provider.
func (c *LLMClientConfig) SetActiveModel(model string) {
	switch c.Backend() {
	case "anthropic":
		c.Anthropic.Model = model
	case "google":
		c.Google.Model = model
	default:
		c.OpenAI.Model = model
	}
}

// SetActiveAPIKey writes key to the sub-config backing the active provider.
func (c *LLMClientConfig) SetActiveAPIKey(key string) {
	switch c.Backend() {
	case "anthropic":
		c.Anthropic.APIKey = key
	case "google":
		c.Google.APIKey = key
	default:
		c.OpenAI.APIKey = key
	}
}

// SetActiveBaseURL writes baseURL to the sub-config backing the active provider.
func (c *LLMClientConfig) SetActiveBaseURL(baseURL string) {
	switch c.Backend() {
	case "anthropic":
		c.Anthropic.BaseURL = baseURL
	case "google":
		c.Google.BaseURL = baseURL
	default:
		c.OpenAI.BaseURL = baseURL
	}
}
