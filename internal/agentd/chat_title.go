package agentd

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	chatTitleMaxRunes = 48
)

var defaultSessionNames = map[string]struct{}{
	"":             {},
	"new chat":     {},
	"conversation": {},
}

func isDefaultSessionName(name string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	_, ok := defaultSessionNames[trimmed]
	return ok
}

func (a *app) generateChatTitle(ctx context.Context, prompt string) (string, error) { // ctx kept for signature compatibility
	_ = ctx
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt required")
	}

	// Derive title locally: first sentence of the first prompt, then truncate.
	sentence := firstSentence(prompt)
	if strings.TrimSpace(sentence) == "" {
		sentence = prompt
	}
	sentence = collapseWhitespace(sentence)
	if sentence == "" {
		return fallbackChatTitle(prompt), nil
	}
	return truncateRunes(sentence, chatTitleMaxRunes), nil
}

func sanitizeGeneratedTitle(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	trimmed := strings.Trim(raw, " \n\r\t\"'`“”‘’")
	trimmed = strings.Trim(trimmed, "-–—•")
	trimmed = collapseWhitespace(trimmed)
	trimmed = strings.Trim(trimmed, ".!?,")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	if utf8.RuneCountInString(trimmed) > chatTitleMaxRunes {
		trimmed = truncateRunes(trimmed, chatTitleMaxRunes)
	}
	return trimmed
}

func fallbackChatTitle(prompt string) string {
	collapsed := collapseWhitespace(prompt)
	if collapsed == "" {
		return "Conversation"
	}
	return truncateRunes(collapsed, chatTitleMaxRunes)
}

func collapseWhitespace(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(runes[:max]))
}

// firstSentence returns the substring up to and including the first sentence terminator.
// Sentence terminators considered: '.', '?', '!', and a newline. If none found, returns s.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for i, r := range s {
		switch r {
		case '.', '?', '!', '\n':
			// include the terminator for a natural-looking title
			return strings.TrimSpace(s[:i+1])
		}
	}
	return s
}
