package store

import (
	"context"
	"testing"
	"time"

	"manifold/internal/codeqa"
	sqlitep "manifold/internal/persistence/sqlite"
)

func TestSQLiteStoreRunAndEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sqlitep.Open(ctx, sqlitep.Config{
		Path:          t.TempDir() + "/codeqa.db",
		WAL:           true,
		BusyTimeoutMs: 10000,
		MaxOpenConns:  1,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewSQLiteStore(db)
	if err := store.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}

	run := codeqa.RunResult{
		RunID:      "run-1",
		Mode:       codeqa.ModeJudge,
		Status:     codeqa.StatusRunning,
		Repository: "/repo",
		StartedAt:  time.Now().UTC(),
	}
	if err := store.Save(ctx, 7, run); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := store.Get(ctx, 7, "run-1")
	if err != nil || !ok {
		t.Fatalf("get ok=%v err=%v", ok, err)
	}
	if got.RunID != run.RunID || got.Status != run.Status {
		t.Fatalf("got run = %+v", got)
	}

	event, err := store.AppendEvent(ctx, 7, codeqa.RunEvent{
		RunID:      "run-1",
		Type:       codeqa.RunEventStarted,
		Payload:    map[string]any{"step": "start"},
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("append event: %v", err)
	}
	if event.Sequence != 1 {
		t.Fatalf("event sequence = %d, want 1", event.Sequence)
	}
	events, err := store.ListEvents(ctx, 7, "run-1")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].Type != codeqa.RunEventStarted {
		t.Fatalf("events = %+v", events)
	}
}
