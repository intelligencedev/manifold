package durable

import (
	"context"
	"errors"
	"strings"
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

func TestWorkerStartRunsConfiguredQueueConcurrently(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMemoryStore()
	client := NewClient(store)
	registry := NewRegistry()
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	released := false

	registry.Register("chat", "slow", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return map[string]any{"ok": true}, nil
		}
	})

	spawns := make([]SpawnResult, 0, 2)
	for range 2 {
		spawn, err := client.Spawn(ctx, SpawnRequest{Queue: "chat", Name: "slow", UserID: 7})
		if err != nil {
			t.Fatalf("spawn chat task: %v", err)
		}
		spawns = append(spawns, spawn)
	}

	worker := NewWorker(store, client, registry, WorkerOptions{
		WorkerID:     "test",
		Lease:        time.Minute,
		PollInterval: 5 * time.Millisecond,
		QueueConcurrency: map[string]int{
			"chat": 2,
		},
	})
	worker.Start(ctx)
	defer func() {
		if !released {
			close(release)
		}
		if err := worker.Close(); err != nil {
			t.Fatalf("close worker: %v", err)
		}
	}()

	for i := range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("chat task %d did not start concurrently", i+1)
		}
	}

	close(release)
	released = true
	doneCtx, doneCancel := context.WithTimeout(ctx, time.Second)
	defer doneCancel()
	for _, spawn := range spawns {
		snapshot, err := client.AwaitResult(doneCtx, 7, spawn.TaskID, 5*time.Millisecond)
		if err != nil {
			t.Fatalf("await task %s: %v", spawn.TaskID, err)
		}
		if snapshot.State != TaskStatusCompleted {
			t.Fatalf("task %s status = %s, want completed", spawn.TaskID, snapshot.State)
		}
	}
}

func TestMemoryStoreListTasksFiltersByUserQueueStatusAndName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	if _, err := client.Spawn(ctx, SpawnRequest{Queue: "ops", Name: "deploy", UserID: 7}); err != nil {
		t.Fatalf("spawn deploy: %v", err)
	}
	if _, err := client.Spawn(ctx, SpawnRequest{Queue: "mail", Name: "digest", UserID: 7}); err != nil {
		t.Fatalf("spawn digest: %v", err)
	}
	if _, err := client.Spawn(ctx, SpawnRequest{Queue: "other", Name: "deploy", UserID: 8}); err != nil {
		t.Fatalf("spawn other user: %v", err)
	}
	claimed, _, ok, err := store.ClaimNext(ctx, []string{"ops"}, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim ops task ok=%v err=%v", ok, err)
	}
	if claimed.UserID != 7 {
		t.Fatalf("claimed user = %d, want 7", claimed.UserID)
	}
	all, err := client.ListTasks(ctx, 7, TaskListFilter{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("list all count = %d, want 2: %+v", len(all), all)
	}
	running, err := client.ListTasks(ctx, 7, TaskListFilter{Queue: "ops", Status: TaskStatusRunning})
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(running) != 1 || running[0].ID != claimed.ID {
		t.Fatalf("running filter = %+v, want claimed task %s", running, claimed.ID)
	}
	named, err := client.ListTasks(ctx, 7, TaskListFilter{Name: "digest", Limit: 1})
	if err != nil {
		t.Fatalf("list named: %v", err)
	}
	if len(named) != 1 || named[0].Name != "digest" || named[0].Queue != "mail" {
		t.Fatalf("name filter = %+v, want mail digest", named)
	}
}

