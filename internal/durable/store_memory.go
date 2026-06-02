package durable

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryStore struct {
	mu          sync.Mutex
	tasks       map[string]Task
	runs        map[string]Run
	checkpoints map[string]json.RawMessage
	events      []Event
	waits       map[string]Wait
	nextTaskID  int64
	nextRunID   int64
	nextEventID int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tasks:       map[string]Task{},
		runs:        map[string]Run{},
		checkpoints: map[string]json.RawMessage{},
		waits:       map[string]Wait{},
	}
}

func (s *MemoryStore) Init(context.Context) error { return nil }

func (s *MemoryStore) Close() {}

func (s *MemoryStore) SpawnTask(_ context.Context, req SpawnRequest) (Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req.Queue = normalizeQueue(req.Queue)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return Task{}, false, fmt.Errorf("durable task name required")
	}
	if req.IdempotencyKey != "" {
		for _, task := range s.tasks {
			if task.Queue == req.Queue && task.IdempotencyKey == req.IdempotencyKey && task.UserID == req.UserID {
				return cloneTask(task), false, nil
			}
		}
	}
	now := time.Now().UTC()
	if req.AvailableAt.IsZero() {
		req.AvailableAt = now
	}
	s.nextTaskID++
	id := fmt.Sprintf("dtask_%d", s.nextTaskID)
	task := Task{
		ID:             id,
		Queue:          req.Queue,
		Name:           req.Name,
		UserID:         req.UserID,
		Status:         TaskStatusQueued,
		Params:         cloneMap(req.Params),
		Headers:        cloneMap(req.Headers),
		IdempotencyKey: req.IdempotencyKey,
		ParentTaskID:   req.ParentTaskID,
		ParentRunID:    req.ParentRunID,
		RetryPolicy:    normalizeRetryPolicy(req.RetryPolicy),
		AvailableAt:    req.AvailableAt.UTC(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.tasks[id] = task
	return cloneTask(task), true, nil
}

func (s *MemoryStore) GetTask(_ context.Context, userID int64, taskID string) (Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, false, nil
	}
	return cloneTask(task), true, nil
}

func (s *MemoryStore) ListTasks(_ context.Context, userID int64, filter TaskListFilter) ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filter.Queue = strings.TrimSpace(filter.Queue)
	filter.Name = strings.TrimSpace(filter.Name)
	limit := normalizeTaskListLimit(filter.Limit)
	out := make([]Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task.UserID != userID {
			continue
		}
		if filter.Queue != "" && task.Queue != filter.Queue {
			continue
		}
		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		if filter.Name != "" && task.Name != filter.Name {
			continue
		}
		out = append(out, cloneTask(task))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) ListTaskEvents(_ context.Context, userID int64, taskID string, afterSequence int64) ([]Event, TaskStatus, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, "", false, nil
	}
	out := []Event{}
	for _, ev := range s.events {
		if ev.TaskID == taskID && ev.Sequence > afterSequence {
			out = append(out, cloneEvent(ev))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out, task.Status, true, nil
}

func (s *MemoryStore) ListTaskEventsPage(_ context.Context, userID int64, taskID string, filter EventListFilter) (EventPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return EventPage{}, nil
	}
	events := s.collectTaskEvents(taskID)
	return buildMemoryEventPage(events, task.Status, filter), nil
}

