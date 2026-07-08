package agentd

import (
	"context"
	"encoding/json"
	"strings"

	"manifold/internal/llm"
	persist "manifold/internal/persistence"
)

func (a *app) hydrateChatMessagesForRequest(ctx context.Context, userID *int64, sessionID string, raw []persist.ChatMessage) []persist.ChatMessage {
	msgs := hydrateChatMessages(raw)
	if a == nil || a.llmRequestStore == nil || len(msgs) == 0 {
		return msgs
	}
	for i := range msgs {
		if msgs[i].Role != "assistant" {
			continue
		}
		reqs, err := a.llmRequestStore.ListLLMRequestsForMessage(ctx, userID, sessionID, msgs[i].ID)
		if err == nil {
			msgs[i].LLMRequestCount = len(reqs)
		}
	}
	return msgs
}

func hydrateChatMessages(raw []persist.ChatMessage) []persist.ChatMessage {
	out := make([]persist.ChatMessage, 0, len(raw))
	metaByID := make(map[string]toolMeta)

	for _, msg := range raw {
		msg, keep := hydrateChatMessage(msg, metaByID)
		if !keep {
			continue
		}
		out = append(out, msg)
	}

	return out
}

type toolMeta struct {
	name string
	args string
}

func hydrateChatMessage(msg persist.ChatMessage, metaByID map[string]toolMeta) (persist.ChatMessage, bool) {
	trimmed := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(trimmed, "{") {
		return msg, true
	}
	switch msg.Role {
	case "assistant":
		return hydrateAssistantChatMessage(msg, trimmed, metaByID)
	case "tool":
		return hydrateToolChatMessage(msg, trimmed, metaByID), true
	default:
		return msg, true
	}
}

func hydrateAssistantChatMessage(
	msg persist.ChatMessage,
	trimmed string,
	metaByID map[string]toolMeta,
) (persist.ChatMessage, bool) {
	var data struct {
		Content   string         `json:"content"`
		ToolCalls []llm.ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return msg, true
	}
	if data.Content != "" {
		msg.Content = data.Content
	}
	for _, tc := range data.ToolCalls {
		args := strings.TrimSpace(string(tc.Args))
		metaByID[tc.ID] = toolMeta{name: tc.Name, args: args}
	}
	// Assistant messages that only carried tool_calls should not render in the chat pane.
	if strings.TrimSpace(data.Content) == "" && len(data.ToolCalls) > 0 {
		return msg, false
	}
	return msg, true
}

func hydrateToolChatMessage(msg persist.ChatMessage, trimmed string, metaByID map[string]toolMeta) persist.ChatMessage {
	var data struct {
		Content string `json:"content"`
		ToolID  string `json:"tool_id"`
	}
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return msg
	}
	if data.Content != "" {
		msg.Content = data.Content
	}
	if data.ToolID == "" {
		return msg
	}
	msg.ToolID = data.ToolID
	if meta, ok := metaByID[data.ToolID]; ok {
		msg.Title = meta.name
		msg.ToolArgs = meta.args
	}
	return msg
}
