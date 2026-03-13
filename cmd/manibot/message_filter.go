package main

import (
	"regexp"
	"strings"
)

func promptFromMessage(body, prefix string) (prompt string, matched bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", false
	}

	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return trimmed, true
	}

	if isInlineMentionPrefix(prefix) {
		if !containsStandalonePrefix(trimmed, prefix) {
			return "", false
		}
		return trimmed, true
	}

	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}

	return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), true
}

func isInlineMentionPrefix(prefix string) bool {
	return strings.HasPrefix(strings.TrimSpace(prefix), "@")
}

func containsStandalonePrefix(body, prefix string) bool {
	pattern := `(^|[^[:alnum:]_])` + regexp.QuoteMeta(prefix) + `($|[^[:alnum:]_])`
	return regexp.MustCompile(pattern).FindStringIndex(body) != nil
}
