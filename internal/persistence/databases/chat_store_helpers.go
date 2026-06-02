package databases

import (
	"context"
	"strings"

	"manifold/internal/persistence"
)

type chatAppendMessagesRequest struct {
	ctx          context.Context
	userID       *int64
	sessionID    string
	messages     []persistence.ChatMessage
	preview      string
	model        string
	skipExisting bool
}

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