func (s *MemoryStore) collectTaskEvents(taskID string) []Event {
	out := []Event{}
	for _, ev := range s.events {
		if ev.TaskID == taskID {
			out = append(out, cloneEvent(ev))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}

func (s *MemoryStore) AppendTaskEvent(_ context.Context, taskID string, name string, payload map[string]any) (Event, error) {
	return s.appendTaskEvent(taskID, "", name, payload)
}

func (s *MemoryStore) AppendTaskEventOnce(_ context.Context, taskID string, eventKey string, name string, payload map[string]any) (Event, error) {
	return s.appendTaskEvent(taskID, strings.TrimSpace(eventKey), name, payload)
}

func (s *MemoryStore) appendTaskEvent(taskID string, eventKey string, name string, payload map[string]any) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return Event{}, ErrTaskNotFound
	}
	if eventKey != "" {
		for _, ev := range s.events {
			if ev.TaskID == taskID && ev.EventKey == eventKey {
				return cloneEvent(ev), nil
			}
		}
	}
	seq := int64(1)
	for _, ev := range s.events {
		if ev.TaskID == taskID && ev.Sequence >= seq {
			seq = ev.Sequence + 1
		}
	}
	s.nextEventID++
	ev := Event{ID: s.nextEventID, TaskID: taskID, Queue: task.Queue, Name: name, Sequence: seq, EventKey: eventKey, Payload: cloneMap(payload), OccurredAt: time.Now().UTC()}
	s.events = append(s.events, ev)
	return cloneEvent(ev), nil
}

func (s *MemoryStore) EmitEvent(_ context.Context, userID int64, queue string, name string, payload map[string]any) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue = normalizeQueue(queue)
	name = strings.TrimSpace(name)
	s.nextEventID++
	ev := Event{ID: s.nextEventID, Queue: queue, Name: name, Payload: cloneMap(payload), OccurredAt: time.Now().UTC()}
	s.events = append(s.events, ev)
	for id, wait := range s.waits {
		task := s.tasks[wait.TaskID]
		if task.UserID != userID || task.Queue != queue || wait.Kind != WaitKindEvent || wait.EventName != name || wait.Status != "waiting" {
			continue
		}
		firedAt := ev.OccurredAt
		wait.Status = "fired"
		wait.FiredAt = &firedAt
		s.waits[id] = wait
		task.Status = TaskStatusQueued
		task.AvailableAt = time.Now().UTC()
		task.UpdatedAt = time.Now().UTC()
		s.tasks[task.ID] = task
		raw, _ := json.Marshal(payload)
		s.checkpoints[checkpointKey(task.ID, "event:"+name)] = raw
	}
	return cloneEvent(ev), nil
}

func (s *MemoryStore) ClaimNext(_ context.Context, queues []string, workerID string, lease time.Duration) (Task, Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queueSet := map[string]bool{}
	for _, q := range queues {
		queueSet[normalizeQueue(q)] = true
	}
	now := time.Now().UTC()
	for runID, run := range s.runs {
		if run.Status != RunStatusRunning || run.LeaseUntil.After(now) {
			continue
		}
		task := s.tasks[run.TaskID]
		if task.Status == TaskStatusRunning {
			task.Status = TaskStatusQueued
			task.UpdatedAt = now
			s.tasks[task.ID] = task
		}
		run.Status = RunStatusFailed
		run.Error = "lease expired"
		completedAt := now
		run.CompletedAt = &completedAt
		s.runs[runID] = run
	}
	var ids []string
	for id, task := range s.tasks {
		if !queueSet[task.Queue] || task.AvailableAt.After(now) {
			continue
		}
		if task.Status == TaskStatusQueued {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return Task{}, Run{}, false, nil
	}
	task := s.tasks[ids[0]]
	task.Status = TaskStatusRunning
	task.Attempt++
	task.UpdatedAt = now
	s.tasks[task.ID] = task
	s.nextRunID++
	run := Run{ID: fmt.Sprintf("drun_%d", s.nextRunID), TaskID: task.ID, Attempt: task.Attempt, Status: RunStatusRunning, WorkerID: workerID, LeaseUntil: now.Add(lease), StartedAt: now}
	s.runs[run.ID] = run
	return cloneTask(task), run, true, nil
}

func (s *MemoryStore) Heartbeat(_ context.Context, taskID, runID, workerID string, leaseUntil time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.TaskID != taskID || run.WorkerID != workerID {
		return ErrTaskNotFound
	}
	run.LeaseUntil = leaseUntil.UTC()
	s.runs[runID] = run
	return nil
}

func (s *MemoryStore) CompleteTask(_ context.Context, taskID, runID string, result json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	if task.Status == TaskStatusCancelled {
		return ErrCancelled
	}
	task.Status = TaskStatusCompleted
	task.Result = append(json.RawMessage(nil), result...)
	task.CompletedAt = &now
	task.UpdatedAt = now
	s.tasks[taskID] = task
	run := s.runs[runID]
	run.Status = RunStatusCompleted
	run.CompletedAt = &now
	s.runs[runID] = run
	return nil
}

func (s *MemoryStore) FailTask(_ context.Context, taskID, runID string, failure json.RawMessage, errText string, nextAttemptAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	run := s.runs[runID]
	run.Status = RunStatusFailed
	run.Error = errText
	run.CompletedAt = &now
	s.runs[runID] = run
	maxAttempts := task.RetryPolicy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if !nextAttemptAt.IsZero() && task.Attempt < maxAttempts {
		task.Status = TaskStatusQueued
		task.AvailableAt = nextAttemptAt.UTC()
	} else {
		task.Status = TaskStatusFailed
		task.Failure = append(json.RawMessage(nil), failure...)
		task.Error = errText
		task.CompletedAt = &now
	}
	task.UpdatedAt = now
	s.tasks[taskID] = task
	return nil
}

func (s *MemoryStore) MarkTaskWaiting(_ context.Context, taskID, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	task := s.tasks[taskID]
	task.Status = TaskStatusWaiting
	task.UpdatedAt = now
	s.tasks[taskID] = task
	run := s.runs[runID]
	run.Status = RunStatusWaiting
	completed := now
	run.CompletedAt = &completed
	s.runs[runID] = run
	return nil
}

func (s *MemoryStore) CancelTask(_ context.Context, userID int64, taskID string) error {
	_, err := s.CancelTaskTree(context.Background(), userID, taskID)
	return err
}

func (s *MemoryStore) CancelTaskTree(_ context.Context, userID int64, taskID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	taskIDs, err := s.cancelTaskTreeLocked(userID, taskID)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), taskIDs...), nil
}

