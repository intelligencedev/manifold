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
