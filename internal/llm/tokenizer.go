package llm

import "context"

// Tokenizer provides accurate token counting for a specific provider.
type Tokenizer interface {
	// CountTokens returns the number of tokens in the given text.
	// Returns an error if tokenization fails.
	CountTokens(ctx context.Context, text string) (int, error)

	// CountMessagesTokens returns token count for a conversation.
	// This accounts for message formatting overhead (roles, separators, etc.)
	CountMessagesTokens(ctx context.Context, msgs []Message) (int, error)
}

// TokenizableProvider is an optional interface that providers can implement
// to offer accurate token counting.
type TokenizableProvider interface {
	Provider
	Tokenizer() Tokenizer
}

// EstimateTokens provides a heuristic fallback (chars/4) when accurate
// tokenization is unavailable.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	// Simple heuristic: 4 characters per token on average.
	return len([]rune(s))/4 + 1
}

// EstimateTokensForMessages provides a rough token estimate for a slice
// of messages by summing EstimateTokens over their content.
func EstimateTokensForMessages(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.Content)
	}
	return total
}
