package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"intelligence.dev/internal/llm"
)

func TestAdaptSchemas(t *testing.T) {
	schemas := []llm.ToolSchema{
		{
			Name:        "do_thing",
			Description: "does a thing",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"foo": map[string]any{"type": "string"},
				},
			},
		},
	}
	out := AdaptSchemas(schemas)
	if len(out) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(out))
	}
	// Marshal to JSON and ensure name/description appear
	b, err := json.Marshal(out[0])
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "do_thing") {
		t.Fatalf("expected name in json: %s", s)
	}
	if !strings.Contains(s, "does a thing") {
		t.Fatalf("expected description in json: %s", s)
	}
}

func TestAdaptMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: ""},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "", ToolCalls: nil},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "x", Args: []byte("{}")}}},
		{Role: "tool", Content: "", ToolID: "tool-1"},
	}
	out := AdaptMessages(msgs)
	if len(out) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(out))
	}
	// Marshal each to JSON and check for expected content types
	js0, _ := json.Marshal(out[0])
	if !strings.Contains(string(js0), "You are a helpful assistant.") {
		t.Fatalf("expected default system content in %s", string(js0))
	}
	js1, _ := json.Marshal(out[1])
	if !strings.Contains(string(js1), "hello") {
		t.Fatalf("expected user content in %s", string(js1))
	}
	js2, _ := json.Marshal(out[2])
	// assistant without toolcalls should have content (space)
	if !strings.Contains(string(js2), " ") {
		t.Fatalf("expected assistant content placeholder in %s", string(js2))
	}
	js3, _ := json.Marshal(out[3])
	if !strings.Contains(string(js3), "x") {
		t.Fatalf("expected toolcall name in %s", string(js3))
	}
	js4, _ := json.Marshal(out[4])
	if !strings.Contains(string(js4), "tool-1") {
		t.Fatalf("expected tool id in %s", string(js4))
	}
}
