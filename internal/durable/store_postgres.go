package durable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewStore(pool *pgxpool.Pool) Store {
	if pool == nil {
		return NewMemoryStore()
	}
	return &PostgresStore{pool: pool}
}

type PostgresStore struct {
	pool *pgxpool.Pool
}

func (s *PostgresStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("durable postgres store requires pool")
	}
	_, err := s.pool.Exec(ctx, durableSchema)
	return err
}

const durableSchema = `
CREATE TABLE IF NOT EXISTS durable_tasks (
	id TEXT PRIMARY KEY,
	queue TEXT NOT NULL,
	name TEXT NOT NULL,
	user_id BIGINT NOT NULL DEFAULT 0,
	params JSONB NOT NULL DEFAULT '{}'::jsonb,
	headers JSONB NOT NULL DEFAULT '{}'::jsonb,
	status TEXT NOT NULL,
	idempotency_key TEXT NOT NULL DEFAULT '',
	parent_task_id TEXT,
	parent_run_id TEXT,
	retry_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
	attempt INTEGER NOT NULL DEFAULT 0,
	available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	result JSONB,
	failure JSONB,
	error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	completed_at TIMESTAMPTZ
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
	lease_until TIMESTAMPTZ,
	started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	completed_at TIMESTAMPTZ,
	error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS durable_runs_task_idx ON durable_runs(task_id, attempt DESC);
CREATE INDEX IF NOT EXISTS durable_runs_lease_idx ON durable_runs(status, lease_until);

CREATE TABLE IF NOT EXISTS durable_checkpoints (
	task_id TEXT NOT NULL REFERENCES durable_tasks(id) ON DELETE CASCADE,
	step_key TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'completed',
	result JSONB,
	error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (task_id, step_key)
);

CREATE TABLE IF NOT EXISTS durable_events (
	id BIGSERIAL PRIMARY KEY,
	task_id TEXT REFERENCES durable_tasks(id) ON DELETE CASCADE,
	queue TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	sequence BIGINT NOT NULL DEFAULT 0,
	event_key TEXT NOT NULL DEFAULT '',
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE durable_events
	ADD COLUMN IF NOT EXISTS event_key TEXT NOT NULL DEFAULT '';
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
	wake_at TIMESTAMPTZ,
	status TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	fired_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS durable_waits_timer_idx ON durable_waits(kind, status, wake_at);
CREATE INDEX IF NOT EXISTS durable_waits_event_idx ON durable_waits(kind, status, event_name);
CREATE INDEX IF NOT EXISTS durable_waits_child_idx ON durable_waits(kind, status, child_task_id);

CREATE TABLE IF NOT EXISTS durable_outbox (
	id BIGSERIAL PRIMARY KEY,
	task_id TEXT,
	event_id BIGINT,
	topic TEXT NOT NULL,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	delivered_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS durable_outbox_pending_idx ON durable_outbox(delivered_at, id);
`

func (s *PostgresStore) SpawnTask(ctx context.Context, req SpawnRequest) (Task, bool, error) {
	req.Queue = normalizeQueue(req.Queue)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return Task{}, false, fmt.Errorf("durable task name required")
	}
	if req.AvailableAt.IsZero() {
		req.AvailableAt = time.Now().UTC()
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
	id := newID("dtask")
	if req.IdempotencyKey != "" {
		var existing Task
		row := s.pool.QueryRow(ctx, `
SELECT id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
FROM durable_tasks WHERE user_id=$1 AND queue=$2 AND idempotency_key=$3
`, req.UserID, req.Queue, req.IdempotencyKey)
		if err := scanTask(row, &existing); err == nil {
			return existing, false, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Task{}, false, err
		}
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO durable_tasks (id, queue, name, user_id, params, headers, status, idempotency_key, parent_task_id, parent_run_id, retry_policy, available_at)
VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,$8,NULLIF($9,''),NULLIF($10,''),$11::jsonb,$12)
ON CONFLICT DO NOTHING
RETURNING id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
`, id, req.Queue, req.Name, req.UserID, params, headers, string(TaskStatusQueued), req.IdempotencyKey, req.ParentTaskID, req.ParentRunID, retryPolicy, req.AvailableAt.UTC())
	var task Task
	if err := scanTask(row, &task); err != nil {
		if errors.Is(err, pgx.ErrNoRows) && req.IdempotencyKey != "" {
			row := s.pool.QueryRow(ctx, `
SELECT id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
FROM durable_tasks WHERE user_id=$1 AND queue=$2 AND idempotency_key=$3
`, req.UserID, req.Queue, req.IdempotencyKey)
			var existing Task
			if err := scanTask(row, &existing); err == nil {
				return existing, false, nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return Task{}, false, err
			}
		}
		return Task{}, false, err
	}
	return task, true, nil
}

func (s *PostgresStore) GetTask(ctx context.Context, userID int64, taskID string) (Task, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
FROM durable_tasks WHERE id=$1 AND user_id=$2
`, strings.TrimSpace(taskID), userID)
	var task Task
	if err := scanTask(row, &task); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, false, nil
		}
		return Task{}, false, err
	}
	return task, true, nil
}

