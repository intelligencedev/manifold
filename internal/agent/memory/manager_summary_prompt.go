package memory

import (
	"encoding/json"
	"strings"

	"manifold/internal/llm"
	"manifold/internal/persistence"
)

type summaryPromptMessage struct {
	Role      string
	Content   string
	ToolCalls []string
	ToolID    string
}

func buildSummaryPromptMessage(msg persistence.ChatMessage) summaryPromptMessage {
	out := summaryPromptMessage{
		Role:    msg.Role,
		Content: strings.TrimSpace(msg.Content),
	}
	raw := strings.TrimSpace(msg.Content)
	if raw == "" {
		return out
	}
	if decoded, ok := decodeSummaryAssistantMessage(msg, raw); ok {
		return decoded
	}
	if decoded, ok := decodeSummaryToolMessage(msg, raw, out); ok {
		return decoded
	}
	return out
}

func decodeSummaryAssistantMessage(msg persistence.ChatMessage, raw string) (summaryPromptMessage, bool) {
	if msg.Role != "assistant" || !strings.HasPrefix(raw, "{") {
		return summaryPromptMessage{}, false
	}
	var data struct {
		Content   string         `json:"content"`
		ToolCalls []llm.ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil || len(data.ToolCalls) == 0 {
		return summaryPromptMessage{}, false
	}
	return summaryPromptMessage{
		Role:      msg.Role,
		Content:   strings.TrimSpace(data.Content),
		ToolCalls: summarizeToolCalls(data.ToolCalls),
	}, true
}
