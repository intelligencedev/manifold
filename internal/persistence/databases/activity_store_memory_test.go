package databases

import (
	"context"
	"testing"
	"time"

	"manifold/internal/persistence"
)

func TestMemSpecialistActivityStoreUpsertAndList(t *testing.T) {
	t.Parallel()

	store := NewMemorySpecialistActivityStore()
	userID := int64(7)
	now := time.Now().UTC()
	activities := []persistence.SpecialistActivityRecord{{
		ID:        "run-1:call-1",
		SessionID: "sess-1",
		RunID:     "run-1",
		CallID:    "call-1",
		Agent:     "developer",
		Status:    "running",
		StartedAt: now,
		UpdatedAt: now,
	}}
	if err := store.UpsertSessionActivities(context.Background(), &userID, "sess-1", activities); err != nil {
		t.Fatalf("UpsertSessionActivities: %v", err)
	}

	got, err := store.ListSessionActivities(context.Background(), &userID, "sess-1")
	if err != nil {
		t.Fatalf("ListSessionActivities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(got))
	}
	if got[0].Agent != "developer" {
		t.Fatalf("expected developer agent, got %q", got[0].Agent)
	}
	if got[0].UserID == nil || *got[0].UserID != userID {
		t.Fatalf("expected stored userID %d, got %#v", userID, got[0].UserID)
	}
}

func TestMemSpecialistActivityStoreDeleteRun(t *testing.T) {
	t.Parallel()

	store := NewMemorySpecialistActivityStore()
	userID := int64(9)
	now := time.Now().UTC()
	activities := []persistence.SpecialistActivityRecord{
		{ID: "run-1:call-1", SessionID: "sess-1", RunID: "run-1", CallID: "call-1", Agent: "developer", Status: "done", StartedAt: now, UpdatedAt: now},
		{ID: "run-2:call-2", SessionID: "sess-1", RunID: "run-2", CallID: "call-2", Agent: "reviewer", Status: "done", StartedAt: now, UpdatedAt: now.Add(time.Second)},
	}
	if err := store.UpsertSessionActivities(context.Background(), &userID, "sess-1", activities); err != nil {
		t.Fatalf("UpsertSessionActivities: %v", err)
	}
	if err := store.DeleteRunActivities(context.Background(), &userID, "sess-1", "run-1"); err != nil {
		t.Fatalf("DeleteRunActivities: %v", err)
	}

	got, err := store.ListSessionActivities(context.Background(), &userID, "sess-1")
	if err != nil {
		t.Fatalf("ListSessionActivities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 remaining activity, got %d", len(got))
	}
	if got[0].RunID != "run-2" {
		t.Fatalf("expected remaining run-2 activity, got %q", got[0].RunID)
	}
}
