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
	room, err := store.EnsureRoom(ctx, "!room:test", "@manibot:matrix.test")
	if err != nil {
		t.Fatalf("ensure room: %v", err)
	}
	_, err = store.UpsertTask(ctx, persistence.PulseTask{
		RoomID:          room.RoomID,
		BotID:           room.BotID,
		Title:           "Check queue",
		Prompt:          "Inspect the queue and summarize backlog",
		IntervalSeconds: 300,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	tasks, err := store.ListTasks(ctx, room.RoomID, room.BotID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	claimToken := uuid.NewString()
	claimed, err := store.ClaimRoom(ctx, room.RoomID, room.BotID, claimToken, time.Now().Add(2*time.Minute))
	if err != nil {
		t.Fatalf("claim room: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim to succeed")
	}
	if err := store.CompleteRoomPulse(ctx, room.RoomID, room.BotID, claimToken, time.Now().UTC(), "completed", "", []string{tasks[0].ID}); err != nil {
		t.Fatalf("complete room pulse: %v", err)
	}

	updatedTasks, err := store.ListTasks(ctx, room.RoomID, room.BotID)
	if err != nil {
		t.Fatalf("list updated tasks: %v", err)
	}
	if updatedTasks[0].LastRunAt.IsZero() {
		t.Fatalf("expected task last_run_at to be updated")
	}
	updatedRoom, err := store.GetRoom(ctx, room.RoomID, room.BotID)
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
	room, err := store.EnsureRoom(ctx, "!room:test", "@manibot:matrix.test")
	if err != nil {
		t.Fatalf("ensure room: %v", err)
	}

	claimToken := uuid.NewString()
	claimed, err := store.ClaimRoom(ctx, room.RoomID, room.BotID, claimToken, time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("claim room: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim to succeed")
	}

	if err := store.ClearRoomClaim(ctx, room.RoomID, room.BotID); err != nil {
		t.Fatalf("clear room claim: %v", err)
	}

	updatedRoom, err := store.GetRoom(ctx, room.RoomID, room.BotID)
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

func TestMemPulseStoreSeparatesBotsInSameRoom(t *testing.T) {
	t.Parallel()

	store := NewPulseStore(nil)
	ctx := context.Background()
	roomID := "!room:test"
	botA := "@manibot:matrix.test"
	botB := "@gpt_bot:matrix.test"

	if _, err := store.EnsureRoom(ctx, roomID, botA); err != nil {
		t.Fatalf("ensure room botA: %v", err)
	}
	if _, err := store.EnsureRoom(ctx, roomID, botB); err != nil {
		t.Fatalf("ensure room botB: %v", err)
	}
	if _, err := store.UpsertTask(ctx, persistence.PulseTask{RoomID: roomID, BotID: botA, Title: "A", Prompt: "Do A", IntervalSeconds: 60, Enabled: true}); err != nil {
		t.Fatalf("upsert task botA: %v", err)
	}
	if _, err := store.UpsertTask(ctx, persistence.PulseTask{RoomID: roomID, BotID: botB, Title: "B", Prompt: "Do B", IntervalSeconds: 60, Enabled: true}); err != nil {
		t.Fatalf("upsert task botB: %v", err)
	}

	tasksA, err := store.ListTasks(ctx, roomID, botA)
	if err != nil {
		t.Fatalf("list tasks botA: %v", err)
	}
	tasksB, err := store.ListTasks(ctx, roomID, botB)
	if err != nil {
		t.Fatalf("list tasks botB: %v", err)
	}
	if len(tasksA) != 1 || tasksA[0].BotID != botA {
		t.Fatalf("expected isolated botA tasks, got %#v", tasksA)
	}
	if len(tasksB) != 1 || tasksB[0].BotID != botB {
		t.Fatalf("expected isolated botB tasks, got %#v", tasksB)
	}
}
