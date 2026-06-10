package databases

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
		RouteTarget:     room.RouteTarget,
		Title:           "Check queue",
		Prompt:          "Inspect the queue and summarize backlog",
		IntervalSeconds: 300,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	tasks, err := store.ListTasks(ctx, room.RoomID, room.RouteTarget)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	claimToken := uuid.NewString()
	claimed, err := store.ClaimRoom(ctx, room.RoomID, room.RouteTarget, claimToken, time.Now().Add(2*time.Minute))
	if err != nil {
		t.Fatalf("claim room: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim to succeed")
	}
	if err := store.CompleteRoomPulse(ctx, persistence.RoomPulseCompletion{
		RoomID:      room.RoomID,
		RouteTarget: room.RouteTarget,
		Token:       claimToken,
		CompletedAt: time.Now().UTC(),
		Summary:     "completed",
		DueTaskIDs:  []string{tasks[0].ID},
	}); err != nil {
		t.Fatalf("complete room pulse: %v", err)
	}

	updatedTasks, err := store.ListTasks(ctx, room.RoomID, room.RouteTarget)
	if err != nil {
		t.Fatalf("list updated tasks: %v", err)
	}
	if updatedTasks[0].LastRunAt.IsZero() {
		t.Fatalf("expected task last_run_at to be updated")
	}
	updatedRoom, err := store.GetRoom(ctx, room.RoomID, room.RouteTarget)
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

func TestMemPulseStoreTracksDurableTaskRunState(t *testing.T) {
	t.Parallel()

	store := NewPulseStore(nil)
	exercisePulseStoreDurableTaskRunState(t, store)
}

func TestSQLitePulseStoreTracksDurableTaskRunState(t *testing.T) {
	t.Parallel()

	store := NewSQLitePulseStore(openTestSQLite(t))
	exercisePulseStoreDurableTaskRunState(t, store)
}

func TestPostgresPulseStoreTracksDurableTaskRunState(t *testing.T) {
	t.Parallel()

	dsn := strings.TrimSpace(os.Getenv("MANIFOLD_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	}
	if dsn == "" {
		t.Skip("set MANIFOLD_TEST_POSTGRES_DSN or POSTGRES_TEST_DSN to run Postgres Pulse store integration tests")
	}

	store := NewPulseStore(openTestPostgresSchema(t, dsn))
	exercisePulseStoreDurableTaskRunState(t, store)
}

func exercisePulseStoreDurableTaskRunState(t *testing.T, store persistence.PulseStore) {
	t.Helper()
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
	room, err := store.EnsureRoom(ctx, "!room:test", "weather")
	if err != nil {
		t.Fatalf("ensure room: %v", err)
	}
	task, err := store.UpsertTask(ctx, persistence.PulseTask{
		RoomID:          room.RoomID,
		RouteTarget:     room.RouteTarget,
		Title:           "Check queue",
		Prompt:          "Inspect the queue and summarize backlog",
		IntervalSeconds: 300,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	queued, err := store.MarkTaskRunQueued(ctx, room.RoomID, room.RouteTarget, task.ID, "durable-1")
	if err != nil {
		t.Fatalf("mark task run queued: %v", err)
	}
	if queued.ActiveDurableTaskID != "durable-1" {
		t.Fatalf("expected active durable task id, got %#v", queued)
	}
	claimToken := uuid.NewString()
	claimed, err := store.ClaimRoom(ctx, room.RoomID, room.RouteTarget, claimToken, time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("claim room: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim to succeed")
	}

	if err := store.CompleteRoomPulse(ctx, persistence.RoomPulseCompletion{
		RoomID:        room.RoomID,
		RouteTarget:   room.RouteTarget,
		DurableTaskID: "durable-1",
		CompletedAt:   time.Now().UTC(),
		Summary:       "completed",
		DueTaskIDs:    []string{task.ID},
	}); err != nil {
		t.Fatalf("complete room pulse: %v", err)
	}
	tasks, err := store.ListTasks(ctx, room.RoomID, room.RouteTarget)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if tasks[0].ActiveDurableTaskID != "" || tasks[0].LastDurableTaskID != "durable-1" {
		t.Fatalf("expected durable task linkage to move to last run, got %#v", tasks[0])
	}
	claimedRoom, err := store.GetRoom(ctx, room.RoomID, room.RouteTarget)
	if err != nil {
		t.Fatalf("get claimed room: %v", err)
	}
	if claimedRoom.ActiveClaimToken != claimToken {
		t.Fatalf("expected tokenless durable completion to preserve active claim %q, got %#v", claimToken, claimedRoom)
	}
}

func openTestPostgresSchema(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	adminPool, err := OpenPool(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres pool: %v", err)
	}
	schema := "pulse_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create postgres test schema: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		adminPool.Close()
		t.Fatalf("parse postgres dsn: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		adminPool.Close()
		t.Fatalf("open postgres schema pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
		adminPool.Close()
	})
	return pool
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
	claimed, err := store.ClaimRoom(ctx, room.RoomID, room.RouteTarget, claimToken, time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("claim room: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim to succeed")
	}

	if err := store.ClearRoomClaim(ctx, room.RoomID, room.RouteTarget); err != nil {
		t.Fatalf("clear room claim: %v", err)
	}

	updatedRoom, err := store.GetRoom(ctx, room.RoomID, room.RouteTarget)
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
	if _, err := store.UpsertTask(ctx, persistence.PulseTask{RoomID: roomID, RouteTarget: botA, Title: "A", Prompt: "Do A", IntervalSeconds: 60, Enabled: true}); err != nil {
		t.Fatalf("upsert task botA: %v", err)
	}
	if _, err := store.UpsertTask(ctx, persistence.PulseTask{RoomID: roomID, RouteTarget: botB, Title: "B", Prompt: "Do B", IntervalSeconds: 60, Enabled: true}); err != nil {
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
	if len(tasksA) != 1 || tasksA[0].RouteTarget != botA {
		t.Fatalf("expected isolated botA tasks, got %#v", tasksA)
	}
	if len(tasksB) != 1 || tasksB[0].RouteTarget != botB {
		t.Fatalf("expected isolated botB tasks, got %#v", tasksB)
	}
}

func TestMemPulseStoreListRoomsReturnsAllScopesWhenBotFilterEmpty(t *testing.T) {
	t.Parallel()

	store := NewPulseStore(nil)
	ctx := context.Background()
	if _, err := store.EnsureRoom(ctx, "!room:test", "weather"); err != nil {
		t.Fatalf("ensure weather room: %v", err)
	}
	if _, err := store.EnsureRoom(ctx, "!room:test", "gpt"); err != nil {
		t.Fatalf("ensure gpt room: %v", err)
	}

	rooms, err := store.ListRooms(ctx, "")
	if err != nil {
		t.Fatalf("list all rooms: %v", err)
	}
	if len(rooms) != 2 {
		t.Fatalf("expected 2 rooms, got %#v", rooms)
	}
}
