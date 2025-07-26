package documents

import (
	"strings"
	"unicode/utf8"
)

// Language identifies document language for heuristics.
// The stringer directive will generate String() for the enum.
//
//go:generate stringer -type=Language
type Language int

const (
	Unknown Language = iota
	Go
	Markdown
	Plain
)

// Tokenizer counts tokens in a string.
type Tokenizer interface {
	Count(s string) int
	Name() string
}

// RuneTokenizer is a simple utf8 rune counter.
type RuneTokenizer struct{}

func (RuneTokenizer) Count(s string) int { return utf8.RuneCountInString(s) }
func (RuneTokenizer) Name() string       { return "rune" }

// DeduceLanguage returns a Language from the filename.
func DeduceLanguage(path string) Language {
	switch {
	case strings.HasSuffix(path, ".go"):
		return Go
	case strings.HasSuffix(path, ".md"):
		return Markdown
	default:
		return Plain
	}
}
