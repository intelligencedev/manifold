package databases

import (
	"context"
	"testing"
	"time"

	"manifold/internal/persistence"
)

func TestMemoryLLMRequestStoreListsParentOnlySnapshotsForAssistantTurn(t *testing.T) {
	t.Parallel()

	chat := newMemoryChatStore()
	store := NewMemoryLLMRequestStore(chat)
	assertLLMRequestStoreListsParentOnlySnapshotsForAssistantTurn(t, chat, store)
}

func TestSQLiteLLMRequestStoreListsParentOnlySnapshotsForAssistantTurn(t *testing.T) {
	t.Parallel()

	db := openTestSQLite(t)
	chat := NewSQLiteChatStore(db)
	store := NewSQLiteLLMRequestStore(db, chat)
	assertLLMRequestStoreListsParentOnlySnapshotsForAssistantTurn(t, chat, store)
}

func assertLLMRequestStoreListsParentOnlySnapshotsForAssistantTurn(t *testing.T, chat persistence.ChatStore, store persistence.LLMRequestStore) {
	t.Helper()

	ctx := context.Background()
	if _, err := chat.EnsureSession(ctx, nil, "session-1", "Session"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	if err := chat.AppendMessages(ctx, nil, "session-1", []persistence.ChatMessage{
		{ID: "user-1", Role: "user", Content: "first prompt", CreatedAt: now},
		{ID: "assistant-1", Role: "assistant", Content: "first response", CreatedAt: now.Add(time.Millisecond)},
		{ID: "user-2", Role: "user", Content: "second prompt", CreatedAt: now.Add(2 * time.Millisecond)},
		{ID: "assistant-2", Role: "assistant", Content: "second response", CreatedAt: now.Add(3 * time.Millisecond)},
	}, "second response", ""); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	if err := store.AppendLLMRequest(ctx, persistence.LLMRequest{
		ID:                  "req-1",
		SessionID:           "session-1",
		ParentUserMessageID: "user-1",
		Model:               "gpt-test",
		Payload:             []byte(`{"messages":[]}`),
		CreatedAt:           now,
	}); err != nil {
		t.Fatalf("AppendLLMRequest: %v", err)
	}
	parentUserMessageID, ok, err := parentUserMessageIDForAssistantMessage(ctx, chat, nil, "session-1", "assistant-1")
	if err != nil {
		t.Fatalf("parentUserMessageIDForAssistantMessage: %v", err)
	}
	if !ok || parentUserMessageID != "user-1" {
		messages, listErr := chat.ListMessages(ctx, nil, "session-1", 0)
		if listErr != nil {
			t.Fatalf("ListMessages after parent resolution miss: %v", listErr)
		}
		t.Fatalf("expected assistant-1 to resolve to user-1, got id=%q ok=%v messages=%#v", parentUserMessageID, ok, messages)
	}

	got, err := store.ListLLMRequestsForMessage(ctx, nil, "session-1", "assistant-1")
	if err != nil {
		t.Fatalf("ListLLMRequestsForMessage assistant-1: %v", err)
	}
	if len(got) != 1 || got[0].ID != "req-1" {
		t.Fatalf("expected parent-linked request for assistant-1, got %#v", got)
	}

	next, err := store.ListLLMRequestsForMessage(ctx, nil, "session-1", "assistant-2")
	if err != nil {
		t.Fatalf("ListLLMRequestsForMessage assistant-2: %v", err)
	}
	if len(next) != 0 {
		t.Fatalf("expected no parent-linked requests for assistant-2, got %#v", next)
	}
}
