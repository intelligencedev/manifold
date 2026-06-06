package durable

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) Store {
	if db == nil {
		return NewMemoryStore()
	}
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) Close() {}

func (s *SQLiteStore) Init(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("durable sqlite store requires db")
	}
	_, err := s.db.ExecContext(ctx, sqliteDurableSchema)
	return err
}

const sqliteDurableSchema = `
CREATE TABLE IF NOT EXISTS durable_tasks (
	id TEXT PRIMARY KEY,
	queue TEXT NOT NULL,
	name TEXT NOT NULL,
	user_id INTEGER NOT NULL DEFAULT 0,
	params TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
	headers TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(headers)),
	status TEXT NOT NULL,
	idempotency_key TEXT NOT NULL DEFAULT '',
	parent_task_id TEXT,
	parent_run_id TEXT,
	retry_policy TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(retry_policy)),
	attempt INTEGER NOT NULL DEFAULT 0,
	available_at DATETIME NOT NULL,
	result TEXT CHECK (result IS NULL OR json_valid(result)),
	failure TEXT CHECK (failure IS NULL OR json_valid(failure)),
	error TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	completed_at DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS durable_tasks_idempotency_idx
	ON durable_tasks(user_id, queue, idempotency_key)
	WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS durable_tasks_runnable_idx
	ON durable_tasks(queue, status, available_at);
CREATE INDEX IF NOT EXISTS durable_tasks_parent_idx
	ON durable_tasks(parent_task_id);

CREATE TABLE IF NOT EXISTS durable_runs (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL REFERENCES durable_tasks(id) ON DELETE CASCADE,
	attempt INTEGER NOT NULL,
	status TEXT NOT NULL,
	worker_id TEXT NOT NULL DEFAULT '',
	lease_until DATETIME,
	started_at DATETIME NOT NULL,
	completed_at DATETIME,
	error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS durable_runs_task_idx ON durable_runs(task_id, attempt DESC);
CREATE INDEX IF NOT EXISTS durable_runs_lease_idx ON durable_runs(status, lease_until);

CREATE TABLE IF NOT EXISTS durable_checkpoints (
	task_id TEXT NOT NULL REFERENCES durable_tasks(id) ON DELETE CASCADE,
	step_key TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'completed',
	result TEXT CHECK (result IS NULL OR json_valid(result)),
	error TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	PRIMARY KEY(task_id, step_key)
);

CREATE TABLE IF NOT EXISTS durable_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT REFERENCES durable_tasks(id) ON DELETE CASCADE,
	queue TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	sequence INTEGER NOT NULL DEFAULT 0,
	event_key TEXT NOT NULL DEFAULT '',
	payload TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
	occurred_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS durable_events_task_sequence_idx
	ON durable_events(task_id, sequence)
	WHERE task_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS durable_events_task_event_key_idx
	ON durable_events(task_id, event_key)
	WHERE task_id IS NOT NULL AND event_key <> '';
CREATE INDEX IF NOT EXISTS durable_events_task_idx ON durable_events(task_id, sequence);
CREATE INDEX IF NOT EXISTS durable_events_queue_name_idx ON durable_events(queue, name, occurred_at DESC);

CREATE TABLE IF NOT EXISTS durable_waits (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL REFERENCES durable_tasks(id) ON DELETE CASCADE,
	run_id TEXT,
	kind TEXT NOT NULL,
	event_name TEXT NOT NULL DEFAULT '',
	child_task_id TEXT NOT NULL DEFAULT '',
	wake_at DATETIME,
	status TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	fired_at DATETIME
);
CREATE INDEX IF NOT EXISTS durable_waits_timer_idx ON durable_waits(kind, status, wake_at);
CREATE INDEX IF NOT EXISTS durable_waits_event_idx ON durable_waits(kind, status, event_name);
CREATE INDEX IF NOT EXISTS durable_waits_child_idx ON durable_waits(kind, status, child_task_id);

CREATE TABLE IF NOT EXISTS durable_outbox (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT,
	event_id INTEGER,
	topic TEXT NOT NULL,
	payload TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
	created_at DATETIME NOT NULL,
	delivered_at DATETIME
);
CREATE INDEX IF NOT EXISTS durable_outbox_pending_idx ON durable_outbox(delivered_at, id);
`

