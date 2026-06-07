package databases

import (
	"context"
	"testing"
	"time"

	"manifold/internal/persistence"
)

func TestSQLiteChatStoreScansTextTimestamps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewSQLiteChatStore(openTestSQLite(t))

	session, err := store.EnsureSession(ctx, nil, "session-sqlite", "SQLite")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if session.CreatedAt.IsZero() || session.UpdatedAt.IsZero() {
		t.Fatalf("expected session timestamps, got created=%v updated=%v", session.CreatedAt, session.UpdatedAt)
	}

	sessions, err := store.ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "session-sqlite" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}

	now := time.Now().UTC()
	if err := store.AppendMessages(ctx, nil, "session-sqlite", []persistence.ChatMessage{
		{ID: "m1", Role: "user", Content: "hello", CreatedAt: now},
		{ID: "m2", Role: "assistant", Content: "working", CreatedAt: now.Add(time.Second)},
		{ID: "m3", Role: "assistant", Content: "done", CreatedAt: now.Add(2 * time.Second)},
	}, "done", "test-model"); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	messages, err := store.ListMessages(ctx, nil, "session-sqlite", 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 3 || messages[0].CreatedAt.IsZero() {
		t.Fatalf("unexpected messages: %#v", messages)
	}

	if err := store.DeleteMessagesAfter(ctx, nil, "session-sqlite", "m2", false); err != nil {
		t.Fatalf("DeleteMessagesAfter: %v", err)
	}
	messages, err = store.ListMessages(ctx, nil, "session-sqlite", 0)
	if err != nil {
		t.Fatalf("ListMessages after delete: %v", err)
	}
	if len(messages) != 2 || messages[1].ID != "m2" {
		t.Fatalf("unexpected remaining messages: %#v", messages)
	}
}