func (s *MemoryStore) cancelTaskTreeLocked(userID int64, taskID string) ([]string, error) {
	root, ok := s.tasks[taskID]
	if !ok || root.UserID != userID {
		return nil, ErrTaskNotFound
	}
	now := time.Now().UTC()
	taskIDs := s.descendantTaskIDsLocked(userID, taskID)
	cancelled := make([]string, 0, len(taskIDs))
	cancelledSet := map[string]bool{}
	for _, id := range taskIDs {
		task := s.tasks[id]
		if task.Status == TaskStatusCompleted {
			continue
		}
		task.Status = TaskStatusCancelled
		task.Error = "cancelled"
		task.CompletedAt = &now
		task.UpdatedAt = now
		s.tasks[id] = task
		cancelled = append(cancelled, id)
		cancelledSet[id] = true
		s.cancelRunsForTaskLocked(id, now)
	}
	s.cancelWaitsForTasksLocked(cancelledSet, now)
	return cancelled, nil
}

func (s *MemoryStore) descendantTaskIDsLocked(userID int64, rootID string) []string {
	out := []string{rootID}
	for i := 0; i < len(out); i++ {
		parentID := out[i]
		for _, task := range s.tasks {
			if task.UserID == userID && task.ParentTaskID == parentID {
				out = append(out, task.ID)
			}
		}
	}
	return out
}

func (s *MemoryStore) cancelRunsForTaskLocked(taskID string, now time.Time) {
	for runID, run := range s.runs {
		if run.TaskID != taskID || run.Status == RunStatusCompleted || run.Status == RunStatusCancelled {
			continue
		}
		run.Status = RunStatusCancelled
		run.Error = "cancelled"
		run.CompletedAt = &now
		s.runs[runID] = run
	}
}

