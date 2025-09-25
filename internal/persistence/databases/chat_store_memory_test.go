package databases

import (
	"context"
	"testing"
	"time"

	"intelligence.dev/internal/persistence"
)

func TestMemChatStoreLifecycle(t *testing.T) {
	store := newMemoryChatStore()
	ctx := context.Background()

	sess, err := store.EnsureSession(ctx, "session-1", "First")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if sess.ID != "session-1" {
		t.Fatalf("unexpected session id: %s", sess.ID)
	}

	if err := store.AppendMessages(ctx, "session-1", nil, "", ""); err != nil {
		t.Fatalf("AppendMessages with empty slice: %v", err)
	}

	if err := store.AppendMessages(ctx, "session-1", []persistence.ChatMessage{
		{Role: "user", Content: "Hello", CreatedAt: time.Now()},
		{Role: "assistant", Content: "Hi there", CreatedAt: time.Now().Add(time.Second)},
	}, "Hi there", "test-model"); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	msgs, err := store.ListMessages(ctx, "session-1", 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %#v", msgs)
	}

	sessions, err := store.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].LastMessagePreview != "Hi there" {
		t.Fatalf("unexpected preview: %s", sessions[0].LastMessagePreview)
	}

	if _, err := store.RenameSession(ctx, "session-1", "Updated"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}

	if err := store.DeleteSession(ctx, "session-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := store.ListMessages(ctx, "session-1", 0); err != nil {
		t.Fatalf("ListMessages after delete should not error: %v", err)
	}
}
