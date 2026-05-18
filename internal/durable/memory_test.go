package durable

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryStoreSpawnIdempotency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)

	first, err := client.Spawn(ctx, SpawnRequest{
		Name:           "noop",
		UserID:         7,
		IdempotencyKey: "same",
	})
	if err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	if !first.Created {
		t.Fatal("first spawn should create a task")
	}
	second, err := client.Spawn(ctx, SpawnRequest{
		Name:           "noop",
		UserID:         7,
		IdempotencyKey: "same",
	})
	if err != nil {
		t.Fatalf("spawn second: %v", err)
	}
	if second.Created {
		t.Fatal("second spawn should reuse existing task")
	}
	if first.TaskID != second.TaskID {
		t.Fatalf("idempotent spawn returned different task IDs: %q != %q", first.TaskID, second.TaskID)
	}
}

func TestMemoryStoreClaimPreventsDuplicateActiveClaims(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	if _, _, err := store.SpawnTask(ctx, SpawnRequest{Name: "work"}); err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	if _, _, ok, err := store.ClaimNext(ctx, []string{DefaultQueue}, "worker-1", time.Minute); err != nil || !ok {
		t.Fatalf("first claim ok=%v err=%v", ok, err)
	}
	if _, _, ok, err := store.ClaimNext(ctx, []string{DefaultQueue}, "worker-2", time.Minute); err != nil {
		t.Fatalf("second claim: %v", err)
	} else if ok {
		t.Fatal("second worker claimed an actively leased task")
	}
}

func TestWorkerReusesCheckpointAcrossRetry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	registry := NewRegistry()
	var handlerCalls int32
	var stepCalls int32
	registry.Register(DefaultQueue, "checkpointed", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		call := atomic.AddInt32(&handlerCalls, 1)
		value, err := Step[int](ctx, "expensive", func(context.Context) (int, error) {
			atomic.AddInt32(&stepCalls, 1)
			return 42, nil
		})
		if err != nil {
			return nil, err
		}
		if call == 1 {
			return nil, errors.New("transient")
		}
		return map[string]any{"value": value}, nil
	})
	if _, err := client.Spawn(ctx, SpawnRequest{
		Name:        "checkpointed",
		RetryPolicy: RetryPolicy{MaxAttempts: 2, Backoff: "none"},
	}); err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	worker := NewWorker(store, client, registry, WorkerOptions{WorkerID: "test", Lease: time.Minute})
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if got := atomic.LoadInt32(&stepCalls); got != 1 {
		t.Fatalf("step called %d times, want 1", got)
	}
	stats, err := store.QueueStats(ctx)
	if err != nil {
		t.Fatalf("queue stats: %v", err)
	}
	if len(stats) != 1 || stats[0].Completed != 1 {
		t.Fatalf("task did not complete after retry: %+v", stats)
	}
}

func TestTimerWaitResumesTask(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	registry := NewRegistry()
	registry.Register(DefaultQueue, "timer", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		if err := SleepFor(ctx, "delay", time.Hour); err != nil {
			return nil, err
		}
		return map[string]any{"done": true}, nil
	})
	spawn, err := client.Spawn(ctx, SpawnRequest{Name: "timer"})
	if err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	worker := NewWorker(store, client, registry, WorkerOptions{WorkerID: "test", Lease: time.Minute})
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("initial run: %v", err)
	}
	task, ok, err := store.GetTask(ctx, 0, spawn.TaskID)
	if err != nil || !ok {
		t.Fatalf("get task ok=%v err=%v", ok, err)
	}
	if task.Status != TaskStatusWaiting {
		t.Fatalf("status = %s, want waiting", task.Status)
	}
	if fired, err := store.FireDueTimers(ctx, time.Now().Add(2*time.Hour), 10); err != nil {
		t.Fatalf("fire timers: %v", err)
	} else if fired != 1 {
		t.Fatalf("fired timers = %d, want 1", fired)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	snapshot, err := client.FetchResult(ctx, 0, spawn.TaskID)
	if err != nil {
		t.Fatalf("fetch result: %v", err)
	}
	if snapshot.State != TaskStatusCompleted {
		t.Fatalf("status = %s, want completed", snapshot.State)
	}
}

func TestEventWaitResumesTask(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	registry := NewRegistry()
	registry.Register(DefaultQueue, "event", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		payload, err := AwaitEvent[map[string]any](ctx, "continue", 0)
		if err != nil {
			return nil, err
		}
		return map[string]any{"payload": payload}, nil
	})
	spawn, err := client.Spawn(ctx, SpawnRequest{Name: "event", UserID: 9})
	if err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	worker := NewWorker(store, client, registry, WorkerOptions{WorkerID: "test", Lease: time.Minute})
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("initial run: %v", err)
	}
	if _, err := client.EmitEvent(ctx, 9, DefaultQueue, "continue", map[string]any{"ok": true}); err != nil {
		t.Fatalf("emit event: %v", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	snapshot, err := client.FetchResult(ctx, 9, spawn.TaskID)
	if err != nil {
		t.Fatalf("fetch result: %v", err)
	}
	if snapshot.State != TaskStatusCompleted {
		t.Fatalf("status = %s, want completed", snapshot.State)
	}
}

func TestChildCompletionResumesParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	registry := NewRegistry()
	registry.Register(DefaultQueue, "child", func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{"value": "child-result"}, nil
	})
	registry.Register(DefaultQueue, "parent", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		child, err := SpawnChild(ctx, "child", SpawnRequest{Name: "child"})
		if err != nil {
			return nil, err
		}
		snapshot, err := AwaitChild(ctx, child.TaskID)
		if err != nil {
			return nil, err
		}
		var childResult map[string]any
		if err := snapshot.DecodeResult(&childResult); err != nil {
			return nil, err
		}
		return map[string]any{"child": childResult}, nil
	})
	spawn, err := client.Spawn(ctx, SpawnRequest{Name: "parent", UserID: 11})
	if err != nil {
		t.Fatalf("spawn parent: %v", err)
	}
	worker := NewWorker(store, client, registry, WorkerOptions{WorkerID: "test", Lease: time.Minute})
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("parent wait run: %v", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("child run: %v", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("parent resume run: %v", err)
	}
	snapshot, err := client.FetchResult(ctx, 11, spawn.TaskID)
	if err != nil {
		t.Fatalf("fetch parent result: %v", err)
	}
	if snapshot.State != TaskStatusCompleted {
		t.Fatalf("status = %s, want completed", snapshot.State)
	}
}

func TestCancellationPreventsLateCompletion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	registry := NewRegistry()
	registry.Register(DefaultQueue, "self-cancel", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		tc, ok := FromContext(ctx)
		if !ok {
			return nil, errors.New("missing task context")
		}
		if err := tc.Client.Cancel(ctx, tc.Task.UserID, tc.Task.ID); err != nil {
			return nil, err
		}
		return map[string]any{"late": true}, nil
	})
	spawn, err := client.Spawn(ctx, SpawnRequest{Name: "self-cancel", UserID: 12})
	if err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	worker := NewWorker(store, client, registry, WorkerOptions{WorkerID: "test", Lease: time.Minute})
	if err := worker.RunOnce(ctx); !errors.Is(err, ErrCancelled) {
		t.Fatalf("run err = %v, want ErrCancelled", err)
	}
	snapshot, err := client.FetchResult(ctx, 12, spawn.TaskID)
	if err != nil {
		t.Fatalf("fetch result: %v", err)
	}
	if snapshot.State != TaskStatusCancelled {
		t.Fatalf("status = %s, want cancelled", snapshot.State)
	}
}
