package tokenizer

import (
	"strings"
	"unicode"
)

// Normalizer transforms text before tokenization.
type Normalizer interface {
	Normalize(input string) string
}

// SequenceNormalizer applies a chain of normalizers in order.
type SequenceNormalizer struct {
	Normalizers []Normalizer
}

// Normalize applies each normalizer in sequence.
func (s *SequenceNormalizer) Normalize(input string) string {
	for _, n := range s.Normalizers {
		input = n.Normalize(input)
	}
	return input
}

// PrependNormalizer prepends a fixed string to the input.
type PrependNormalizer struct {
	Prepend string
}

// Normalize prepends the configured string.
func (p *PrependNormalizer) Normalize(input string) string {
	return p.Prepend + input
}

// ReplaceNormalizer replaces all occurrences of a pattern with content.
type ReplaceNormalizer struct {
	Pattern string
	Content string
}

// Normalize replaces all pattern occurrences.
func (r *ReplaceNormalizer) Normalize(input string) string {
	return strings.ReplaceAll(input, r.Pattern, r.Content)
}

// LowercaseNormalizer converts text to lowercase.
type LowercaseNormalizer struct{}

// Normalize lowercases the input.
func (l *LowercaseNormalizer) Normalize(input string) string {
	return strings.ToLower(input)
}

// StripNormalizer removes leading and trailing whitespace.
type StripNormalizer struct{}

// Normalize strips whitespace.
func (s *StripNormalizer) Normalize(input string) string {
	return strings.TrimFunc(input, unicode.IsSpace)
}
