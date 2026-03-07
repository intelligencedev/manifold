package llm

import "testing"

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"a", 1},           // 1/4 + 1 = 1
		{"hello", 2},       // 5/4 + 1 = 2
		{"hello world", 3}, // 11/4 + 1 = 3
		{"this is a longer sentence for testing", 10}, // 38/4 + 1 = 10
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := EstimateTokens(tt.input)
			if got != tt.expected {
				t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEstimateTokensForMessages(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
	}

	total := EstimateTokensForMessages(msgs)
	expected := EstimateTokens("You are a helpful assistant.") + EstimateTokens("Hello")

	if total != expected {
		t.Errorf("EstimateTokensForMessages() = %d, want %d", total, expected)
	}
}
