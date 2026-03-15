package agentd

import (
	"testing"

	"manifold/internal/config"
	anthropicllm "manifold/internal/llm/anthropic"
	googlellm "manifold/internal/llm/google"
	openaillm "manifold/internal/llm/openai"
)

func TestVisionClientSelectionSupportsCompaction(t *testing.T) {
	t.Parallel()

	googleClient, err := googlellm.New(config.GoogleConfig{BaseURL: "http://127.0.0.1:1", APIKey: "test", Model: "gemini-1.5-flash"}, nil)
	if err != nil {
		t.Fatalf("build google client: %v", err)
	}

	tests := []struct {
		name string
		sel  visionClientSelection
		want bool
	}{
		{
			name: "openai supports compaction",
			sel:  visionClientSelection{OpenAI: openaillm.New(config.OpenAIConfig{APIKey: "test", BaseURL: "http://127.0.0.1:1", Model: "gpt-5.4"}, nil)},
			want: true,
		},
		{
			name: "anthropic does not support compaction",
			sel:  visionClientSelection{Anthropic: anthropicllm.New(config.AnthropicConfig{APIKey: "test", BaseURL: "http://127.0.0.1:1", Model: "claude-3-7-sonnet"}, nil)},
			want: false,
		},
		{
			name: "google does not support compaction",
			sel:  visionClientSelection{Google: googleClient},
			want: false,
		},
		{
			name: "empty selection does not support compaction",
			sel:  visionClientSelection{},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.sel.supportsCompaction(); got != tc.want {
				t.Fatalf("supportsCompaction() = %v, want %v", got, tc.want)
			}
		})
	}
}
