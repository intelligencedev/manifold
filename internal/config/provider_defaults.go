package config

import "strings"

// ProviderDefault describes the built-in defaults for an LLM provider: which
// client backs it, the default API surface and base URL, and the request
// ExtraParams pre-populated for onboarding and new specialists.
type ProviderDefault struct {
	// Backend selects the client and config sub-block that serve the provider:
	// "openai" (OpenAI-compatible), "anthropic" (Anthropic Messages API), or
	// "google". Providers sharing a Backend share that sub-config.
	Backend string
	// API is the OpenAI-compatible surface: "responses", "completions", or ""
	// (unused by non-openai backends).
	API string
	// BaseURL is the default endpoint; "" means the user must supply one.
	BaseURL string
	// ExtraParams are the default request params for the provider.
	ExtraParams map[string]any
}

// knownProviders is the supported provider identifiers in display order.
var knownProviders = []string{"openai", "anthropic", "google", "openrouter", "llamacpp", "local"}

// KnownProviders returns the supported provider identifiers in display order.
func KnownProviders() []string {
	return append([]string(nil), knownProviders...)
}

// providerDefaults holds the canonical defaults. Callers receive deep copies via
// ProviderDefaults so the canonical ExtraParams cannot be mutated.
var providerDefaults = map[string]ProviderDefault{
	"openai": {
		Backend: "openai",
		API:     "responses",
		BaseURL: OpenAIAPIV1BaseURL,
		ExtraParams: map[string]any{
			"prompt_cache_key":    "manifold-orchestrator",
			"parallel_tool_calls": true,
			"reasoning_effort":    "medium",
			"summary":             "auto",
		},
	},
	"anthropic": {
		Backend: "anthropic",
		BaseURL: "https://api.anthropic.com",
		ExtraParams: map[string]any{
			"cache_control": map[string]any{"type": "ephemeral"},
			"max_tokens":    16384,
			"thinking":      map[string]any{"type": "ephemeral"},
		},
	},
	"google": {
		Backend: "google",
		BaseURL: "https://generativelanguage.googleapis.com/",
		ExtraParams: map[string]any{
			"generation_config": map[string]any{
				"thinking_level":     "medium",
				"thinking_summaries": "auto",
			},
		},
	},
	"openrouter": {
		// OpenRouter exposes an OpenAI-compatible Responses API at /api/v1/responses.
		Backend: "openai",
		API:     "responses",
		BaseURL: "https://openrouter.ai/api/v1",
		ExtraParams: map[string]any{
			"max_output_tokens":   16384,
			"parallel_tool_calls": true,
			"reasoning":           map[string]any{"effort": "medium"},
		},
	},
	"llamacpp": {
		Backend: "openai",
		API:     "responses",
		BaseURL: "",
		ExtraParams: map[string]any{
			"cache_prompt": true,
		},
	},
	"local": {
		Backend:     "openai",
		API:         "completions",
		BaseURL:     "",
		ExtraParams: map[string]any{},
	},
}

// NormalizeProvider lower-cases and trims a provider identifier.
func NormalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// ProviderDefaults returns a deep copy of the built-in defaults for a provider
// identifier (case-insensitive, whitespace-trimmed). ok is false when unknown.
func ProviderDefaults(provider string) (ProviderDefault, bool) {
	pd, ok := providerDefaults[NormalizeProvider(provider)]
	if !ok {
		return ProviderDefault{}, false
	}
	pd.ExtraParams = deepCopyAnyMap(pd.ExtraParams)
	return pd, true
}

// DefaultProviderExtraParams returns a deep copy of a provider's default request
// params (empty, non-nil map when the provider is unknown or has none).
func DefaultProviderExtraParams(provider string) map[string]any {
	pd, ok := ProviderDefaults(provider)
	if !ok || pd.ExtraParams == nil {
		return map[string]any{}
	}
	return pd.ExtraParams
}

// deepCopyAnyMap returns a recursive copy of a map[string]any so nested default
// maps are not shared with callers.
func deepCopyAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if nested, ok := v.(map[string]any); ok {
			out[k] = deepCopyAnyMap(nested)
			continue
		}
		out[k] = v
	}
	return out
}