func (s *PostgresStore) ListTasks(ctx context.Context, userID int64, filter TaskListFilter) ([]Task, error) {
	filter.Queue = strings.TrimSpace(filter.Queue)
	filter.Name = strings.TrimSpace(filter.Name)
	status := strings.TrimSpace(string(filter.Status))
	rows, err := s.pool.Query(ctx, `
SELECT id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
FROM durable_tasks
WHERE user_id=$1
  AND ($2 = '' OR queue=$2)
  AND ($3 = '' OR status=$3)
  AND ($4 = '' OR name=$4)
ORDER BY updated_at DESC, created_at DESC
LIMIT $5
`, userID, filter.Queue, status, filter.Name, normalizeTaskListLimit(filter.Limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		var task Task
		if err := scanTask(rows, &task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *PostgresStore) ListTaskEvents(ctx context.Context, userID int64, taskID string, afterSequence int64) ([]Event, TaskStatus, bool, error) {
	task, ok, err := s.GetTask(ctx, userID, taskID)
	if err != nil || !ok {
		return nil, "", ok, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
FROM durable_events WHERE task_id=$1 AND sequence > $2 ORDER BY sequence ASC
`, taskID, afterSequence)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
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

func (s *PostgresStore) AppendTaskEvent(ctx context.Context, taskID string, name string, payload map[string]any) (Event, error) {
	return s.appendTaskEvent(ctx, taskID, "", name, payload)
}

func (s *PostgresStore) AppendTaskEventOnce(ctx context.Context, taskID string, eventKey string, name string, payload map[string]any) (Event, error) {
	return s.appendTaskEvent(ctx, taskID, strings.TrimSpace(eventKey), name, payload)
}

func (s *PostgresStore) appendTaskEvent(ctx context.Context, taskID string, eventKey string, name string, payload map[string]any) (Event, error) {
	payloadBytes, err := json.Marshal(nonNilMap(payload))
	if err != nil {
		return Event{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback(ctx)
	var queue string
	if err := tx.QueryRow(ctx, `SELECT queue FROM durable_tasks WHERE id=$1`, taskID).Scan(&queue); err != nil {
		return Event{}, err
	}
	if eventKey != "" {
		row := tx.QueryRow(ctx, `
SELECT id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
FROM durable_events WHERE task_id=$1 AND event_key=$2
`, taskID, eventKey)
		if existing, err := scanEvent(row); err == nil {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return Event{}, commitErr
			}
			return existing, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Event{}, err
		}
	}
	var seq int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM durable_events WHERE task_id=$1`, taskID).Scan(&seq); err != nil {
		return Event{}, err
	}
	row := tx.QueryRow(ctx, `
INSERT INTO durable_events (task_id, queue, name, sequence, event_key, payload)
VALUES ($1,$2,$3,$4,$5,$6::jsonb)
ON CONFLICT (task_id, event_key) WHERE task_id IS NOT NULL AND event_key <> '' DO NOTHING
RETURNING id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
`, taskID, queue, strings.TrimSpace(name), seq, eventKey, payloadBytes)
	ev, err := scanEvent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) && eventKey != "" {
			row := tx.QueryRow(ctx, `
SELECT id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
FROM durable_events WHERE task_id=$1 AND event_key=$2
`, taskID, eventKey)
			existing, scanErr := scanEvent(row)
			if scanErr != nil {
				return Event{}, scanErr
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return Event{}, commitErr
			}
			return existing, nil
		}
		return Event{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO durable_outbox (task_id, event_id, topic, payload) VALUES ($1,$2,$3,$4::jsonb)`, taskID, ev.ID, name, payloadBytes); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, err
	}
	return ev, nil
}

func (s *PostgresStore) EmitEvent(ctx context.Context, userID int64, queue string, name string, payload map[string]any) (Event, error) {
	queue = normalizeQueue(queue)
	name = strings.TrimSpace(name)
	payloadBytes, err := json.Marshal(nonNilMap(payload))
	if err != nil {
		return Event{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
INSERT INTO durable_events (queue, name, payload)
VALUES ($1,$2,$3::jsonb)
RETURNING id, COALESCE(task_id,''), queue, name, sequence, event_key, payload, occurred_at
`, queue, name, payloadBytes)
	ev, err := scanEvent(row)
	if err != nil {
		return Event{}, err
	}
	rows, err := tx.Query(ctx, `
SELECT w.id, w.task_id
FROM durable_waits w
JOIN durable_tasks t ON t.id = w.task_id
WHERE w.kind='event' AND w.status='waiting' AND w.event_name=$1 AND t.user_id=$2 AND t.queue=$3
`, name, userID, queue)
	if err != nil {
		return Event{}, err
	}
	type wake struct{ waitID, taskID string }
	var wakes []wake
	for rows.Next() {
		var w wake
		if err := rows.Scan(&w.waitID, &w.taskID); err != nil {
			rows.Close()
			return Event{}, err
		}
		wakes = append(wakes, w)
	}
	rows.Close()
	for _, w := range wakes {
		if _, err := tx.Exec(ctx, `UPDATE durable_waits SET status='fired', fired_at=NOW() WHERE id=$1`, w.waitID); err != nil {
			return Event{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE durable_tasks SET status='queued', available_at=NOW(), updated_at=NOW() WHERE id=$1 AND status='waiting'`, w.taskID); err != nil {
			return Event{}, err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO durable_checkpoints (task_id, step_key, result, updated_at)
VALUES ($1,$2,$3::jsonb,NOW())
ON CONFLICT (task_id, step_key) DO NOTHING
`, w.taskID, "event:"+name, payloadBytes); err != nil {
			return Event{}, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO durable_outbox (event_id, topic, payload) VALUES ($1,$2,$3::jsonb)`, ev.ID, name, payloadBytes); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, err
	}
	return ev, nil
}

func (s *PostgresStore) ClaimNext(ctx context.Context, queues []string, workerID string, lease time.Duration) (Task, Run, bool, error) {
	if len(queues) == 0 {
		return Task{}, Run{}, false, nil
	}
	for i := range queues {
		queues[i] = normalizeQueue(queues[i])
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Task{}, Run{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
UPDATE durable_tasks t SET status='queued', updated_at=NOW()
WHERE t.status='running'
  AND EXISTS (
	SELECT 1 FROM durable_runs r
	WHERE r.task_id=t.id AND r.status='running' AND r.lease_until <= NOW()
  )
`); err != nil {
		return Task{}, Run{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE durable_runs SET status='failed', error='lease expired', completed_at=NOW() WHERE status='running' AND lease_until <= NOW()`); err != nil {
		return Task{}, Run{}, false, err
	}
	row := tx.QueryRow(ctx, `
WITH candidate AS (
	SELECT id
	FROM durable_tasks
	WHERE queue = ANY($1)
	  AND status = 'queued'
	  AND available_at <= NOW()
	ORDER BY available_at ASC, created_at ASC
	LIMIT 1
	FOR UPDATE SKIP LOCKED
)
UPDATE durable_tasks t
SET status='running', attempt=attempt+1, updated_at=NOW()
FROM candidate
WHERE t.id = candidate.id
RETURNING t.id, t.queue, t.name, t.user_id, t.params, t.headers, t.status, t.idempotency_key, COALESCE(t.parent_task_id,''), COALESCE(t.parent_run_id,''), t.retry_policy, t.attempt, t.available_at, t.result, t.failure, t.error, t.created_at, t.updated_at, t.completed_at
`, queues)
	var task Task
	if err := scanTask(row, &task); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, Run{}, false, tx.Commit(ctx)
		}
		return Task{}, Run{}, false, err
	}
	run := Run{ID: newID("drun"), TaskID: task.ID, Attempt: task.Attempt, Status: RunStatusRunning, WorkerID: workerID, LeaseUntil: time.Now().UTC().Add(lease), StartedAt: time.Now().UTC()}
	if _, err := tx.Exec(ctx, `
INSERT INTO durable_runs (id, task_id, attempt, status, worker_id, lease_until, started_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
`, run.ID, run.TaskID, run.Attempt, string(run.Status), run.WorkerID, run.LeaseUntil, run.StartedAt); err != nil {
		return Task{}, Run{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, Run{}, false, err
	}
	return task, run, true, nil
}

func (s *PostgresStore) Heartbeat(ctx context.Context, taskID, runID, workerID string, leaseUntil time.Time) error {
	_, err := s.pool.Exec(ctx, `
UPDATE durable_runs SET lease_until=$4 WHERE id=$1 AND task_id=$2 AND worker_id=$3 AND status='running'
`, runID, taskID, workerID, leaseUntil.UTC())
	return err
}

func (s *PostgresStore) CompleteTask(ctx context.Context, taskID, runID string, result json.RawMessage) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	cmd, err := tx.Exec(ctx, `
UPDATE durable_tasks SET status='completed', result=$2::jsonb, updated_at=NOW(), completed_at=NOW() WHERE id=$1 AND status <> 'cancelled'
`, taskID, nullJSON(result))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrCancelled
	}
	if _, err := tx.Exec(ctx, `UPDATE durable_runs SET status='completed', completed_at=NOW() WHERE id=$1 AND task_id=$2`, runID, taskID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.AppendCompletionEvent(ctx, taskID, "task_completed", result)
}

func (s *PostgresStore) AppendCompletionEvent(ctx context.Context, taskID, name string, result json.RawMessage) error {
	payload := map[string]any{"result": json.RawMessage(nullJSON(result))}
	_, err := s.AppendTaskEvent(ctx, taskID, name, payload)
	return err
}

func (s *PostgresStore) FailTask(ctx context.Context, taskID, runID string, failure json.RawMessage, errText string, nextAttemptAt time.Time) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var attempt int
	var policy RetryPolicy
	var rawPolicy []byte
	if err := tx.QueryRow(ctx, `SELECT attempt, retry_policy FROM durable_tasks WHERE id=$1`, taskID).Scan(&attempt, &rawPolicy); err != nil {
		return err
	}
	_ = json.Unmarshal(rawPolicy, &policy)
	policy = normalizeRetryPolicy(policy)
	status := string(TaskStatusFailed)
	availableAt := time.Now().UTC()
	completedSQL := `NOW()`
	if !nextAttemptAt.IsZero() && attempt < policy.MaxAttempts {
		status = string(TaskStatusQueued)
		availableAt = nextAttemptAt.UTC()
		completedSQL = `NULL`
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
UPDATE durable_tasks SET status=$2, failure=$3::jsonb, error=$4, available_at=$5, updated_at=NOW(), completed_at=%s WHERE id=$1
`, completedSQL), taskID, status, nullJSON(failure), errText, availableAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE durable_runs SET status='failed', error=$3, completed_at=NOW() WHERE id=$1 AND task_id=$2`, runID, taskID, errText); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) MarkTaskWaiting(ctx context.Context, taskID, runID string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE durable_tasks SET status='waiting', updated_at=NOW() WHERE id=$1`, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE durable_runs SET status='waiting', completed_at=NOW() WHERE id=$1 AND task_id=$2`, runID, taskID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) CancelTask(ctx context.Context, userID int64, taskID string) error {
	_, err := s.CancelTaskTree(ctx, userID, taskID)
	return err
}

func (s *PostgresStore) CancelTaskTree(ctx context.Context, userID int64, taskID string) ([]string, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	taskIDs, err := s.cancelTaskTreeIDs(ctx, tx, userID, strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	cancelledIDs, err := s.cancelTaskTreeTx(ctx, tx, taskIDs)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	for _, id := range cancelledIDs {
		_, _ = s.AppendTaskEvent(ctx, id, "task_cancelled", map[string]any{"status": string(TaskStatusCancelled)})
	}
	return cancelledIDs, nil
}

func (s *PostgresStore) cancelTaskTreeIDs(ctx context.Context, tx pgx.Tx, userID int64, taskID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
WITH RECURSIVE task_tree AS (
	SELECT id FROM durable_tasks WHERE id=$1 AND user_id=$2
	UNION ALL
	SELECT child.id
	FROM durable_tasks child
	JOIN task_tree parent ON child.parent_task_id = parent.id
	WHERE child.user_id=$2
)
SELECT id FROM task_tree
`, taskID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	taskIDs, err := scanStringRows(rows)
	if err != nil {
		return nil, err
	}
	if len(taskIDs) == 0 {
		return nil, ErrTaskNotFound
	}
	return taskIDs, nil
}

func (s *PostgresStore) cancelTaskTreeTx(ctx context.Context, tx pgx.Tx, taskIDs []string) ([]string, error) {
	rows, err := tx.Query(ctx, `
UPDATE durable_tasks
SET status='cancelled', error='cancelled', updated_at=NOW(), completed_at=NOW()
WHERE id = ANY($1) AND status <> 'completed'
RETURNING id
`, taskIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cancelledIDs, err := scanStringRows(rows)
	if err != nil || len(cancelledIDs) == 0 {
		return cancelledIDs, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE durable_runs
SET status='cancelled', error='cancelled', completed_at=NOW()
WHERE task_id = ANY($1) AND status NOT IN ('completed','cancelled')
`, cancelledIDs); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE durable_waits
SET status='cancelled', fired_at=NOW()
WHERE task_id = ANY($1) AND status='waiting'
`, cancelledIDs); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
WITH fired AS (
	UPDATE durable_waits
	SET status='fired', fired_at=NOW()
	WHERE child_task_id = ANY($1) AND status='waiting' AND NOT (task_id = ANY($1))
	RETURNING task_id
)
UPDATE durable_tasks t
SET status='queued', available_at=NOW(), updated_at=NOW()
FROM fired
WHERE t.id=fired.task_id AND t.status='waiting'
`, cancelledIDs); err != nil {
		return nil, err
	}
	return cancelledIDs, nil
}

func (s *PostgresStore) RetryTask(ctx context.Context, userID int64, taskID string, resetCheckpoints bool) (Task, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
SELECT id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
FROM durable_tasks WHERE id=$1 AND user_id=$2
FOR UPDATE
`, strings.TrimSpace(taskID), userID)
	var existing Task
	if err := scanTask(row, &existing); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, err
	}
	if existing.Status != TaskStatusFailed && existing.Status != TaskStatusCancelled {
		return Task{}, ErrInvalidState
	}
	row = tx.QueryRow(ctx, `
UPDATE durable_tasks
SET status='queued',
	attempt=0,
	available_at=NOW(),
	result=NULL,
	failure=NULL,
	error='',
	completed_at=NULL,
	updated_at=NOW()
WHERE id=$1 AND user_id=$2
RETURNING id, queue, name, user_id, params, headers, status, idempotency_key, COALESCE(parent_task_id,''), COALESCE(parent_run_id,''), retry_policy, attempt, available_at, result, failure, error, created_at, updated_at, completed_at
`, strings.TrimSpace(taskID), userID)
	var task Task
	if err := scanTask(row, &task); err != nil {
		return Task{}, err
	}
	if resetCheckpoints {
		if _, err := tx.Exec(ctx, `DELETE FROM durable_checkpoints WHERE task_id=$1`, task.ID); err != nil {
			return Task{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE durable_waits SET status='cancelled', fired_at=NOW() WHERE task_id=$1 AND status='waiting'`, task.ID); err != nil {
		return Task{}, err
	}
	payloadBytes, err := json.Marshal(map[string]any{"reset_checkpoints": resetCheckpoints})
	if err != nil {
		return Task{}, err
	}
	var seq int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM durable_events WHERE task_id=$1`, task.ID).Scan(&seq); err != nil {
		return Task{}, err
	}
	var eventID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO durable_events (task_id, queue, name, sequence, payload)
VALUES ($1,$2,$3,$4,$5::jsonb)
RETURNING id
`, task.ID, task.Queue, "task_retried", seq, payloadBytes).Scan(&eventID); err != nil {
		return Task{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO durable_outbox (task_id, event_id, topic, payload) VALUES ($1,$2,$3,$4::jsonb)`, task.ID, eventID, "task_retried", payloadBytes); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return task, nil
}
