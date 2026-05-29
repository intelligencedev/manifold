package memory

import (
	"encoding/json"
	"strings"

	"manifold/internal/persistence"
)

func decodeSummaryToolMessage(msg persistence.ChatMessage, raw string, fallback summaryPromptMessage) (summaryPromptMessage, bool) {
	if msg.Role != "tool" || !strings.HasPrefix(raw, "{") {
		return summaryPromptMessage{}, false
	}
	var data struct {
		Content string `json:"content"`
		ToolID  string `json:"tool_id"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return summaryPromptMessage{}, false
	}
	fallback.Content = strings.TrimSpace(data.Content)
	fallback.ToolID = strings.TrimSpace(data.ToolID)
	return fallback, true
}
