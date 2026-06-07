package durable

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	sqlitep "manifold/internal/persistence/sqlite"
)

func TestSQLiteStorePersistsTasksAcrossReopen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := t.TempDir() + "/durable.db"

	store, db := newSQLiteDurableStore(t, path)
	client := NewClient(store)
	spawned, err := client.Spawn(ctx, SpawnRequest{Name: "persisted", UserID: 42, IdempotencyKey: "same"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	again, err := client.Spawn(ctx, SpawnRequest{Name: "persisted", UserID: 42, IdempotencyKey: "same"})
	if err != nil {
		t.Fatalf("spawn again: %v", err)
	}
	if again.Created || again.TaskID != spawned.TaskID {
		t.Fatalf("idempotent spawn = %+v, want existing task %s", again, spawned.TaskID)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	reopened, reopenedDB := newSQLiteDurableStore(t, path)
	t.Cleanup(func() { _ = reopenedDB.Close() })
	task, found, err := reopened.GetTask(ctx, 42, spawned.TaskID)
	if err != nil || !found {
		t.Fatalf("reopened task found=%v err=%v", found, err)
	}
	if task.Name != "persisted" || task.Status != TaskStatusQueued {
		t.Fatalf("reopened task = %+v", task)
	}
}

func TestSQLiteStoreClaimCompleteRetryAndCheckpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, db := newSQLiteDurableStore(t, t.TempDir()+"/durable.db")
	t.Cleanup(func() { _ = db.Close() })
	client := NewClient(store)

	completable, err := client.Spawn(ctx, SpawnRequest{Name: "complete-me", UserID: 7})
	if err != nil {
		t.Fatalf("spawn completable: %v", err)
	}
	task, run, ok, err := store.ClaimNext(ctx, []string{DefaultQueue}, "worker-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim completable ok=%v err=%v", ok, err)
	}
	if task.ID != completable.TaskID {
		t.Fatalf("claimed task %s, want %s", task.ID, completable.TaskID)
	}
	if _, _, ok, err := store.ClaimNext(ctx, []string{DefaultQueue}, "worker-2", time.Minute); err != nil {
		t.Fatalf("second claim: %v", err)
	} else if ok {
		t.Fatal("second worker claimed an actively leased task")
	}
	if err := store.Heartbeat(ctx, task.ID, run.ID, "worker-1", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := store.CompleteTask(ctx, task.ID, run.ID, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	events, _, found, err := client.ListEvents(ctx, 7, completable.TaskID, 0)
	if err != nil || !found {
		t.Fatalf("list events found=%v err=%v", found, err)
	}
	if len(events) != 1 || events[0].Name != "task_completed" {
		t.Fatalf("events = %+v, want task_completed", events)
	}

	retryable, err := client.Spawn(ctx, SpawnRequest{Name: "retry-me", UserID: 7, RetryPolicy: RetryPolicy{MaxAttempts: 1}})
	if err != nil {
		t.Fatalf("spawn retryable: %v", err)
	}
	task, run, ok, err = store.ClaimNext(ctx, []string{DefaultQueue}, "worker-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim retryable ok=%v err=%v", ok, err)
	}
	if _, err := store.SaveCheckpoint(ctx, task.ID, "step", []byte(`{"saved":true}`)); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	if err := store.FailTask(ctx, task.ID, run.ID, []byte(`{"error":"boom"}`), "boom", time.Time{}); err != nil {
		t.Fatalf("fail retryable: %v", err)
	}
	retried, err := client.Retry(ctx, 7, retryable.TaskID, true)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retried.Status != TaskStatusQueued || retried.Attempt != 0 || retried.CompletedAt != nil {
		t.Fatalf("retried task = %+v", retried)
	}
	if _, found, err := store.GetCheckpoint(ctx, task.ID, "step"); err != nil {
		t.Fatalf("get checkpoint: %v", err)
	} else if found {
		t.Fatal("checkpoint remained after reset retry")
	}
}

func TestSQLiteStoreParallelClaimsAreUnique(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, db := newSQLiteDurableStore(t, t.TempDir()+"/durable.db")
	t.Cleanup(func() { _ = db.Close() })
	client := NewClient(store)
	for i := 0; i < 25; i++ {
		if _, err := client.Spawn(ctx, SpawnRequest{Name: "parallel", UserID: 9}); err != nil {
			t.Fatalf("spawn %d: %v", i, err)
		}
	}

	var mu sync.Mutex
	claimed := map[string]bool{}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for {
				task, run, ok, err := store.ClaimNext(ctx, []string{DefaultQueue}, "worker", time.Minute)
				if err != nil {
					t.Errorf("claim worker %d: %v", worker, err)
					return
				}
				if !ok {
					return
				}
				mu.Lock()
				if claimed[task.ID] {
					t.Errorf("duplicate claim for task %s", task.ID)
				}
				claimed[task.ID] = true
				mu.Unlock()
				if err := store.CompleteTask(ctx, task.ID, run.ID, []byte(`{"ok":true}`)); err != nil {
					t.Errorf("complete %s: %v", task.ID, err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	if len(claimed) != 25 {
		t.Fatalf("claimed %d tasks, want 25", len(claimed))
	}
}

func newSQLiteDurableStore(t *testing.T, path string) (*SQLiteStore, *sql.DB) {
	t.Helper()
	db, err := sqlitep.Open(context.Background(), sqlitep.Config{
		Path:          path,
		WAL:           true,
		BusyTimeoutMs: 10000,
		MaxOpenConns:  1,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	store := &SQLiteStore{db: db}
	if err := store.Init(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("init sqlite durable store: %v", err)
	}
	return store, db
}
