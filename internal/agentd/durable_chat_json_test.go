package agentd

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
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

func TestDurableCheckpointJSONSafeRoundTripsToolCalls(t *testing.T) {
	t.Parallel()

	msg := llm.Message{
		Role:    "assistant",
		Content: "checking",
		ToolCalls: []llm.ToolCall{{
			ID:   "call-local",
			Name: "local_tool",
			Args: json.RawMessage{},
		}},
	}
	raw, err := json.Marshal(durableCheckpointJSONSafe(msg))
	if err != nil {
		t.Fatalf("marshal checkpoint message: %v", err)
	}
	var decoded llm.Message
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal checkpoint message: %v", err)
	}
	if decoded.Role != "assistant" || decoded.Content != "checking" {
		t.Fatalf("decoded message = %+v", decoded)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Fatalf("decoded tool calls = %+v, want one", decoded.ToolCalls)
	}
	if decoded.ToolCalls[0].ID != "call-local" || decoded.ToolCalls[0].Name != "local_tool" {
		t.Fatalf("decoded tool call = %+v", decoded.ToolCalls[0])
	}
	if !json.Valid(decoded.ToolCalls[0].Args) {
		t.Fatalf("decoded args are not valid JSON: %q", string(decoded.ToolCalls[0].Args))
	}
}

func TestUnmarshalDurableCheckpointSupportsLegacySnakeCaseToolCalls(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"role":"assistant","content":"","tool_calls":[{"id":"call-local","name":"local_tool","args":{"command":"date"}}]}`)
	var decoded llm.Message
	if err := unmarshalDurableCheckpoint(raw, &decoded); err != nil {
		t.Fatalf("unmarshal checkpoint: %v", err)
	}
	if decoded.Role != "assistant" {
		t.Fatalf("role = %q, want assistant", decoded.Role)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Fatalf("decoded tool calls = %+v, want one", decoded.ToolCalls)
	}
	if decoded.ToolCalls[0].ID != "call-local" || decoded.ToolCalls[0].Name != "local_tool" {
		t.Fatalf("decoded tool call = %+v", decoded.ToolCalls[0])
	}
	if string(decoded.ToolCalls[0].Args) != `{"command":"date"}` {
		t.Fatalf("args = %s", decoded.ToolCalls[0].Args)
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

func TestWithCheckedOutWorkspaceContextAttachesProjectWorkspace(t *testing.T) {
	t.Parallel()

	ctx := withCheckedOutWorkspaceContext(context.Background(), chatRunRequest{ProjectID: "project-1"}, &workspaces.Workspace{
		ProjectID: "project-1",
		BaseDir:   "/tmp/workdir/users/42/projects/project-1",
	})

	if got, ok := sandbox.BaseDirFromContext(ctx); !ok || got != "/tmp/workdir/users/42/projects/project-1" {
		t.Fatalf("base dir = %q ok=%v, want checked-out project workspace", got, ok)
	}
	if got, ok := sandbox.ProjectIDFromContext(ctx); !ok || got != "project-1" {
		t.Fatalf("project id = %q ok=%v, want project-1", got, ok)
	}
}
