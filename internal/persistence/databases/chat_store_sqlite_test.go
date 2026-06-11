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
	if session.MessageCount != 0 {
		t.Fatalf("new session should have 0 messages, got %d", session.MessageCount)
	}

	sessions, err := store.ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "session-sqlite" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
	if sessions[0].MessageCount != 0 {
		t.Fatalf("expected listed message count 0 before append, got %d", sessions[0].MessageCount)
	}

	now := time.Now().UTC()
	durationMs := int64(12437)
	if err := store.AppendMessages(ctx, nil, "session-sqlite", []persistence.ChatMessage{
		{ID: "m1", Role: "user", Content: "hello", CreatedAt: now},
		{ID: "m2", Role: "assistant", Content: "working", CreatedAt: now.Add(time.Second)},
		{ID: "m3", Role: "assistant", Content: "done", CreatedAt: now.Add(2 * time.Second), DurationMs: &durationMs},
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
	if messages[2].DurationMs == nil || *messages[2].DurationMs != durationMs {
		t.Fatalf("expected duration to round trip, got %#v", messages[2].DurationMs)
	}
	session, err = store.GetSession(ctx, nil, "session-sqlite")
	if err != nil {
		t.Fatalf("GetSession after append: %v", err)
	}
	if session.MessageCount != 3 {
		t.Fatalf("expected message count 3 after append, got %d", session.MessageCount)
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
	sessions, err = store.ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("ListSessions after delete: %v", err)
	}
	if len(sessions) != 1 || sessions[0].MessageCount != 2 {
		t.Fatalf("expected listed message count 2 after delete, got %#v", sessions)
	}
}

func TestSQLiteChatStoreListSessionsPinsFirst(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewSQLiteChatStore(openTestSQLite(t))

	if _, err := store.EnsureSession(ctx, nil, "sqlite-regular", "Regular"); err != nil {
		t.Fatalf("EnsureSession regular: %v", err)
	}
	if _, err := store.EnsureSession(ctx, nil, "sqlite-pinned", "Pinned"); err != nil {
		t.Fatalf("EnsureSession pinned: %v", err)
	}
	pinned, err := store.SetSessionPinned(ctx, nil, "sqlite-pinned", true)
	if err != nil {
		t.Fatalf("SetSessionPinned: %v", err)
	}
	if !pinned.Pinned {
		t.Fatal("expected pinned session")
	}

	sessions, err := store.ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != "sqlite-pinned" || !sessions[0].Pinned {
		t.Fatalf("expected pinned session first, got %#v", sessions)
	}
}