func (s *MemoryStore) cancelWaitsForTasksLocked(cancelledSet map[string]bool, now time.Time) {
	for id, wait := range s.waits {
		if wait.Status != "waiting" {
			continue
		}
		if cancelledSet[wait.TaskID] {
			wait.Status = "cancelled"
			wait.FiredAt = &now
			s.waits[id] = wait
			continue
		}
		if cancelledSet[wait.ChildTaskID] {
			wait.Status = "fired"
			wait.FiredAt = &now
			s.waits[id] = wait
			task := s.tasks[wait.TaskID]
			if task.Status == TaskStatusWaiting {
				task.Status = TaskStatusQueued
				task.AvailableAt = now
				task.UpdatedAt = now
				s.tasks[task.ID] = task
			}
		}
	}
}

func (s *MemoryStore) RetryTask(_ context.Context, userID int64, taskID string, resetCheckpoints bool) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, ErrTaskNotFound
	}
	if task.Status != TaskStatusFailed && task.Status != TaskStatusCancelled {
		return Task{}, ErrInvalidState
	}
	now := time.Now().UTC()
	task.Status = TaskStatusQueued
	task.Attempt = 0
	task.AvailableAt = now
	task.Result = nil
	task.Failure = nil
	task.Error = ""
	task.CompletedAt = nil
	task.UpdatedAt = now
	s.tasks[taskID] = task
	if resetCheckpoints {
		prefix := taskID + "\x00"
		for key := range s.checkpoints {
			if strings.HasPrefix(key, prefix) {
				delete(s.checkpoints, key)
			}
		}
	}
	for id, wait := range s.waits {
		if wait.TaskID == taskID && wait.Status == "waiting" {
			wait.Status = "cancelled"
			wait.FiredAt = &now
			s.waits[id] = wait
		}
	}
	seq := int64(1)
	for _, ev := range s.events {
		if ev.TaskID == taskID && ev.Sequence >= seq {
			seq = ev.Sequence + 1
		}
	}
	s.nextEventID++
	s.events = append(s.events, Event{
		ID:         s.nextEventID,
		TaskID:     taskID,
		Queue:      task.Queue,
		Name:       "task_retried",
		Sequence:   seq,
		Payload:    map[string]any{"reset_checkpoints": resetCheckpoints},
		OccurredAt: now,
	})
	return cloneTask(task), nil
}

func (s *MemoryStore) GetCheckpoint(_ context.Context, taskID, stepKey string) (json.RawMessage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.checkpoints[checkpointKey(taskID, stepKey)]
	if !ok {
		return nil, false, nil
	}
	return append(json.RawMessage(nil), raw...), true, nil
}

func (s *MemoryStore) SaveCheckpoint(_ context.Context, taskID, stepKey string, value json.RawMessage) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := checkpointKey(taskID, stepKey)
	if existing, ok := s.checkpoints[key]; ok {
		return append(json.RawMessage(nil), existing...), nil
	}
	s.checkpoints[key] = append(json.RawMessage(nil), value...)
	return append(json.RawMessage(nil), value...), nil
}

func (s *MemoryStore) CreateWait(_ context.Context, wait Wait) (Wait, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.waits[wait.ID]; ok {
		return existing, nil
	}
	wait.CreatedAt = time.Now().UTC()
	if wait.Status == "" {
		wait.Status = "waiting"
	}
	s.waits[wait.ID] = wait
	return wait, nil
}

func (s *MemoryStore) GetWait(_ context.Context, waitID string) (Wait, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wait, ok := s.waits[waitID]
	return wait, ok, nil
}

func (s *MemoryStore) FireDueTimers(_ context.Context, now time.Time, limit int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for id, wait := range s.waits {
		if limit > 0 && count >= limit {
			break
		}
		if wait.Status != "waiting" || wait.WakeAt == nil || wait.WakeAt.After(now) {
			continue
		}
		if wait.Kind != WaitKindTimer && wait.Kind != WaitKindEvent {
			continue
		}
		firedAt := now.UTC()
		wait.Status = "fired"
		wait.FiredAt = &firedAt
		s.waits[id] = wait
		readyAt := time.Now().UTC()
		task := s.tasks[wait.TaskID]
		task.Status = TaskStatusQueued
		task.AvailableAt = readyAt
		task.UpdatedAt = readyAt
		s.tasks[task.ID] = task
		count++
	}
	return count, nil
}