func (s *SQLiteStore) SpawnTask(ctx context.Context, req SpawnRequest) (Task, bool, error) {
	if err := s.Init(ctx); err != nil {
		return Task{}, false, err
	}
	req.Queue = normalizeQueue(req.Queue)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return Task{}, false, fmt.Errorf("durable task name required")
	}
	now := time.Now().UTC()
	if req.AvailableAt.IsZero() {
		req.AvailableAt = now
	}
	req.RetryPolicy = normalizeRetryPolicy(req.RetryPolicy)
	params, err := json.Marshal(nonNilMap(req.Params))
	if err != nil {
		return Task{}, false, err
	}
	headers, err := json.Marshal(nonNilMap(req.Headers))
	if err != nil {
		return Task{}, false, err
	}
	retryPolicy, err := json.Marshal(req.RetryPolicy)
	if err != nil {
		return Task{}, false, err
	}
	if req.IdempotencyKey != "" {
		existing, ok, err := s.getTaskByIdempotency(ctx, req.UserID, req.Queue, req.IdempotencyKey)
		if err != nil || ok {
			return existing, false, err
		}
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO durable_tasks (
	id, queue, name, user_id, params, headers, status, idempotency_key, parent_task_id, parent_run_id,
	retry_policy, available_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)
RETURNING id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
`, newID("dtask"), req.Queue, req.Name, req.UserID, string(params), string(headers), string(TaskStatusQueued), req.IdempotencyKey, req.ParentTaskID, req.ParentRunID, string(retryPolicy), req.AvailableAt.UTC(), now, now)
	var task Task
	if err := scanTask(row, &task); err != nil {
		if req.IdempotencyKey != "" && strings.Contains(strings.ToLower(err.Error()), "constraint") {
			existing, ok, getErr := s.getTaskByIdempotency(ctx, req.UserID, req.Queue, req.IdempotencyKey)
			return existing, false, getErrOrMissing(getErr, ok, err)
		}
		return Task{}, false, err
	}
	return task, true, nil
}

func (s *SQLiteStore) getTaskByIdempotency(ctx context.Context, userID int64, queue, idempotencyKey string) (Task, bool, error) {
	row := s.db.QueryRowContext(ctx, sqliteTaskSelectSQL+`
WHERE user_id = ? AND queue = ? AND idempotency_key = ?`, userID, queue, idempotencyKey)
	var task Task
	if err := scanTask(row, &task); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, false, nil
		}
		return Task{}, false, err
	}
	return task, true, nil
}

func getErrOrMissing(err error, ok bool, fallback error) error {
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return fallback
}

func (s *SQLiteStore) GetTask(ctx context.Context, userID int64, taskID string) (Task, bool, error) {
	if err := s.Init(ctx); err != nil {
		return Task{}, false, err
	}
	row := s.db.QueryRowContext(ctx, sqliteTaskSelectSQL+`
WHERE id = ? AND user_id = ?`, strings.TrimSpace(taskID), userID)
	var task Task
	if err := scanTask(row, &task); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, false, nil
		}
		return Task{}, false, err
	}
	return task, true, nil
}

func (s *SQLiteStore) ListTasks(ctx context.Context, userID int64, filter TaskListFilter) ([]Task, error) {
	page, err := s.ListTasksPage(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	return page.Tasks, nil
}

func (s *SQLiteStore) ListTasksPage(ctx context.Context, userID int64, filter TaskListFilter) (TaskListPage, error) {
	if err := s.Init(ctx); err != nil {
		return TaskListPage{}, err
	}
	filter.Queue = strings.TrimSpace(filter.Queue)
	filter.Name = strings.TrimSpace(filter.Name)
	status := strings.TrimSpace(string(filter.Status))
	limit := normalizeTaskListLimit(filter.Limit)
	offset := normalizeTaskListOffset(filter.Offset)
	var total int64
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM durable_tasks
WHERE user_id = ?
  AND (? = '' OR queue = ?)
  AND (? = '' OR status = ?)
  AND (? = '' OR name = ?)
`, userID, filter.Queue, filter.Queue, status, status, filter.Name, filter.Name).Scan(&total); err != nil {
		return TaskListPage{}, err
	}
	rows, err := s.db.QueryContext(ctx, sqliteTaskSelectSQL+`
WHERE user_id = ?
  AND (? = '' OR queue = ?)
  AND (? = '' OR status = ?)
  AND (? = '' OR name = ?)
ORDER BY updated_at DESC, created_at DESC, id DESC
LIMIT ?
OFFSET ?`, userID, filter.Queue, filter.Queue, status, status, filter.Name, filter.Name, limit, offset)
	if err != nil {
		return TaskListPage{}, err
	}
	defer func() { _ = rows.Close() }()
	tasks := []Task{}
	for rows.Next() {
		var task Task
		if err := scanTask(rows, &task); err != nil {
			return TaskListPage{}, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return TaskListPage{}, err
	}
	return TaskListPage{Tasks: tasks, Limit: limit, Offset: offset, Total: total, HasMore: int64(offset+len(tasks)) < total}, nil
}

