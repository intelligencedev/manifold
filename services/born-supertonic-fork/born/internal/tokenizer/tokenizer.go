package tokenizer

// Tokenizer is the core interface for text tokenization.
//
// All tokenizer implementations (tiktoken, BPE, etc.) must implement this interface.
type Tokenizer interface {
	// Encode converts text to token IDs.
	Encode(text string) ([]int32, error)

	// Decode converts token IDs back to text.
	Decode(tokens []int32) (string, error)

	// VocabSize returns the total vocabulary size.
	VocabSize() int

	// BosToken returns the beginning-of-sequence token ID.
	// Returns -1 if not applicable.
	BosToken() int32

	// EosToken returns the end-of-sequence token ID.
	// Returns -1 if not applicable.
	EosToken() int32

	// PadToken returns the padding token ID.
	// Returns -1 if not applicable.
	PadToken() int32

	// UnkToken returns the unknown token ID.
	// Returns -1 if not applicable.
	UnkToken() int32

	// IsSpecialToken checks if a token ID is a special token.
	IsSpecialToken(token int32) bool
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	// Role specifies the message role ("system", "user", "assistant").
	Role string

	// Content is the message text.
	Content string
}

// ChatTemplate formats messages for conversational models.
type ChatTemplate interface {
	// Apply formats a sequence of messages into a prompt string.
	Apply(messages []ChatMessage) string

	// Name returns the template name (e.g., "ChatML", "LLaMA").
	Name() string
}
