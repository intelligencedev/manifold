package specialists

import (
	"testing"

	"manifold/internal/config"
	"manifold/internal/persistence"

	"github.com/stretchr/testify/require"
)

func TestApplyOrchestratorConfig_OpenAI(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		LLMClient: config.LLMClientConfig{Provider: "openai"},
		OpenAI:    config.OpenAIConfig{APIKey: "orig"},
	}
	sp := persistence.Specialist{
		BaseURL:      "https://example.com",
		APIKey:       "key",
		Model:        "model",
		EnableTools:  true,
		AllowTools:   []string{"a", "b"},
		System:       "system",
		ExtraHeaders: map[string]string{"x": "y"},
		ExtraParams:  map[string]any{"temp": 0.2},
	}

	provider := ApplyOrchestratorConfig(cfg, sp)

	require.Equal(t, "openai", provider)
	require.Equal(t, "key", cfg.LLMClient.OpenAI.APIKey)
	require.Equal(t, "model", cfg.LLMClient.OpenAI.Model)
	require.Equal(t, "https://example.com", cfg.LLMClient.OpenAI.BaseURL)
	require.Equal(t, "system", cfg.SystemPrompt)
	require.True(t, cfg.EnableTools)
	require.Equal(t, []string{"a", "b"}, cfg.ToolAllowList)
	require.Equal(t, cfg.LLMClient.OpenAI, cfg.OpenAI)
}

func TestApplyLLMClientOverride_OpenAI(t *testing.T) {
	t.Parallel()

	base := config.LLMClientConfig{
		Provider: "openai",
		OpenAI:   config.OpenAIConfig{APIKey: "orig"},
	}
	sp := persistence.Specialist{
		BaseURL:      "https://example.com",
		APIKey:       "key",
		Model:        "model",
		ExtraHeaders: map[string]string{"x": "y"},
		ExtraParams:  map[string]any{"temp": 0.7},
	}

	got, provider := ApplyLLMClientOverride(base, sp)

	require.Equal(t, "openai", provider)
	require.Equal(t, "https://example.com", got.OpenAI.BaseURL)
	require.Equal(t, "key", got.OpenAI.APIKey)
	require.Equal(t, "model", got.OpenAI.Model)
	require.Equal(t, map[string]string{"x": "y"}, got.OpenAI.ExtraHeaders)
	require.Equal(t, map[string]any{"temp": 0.7}, got.OpenAI.ExtraParams)
}

func TestApplyOrchestratorConfig_Anthropic(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		LLMClient: config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{APIKey: "orig"}},
		OpenAI:    config.OpenAIConfig{APIKey: "orig"},
	}
	sp := persistence.Specialist{
		Provider: "anthropic",
		BaseURL:  "https://anthropic.example",
		APIKey:   "anthro-key",
		Model:    "claude",
	}

	provider := ApplyOrchestratorConfig(cfg, sp)

	require.Equal(t, "anthropic", provider)
	require.Equal(t, "https://anthropic.example", cfg.LLMClient.Anthropic.BaseURL)
	require.Equal(t, "anthro-key", cfg.LLMClient.Anthropic.APIKey)
	require.Equal(t, "claude", cfg.LLMClient.Anthropic.Model)
	require.Equal(t, "orig", cfg.OpenAI.APIKey)
}
