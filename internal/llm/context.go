package llm

import "os"

// ContextSize returns an approximate context window (in tokens) for the given
// model name.
//
// It uses a small built-in map of popular models and then consults
// environment-variable overrides for custom/self‑hosted models. The bool
// indicates whether the value came from a known mapping or explicit override
// (true) versus a conservative default fallback (false).
func ContextSize(model string) (tokens int, known bool) {
	if model == "" {
		return 0, false
	}

	// Per‑model override takes precedence.
	if v, ok := lookupContextOverride(model); ok && v > 0 {
		return v, true
	}

	// Known model families.
	if size, ok := knownContextWindows[model]; ok {
		return size, true
	}
	for prefix, size := range knownContextWindows {
		if hasModelPrefix(model, prefix) {
			return size, true
		}
	}

	// Global override used as a catch‑all for unknown models.
	if v, ok := lookupContextOverride("*"); ok && v > 0 {
		return v, true
	}

	// Conservative default when we know nothing.
	return 32_000, false
}

// knownContextWindows holds approximate context sizes for common model
// families. Values are intentionally approximate; they are used only for
// memory budgeting, not for provider feature gating.
var knownContextWindows = map[string]int{
	// OpenAI GPT-5 / GPT-5.x
	"gpt-5.2":            400_000,
	"gpt-5.2-pro":        400_000,
	"gpt-5.1":            400_000,
	"gpt-5":              400_000,
	"gpt-5-mini":         400_000,
	"gpt-5-nano":         400_000,
	"gpt-5-codex":        400_000,
	"gpt-5.1-codex":      400_000,
	"gpt-5.1-codex-mini": 400_000,
	"gpt-5.1-codex-max":  400_000,

	// (Optional) keep if you historically used these string keys; OpenAI docs publish gpt-5-mini/nano (not version-suffixed)
	"gpt-5.1-mini": 400_000,
	"gpt-5.1-nano": 400_000,

	// OpenAI GPT-4o / GPT-4.x
	"gpt-4o":      128_000,
	"gpt-4o-mini": 128_000,

	// GPT-4.1 family (note: ~1M context window)
	"gpt-4.1":      1_047_576,
	"gpt-4.1-mini": 1_047_576,
	"gpt-4.1-nano": 1_047_576,

	"gpt-4-turbo":   128_000,
	"gpt-4":         8_192,
	"gpt-3.5-turbo": 16_385,

	// OpenAI Realtime (preview)
	"gpt-4o-mini-realtime-preview": 16_000,

	// Anthropic Claude 4.5 (Sonnet can do 1M with a beta header; default is 200K)
	"claude-sonnet-4-5": 200_000,
	"claude-haiku-4-5":  200_000,
	"claude-opus-4-5":   200_000,

	// Anthropic snapshot IDs (published alongside aliases)
	"claude-sonnet-4-5-20250929": 200_000,
	"claude-haiku-4-5-20251001":  200_000,
	"claude-opus-4-5-20251101":   200_000,

	// Anthropic Claude 3.x / 3.5 (kept for compatibility)
	"claude-3.5":        200_000,
	"claude-3-opus":     200_000,
	"claude-3-sonnet":   200_000,
	"claude-3-haiku":    200_000,
	"claude-3.5-sonnet": 200_000,

	// Google Gemini (token limits shown in docs are *input* token limits; output limit is separate)
	"gemini-3-pro-preview":  1_048_576,
	"gemini-2.5-pro":        1_048_576,
	"gemini-2.5-flash":      1_048_576,
	"gemini-2.5-flash-lite": 1_048_576,

	// Previous Gemini models still listed in the Gemini API docs
	"gemini-2.0-flash":      1_048_576,
	"gemini-2.0-flash-lite": 1_048_576,

	// Legacy (may be discontinued depending on the Google product surface)
	"gemini-1.5-pro":   1_000_000,
	"gemini-1.5-flash": 1_000_000,
	"gemini-1.0-pro":   32_000,
}

// lookupContextOverride checks for environment overrides.
//
// Precedence:
//  1. MODEL_<SANITIZED_NAME>_CONTEXT_TOKENS
//  2. MEMORY_AUTO_CONTEXT_WINDOW_TOKENS (global catch‑all)
//
// When model == "*", only the global override is consulted.
func lookupContextOverride(model string) (int, bool) {
	if model == "*" {
		if v := os.Getenv("MEMORY_AUTO_CONTEXT_WINDOW_TOKENS"); v != "" {
			if n, ok := parseIntEnv(v); ok {
				return n, true
			}
		}
		return 0, false
	}

	// Per‑model override.
	key := "MODEL_" + sanitizeModelForEnv(model) + "_CONTEXT_TOKENS"
	if v := os.Getenv(key); v != "" {
		if n, ok := parseIntEnv(v); ok {
			return n, true
		}
	}

	// Global catch‑all.
	if v := os.Getenv("MEMORY_AUTO_CONTEXT_WINDOW_TOKENS"); v != "" {
		if n, ok := parseIntEnv(v); ok {
			return n, true
		}
	}

	return 0, false
}

// sanitizeModelForEnv converts a model name into an env‑var‑friendly token.
func sanitizeModelForEnv(model string) string {
	out := make([]rune, 0, len(model))
	for _, r := range model {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// hasModelPrefix treats prefix matches as sufficient to select a context size.
// This allows e.g. "gpt-4o-mini-2024-07-18" to match "gpt-4o-mini".
func hasModelPrefix(model, prefix string) bool {
	if len(model) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if model[i] != prefix[i] {
			return false
		}
	}
	return true
}

// parseIntEnv parses a non‑negative int from an environment variable string.
func parseIntEnv(v string) (int, bool) {
	if v == "" {
		return 0, false
	}
	n := 0
	found := false
	for _, r := range v {
		if r < '0' || r > '9' {
			continue
		}
		found = true
		n = n*10 + int(r-'0')
	}
	if !found {
		return 0, false
	}
	return n, true
}
