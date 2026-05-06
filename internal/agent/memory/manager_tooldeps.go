package memory

import (
	"encoding/json"
	"manifold/internal/llm"
	"manifold/internal/persistence"
	"strings"
)

func adjustIndexForToolDeps(msgs []persistence.ChatMessage, start, cutIndex int) int {
	if cutIndex <= start || cutIndex >= len(msgs) {
		return cutIndex
	}

	extractToolID := func(m persistence.ChatMessage) string {
		if id := strings.TrimSpace(m.ToolID); id != "" {
			return id
		}
		trimmed := strings.TrimSpace(m.Content)
		if strings.HasPrefix(trimmed, "{") {
			var data struct {
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
				return strings.TrimSpace(data.ToolID)
			}
		}
		return ""
	}

	required := make(map[string]struct{})
	for i := cutIndex; i < len(msgs); i++ {
		if msgs[i].Role != "tool" {
			continue
		}
		if id := extractToolID(msgs[i]); id != "" {
			required[id] = struct{}{}
		}
	}
	if len(required) == 0 {
		return cutIndex
	}

	containsToolCallID := func(m persistence.ChatMessage, toolID string) bool {
		trimmed := strings.TrimSpace(m.Content)
		if !strings.HasPrefix(trimmed, "{") {
			return false
		}
		var data struct {
			ToolCalls []llm.ToolCall `json:"tool_calls"`
		}
		if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
			return false
		}
		for _, tc := range data.ToolCalls {
			if strings.TrimSpace(tc.ID) == toolID {
				return true
			}
		}
		return false
	}

	earliestNeeded := cutIndex
	for toolID := range required {
		foundIdx := -1
		for i := cutIndex - 1; i >= start; i-- {
			if msgs[i].Role != "assistant" {
				continue
			}
			if containsToolCallID(msgs[i], toolID) {
				foundIdx = i
				break
			}
		}
		if foundIdx != -1 && foundIdx < earliestNeeded {
			earliestNeeded = foundIdx
		}
	}

	return earliestNeeded
}
