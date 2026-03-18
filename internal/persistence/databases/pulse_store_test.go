package databases

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"manifold/internal/persistence"
)

func TestMemPulseStoreClaimAndComplete(t *testing.T) {
	t.Parallel()

	store := NewPulseStore(nil)
	ctx := context.Background()
	room, err := store.EnsureRoom(ctx, "!room:test")
	if err != nil {
		t.Fatalf("ensure room: %v", err)
	}
	_, err = store.UpsertTask(ctx, persistence.PulseTask{
		RoomID:          room.RoomID,
		Title:           "Check queue",
		Prompt:          "Inspect the queue and summarize backlog",
		IntervalSeconds: 300,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	tasks, err := store.ListTasks(ctx, room.RoomID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	claimToken := uuid.NewString()
	claimed, err := store.ClaimRoom(ctx, room.RoomID, claimToken, time.Now().Add(2*time.Minute))
	if err != nil {
		t.Fatalf("claim room: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim to succeed")
	}
	if err := store.CompleteRoomPulse(ctx, room.RoomID, claimToken, time.Now().UTC(), "completed", "", []string{tasks[0].ID}); err != nil {
		t.Fatalf("complete room pulse: %v", err)
	}

	updatedTasks, err := store.ListTasks(ctx, room.RoomID)
	if err != nil {
		t.Fatalf("list updated tasks: %v", err)
	}
	if updatedTasks[0].LastRunAt.IsZero() {
		t.Fatalf("expected task last_run_at to be updated")
	}
	updatedRoom, err := store.GetRoom(ctx, room.RoomID)
	if err != nil {
		t.Fatalf("get updated room: %v", err)
	}
	if updatedRoom.ActiveClaimToken != "" {
		t.Fatalf("expected room claim token to be cleared, got %q", updatedRoom.ActiveClaimToken)
	}
	if updatedRoom.LastPulseSummary != "completed" {
		t.Fatalf("expected last pulse summary to be recorded, got %q", updatedRoom.LastPulseSummary)
	}
}

func TestMemPulseStoreClearRoomClaim(t *testing.T) {
	t.Parallel()

	store := NewPulseStore(nil)
	ctx := context.Background()
	room, err := store.EnsureRoom(ctx, "!room:test")
	if err != nil {
		t.Fatalf("ensure room: %v", err)
	}

	claimToken := uuid.NewString()
	claimed, err := store.ClaimRoom(ctx, room.RoomID, claimToken, time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("claim room: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim to succeed")
	}

	if err := store.ClearRoomClaim(ctx, room.RoomID); err != nil {
		t.Fatalf("clear room claim: %v", err)
	}

	updatedRoom, err := store.GetRoom(ctx, room.RoomID)
	if err != nil {
		t.Fatalf("get updated room: %v", err)
	}
	if updatedRoom.ActiveClaimToken != "" {
		t.Fatalf("expected room claim token to be cleared, got %q", updatedRoom.ActiveClaimToken)
	}
	if !updatedRoom.ActiveClaimUntil.IsZero() {
		t.Fatalf("expected room claim expiry to be cleared, got %v", updatedRoom.ActiveClaimUntil)
	}
	if updatedRoom.Revision <= room.Revision {
		t.Fatalf("expected room revision to advance, got %d <= %d", updatedRoom.Revision, room.Revision)
	}
}
