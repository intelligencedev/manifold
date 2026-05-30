package durable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type Worker struct {
	store    Store
	client   *Client
	registry *Registry
	workerID string
	lease    time.Duration
	poll     time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
	once     sync.Once
}

type WorkerOptions struct {
	WorkerID     string
	Lease        time.Duration
	PollInterval time.Duration
}

func NewWorker(store Store, client *Client, registry *Registry, opts WorkerOptions) *Worker {
	workerID := opts.WorkerID
	if workerID == "" {
		workerID = fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}
	lease := opts.Lease
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	poll := opts.PollInterval
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	return &Worker{store: store, client: client, registry: registry, workerID: workerID, lease: lease, poll: poll}
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil || w.store == nil || w.registry == nil {
		return
	}
	if w.cancel != nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		w.loop(runCtx)
	}()
}

func (w *Worker) Close() error {
	if w == nil {
		return nil
	}
	if w.cancel != nil {
		w.cancel()
	}
	if w.done != nil {
		<-w.done
	}
	w.cancel = nil
	w.done = nil
	return nil
}

func (w *Worker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		_ = w.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if w == nil || w.store == nil || w.registry == nil {
		return nil
	}
	_, _ = w.store.FireDueTimers(ctx, time.Now().UTC(), 100)
	queues := w.registry.Queues()
	if len(queues) == 0 {
		return nil
	}
	task, run, ok, err := w.store.ClaimNext(ctx, queues, w.workerID, w.lease)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	spec, ok := w.registry.Get(task.Queue, task.Name)
	if !ok {
		return w.store.FailTask(ctx, task.ID, run.ID, []byte(`{"error":"handler not found"}`), ErrHandlerNotFound.Error(), time.Time{})
	}
	tc := TaskContext{Task: task, Run: run, Store: w.store, Client: w.client}
	runCtx := WithTaskContext(ctx, tc)
	result, runErr := spec.Fn(runCtx, cloneMap(task.Params))
	if runErr != nil {
		if errors.Is(runErr, ErrSuspended) {
			return w.store.MarkTaskWaiting(ctx, task.ID, run.ID)
		}
		if errors.Is(runErr, ErrCancelled) {
			return w.store.CancelTask(ctx, task.UserID, task.ID)
		}
		next := nextAttemptAt(task.RetryPolicy, run.Attempt)
		failure, _ := json.Marshal(map[string]any{"error": runErr.Error()})
		return w.store.FailTask(ctx, task.ID, run.ID, failure, runErr.Error(), next)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		next := nextAttemptAt(task.RetryPolicy, run.Attempt)
		failure, _ := json.Marshal(map[string]any{"error": err.Error()})
		return w.store.FailTask(ctx, task.ID, run.ID, failure, err.Error(), next)
	}
	if err := w.store.CompleteTask(ctx, task.ID, run.ID, raw); err != nil {
		log.Warn().Err(err).Str("task_id", task.ID).Msg("durable_complete_task_failed")
		return err
	}
	return w.store.WakeChildWaits(ctx, task.ID)
}

func nextAttemptAt(policy RetryPolicy, attempt int) time.Time {
	max := policy.MaxAttempts
	if max <= 0 {
		max = 3
	}
	if attempt >= max {
		return time.Time{}
	}
	base := policy.BaseDelaySeconds
	if base <= 0 {
		base = 1
	}
	delay := time.Duration(base) * time.Second
	switch policy.Backoff {
	case "exponential":
		for i := 1; i < attempt; i++ {
			delay *= 2
		}
	case "none":
		delay = 0
	}
	if policy.MaxDelaySeconds > 0 && delay > time.Duration(policy.MaxDelaySeconds)*time.Second {
		delay = time.Duration(policy.MaxDelaySeconds) * time.Second
	}
	return time.Now().UTC().Add(delay)
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	raw, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}
