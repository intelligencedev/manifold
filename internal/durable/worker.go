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
	activeMu sync.Mutex
	active   map[string]context.CancelFunc
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
	return &Worker{store: store, client: client, registry: registry, workerID: workerID, lease: lease, poll: poll, active: map[string]context.CancelFunc{}}
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
	taskCtx, cancelTask := context.WithCancel(ctx)
	unregister := w.registerActive(task.ID, cancelTask)
	defer unregister()
	defer cancelTask()
	stopWatcher := w.watchTaskCancellation(taskCtx, task.UserID, task.ID, cancelTask)
	defer stopWatcher()

	runCtx := WithTaskContext(taskCtx, tc)
	result, runErr := spec.Fn(runCtx, cloneMap(task.Params))
	if runErr != nil {
		if errors.Is(runErr, ErrSuspended) {
			if w.taskWasCancelled(ctx, task) {
				return w.store.CancelTask(ctx, task.UserID, task.ID)
			}
			return w.store.MarkTaskWaiting(ctx, task.ID, run.ID)
		}
		if errors.Is(runErr, ErrCancelled) || w.taskWasCancelled(ctx, task) && errors.Is(runErr, context.Canceled) {
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

func (w *Worker) CancelTask(ctx context.Context, userID int64, taskID string) error {
	if w == nil {
		return ErrNotFound
	}
	var taskIDs []string
	var err error
	if w.client != nil {
		taskIDs, err = w.client.CancelTree(ctx, userID, taskID)
	} else if w.store != nil {
		taskIDs, err = w.store.CancelTaskTree(ctx, userID, taskID)
	} else {
		err = ErrNotFound
	}
	if err != nil {
		return err
	}
	w.cancelActive(taskIDs)
	return nil
}

func (w *Worker) registerActive(taskID string, cancel context.CancelFunc) func() {
	if w == nil || cancel == nil {
		return func() {}
	}
	w.activeMu.Lock()
	w.active[taskID] = cancel
	w.activeMu.Unlock()
	return func() {
		w.activeMu.Lock()
		delete(w.active, taskID)
		w.activeMu.Unlock()
	}
}

func (w *Worker) cancelActive(taskIDs []string) {
	if w == nil || len(taskIDs) == 0 {
		return
	}
	w.activeMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if cancel := w.active[taskID]; cancel != nil {
			cancels = append(cancels, cancel)
		}
	}
	w.activeMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (w *Worker) watchTaskCancellation(ctx context.Context, userID int64, taskID string, cancel context.CancelFunc) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(w.poll)
		defer ticker.Stop()
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				if w.taskIDWasCancelled(userID, taskID) {
					return
				}
			}
		}
	}()
	return func() { close(stop) }
}

func (w *Worker) taskWasCancelled(ctx context.Context, task Task) bool {
	if task.Status == TaskStatusCancelled {
		return true
	}
	if w == nil || w.store == nil {
		return false
	}
	latest, found, err := w.store.GetTask(ctx, task.UserID, task.ID)
	return err == nil && found && latest.Status == TaskStatusCancelled
}

func (w *Worker) taskIDWasCancelled(userID int64, taskID string) bool {
	if w == nil || w.store == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	task, found, err := w.store.GetTask(ctx, userID, taskID)
	return err == nil && found && task.Status == TaskStatusCancelled
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
