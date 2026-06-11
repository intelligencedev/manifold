package databases

import (
	"context"
	"errors"
	"testing"
	"time"

	"manifold/internal/persistence"
)

//go:fix inline
func int64ptr(v int64) *int64 { return new(v) }

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
	if sess.MessageCount != 0 {
		t.Fatalf("new session should have 0 messages, got %d", sess.MessageCount)
	}
	if sess.MemoryEnabled || sess.EvolvingMemoryEnabled || sess.BeliefMemoryEnabled {
		t.Fatalf("new sessions should default unified memory off, got memory=%v evolving=%v belief=%v", sess.MemoryEnabled, sess.EvolvingMemoryEnabled, sess.BeliefMemoryEnabled)
	}

	if err := store.AppendMessages(ctx, nil, "session-1", nil, "", ""); err != nil {
		t.Fatalf("AppendMessages with empty slice: %v", err)
	}

	durationMs := int64(1200)
	if err := store.AppendMessages(ctx, nil, "session-1", []persistence.ChatMessage{
		{Role: "user", Content: "Hello", CreatedAt: time.Now()},
		{Role: "assistant", Content: "Hi there", CreatedAt: time.Now().Add(time.Second), DurationMs: &durationMs},
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
	if msgs[1].DurationMs == nil || *msgs[1].DurationMs != durationMs {
		t.Fatalf("expected assistant duration to round trip, got %#v", msgs[1].DurationMs)
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
	if updated.MessageCount != 2 {
		t.Fatalf("expected message count 2 after append, got %d", updated.MessageCount)
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
	if sessions[0].MessageCount != 2 {
		t.Fatalf("expected listed message count 2, got %d", sessions[0].MessageCount)
	}

	renamed, err := store.RenameSession(ctx, nil, "session-1", "Updated")
	if err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	if renamed.MessageCount != 2 {
		t.Fatalf("expected renamed session message count 2, got %d", renamed.MessageCount)
	}
	projectSession, err := store.SetSessionProject(ctx, nil, "session-1", "project-1")
	if err != nil {
		t.Fatalf("SetSessionProject: %v", err)
	}
	if projectSession.MessageCount != 2 {
		t.Fatalf("expected project session message count 2, got %d", projectSession.MessageCount)
	}
	locked, err := store.GetSession(ctx, nil, "session-1")
	if err != nil {
		t.Fatalf("GetSession after project lock: %v", err)
	}
	if locked.ProjectID != "project-1" {
		t.Fatalf("expected project lock, got %q", locked.ProjectID)
	}
	pinned, err := store.SetSessionPinned(ctx, nil, "session-1", true)
	if err != nil {
		t.Fatalf("SetSessionPinned: %v", err)
	}
	if !pinned.Pinned {
		t.Fatal("expected pinned session")
	}

	if err := store.DeleteSession(ctx, nil, "session-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := store.ListMessages(ctx, nil, "session-1", 0); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMemChatStoreListSessionsPinsFirst(t *testing.T) {
	store := newMemoryChatStore()
	ctx := context.Background()

	if _, err := store.EnsureSession(ctx, nil, "old-pinned", "Old pinned"); err != nil {
		t.Fatalf("EnsureSession old-pinned: %v", err)
	}
	time.Sleep(time.Millisecond)
	if _, err := store.EnsureSession(ctx, nil, "new-regular", "New regular"); err != nil {
		t.Fatalf("EnsureSession new-regular: %v", err)
	}
	if _, err := store.SetSessionPinned(ctx, nil, "old-pinned", true); err != nil {
		t.Fatalf("SetSessionPinned: %v", err)
	}

	sessions, err := store.ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != "old-pinned" || !sessions[0].Pinned {
		t.Fatalf("expected pinned session first, got %#v", sessions)
	}
}

func TestMemChatStoreOwnership(t *testing.T) {
	store := newMemoryChatStore()
	ctx := context.Background()
	user1 := new(int64(1))
	user2 := new(int64(2))

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

	if err := store.AppendMessages(ctx, user1, sess.ID, []persistence.ChatMessage{{Role: "user", Content: "owned"}}, "owned", ""); err != nil {
		t.Fatalf("AppendMessages owner: %v", err)
	}

	adminSession, err := store.GetSession(ctx, nil, sess.ID)
	if err != nil {
		t.Fatalf("admin (nil user) should access session: %v", err)
	}
	if adminSession.MessageCount != 1 {
		t.Fatalf("expected admin to see owner message count 1, got %d", adminSession.MessageCount)
	}
}

func TestMemChatStoreEnsureSessionOwnership(t *testing.T) {
	store := newMemoryChatStore()
	ctx := context.Background()
	user1 := new(int64(1))
	user2 := new(int64(2))

	if _, err := store.EnsureSession(ctx, user1, "s", "mine"); err != nil {
		t.Fatalf("EnsureSession owner: %v", err)
	}
	if _, err := store.EnsureSession(ctx, user2, "s", "theirs"); !errors.Is(err, persistence.ErrForbidden) {
		t.Fatalf("expected ErrForbidden when ensuring existing session for different user, got %v", err)
	}
}

func TestMemChatStoreListSessionsHidesMatrixKindByDefault(t *testing.T) {
	store := newMemoryChatStore()
	ctx := context.Background()

	if _, err := store.EnsureSession(ctx, nil, "chat-1", "Chat"); err != nil {
		t.Fatalf("EnsureSession chat: %v", err)
	}
	if _, err := store.EnsureSessionKind(ctx, nil, "matrix-1", "Matrix Room !room:test", persistence.ChatSessionKindMatrix); err != nil {
		t.Fatalf("EnsureSessionKind matrix: %v", err)
	}

	sessions, err := store.ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 non-matrix session, got %d", len(sessions))
	}
	if sessions[0].ID != "chat-1" {
		t.Fatalf("expected chat session only, got %#v", sessions[0])
	}

	matrixSessions, err := store.ListSessionsByKind(ctx, nil, persistence.ChatSessionKindMatrix)
	if err != nil {
		t.Fatalf("ListSessionsByKind matrix: %v", err)
	}
	if len(matrixSessions) != 1 || matrixSessions[0].ID != "matrix-1" {
		t.Fatalf("expected matrix session via kind query, got %#v", matrixSessions)
	}
}

func TestMemChatStoreDeleteMessageWithRelated(t *testing.T) {
	store := newMemoryChatStore().(*memChatStore)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := store.EnsureSession(ctx, nil, "session-atomic", "Atomic"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if err := store.AppendMessages(ctx, nil, "session-atomic", []persistence.ChatMessage{
		{ID: "user-1", Role: "user", Content: "hello", CreatedAt: now},
		{ID: "assistant-1", Role: "assistant", Content: `{"content":"Working","tool_calls":[{"name":"search_docs","id":"call-1","args":{"q":"foo"}}]}`, CreatedAt: now.Add(time.Second)},
		{ID: "tool-1", Role: "tool", Content: `{"content":"result","tool_id":"call-1"}`, CreatedAt: now.Add(2 * time.Second)},
		{ID: "assistant-2", Role: "assistant", Content: "done", CreatedAt: now.Add(3 * time.Second)},
	}, "done", "test-model"); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}
	if err := store.UpdateSummary(ctx, nil, "session-atomic", "summary", 3); err != nil {
		t.Fatalf("UpdateSummary: %v", err)
	}

	if err := store.DeleteMessageWithRelated(ctx, nil, "session-atomic", "assistant-1", []string{"tool-1"}, true); err != nil {
		t.Fatalf("DeleteMessageWithRelated: %v", err)
	}

	msgs, err := store.ListMessages(ctx, nil, "session-atomic", 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after delete, got %d", len(msgs))
	}
	if msgs[0].ID != "user-1" || msgs[1].ID != "assistant-2" {
		t.Fatalf("unexpected remaining messages: %#v", msgs)
	}

	sess, err := store.GetSession(ctx, nil, "session-atomic")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.Summary != "" || sess.SummarizedCount != 0 {
		t.Fatalf("expected cleared summary, got %#v", sess)
	}
	if sess.MessageCount != 2 {
		t.Fatalf("expected message count 2 after related delete, got %d", sess.MessageCount)
	}
	if sess.LastMessagePreview != "done" {
		t.Fatalf("expected preview to follow remaining tail, got %q", sess.LastMessagePreview)
	}
}

func TestMemChatStoreDeleteMessagesAfterWithRelated(t *testing.T) {
	store := newMemoryChatStore().(*memChatStore)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := store.EnsureSession(ctx, nil, "session-tail", "Tail"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if err := store.AppendMessages(ctx, nil, "session-tail", []persistence.ChatMessage{
		{ID: "user-1", Role: "user", Content: "hello", CreatedAt: now},
		{ID: "assistant-1", Role: "assistant", Content: "working", CreatedAt: now.Add(time.Second)},
		{ID: "tool-1", Role: "tool", Content: `{"content":"result","tool_id":"call-1"}`, CreatedAt: now.Add(2 * time.Second)},
		{ID: "assistant-2", Role: "assistant", Content: "done", CreatedAt: now.Add(3 * time.Second)},
	}, "done", "test-model"); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}
	if err := store.UpdateSummary(ctx, nil, "session-tail", "summary", 4); err != nil {
		t.Fatalf("UpdateSummary: %v", err)
	}

	if err := store.DeleteMessagesAfterWithRelated(ctx, persistence.ChatDeleteAfterRequest{
		SessionID:    "session-tail",
		MessageID:    "assistant-1",
		ResetSummary: true,
	}); err != nil {
		t.Fatalf("DeleteMessagesAfterWithRelated: %v", err)
	}

	msgs, err := store.ListMessages(ctx, nil, "session-tail", 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 remaining messages, got %d", len(msgs))
	}
	if msgs[0].ID != "user-1" || msgs[1].ID != "assistant-1" {
		t.Fatalf("unexpected remaining messages: %#v", msgs)
	}

	sess, err := store.GetSession(ctx, nil, "session-tail")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.Summary != "" || sess.SummarizedCount != 0 {
		t.Fatalf("expected summary reset after tail delete, got %#v", sess)
	}
	if sess.LastMessagePreview != "working" {
		t.Fatalf("expected preview to be 'working', got %q", sess.LastMessagePreview)
	}
}