func (s *MemoryStore) WakeChildWaits(_ context.Context, childTaskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for id, wait := range s.waits {
		if wait.Kind != WaitKindChild || wait.ChildTaskID != childTaskID || wait.Status != "waiting" {
			continue
		}
		wait.Status = "fired"
		wait.FiredAt = &now
		s.waits[id] = wait
		task := s.tasks[wait.TaskID]
		task.Status = TaskStatusQueued
		task.AvailableAt = now
		task.UpdatedAt = now
		s.tasks[task.ID] = task
	}
	return nil
}

func (s *MemoryStore) QueueStats(_ context.Context) ([]QueueStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byQueue := map[string]*QueueStats{}
	for _, task := range s.tasks {
		stats := byQueue[task.Queue]
		if stats == nil {
			stats = &QueueStats{Queue: task.Queue}
			byQueue[task.Queue] = stats
		}
		switch task.Status {
		case TaskStatusQueued:
			stats.Queued++
		case TaskStatusRunning:
			stats.Running++
		case TaskStatusWaiting:
			stats.Waiting++
		case TaskStatusCompleted:
			stats.Completed++
		case TaskStatusFailed:
			stats.Failed++
		case TaskStatusCancelled:
			stats.Cancelled++
		}
	}
	out := make([]QueueStats, 0, len(byQueue))
	for _, stats := range byQueue {
		out = append(out, *stats)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Queue < out[j].Queue })
	return out, nil
}

func checkpointKey(taskID, stepKey string) string {
	return taskID + "\x00" + stepKey
}

func normalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 3
	}
	return policy
}

func normalizeTaskListLimit(limit int) int {
	switch {
	case limit <= 0:
		return 50
	case limit > 200:
		return 200
	default:
		return limit
	}
}

func buildMemoryEventPage(events []Event, status TaskStatus, filter EventListFilter) EventPage {
	limit := normalizeEventListLimit(filter.Limit)
	page := EventPage{Status: status, Found: true, Limit: limit}
	switch {
	case filter.BeforeSequence > 0:
		return memoryEventPageBefore(page, events, filter.BeforeSequence, limit)
	case filter.AfterSequence > 0:
		return memoryEventPageAfter(page, events, filter.AfterSequence, limit)
	default:
		return memoryEventPageLatest(page, events, limit)
	}
}

func memoryEventPageBefore(page EventPage, events []Event, beforeSequence int64, limit int) EventPage {
	end := len(events)
	for end > 0 && events[end-1].Sequence >= beforeSequence {
		end--
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page.HasMoreBefore = start > 0
	page.HasMoreAfter = end < len(events)
	return withEventPageEvents(page, events[start:end])
}

func memoryEventPageAfter(page EventPage, events []Event, afterSequence int64, limit int) EventPage {
	start := 0
	for start < len(events) && events[start].Sequence <= afterSequence {
		start++
	}
	end := start + limit
	if end > len(events) {
		end = len(events)
	}
	page.HasMoreBefore = start > 0
	page.HasMoreAfter = end < len(events)
	return withEventPageEvents(page, events[start:end])
}

func memoryEventPageLatest(page EventPage, events []Event, limit int) EventPage {
	start := len(events) - limit
	if start < 0 {
		start = 0
	}
	page.HasMoreBefore = start > 0
	return withEventPageEvents(page, events[start:])
}

func cloneTask(task Task) Task {
	task.Params = cloneMap(task.Params)
	task.Headers = cloneMap(task.Headers)
	task.Result = append(json.RawMessage(nil), task.Result...)
	task.Failure = append(json.RawMessage(nil), task.Failure...)
	if task.CompletedAt != nil {
		t := *task.CompletedAt
		task.CompletedAt = &t
	}
	return task
}

func cloneEvent(event Event) Event {
	event.Payload = cloneMap(event.Payload)
	return event
}
