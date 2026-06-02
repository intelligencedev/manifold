package agentd

import (
	"encoding/json"
	"testing"

	"manifold/internal/llm"
)

func TestDurableChatJSONSafeHandlesEmptyRawToolArgs(t *testing.T) {
	t.Parallel()
	msg := llm.Message{
		Role: "assistant",
		ToolCalls: []llm.ToolCall{{
			ID:   "call-local",
			Name: "local_tool",
			Args: json.RawMessage{},
		}},
	}
	raw, err := json.Marshal(durableChatJSONSafe(msg))
	if err != nil {
		t.Fatalf("marshal safe message: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("safe message is not valid JSON: %s", raw)
	}
	if string(raw) == "" {
		t.Fatal("expected encoded message")
	}
}

func TestDurableChatPayloadMapHandlesInvalidRawMessage(t *testing.T) {
	t.Parallel()
	payload := map[string]any{
		"type": "tool_start",
		"args": json.RawMessage(`{"partial"`),
	}
	out := durableChatPayloadMap(durableChatJSONSafe(payload))
	if out["type"] != "tool_start" {
		t.Fatalf("type = %v, want tool_start", out["type"])
	}
	if _, err := json.Marshal(out); err != nil {
		t.Fatalf("marshal safe event payload: %v", err)
	}
}
