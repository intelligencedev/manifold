package documents

import "strings"

// Language is a type that represents a programming language.
type Language string

const (
	PYTHON   Language = "PYTHON"
	GO       Language = "GO"
	HTML     Language = "HTML"
	JS       Language = "JS"
	TS       Language = "TS"
	MARKDOWN Language = "MARKDOWN"
	JSON     Language = "JSON"
	DEFAULT  Language = "DEFAULT"
)

// isTextFile checks if a file's content appears to be text.
func IsTextFile(data []byte) bool {
	// A simple heuristic: if the file contains a null byte, consider it binary.
	return !strings.Contains(string(data), "\x00")
}

// deduceLanguage inspects the file extension and returns a Language.
func DeduceLanguage(filePath string) Language {
	switch {
	case strings.HasSuffix(filePath, ".go"):
		return GO
	case strings.HasSuffix(filePath, ".py"):
		return PYTHON
	case strings.HasSuffix(filePath, ".md"):
		return MARKDOWN
	case strings.HasSuffix(filePath, ".html"):
		return HTML
	case strings.HasSuffix(filePath, ".js"):
		return JS
	case strings.HasSuffix(filePath, ".ts"):
		return TS
	case strings.HasSuffix(filePath, ".json"):
		return JSON
	default:
		return DEFAULT
	}
}
