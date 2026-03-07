package main

import "strings"

func promptFromMessage(body, prefix string) (prompt string, matched bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", false
	}

	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return trimmed, true
	}

	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}

	return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), true
}