func (s *SQLiteStore) ListTaskEvents(ctx context.Context, userID int64, taskID string, afterSequence int64) ([]Event, TaskStatus, bool, error) {
	task, ok, err := s.GetTask(ctx, userID, taskID)
	if err != nil || !ok {
		return nil, "", ok, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
FROM durable_events
WHERE task_id = ? AND sequence > ?
ORDER BY sequence ASC`, taskID, afterSequence)
	if err != nil {
		return nil, "", false, err
	}
	defer func() { _ = rows.Close() }()
	events := []Event{}
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, "", false, err
		}
		events = append(events, ev)
	}
	return events, task.Status, true, rows.Err()
}

func (s *SQLiteStore) ListTaskEventsPage(ctx context.Context, userID int64, taskID string, filter EventListFilter) (EventPage, error) {
	task, ok, err := s.GetTask(ctx, userID, taskID)
	if err != nil || !ok {
		return EventPage{Found: ok}, err
	}
	filter.Limit = normalizeEventListLimit(filter.Limit)
	events, hasExtra, err := s.queryTaskEventPage(ctx, taskID, filter)
	if err != nil {
		return EventPage{}, err
	}
	page := EventPage{Status: task.Status, Found: true, Limit: filter.Limit}
	switch {
	case filter.BeforeSequence > 0:
		page.HasMoreBefore = hasExtra
		page.HasMoreAfter, err = s.taskEventExistsAtOrAfter(ctx, taskID, filter.BeforeSequence)
	case filter.AfterSequence > 0:
		page.HasMoreBefore, err = s.taskEventExistsAtOrBefore(ctx, taskID, filter.AfterSequence)
		page.HasMoreAfter = hasExtra
	default:
		page.HasMoreBefore = hasExtra
	}
	if err != nil {
		return EventPage{}, err
	}
	return withEventPageEvents(page, events), nil
}

func (s *SQLiteStore) queryTaskEventPage(ctx context.Context, taskID string, filter EventListFilter) ([]Event, bool, error) {
	switch {
	case filter.BeforeSequence > 0:
		return s.queryTaskEvents(ctx, `
SELECT id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
FROM durable_events WHERE task_id = ? AND sequence < ? ORDER BY sequence DESC LIMIT ?
`, true, filter.Limit, taskID, filter.BeforeSequence, filter.Limit+1)
	case filter.AfterSequence > 0:
		return s.queryTaskEvents(ctx, `
SELECT id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
FROM durable_events WHERE task_id = ? AND sequence > ? ORDER BY sequence ASC LIMIT ?
`, false, filter.Limit, taskID, filter.AfterSequence, filter.Limit+1)
	default:
		return s.queryTaskEvents(ctx, `
SELECT id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
FROM durable_events WHERE task_id = ? ORDER BY sequence DESC LIMIT ?
`, true, filter.Limit, taskID, filter.Limit+1)
	}
}

func (s *SQLiteStore) queryTaskEvents(ctx context.Context, query string, reverse bool, limit int, args ...any) ([]Event, bool, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	events := []Event{}
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, false, err
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasExtra := len(events) > limit
	if hasExtra {
		events = events[:limit]
	}
	if reverse {
		reverseEvents(events)
	}
	return events, hasExtra, nil
}

func (s *SQLiteStore) taskEventExistsAtOrAfter(ctx context.Context, taskID string, sequence int64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM durable_events WHERE task_id = ? AND sequence >= ?)`, taskID, sequence).Scan(&exists)
	return exists, err
}

