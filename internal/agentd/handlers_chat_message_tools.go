package agentd

import (
	"context"
	"encoding/json"
	"strings"

	"manifold/internal/llm"
	persist "manifold/internal/persistence"
)

type atomicChatTurnDeleteStore interface {
	DeleteMessageWithRelated(ctx context.Context, userID *int64, sessionID string, messageID string, relatedMessageIDs []string, resetSummary bool) error
	DeleteMessagesAfterWithRelated(ctx context.Context, req persist.ChatDeleteAfterRequest) error
}

func relatedToolMessageIDs(msgs []persist.ChatMessage, target persist.ChatMessage) []string {
	toolIDs := toolCallIDsFromMessage(target)
	if len(toolIDs) == 0 {
		return nil
	}
	toolSet := make(map[string]struct{}, len(toolIDs))
	for _, id := range toolIDs {
		toolSet[id] = struct{}{}
	}
	related := make([]string, 0, len(toolSet))
	seen := make(map[string]struct{})
	for _, msg := range msgs {
		if msg.Role != "tool" {
			continue
		}
		toolID := toolIDFromMessage(msg)
		if _, ok := toolSet[toolID]; !ok {
			continue
		}
		if _, ok := seen[msg.ID]; ok {
			continue
		}
		seen[msg.ID] = struct{}{}
		related = append(related, msg.ID)
	}
	return related
}

func toolCallIDsFromMessage(msg persist.ChatMessage) []string {
	if msg.Role != "assistant" {
		return nil
	}
	trimmed := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var data struct {
		ToolCalls []llm.ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return nil
	}
	if len(data.ToolCalls) == 0 {
		return nil
	}
	ids := make([]string, 0, len(data.ToolCalls))
	for _, tc := range data.ToolCalls {
		id := strings.TrimSpace(tc.ID)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func toolIDFromMessage(msg persist.ChatMessage) string {
	if msg.Role != "tool" {
		return ""
	}
	trimmed := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(trimmed, "{") {
		return ""
	}
	var data struct {
		ToolID string `json:"tool_id"`
	}
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.ToolID)
}
