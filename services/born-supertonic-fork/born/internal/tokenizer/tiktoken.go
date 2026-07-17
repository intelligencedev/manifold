package tokenizer

import (
	"fmt"

	"github.com/pkoukk/tiktoken-go"
)

const (
	// encodingCL100kBase is the encoding name for GPT-4 and GPT-3.5-turbo.
	encodingCL100kBase = "cl100k_base"
	// encodingP50kBase is the encoding name for GPT-3.
	encodingP50kBase = "p50k_base"
	// encodingR50kBase is the encoding name for older GPT-3 models.
	encodingR50kBase = "r50k_base"
)

// TikToken wraps the pkoukk/tiktoken-go library for OpenAI tokenizers.
//
// Supported encodings:
//   - cl100k_base: GPT-4, GPT-3.5-turbo, text-embedding-ada-002
//   - p50k_base: GPT-3, Codex
//   - r50k_base: GPT-3, davinci-002, babbage-002
type TikToken struct {
	encoding *tiktoken.Tiktoken
	name     string
}

// NewTikToken creates a new TikToken tokenizer with the specified encoding.
//
// Supported encodings: "cl100k_base" (GPT-4), "p50k_base" (GPT-3).
func NewTikToken(encodingName string) (*TikToken, error) {
	encoding, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		return nil, fmt.Errorf("failed to load tiktoken encoding %q: %w", encodingName, err)
	}

	return &TikToken{
		encoding: encoding,
		name:     encodingName,
	}, nil
}

// NewTikTokenForModel creates a TikToken tokenizer for a specific model.
//
// Example models: "gpt-4", "gpt-3.5-turbo", "text-embedding-ada-002".
func NewTikTokenForModel(modelName string) (*TikToken, error) {
	encoding, err := tiktoken.EncodingForModel(modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to load tiktoken for model %q: %w", modelName, err)
	}

	return &TikToken{
		encoding: encoding,
		name:     modelName,
	}, nil
}

// Encode converts text to token IDs.
func (t *TikToken) Encode(text string) ([]int32, error) {
	tokens := t.encoding.Encode(text, nil, nil)

	// Convert []int to []int32.
	result := make([]int32, len(tokens))
	for i, tok := range tokens {
		result[i] = int32(tok) //nolint:gosec // G115: safe, token IDs from tiktoken fit in int32
	}

	return result, nil
}

// Decode converts token IDs back to text.
func (t *TikToken) Decode(tokens []int32) (string, error) {
	// Convert []int32 to []int.
	intTokens := make([]int, len(tokens))
	for i, tok := range tokens {
		intTokens[i] = int(tok)
	}

	text := t.encoding.Decode(intTokens)
	return text, nil
}

// VocabSize returns the total vocabulary size.
func (t *TikToken) VocabSize() int {
	// tiktoken-go doesn't expose vocab size directly.
	// cl100k_base: ~100,000, p50k_base: ~50,000.
	// We'll return a conservative estimate.
	switch t.name {
	case encodingCL100kBase:
		return 100256 // Actual vocab size for cl100k_base
	case encodingP50kBase, encodingR50kBase:
		return 50257 // Actual vocab size for p50k_base
	default:
		return 100000 // Conservative default
	}
}

// BosToken returns the beginning-of-sequence token ID.
// tiktoken doesn't use BOS tokens, returns -1.
func (t *TikToken) BosToken() int32 {
	return -1
}

// EosToken returns the end-of-sequence token ID.
// tiktoken uses <|endoftext|> (token 50256 for cl100k_base, 50256 for p50k_base).
func (t *TikToken) EosToken() int32 {
	switch t.name {
	case encodingCL100kBase:
		return 100257 // <|endoftext|> for cl100k_base
	case encodingP50kBase, encodingR50kBase:
		return 50256 // <|endoftext|> for p50k_base
	default:
		return -1
	}
}

// PadToken returns the padding token ID.
// tiktoken doesn't define a padding token, returns -1.
func (t *TikToken) PadToken() int32 {
	return -1
}

// UnkToken returns the unknown token ID.
// tiktoken handles unknown tokens via BPE fallback, returns -1.
func (t *TikToken) UnkToken() int32 {
	return -1
}

// IsSpecialToken checks if a token ID is a special token.
//
// For tiktoken, special tokens are primarily <|endoftext|> and role markers.
func (t *TikToken) IsSpecialToken(token int32) bool {
	// Check against EOS token.
	if token == t.EosToken() {
		return true
	}

	// cl100k_base special tokens: 100256-100276 (ChatML tokens).
	if t.name == encodingCL100kBase && token >= 100256 && token <= 100276 {
		return true
	}

	return false
}

// Name returns the tokenizer name.
func (t *TikToken) Name() string {
	return t.name
}
