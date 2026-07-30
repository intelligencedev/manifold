package chat

import (
	"context"
	"testing"

	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

func TestDurableHelpers(t *testing.T) {
	params, err := DurableRequestParams(struct {
		Prompt string `json:"prompt"`
	}{Prompt: "hello"})
	if err != nil || params["prompt"] != "hello" {
		t.Fatalf("DurableRequestParams()=%v, err=%v", params, err)
	}
	var decoded struct {
		Prompt string `json:"prompt"`
	}
	if err := DecodeDurableParams(params, &decoded); err != nil || decoded.Prompt != "hello" {
		t.Fatalf("DecodeDurableParams()=%+v, err=%v", decoded, err)
	}
	if got := DurableIdempotencyKey("session", "prompt", "assistant", "user"); got != "chat:session:assistant" {
		t.Fatalf("DurableIdempotencyKey()=%q", got)
	}
	result := DurableResultPayload("run", "session", "user", "assistant", "done", 12)
	if result["run_id"] != "run" || result["durationMs"] != int64(12) {
		t.Fatalf("DurableResultPayload()=%#v", result)
	}
	ctx := WithCheckedOutWorkspaceContext(context.Background(), "project", &workspaces.Workspace{BaseDir: "/tmp/project"})
	if baseDir, ok := sandbox.BaseDirFromContext(ctx); !ok || baseDir != "/tmp/project" {
		t.Fatalf("workspace base dir=%q, ok=%v", baseDir, ok)
	}
}

func TestRunDurableTaskClosesLifecycleOnUnavailableEngine(t *testing.T) {
	var started, flushed, stopped bool
	_, err := RunDurableTask(context.Background(), nil, func(context.Context, map[string]any) (DurableExecution, error) {
		return DurableExecution{
			Start: func() { started = true },
			Flush: func() { flushed = true },
			StartHeartbeat: func() func() {
				return func() { stopped = true }
			},
			HandleError: func(err error) (map[string]any, error) { return nil, err },
		}, nil
	})
	if err == nil || !started || !flushed || !stopped {
		t.Fatalf("err=%v started=%v flushed=%v stopped=%v", err, started, flushed, stopped)
	}
}