func (s *SQLiteStore) taskEventExistsAtOrBefore(ctx context.Context, taskID string, sequence int64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM durable_events WHERE task_id = ? AND sequence <= ?)`, taskID, sequence).Scan(&exists)
	return exists, err
}

func (s *SQLiteStore) AppendTaskEvent(ctx context.Context, taskID string, name string, payload map[string]any) (Event, error) {
	return s.appendTaskEvent(ctx, taskID, "", name, payload)
}

func (s *SQLiteStore) AppendTaskEventOnce(ctx context.Context, taskID string, eventKey string, name string, payload map[string]any) (Event, error) {
	return s.appendTaskEvent(ctx, taskID, strings.TrimSpace(eventKey), name, payload)
}

func (s *SQLiteStore) appendTaskEvent(ctx context.Context, taskID string, eventKey string, name string, payload map[string]any) (Event, error) {
	if err := s.Init(ctx); err != nil {
		return Event{}, err
	}
	payloadBytes, err := json.Marshal(nonNilMap(payload))
	if err != nil {
		return Event{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var queue string
	if err := tx.QueryRowContext(ctx, `SELECT queue FROM durable_tasks WHERE id = ?`, taskID).Scan(&queue); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Event{}, ErrTaskNotFound
		}
		return Event{}, err
	}
	if eventKey != "" {
		existing, ok, err := sqliteTaskEventByKey(ctx, tx, taskID, eventKey)
		if err != nil {
			return Event{}, err
		}
		if ok {
			return existing, tx.Commit()
		}
	}
	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM durable_events WHERE task_id = ?`, taskID).Scan(&seq); err != nil {
		return Event{}, err
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO durable_events(task_id, queue, name, sequence, event_key, payload, occurred_at)
VALUES(?, ?, ?, ?, ?, ?, ?)
RETURNING id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
`, taskID, queue, strings.TrimSpace(name), seq, eventKey, string(payloadBytes), time.Now().UTC())
	ev, err := scanEvent(row)
	if err != nil {
		if eventKey != "" && strings.Contains(strings.ToLower(err.Error()), "constraint") {
			existing, ok, scanErr := sqliteTaskEventByKey(ctx, tx, taskID, eventKey)
			if scanErr != nil || !ok {
				return Event{}, scanErr
			}
			return existing, tx.Commit()
		}
		return Event{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO durable_outbox(task_id, event_id, topic, payload, created_at) VALUES(?, ?, ?, ?, ?)`, taskID, ev.ID, name, string(payloadBytes), time.Now().UTC()); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return ev, nil
}

func sqliteTaskEventByKey(ctx context.Context, tx *sql.Tx, taskID, eventKey string) (Event, bool, error) {
	row := tx.QueryRowContext(ctx, `
SELECT id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
FROM durable_events
WHERE task_id = ? AND event_key = ?`, taskID, eventKey)
	ev, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}
	return ev, true, nil
}

