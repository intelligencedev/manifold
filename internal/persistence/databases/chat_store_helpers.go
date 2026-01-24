package databases

import "strings"

func snippetForPreview(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	const maxLen = 120
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen]
}
