package durable

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) GetCheckpoint(ctx context.Context, taskID, stepKey string) (json.RawMessage, bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT result FROM durable_checkpoints WHERE task_id=$1 AND step_key=$2 AND status='completed'`, taskID, stepKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return append(json.RawMessage(nil), raw...), true, nil
}

func (s *PostgresStore) SaveCheckpoint(ctx context.Context, taskID, stepKey string, value json.RawMessage) (json.RawMessage, error) {
	row := s.pool.QueryRow(ctx, `
INSERT INTO durable_checkpoints (task_id, step_key, result, updated_at)
VALUES ($1,$2,$3::jsonb,NOW())
ON CONFLICT (task_id, step_key) DO UPDATE SET step_key = durable_checkpoints.step_key
RETURNING result
`, taskID, stepKey, nullJSON(value))
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), raw...), nil
}

func (s *PostgresStore) CreateWait(ctx context.Context, wait Wait) (Wait, error) {
	row := s.pool.QueryRow(ctx, `
INSERT INTO durable_waits (id, task_id, run_id, kind, event_name, child_task_id, wake_at, status)
VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8)
ON CONFLICT (id) DO UPDATE SET id = durable_waits.id
RETURNING id, task_id, COALESCE(run_id,''), kind, event_name, child_task_id, wake_at, status, created_at, fired_at
`, wait.ID, wait.TaskID, wait.RunID, string(wait.Kind), wait.EventName, wait.ChildTaskID, wait.WakeAt, nonEmpty(wait.Status, "waiting"))
	return scanWait(row)
}

func (s *PostgresStore) GetWait(ctx context.Context, waitID string) (Wait, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, task_id, COALESCE(run_id,''), kind, event_name, child_task_id, wake_at, status, created_at, fired_at FROM durable_waits WHERE id=$1`, waitID)
	wait, err := scanWait(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Wait{}, false, nil
		}
		return Wait{}, false, err
	}
	return wait, true, nil
}

func (s *PostgresStore) FireDueTimers(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	cmd, err := s.pool.Exec(ctx, `
WITH due AS (
	SELECT id, task_id
	FROM durable_waits
	WHERE kind IN ('timer', 'event') AND status='waiting' AND wake_at <= $1
	ORDER BY wake_at ASC
	LIMIT $2
	FOR UPDATE SKIP LOCKED
), fired AS (
	UPDATE durable_waits w SET status='fired', fired_at=NOW()
	FROM due WHERE w.id=due.id
	RETURNING due.task_id
)
UPDATE durable_tasks t SET status='queued', available_at=NOW(), updated_at=NOW()
FROM fired WHERE t.id=fired.task_id AND t.status='waiting'
`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return int(cmd.RowsAffected()), nil
}

func (s *PostgresStore) WakeChildWaits(ctx context.Context, childTaskID string) error {
	_, err := s.pool.Exec(ctx, `
WITH fired AS (
	UPDATE durable_waits SET status='fired', fired_at=NOW()
	WHERE kind='child' AND status='waiting' AND child_task_id=$1
	RETURNING task_id
)
UPDATE durable_tasks t SET status='queued', available_at=NOW(), updated_at=NOW()
FROM fired WHERE t.id=fired.task_id AND t.status='waiting'
`, childTaskID)
	return err
}

func (s *PostgresStore) QueueStats(ctx context.Context) ([]QueueStats, error) {
	rows, err := s.pool.Query(ctx, `
SELECT queue,
	COUNT(*) FILTER (WHERE status='queued') AS queued,
	COUNT(*) FILTER (WHERE status='running') AS running,
	COUNT(*) FILTER (WHERE status='waiting') AS waiting,
	COUNT(*) FILTER (WHERE status='completed') AS completed,
	COUNT(*) FILTER (WHERE status='failed') AS failed,
	COUNT(*) FILTER (WHERE status='cancelled') AS cancelled
FROM durable_tasks
GROUP BY queue
ORDER BY queue
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

func scanTask(row pgx.Row, task *Task) error {
	var params, headers, retryPolicy, result, failure []byte
	var completedAt *time.Time
	var status string
	if err := row.Scan(&task.ID, &task.Queue, &task.Name, &task.UserID, &params, &headers, &status, &task.IdempotencyKey, &task.ParentTaskID, &task.ParentRunID, &retryPolicy, &task.Attempt, &task.AvailableAt, &result, &failure, &task.Error, &task.CreatedAt, &task.UpdatedAt, &completedAt); err != nil {
		return err
	}
	task.Status = TaskStatus(status)
	_ = json.Unmarshal(params, &task.Params)
	_ = json.Unmarshal(headers, &task.Headers)
	_ = json.Unmarshal(retryPolicy, &task.RetryPolicy)
	task.RetryPolicy = normalizeRetryPolicy(task.RetryPolicy)
	if len(result) > 0 {
		task.Result = append(json.RawMessage(nil), result...)
	}
	if len(failure) > 0 {
		task.Failure = append(json.RawMessage(nil), failure...)
	}
	if completedAt != nil {
		t := completedAt.UTC()
		task.CompletedAt = &t
	}
	task.AvailableAt = task.AvailableAt.UTC()
	task.CreatedAt = task.CreatedAt.UTC()
	task.UpdatedAt = task.UpdatedAt.UTC()
	return nil
}

func scanEvent(row pgx.Row) (Event, error) {
	var ev Event
	var payload []byte
	if err := row.Scan(&ev.ID, &ev.TaskID, &ev.Queue, &ev.Name, &ev.Sequence, &payload, &ev.OccurredAt); err != nil {
		return Event{}, err
	}
	_ = json.Unmarshal(payload, &ev.Payload)
	ev.OccurredAt = ev.OccurredAt.UTC()
	return ev, nil
}

func scanWait(row pgx.Row) (Wait, error) {
	var wait Wait
	var kind string
	var wakeAt, firedAt *time.Time
	if err := row.Scan(&wait.ID, &wait.TaskID, &wait.RunID, &kind, &wait.EventName, &wait.ChildTaskID, &wakeAt, &wait.Status, &wait.CreatedAt, &firedAt); err != nil {
		return Wait{}, err
	}
	wait.Kind = WaitKind(kind)
	if wakeAt != nil {
		t := wakeAt.UTC()
		wait.WakeAt = &t
	}
	if firedAt != nil {
		t := firedAt.UTC()
		wait.FiredAt = &t
	}
	wait.CreatedAt = wait.CreatedAt.UTC()
	return wait, nil
}

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func nullJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte(`{}`)
	}
	return raw
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
