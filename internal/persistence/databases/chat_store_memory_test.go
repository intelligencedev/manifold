package databases

import (
	"context"
	"errors"
	"testing"
	"time"

	"manifold/internal/persistence"
)

func int64ptr(v int64) *int64 { return &v }

func TestMemChatStoreLifecycle(t *testing.T) {
	store := newMemoryChatStore()
	ctx := context.Background()

	sess, err := store.EnsureSession(ctx, nil, "session-1", "First")
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if sess.ID != "session-1" {
		t.Fatalf("unexpected session id: %s", sess.ID)
	}

	if err := store.AppendMessages(ctx, nil, "session-1", nil, "", ""); err != nil {
		t.Fatalf("AppendMessages with empty slice: %v", err)
	}

	if err := store.AppendMessages(ctx, nil, "session-1", []persistence.ChatMessage{
		{Role: "user", Content: "Hello", CreatedAt: time.Now()},
		{Role: "assistant", Content: "Hi there", CreatedAt: time.Now().Add(time.Second)},
	}, "Hi there", "test-model"); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	msgs, err := store.ListMessages(ctx, nil, "session-1", 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %#v", msgs)
	}
	limited, err := store.ListMessages(ctx, nil, "session-1", 1)
	if err != nil {
		t.Fatalf("ListMessages limit: %v", err)
	}
	if len(limited) != 1 || limited[0].Role != "assistant" {
		t.Fatalf("expected only assistant message from limited query, got %#v", limited)
	}
	if err := store.UpdateSummary(ctx, nil, "session-1", "summary", 2); err != nil {
		t.Fatalf("UpdateSummary: %v", err)
	}
	updated, err := store.GetSession(ctx, nil, "session-1")
	if err != nil {
		t.Fatalf("GetSession after summary: %v", err)
	}
	if updated.Summary != "summary" || updated.SummarizedCount != 2 {
		t.Fatalf("unexpected summary state: %#v", updated)
	}

	sessions, err := store.ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].LastMessagePreview != "Hi there" {
		t.Fatalf("unexpected preview: %s", sessions[0].LastMessagePreview)
	}

	if _, err := store.RenameSession(ctx, nil, "session-1", "Updated"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}

	if err := store.DeleteSession(ctx, nil, "session-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := store.ListMessages(ctx, nil, "session-1", 0); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMemChatStoreOwnership(t *testing.T) {
	store := newMemoryChatStore()
	ctx := context.Background()
	user1 := int64ptr(1)
	user2 := int64ptr(2)

	sess, err := store.CreateSession(ctx, user1, "Mine")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.UserID == nil || *sess.UserID != *user1 {
		t.Fatalf("expected user ownership, got %#v", sess.UserID)
	}

	if _, err := store.GetSession(ctx, user2, sess.ID); !errors.Is(err, persistence.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for other user get, got %v", err)
	}

	sessions, err := store.ListSessions(ctx, user2)
	if err != nil {
		t.Fatalf("ListSessions other user: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions for other user, got %d", len(sessions))
	}

	if _, err := store.RenameSession(ctx, user2, sess.ID, "Nope"); !errors.Is(err, persistence.ErrForbidden) {
		t.Fatalf("expected ErrForbidden rename, got %v", err)
	}

	if err := store.DeleteSession(ctx, user2, sess.ID); !errors.Is(err, persistence.ErrForbidden) {
		t.Fatalf("expected ErrForbidden delete, got %v", err)
	}

	if err := store.AppendMessages(ctx, user2, sess.ID, []persistence.ChatMessage{{Role: "user", Content: "test"}}, "", ""); !errors.Is(err, persistence.ErrForbidden) {
		t.Fatalf("expected ErrForbidden append, got %v", err)
	}

	if _, err := store.ListMessages(ctx, user2, sess.ID, 0); !errors.Is(err, persistence.ErrForbidden) {
		t.Fatalf("expected ErrForbidden list messages, got %v", err)
	}

	if _, err := store.GetSession(ctx, nil, sess.ID); err != nil {
		t.Fatalf("admin (nil user) should access session: %v", err)
	}
}

func TestMemChatStoreEnsureSessionOwnership(t *testing.T) {
	store := newMemoryChatStore()
	ctx := context.Background()
	user1 := int64ptr(1)
	user2 := int64ptr(2)

	if _, err := store.EnsureSession(ctx, user1, "s", "mine"); err != nil {
		t.Fatalf("EnsureSession owner: %v", err)
	}
	if _, err := store.EnsureSession(ctx, user2, "s", "theirs"); !errors.Is(err, persistence.ErrForbidden) {
		t.Fatalf("expected ErrForbidden when ensuring existing session for different user, got %v", err)
	}
}
