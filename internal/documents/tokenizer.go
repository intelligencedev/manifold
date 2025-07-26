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
	Python
	JavaScript
	TypeScript
	Java
	CSharp
	Rust
	Cpp
	C
	Markdown
	JSON
	YAML
	XML
	HTML
	CSS
	SQL
	Shell
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
	path = strings.ToLower(path)
	switch {
	// Go
	case strings.HasSuffix(path, ".go"):
		return Go
	// Python
	case strings.HasSuffix(path, ".py"), strings.HasSuffix(path, ".pyw"), strings.HasSuffix(path, ".pyi"):
		return Python
	// JavaScript/TypeScript
	case strings.HasSuffix(path, ".js"), strings.HasSuffix(path, ".jsx"), strings.HasSuffix(path, ".mjs"):
		return JavaScript
	case strings.HasSuffix(path, ".ts"), strings.HasSuffix(path, ".tsx"):
		return TypeScript
	// Java
	case strings.HasSuffix(path, ".java"):
		return Java
	// C#
	case strings.HasSuffix(path, ".cs"):
		return CSharp
	// Rust
	case strings.HasSuffix(path, ".rs"):
		return Rust
	// C/C++
	case strings.HasSuffix(path, ".cpp"), strings.HasSuffix(path, ".cxx"), strings.HasSuffix(path, ".cc"), strings.HasSuffix(path, ".c++"):
		return Cpp
	case strings.HasSuffix(path, ".c"), strings.HasSuffix(path, ".h"):
		return C
	// Markup/Config
	case strings.HasSuffix(path, ".md"), strings.HasSuffix(path, ".markdown"):
		return Markdown
	case strings.HasSuffix(path, ".json"):
		return JSON
	case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
		return YAML
	case strings.HasSuffix(path, ".xml"):
		return XML
	case strings.HasSuffix(path, ".html"), strings.HasSuffix(path, ".htm"):
		return HTML
	case strings.HasSuffix(path, ".css"):
		return CSS
	case strings.HasSuffix(path, ".sql"):
		return SQL
	// Shell scripts
	case strings.HasSuffix(path, ".sh"), strings.HasSuffix(path, ".bash"), strings.HasSuffix(path, ".zsh"):
		return Shell
	default:
		return Plain
	}
}
