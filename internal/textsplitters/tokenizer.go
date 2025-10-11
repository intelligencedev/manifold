package textsplitters

import "strings"

// Tokenizer provides tokenization for token-based splitting.
// Implementations should be stateless or concurrency-safe.
type Tokenizer interface {
	Tokenize(text string) []string
	Detokenize(tokens []string) string
}

// WhitespaceTokenizer is a simple tokenizer that splits on runs of
// whitespace and detokenizes by joining with a single space.
type WhitespaceTokenizer struct{}

func (WhitespaceTokenizer) Tokenize(text string) []string {
	return strings.Fields(text)
}

func (WhitespaceTokenizer) Detokenize(tokens []string) string {
	return strings.Join(tokens, " ")
}