func TestMemoryStoreListTasksPagePaginates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	for i := 0; i < 3; i++ {
		if _, err := client.Spawn(ctx, SpawnRequest{Queue: "ops", Name: "deploy", UserID: 7}); err != nil {
			t.Fatalf("spawn task %d: %v", i, err)
		}
	}

	first, err := client.ListTasksPage(ctx, 7, TaskListFilter{Queue: "ops", Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Tasks) != 2 || first.Total != 3 || !first.HasMore || first.Offset != 0 || first.Limit != 2 {
		t.Fatalf("first page = %+v, want 2 of 3 with has_more", first)
	}

	second, err := client.ListTasksPage(ctx, 7, TaskListFilter{Queue: "ops", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Tasks) != 1 || second.Total != 3 || second.HasMore || second.Offset != 2 || second.Limit != 2 {
		t.Fatalf("second page = %+v, want final task", second)
	}
}

func TestMemoryStorePruneTerminalTasks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	oldCompleted, err := client.Spawn(ctx, SpawnRequest{Queue: "ops", Name: "old-completed", UserID: 7})
	if err != nil {
		t.Fatalf("spawn old completed: %v", err)
	}
	oldFailed, err := client.Spawn(ctx, SpawnRequest{Queue: "ops", Name: "old-failed", UserID: 7})
	if err != nil {
		t.Fatalf("spawn old failed: %v", err)
	}
	oldCancelled, err := client.Spawn(ctx, SpawnRequest{Queue: "ops", Name: "old-cancelled", UserID: 7})
	if err != nil {
		t.Fatalf("spawn old cancelled: %v", err)
	}
	activeQueued, err := client.Spawn(ctx, SpawnRequest{Queue: "ops", Name: "active", UserID: 7})
	if err != nil {
		t.Fatalf("spawn active: %v", err)
	}
	recentCompleted, err := client.Spawn(ctx, SpawnRequest{Queue: "ops", Name: "recent", UserID: 7})
	if err != nil {
		t.Fatalf("spawn recent: %v", err)
	}
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	recent := now.Add(-time.Hour)
	store.mu.Lock()
	markTaskTerminalForTest(store, oldCompleted.TaskID, TaskStatusCompleted, old)
	markTaskTerminalForTest(store, oldFailed.TaskID, TaskStatusFailed, old)
	markTaskTerminalForTest(store, oldCancelled.TaskID, TaskStatusCancelled, old)
	markTaskTerminalForTest(store, recentCompleted.TaskID, TaskStatusCompleted, recent)
	store.runs["run-old"] = Run{ID: "run-old", TaskID: oldCompleted.TaskID, Status: RunStatusCompleted}
	store.waits["wait-old"] = Wait{ID: "wait-old", TaskID: oldCompleted.TaskID, Status: "waiting"}
	store.mu.Unlock()
	if _, err := store.AppendTaskEvent(ctx, oldCompleted.TaskID, "old.event", nil); err != nil {
		t.Fatalf("append old event: %v", err)
	}
	if _, err := store.SaveCheckpoint(ctx, oldCompleted.TaskID, "step", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("save old checkpoint: %v", err)
	}

	removed, err := client.PruneTerminalTasks(ctx, now.Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("prune terminal tasks: %v", err)
	}
	if removed != 3 {
		t.Fatalf("removed = %d, want 3", removed)
	}
	remaining, err := client.ListTasks(ctx, 7, TaskListFilter{Queue: "ops"})
	if err != nil {
		t.Fatalf("list remaining: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining = %+v, want active and recent", remaining)
	}
	if _, found, err := store.GetTask(ctx, 7, activeQueued.TaskID); err != nil || !found {
		t.Fatalf("active queued task found=%v err=%v", found, err)
	}
	if _, found, err := store.GetTask(ctx, 7, recentCompleted.TaskID); err != nil || !found {
		t.Fatalf("recent completed task found=%v err=%v", found, err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.runs["run-old"]; ok {
		t.Fatal("old run was not pruned")
	}
	if _, ok := store.waits["wait-old"]; ok {
		t.Fatal("old wait was not pruned")
	}
	for key := range store.checkpoints {
		if strings.HasPrefix(key, oldCompleted.TaskID+"\x00") {
			t.Fatal("old checkpoint was not pruned")
		}
	}
	for _, event := range store.events {
		if event.TaskID == oldCompleted.TaskID {
			t.Fatal("old task event was not pruned")
		}
	}
}

func markTaskTerminalForTest(store *MemoryStore, taskID string, status TaskStatus, completedAt time.Time) {
	task := store.tasks[taskID]
	task.Status = status
	task.CompletedAt = &completedAt
	task.UpdatedAt = completedAt
	store.tasks[taskID] = task
}

func TestMemoryStoreRetryTaskRequeuesAndControlsCheckpoints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	spawn, err := client.Spawn(ctx, SpawnRequest{Name: "retryable", UserID: 15, RetryPolicy: RetryPolicy{MaxAttempts: 1}})
	if err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	task, run, ok, err := store.ClaimNext(ctx, []string{DefaultQueue}, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim task ok=%v err=%v", ok, err)
	}
	if _, err := store.SaveCheckpoint(ctx, task.ID, "step", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	if err := store.FailTask(ctx, task.ID, run.ID, []byte(`{"error":"boom"}`), "boom", time.Time{}); err != nil {
		t.Fatalf("fail task: %v", err)
	}
	retried, err := client.Retry(ctx, 15, spawn.TaskID, false)
	if err != nil {
		t.Fatalf("retry task: %v", err)
	}
	if retried.Status != TaskStatusQueued || retried.Attempt != 0 || retried.Error != "" || len(retried.Failure) != 0 || retried.CompletedAt != nil {
		t.Fatalf("retried task = %+v, want queued with cleared terminal state", retried)
	}
	if _, found, err := store.GetCheckpoint(ctx, task.ID, "step"); err != nil || !found {
		t.Fatalf("checkpoint after preserving retry found=%v err=%v", found, err)
	}
	events, _, found, err := client.ListEvents(ctx, 15, spawn.TaskID, 0)
	if err != nil || !found {
		t.Fatalf("list events found=%v err=%v", found, err)
	}
	if len(events) != 1 || events[0].Name != "task_retried" {
		t.Fatalf("events = %+v, want task_retried", events)
	}
	task, run, ok, err = store.ClaimNext(ctx, []string{DefaultQueue}, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim retried task ok=%v err=%v", ok, err)
	}
	if err := store.FailTask(ctx, task.ID, run.ID, []byte(`{"error":"boom again"}`), "boom again", time.Time{}); err != nil {
		t.Fatalf("fail retried task: %v", err)
	}
	if _, err := client.Retry(ctx, 15, spawn.TaskID, true); err != nil {
		t.Fatalf("retry task with checkpoint reset: %v", err)
	}
	if _, found, err := store.GetCheckpoint(ctx, task.ID, "step"); err != nil {
		t.Fatalf("checkpoint after reset: %v", err)
	} else if found {
		t.Fatal("checkpoint was preserved after reset_checkpoints=true")
	}
}

func TestMemoryStoreAppendTaskEventOnceDeduplicatesEventKey(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()
	task, _, err := store.SpawnTask(ctx, SpawnRequest{
		Queue:  "chat",
		Name:   "chat.run",
		UserID: 7,
	})
	if err != nil {
		t.Fatalf("SpawnTask() error = %v", err)
	}
	first, err := store.AppendTaskEventOnce(ctx, task.ID, "event:1", "chat.delta", map[string]any{"data": "a"})
	if err != nil {
		t.Fatalf("AppendTaskEventOnce first error = %v", err)
	}
	second, err := store.AppendTaskEventOnce(ctx, task.ID, "event:1", "chat.delta", map[string]any{"data": "b"})
	if err != nil {
		t.Fatalf("AppendTaskEventOnce second error = %v", err)
	}
	if first.ID != second.ID || first.Sequence != second.Sequence {
		t.Fatalf("expected same event, first=%#v second=%#v", first, second)
	}
	events, _, found, err := store.ListTaskEvents(ctx, 7, task.ID, 0)
	if err != nil {
		t.Fatalf("ListTaskEvents() error = %v", err)
	}
	if !found {
		t.Fatal("task events not found")
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if got := events[0].Payload["data"]; got != "a" {
		t.Fatalf("payload data = %v, want a", got)
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

func TestWorkerCancelTaskCancelsRunningContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	registry := NewRegistry()
	started := make(chan struct{})
	cancelled := make(chan struct{})
	registry.Register(DefaultQueue, "slow", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	})
	spawn, err := client.Spawn(ctx, SpawnRequest{Name: "slow", UserID: 13})
	if err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	worker := NewWorker(store, client, registry, WorkerOptions{WorkerID: "test", Lease: time.Minute, PollInterval: 10 * time.Millisecond})
	errCh := make(chan error, 1)
	go func() { errCh <- worker.RunOnce(ctx) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("task did not start")
	}
	if err := worker.CancelTask(ctx, 13, spawn.TaskID); err != nil {
		t.Fatalf("cancel task: %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("running task context was not cancelled")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("run once: %v", err)
	}
	snapshot, err := client.FetchResult(ctx, 13, spawn.TaskID)
	if err != nil {
		t.Fatalf("fetch result: %v", err)
	}
	if snapshot.State != TaskStatusCancelled {
		t.Fatalf("status = %s, want cancelled", snapshot.State)
	}
}

func TestCancelTaskTreeCancelsChildrenAndWaits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()
	client := NewClient(store)
	parent, err := client.Spawn(ctx, SpawnRequest{Name: "parent", UserID: 14})
	if err != nil {
		t.Fatalf("spawn parent: %v", err)
	}
	child, err := client.Spawn(ctx, SpawnRequest{Name: "child", UserID: 14, ParentTaskID: parent.TaskID})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	waitID := "wait-parent-input"
	if _, err := store.CreateWait(ctx, Wait{ID: waitID, TaskID: parent.TaskID, Kind: WaitKindEvent, EventName: "input", Status: "waiting"}); err != nil {
		t.Fatalf("create wait: %v", err)
	}
	cancelledIDs, err := client.CancelTree(ctx, 14, parent.TaskID)
	if err != nil {
		t.Fatalf("cancel tree: %v", err)
	}
	if !containsString(cancelledIDs, parent.TaskID) || !containsString(cancelledIDs, child.TaskID) {
		t.Fatalf("cancelled IDs = %v, want parent and child", cancelledIDs)
	}
	childSnapshot, err := client.FetchResult(ctx, 14, child.TaskID)
	if err != nil {
		t.Fatalf("fetch child: %v", err)
	}
	if childSnapshot.State != TaskStatusCancelled {
		t.Fatalf("child status = %s, want cancelled", childSnapshot.State)
	}
	wait, found, err := store.GetWait(ctx, waitID)
	if err != nil || !found {
		t.Fatalf("get wait found=%v err=%v", found, err)
	}
	if wait.Status != "cancelled" {
		t.Fatalf("wait status = %s, want cancelled", wait.Status)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