func (s *SQLiteStore) EmitEvent(ctx context.Context, userID int64, queue string, name string, payload map[string]any) (Event, error) {
	if err := s.Init(ctx); err != nil {
		return Event{}, err
	}
	queue = normalizeQueue(queue)
	name = strings.TrimSpace(name)
	payloadBytes, err := json.Marshal(nonNilMap(payload))
	if err != nil {
		return Event{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
INSERT INTO durable_events(queue, name, payload, occurred_at)
VALUES(?, ?, ?, ?)
RETURNING id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
`, queue, name, string(payloadBytes), time.Now().UTC())
	ev, err := scanEvent(row)
	if err != nil {
		return Event{}, err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT w.id, w.task_id
FROM durable_waits w
JOIN durable_tasks t ON t.id = w.task_id
WHERE w.kind = 'event' AND w.status = 'waiting' AND w.event_name = ? AND t.user_id = ? AND t.queue = ?`, name, userID, queue)
	if err != nil {
		return Event{}, err
	}
	type wake struct{ waitID, taskID string }
	wakes := []wake{}
	for rows.Next() {
		var w wake
		if err := rows.Scan(&w.waitID, &w.taskID); err != nil {
			_ = rows.Close()
			return Event{}, err
		}
		wakes = append(wakes, w)
	}
	if err := rows.Close(); err != nil {
		return Event{}, err
	}
	for _, w := range wakes {
		if _, err := tx.ExecContext(ctx, `UPDATE durable_waits SET status = 'fired', fired_at = ? WHERE id = ?`, time.Now().UTC(), w.waitID); err != nil {
			return Event{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE durable_tasks SET status = 'queued', available_at = ?, updated_at = ? WHERE id = ? AND status = 'waiting'`, time.Now().UTC(), time.Now().UTC(), w.taskID); err != nil {
			return Event{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO durable_checkpoints(task_id, step_key, result, created_at, updated_at)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(task_id, step_key) DO NOTHING
`, w.taskID, "event:"+name, string(payloadBytes), time.Now().UTC(), time.Now().UTC()); err != nil {
			return Event{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO durable_outbox(event_id, topic, payload, created_at) VALUES(?, ?, ?, ?)`, ev.ID, name, string(payloadBytes), time.Now().UTC()); err != nil {
		return Event{}, err
	}
	return ev, tx.Commit()
}

func (s *SQLiteStore) ClaimNext(ctx context.Context, queues []string, workerID string, lease time.Duration) (Task, Run, bool, error) {
	if err := s.Init(ctx); err != nil {
		return Task{}, Run{}, false, err
	}
	if len(queues) == 0 {
		return Task{}, Run{}, false, nil
	}
	for i := range queues {
		queues[i] = normalizeQueue(queues[i])
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, Run{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
UPDATE durable_tasks
SET status = 'queued', updated_at = ?
WHERE status = 'running'
  AND EXISTS (
	SELECT 1 FROM durable_runs r
	WHERE r.task_id = durable_tasks.id AND r.status = 'running' AND r.lease_until <= ?
  )`, now, now); err != nil {
		return Task{}, Run{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE durable_runs SET status = 'failed', error = 'lease expired', completed_at = ? WHERE status = 'running' AND lease_until <= ?`, now, now); err != nil {
		return Task{}, Run{}, false, err
	}
	inSQL, args := sqliteInClause(queues)
	args = append(args, now)
	candidateSQL := fmt.Sprintf(`
SELECT id
FROM durable_tasks
WHERE queue IN (%s) AND status = 'queued' AND available_at <= ?
ORDER BY available_at ASC, created_at ASC
LIMIT 1`, inSQL)
	var taskID string
	if err := tx.QueryRowContext(ctx, candidateSQL, args...).Scan(&taskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, Run{}, false, tx.Commit()
		}
		return Task{}, Run{}, false, err
	}
	row := tx.QueryRowContext(ctx, `
UPDATE durable_tasks
SET status = 'running', attempt = attempt + 1, updated_at = ?
WHERE id = ?
RETURNING id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
`, now, taskID)
	var task Task
	if err := scanTask(row, &task); err != nil {
		return Task{}, Run{}, false, err
	}
	run := Run{ID: newID("drun"), TaskID: task.ID, Attempt: task.Attempt, Status: RunStatusRunning, WorkerID: workerID, LeaseUntil: now.Add(lease), StartedAt: now}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO durable_runs(id, task_id, attempt, status, worker_id, lease_until, started_at)
VALUES(?, ?, ?, ?, ?, ?, ?)`, run.ID, run.TaskID, run.Attempt, string(run.Status), run.WorkerID, run.LeaseUntil, run.StartedAt); err != nil {
		return Task{}, Run{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, Run{}, false, err
	}
	return task, run, true, nil
}

func (s *SQLiteStore) Heartbeat(ctx context.Context, taskID, runID, workerID string, leaseUntil time.Time) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE durable_runs SET lease_until = ?
WHERE id = ? AND task_id = ? AND worker_id = ? AND status = 'running'`, leaseUntil.UTC(), runID, taskID, workerID)
	return err
}

func (s *SQLiteStore) CompleteTask(ctx context.Context, taskID, runID string, result json.RawMessage) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	cmd, err := tx.ExecContext(ctx, `
UPDATE durable_tasks
SET status = 'completed', result = ?, updated_at = ?, completed_at = ?
WHERE id = ? AND status <> 'cancelled'`, string(nullJSON(result)), now, now, taskID)
	if err != nil {
		return err
	}
	if rows, _ := cmd.RowsAffected(); rows == 0 {
		return ErrCancelled
	}
	if _, err := tx.ExecContext(ctx, `UPDATE durable_runs SET status = 'completed', completed_at = ? WHERE id = ? AND task_id = ?`, now, runID, taskID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.AppendCompletionEvent(ctx, taskID, "task_completed", result)
}

func (s *SQLiteStore) AppendCompletionEvent(ctx context.Context, taskID, name string, result json.RawMessage) error {
	payload := map[string]any{"result": json.RawMessage(nullJSON(result))}
	_, err := s.AppendTaskEvent(ctx, taskID, name, payload)
	return err
}

func (s *SQLiteStore) FailTask(ctx context.Context, taskID, runID string, failure json.RawMessage, errText string, nextAttemptAt time.Time) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var attempt int
	var rawPolicy []byte
	if err := tx.QueryRowContext(ctx, `SELECT attempt, retry_policy FROM durable_tasks WHERE id = ?`, taskID).Scan(&attempt, &rawPolicy); err != nil {
		return err
	}
	var policy RetryPolicy
	_ = json.Unmarshal(rawPolicy, &policy)
	policy = normalizeRetryPolicy(policy)
	now := time.Now().UTC()
	status := string(TaskStatusFailed)
	availableAt := now
	var completedAt any = now
	if !nextAttemptAt.IsZero() && attempt < policy.MaxAttempts {
		status = string(TaskStatusQueued)
		availableAt = nextAttemptAt.UTC()
		completedAt = nil
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE durable_tasks
SET status = ?, failure = ?, error = ?, available_at = ?, updated_at = ?, completed_at = ?
WHERE id = ?`, status, string(nullJSON(failure)), errText, availableAt, now, completedAt, taskID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE durable_runs SET status = 'failed', error = ?, completed_at = ? WHERE id = ? AND task_id = ?`, errText, now, runID, taskID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) MarkTaskWaiting(ctx context.Context, taskID, runID string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE durable_tasks SET status = 'waiting', updated_at = ? WHERE id = ?`, now, taskID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE durable_runs SET status = 'waiting', completed_at = ? WHERE id = ? AND task_id = ?`, now, runID, taskID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) CancelTask(ctx context.Context, userID int64, taskID string) error {
	_, err := s.CancelTaskTree(ctx, userID, taskID)
	return err
}

func (s *SQLiteStore) CancelTaskTree(ctx context.Context, userID int64, taskID string) ([]string, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	taskIDs, err := s.cancelTaskTreeIDs(ctx, tx, userID, strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	cancelledIDs, err := s.cancelTaskTreeTx(ctx, tx, taskIDs)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, id := range cancelledIDs {
		_, _ = s.AppendTaskEvent(ctx, id, "task_cancelled", map[string]any{"status": string(TaskStatusCancelled)})
	}
	return cancelledIDs, nil
}

func (s *SQLiteStore) cancelTaskTreeIDs(ctx context.Context, tx *sql.Tx, userID int64, taskID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
WITH RECURSIVE task_tree(id) AS (
	SELECT id FROM durable_tasks WHERE id = ? AND user_id = ?
	UNION ALL
	SELECT child.id
	FROM durable_tasks child
	JOIN task_tree parent ON child.parent_task_id = parent.id
	WHERE child.user_id = ?
)
SELECT id FROM task_tree`, taskID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	taskIDs, err := sqliteScanStringRows(rows)
	if err != nil {
		return nil, err
	}
	if len(taskIDs) == 0 {
		return nil, ErrTaskNotFound
	}
	return taskIDs, nil
}

func (s *SQLiteStore) cancelTaskTreeTx(ctx context.Context, tx *sql.Tx, taskIDs []string) ([]string, error) {
	inSQL, args := sqliteInClause(taskIDs)
	now := time.Now().UTC()
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
UPDATE durable_tasks
SET status = 'cancelled', error = 'cancelled', updated_at = ?, completed_at = ?
WHERE id IN (%s) AND status <> 'completed'
RETURNING id`, inSQL), append([]any{now, now}, args...)...)
	if err != nil {
		return nil, err
	}
	cancelledIDs, err := sqliteScanStringRows(rows)
	if err != nil || len(cancelledIDs) == 0 {
		return cancelledIDs, err
	}
	inSQL, args = sqliteInClause(cancelledIDs)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
UPDATE durable_runs
SET status = 'cancelled', error = 'cancelled', completed_at = ?
WHERE task_id IN (%s) AND status NOT IN ('completed', 'cancelled')`, inSQL), append([]any{now}, args...)...); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
UPDATE durable_waits
SET status = 'cancelled', fired_at = ?
WHERE task_id IN (%s) AND status = 'waiting'`, inSQL), append([]any{now}, args...)...); err != nil {
		return nil, err
	}
	parentRows, err := tx.QueryContext(ctx, fmt.Sprintf(`
SELECT DISTINCT task_id
FROM durable_waits
WHERE child_task_id IN (%s) AND status = 'waiting' AND task_id NOT IN (%s)`, inSQL, inSQL), append(args, args...)...)
	if err != nil {
		return nil, err
	}
	parentIDs, err := sqliteScanStringRows(parentRows)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
UPDATE durable_waits
SET status = 'fired', fired_at = ?
WHERE child_task_id IN (%s) AND status = 'waiting' AND task_id NOT IN (%s)`, inSQL, inSQL), append([]any{now}, append(args, args...)...)...); err != nil {
		return nil, err
	}
	for _, parentID := range parentIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE durable_tasks SET status = 'queued', available_at = ?, updated_at = ? WHERE id = ? AND status = 'waiting'`, now, now, parentID); err != nil {
			return nil, err
		}
	}
	return cancelledIDs, nil
}

func (s *SQLiteStore) RetryTask(ctx context.Context, userID int64, taskID string, resetCheckpoints bool) (Task, error) {
	if err := s.Init(ctx); err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, sqliteTaskSelectSQL+`
WHERE id = ? AND user_id = ?`, strings.TrimSpace(taskID), userID)
	var existing Task
	if err := scanTask(row, &existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, err
	}
	if existing.Status != TaskStatusFailed && existing.Status != TaskStatusCancelled {
		return Task{}, ErrInvalidState
	}
	now := time.Now().UTC()
	row = tx.QueryRowContext(ctx, `
UPDATE durable_tasks
SET status = 'queued',
	attempt = 0,
	available_at = ?,
	result = NULL,
	failure = NULL,
	error = '',
	completed_at = NULL,
	updated_at = ?
WHERE id = ? AND user_id = ?
RETURNING id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
`, now, now, strings.TrimSpace(taskID), userID)
	var task Task
	if err := scanTask(row, &task); err != nil {
		return Task{}, err
	}
	if resetCheckpoints {
		if _, err := tx.ExecContext(ctx, `DELETE FROM durable_checkpoints WHERE task_id = ?`, task.ID); err != nil {
			return Task{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE durable_waits SET status = 'cancelled', fired_at = ? WHERE task_id = ? AND status = 'waiting'`, now, task.ID); err != nil {
		return Task{}, err
	}
	payloadBytes, err := json.Marshal(map[string]any{"reset_checkpoints": resetCheckpoints})
	if err != nil {
		return Task{}, err
	}
	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM durable_events WHERE task_id = ?`, task.ID).Scan(&seq); err != nil {
		return Task{}, err
	}
	var eventID int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO durable_events(task_id, queue, name, sequence, payload, occurred_at)
VALUES(?, ?, ?, ?, ?, ?)
RETURNING id`, task.ID, task.Queue, "task_retried", seq, string(payloadBytes), now).Scan(&eventID); err != nil {
		return Task{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO durable_outbox(task_id, event_id, topic, payload, created_at) VALUES(?, ?, ?, ?, ?)`, task.ID, eventID, "task_retried", string(payloadBytes), now); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *SQLiteStore) GetCheckpoint(ctx context.Context, taskID, stepKey string) (json.RawMessage, bool, error) {
	if err := s.Init(ctx); err != nil {
		return nil, false, err
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT result FROM durable_checkpoints WHERE task_id = ? AND step_key = ? AND status = 'completed'`, taskID, stepKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return append(json.RawMessage(nil), raw...), true, nil
}

func (s *SQLiteStore) SaveCheckpoint(ctx context.Context, taskID, stepKey string, value json.RawMessage) (json.RawMessage, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO durable_checkpoints(task_id, step_key, result, created_at, updated_at)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(task_id, step_key) DO UPDATE SET step_key = durable_checkpoints.step_key
RETURNING result
`, taskID, stepKey, string(nullJSON(value)), time.Now().UTC(), time.Now().UTC())
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), raw...), nil
}

func (s *SQLiteStore) CreateWait(ctx context.Context, wait Wait) (Wait, error) {
	if err := s.Init(ctx); err != nil {
		return Wait{}, err
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO durable_waits(id, task_id, run_id, kind, event_name, child_task_id, wake_at, status, created_at)
VALUES(?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET id = durable_waits.id
RETURNING id, task_id, COALESCE(run_id,''), kind, event_name, child_task_id, wake_at, status, created_at, fired_at
`, wait.ID, wait.TaskID, wait.RunID, string(wait.Kind), wait.EventName, wait.ChildTaskID, wait.WakeAt, nonEmpty(wait.Status, "waiting"), time.Now().UTC())
	return scanWait(row)
}

