package matrixroom

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/sandbox"
)

func TestToolQueuesMessageToCurrentRoom(t *testing.T) {
	t.Parallel()

	tool := &Tool{}
	outbox := sandbox.NewMatrixOutbox()
	ctx := sandbox.WithMatrixOutbox(sandbox.WithRoomID(context.Background(), "!room:test"), outbox)
	raw, err := json.Marshal(map[string]any{"text": "Task completed"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	resp, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("Call() error: %v", err)
	}
	result, ok := resp.(toolResponse)
	if !ok {
		t.Fatalf("unexpected response type: %#v", resp)
	}
	if !result.OK || !result.Queued {
		t.Fatalf("expected queued response, got %#v", result)
	}
	messages := outbox.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 queued message, got %d", len(messages))
	}
	if messages[0].RoomID != "!room:test" || messages[0].Text != "Task completed" {
		t.Fatalf("unexpected queued message: %#v", messages[0])
	}
}

func TestToolRequiresRoomScopedContext(t *testing.T) {
	t.Parallel()

	tool := &Tool{}
	outbox := sandbox.NewMatrixOutbox()
	ctx := sandbox.WithMatrixOutbox(context.Background(), outbox)
	raw, err := json.Marshal(map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	resp, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("Call() error: %v", err)
	}
	result := resp.(toolResponse)
	if result.OK || result.Queued {
		t.Fatalf("expected failure response, got %#v", result)
	}
	if len(outbox.Messages()) != 0 {
		t.Fatalf("expected no queued messages")
	}
}
