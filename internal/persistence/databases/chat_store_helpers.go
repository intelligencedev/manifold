package databases

import (
	"context"
	"database/sql"
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

func nullableInt64Value(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func int64PtrFromNull(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	out := value.Int64
	return &out
}