func (s *SQLiteStore) GetWait(ctx context.Context, waitID string) (Wait, bool, error) {
	if err := s.Init(ctx); err != nil {
		return Wait{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, task_id, COALESCE(run_id,''), kind, event_name, child_task_id, wake_at, status, created_at, fired_at FROM durable_waits WHERE id = ?`, waitID)
	wait, err := scanWait(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Wait{}, false, nil
	}
	if err != nil {
		return Wait{}, false, err
	}
	return wait, true, nil
}

func (s *SQLiteStore) FireDueTimers(ctx context.Context, now time.Time, limit int) (int, error) {
	if err := s.Init(ctx); err != nil {
		return 0, err
	}
	if limit <= 0 {
		limit = 100
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
SELECT id, task_id
FROM durable_waits
WHERE kind IN ('timer', 'event') AND status = 'waiting' AND wake_at <= ?
ORDER BY wake_at ASC
LIMIT ?`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	type dueWait struct{ waitID, taskID string }
	due := []dueWait{}
	for rows.Next() {
		var item dueWait
		if err := rows.Scan(&item.waitID, &item.taskID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		due = append(due, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, item := range due {
		if _, err := tx.ExecContext(ctx, `UPDATE durable_waits SET status = 'fired', fired_at = ? WHERE id = ?`, time.Now().UTC(), item.waitID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE durable_tasks SET status = 'queued', available_at = ?, updated_at = ? WHERE id = ? AND status = 'waiting'`, time.Now().UTC(), time.Now().UTC(), item.taskID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(due), nil
}

func (s *SQLiteStore) WakeChildWaits(ctx context.Context, childTaskID string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT task_id FROM durable_waits WHERE kind = 'child' AND status = 'waiting' AND child_task_id = ?`, childTaskID)
	if err != nil {
		return err
	}
	taskIDs, err := sqliteScanStringRows(rows)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE durable_waits SET status = 'fired', fired_at = ? WHERE kind = 'child' AND status = 'waiting' AND child_task_id = ?`, now, childTaskID); err != nil {
		return err
	}
	for _, taskID := range taskIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE durable_tasks SET status = 'queued', available_at = ?, updated_at = ? WHERE id = ? AND status = 'waiting'`, now, now, taskID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) QueueStats(ctx context.Context) ([]QueueStats, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT queue,
	SUM(CASE WHEN status = 'queued' THEN 1 ELSE 0 END) AS queued,
	SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END) AS running,
	SUM(CASE WHEN status = 'waiting' THEN 1 ELSE 0 END) AS waiting,
	SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed,
	SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed,
	SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END) AS cancelled
FROM durable_tasks
GROUP BY queue
ORDER BY queue`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []QueueStats{}
	for rows.Next() {
		var stats QueueStats
		if err := rows.Scan(&stats.Queue, &stats.Queued, &stats.Running, &stats.Waiting, &stats.Completed, &stats.Failed, &stats.Cancelled); err != nil {
			return nil, err
		}
		out = append(out, stats)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) PruneTerminalTasks(ctx context.Context, before time.Time) (int64, error) {
	if err := s.Init(ctx); err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id
FROM durable_tasks
WHERE status IN ('completed', 'failed', 'cancelled') AND completed_at IS NOT NULL AND completed_at < ?`, before.UTC())
	if err != nil {
		return 0, err
	}
	taskIDs, err := sqliteScanStringRows(rows)
	if err != nil || len(taskIDs) == 0 {
		return int64(len(taskIDs)), err
	}
	inSQL, args := sqliteInClause(taskIDs)
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM durable_waits WHERE child_task_id IN (%s)`, inSQL), args...); err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM durable_outbox WHERE task_id IN (%s)`, inSQL), args...); err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM durable_tasks WHERE id IN (%s)`, inSQL), args...); err != nil {
		return 0, err
	}
	return int64(len(taskIDs)), nil
}

const sqliteTaskSelectSQL = `
SELECT id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
FROM durable_tasks
`

func sqliteScanStringRows(rows *sql.Rows) ([]string, error) {
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func sqliteInClause(values []string) (string, []any) {
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, value := range values {
		placeholders[i] = "?"
		args[i] = value
	}
	return strings.Join(placeholders, ","), args
}
