package databases

import (
	"context"
	"manifold/internal/persistence"
	pulsecore "manifold/internal/pulse"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgPulseStore struct {
	pool *pgxpool.Pool
}

func (s *pgPulseStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS pulse_rooms (
	room_id TEXT NOT NULL,
	route_target TEXT NOT NULL DEFAULT '',
	bot_id TEXT NOT NULL DEFAULT '',
    project_id TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    revision BIGINT NOT NULL DEFAULT 1,
    active_claim_token TEXT,
    active_claim_until TIMESTAMPTZ,
    last_pulse_attempt_at TIMESTAMPTZ,
    last_pulse_completed_at TIMESTAMPTZ,
    last_pulse_summary TEXT,
    last_pulse_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (room_id, route_target)
);
ALTER TABLE pulse_rooms ADD COLUMN IF NOT EXISTS route_target TEXT NOT NULL DEFAULT '';
ALTER TABLE pulse_rooms ADD COLUMN IF NOT EXISTS bot_id TEXT NOT NULL DEFAULT '';
UPDATE pulse_rooms
SET route_target = COALESCE(NULLIF(route_target, ''), bot_id, '')
WHERE route_target IS NULL OR route_target = '';
UPDATE pulse_rooms
SET bot_id = COALESCE(NULLIF(bot_id, ''), route_target, '')
WHERE bot_id IS NULL OR bot_id = '';
CREATE TABLE IF NOT EXISTS pulse_tasks (
    id TEXT PRIMARY KEY,
	room_id TEXT NOT NULL,
	route_target TEXT NOT NULL DEFAULT '',
	bot_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    prompt TEXT NOT NULL,
    interval_seconds INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at TIMESTAMPTZ,
    last_result_summary TEXT,
	active_durable_task_id TEXT,
	last_durable_task_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT pulse_tasks_room_id_route_target_fkey
		FOREIGN KEY (room_id, route_target) REFERENCES pulse_rooms(room_id, route_target) ON DELETE CASCADE
);
ALTER TABLE pulse_tasks ADD COLUMN IF NOT EXISTS route_target TEXT NOT NULL DEFAULT '';
ALTER TABLE pulse_tasks ADD COLUMN IF NOT EXISTS bot_id TEXT NOT NULL DEFAULT '';
ALTER TABLE pulse_tasks ADD COLUMN IF NOT EXISTS schedule_type TEXT NOT NULL DEFAULT 'interval';
ALTER TABLE pulse_tasks ADD COLUMN IF NOT EXISTS specific_time TEXT NOT NULL DEFAULT '';
ALTER TABLE pulse_tasks ADD COLUMN IF NOT EXISTS specific_at TIMESTAMPTZ;
ALTER TABLE pulse_tasks ADD COLUMN IF NOT EXISTS active_durable_task_id TEXT;
ALTER TABLE pulse_tasks ADD COLUMN IF NOT EXISTS last_durable_task_id TEXT;
UPDATE pulse_tasks
SET route_target = COALESCE(NULLIF(route_target, ''), bot_id, '')
WHERE route_target IS NULL OR route_target = '';
UPDATE pulse_tasks
SET bot_id = COALESCE(NULLIF(bot_id, ''), route_target, '')
WHERE bot_id IS NULL OR bot_id = '';

DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM information_schema.table_constraints
		WHERE table_schema = current_schema()
		  AND table_name = 'pulse_tasks'
		  AND constraint_name = 'pulse_tasks_room_id_fkey'
	) THEN
		ALTER TABLE pulse_tasks DROP CONSTRAINT pulse_tasks_room_id_fkey;
	END IF;
END
$$;

DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM information_schema.table_constraints
		WHERE table_schema = current_schema()
		  AND table_name = 'pulse_tasks'
		  AND constraint_name = 'pulse_tasks_room_id_bot_id_fkey'
	) THEN
		ALTER TABLE pulse_tasks DROP CONSTRAINT pulse_tasks_room_id_bot_id_fkey;
	END IF;
	IF EXISTS (
		SELECT 1
		FROM information_schema.table_constraints
		WHERE table_schema = current_schema()
		  AND table_name = 'pulse_tasks'
		  AND constraint_name = 'pulse_tasks_room_id_route_target_fkey'
	) THEN
		ALTER TABLE pulse_tasks DROP CONSTRAINT pulse_tasks_room_id_route_target_fkey;
	END IF;
	IF EXISTS (
		SELECT 1
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = current_schema()
		  AND t.relname = 'pulse_rooms'
		  AND c.conname = 'pulse_rooms_pkey'
		  AND c.contype = 'p'
		  AND pg_get_constraintdef(c.oid) <> 'PRIMARY KEY (room_id, route_target)'
	) THEN
		ALTER TABLE pulse_rooms DROP CONSTRAINT pulse_rooms_pkey;
	END IF;
	IF EXISTS (
		SELECT 1
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = current_schema()
		  AND t.relname = 'pulse_rooms'
		  AND c.conname = 'pulse_rooms_room_id_route_target_key'
	) THEN
		ALTER TABLE pulse_rooms DROP CONSTRAINT pulse_rooms_room_id_route_target_key;
	END IF;
END
$$;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM information_schema.table_constraints
		WHERE table_schema = current_schema()
		  AND table_name = 'pulse_rooms'
		  AND constraint_name = 'pulse_rooms_pkey'
	) THEN
		ALTER TABLE pulse_rooms ADD CONSTRAINT pulse_rooms_pkey PRIMARY KEY (room_id, route_target);
	END IF;
	IF NOT EXISTS (
		SELECT 1
		FROM information_schema.table_constraints
		WHERE table_schema = current_schema()
		  AND table_name = 'pulse_rooms'
		  AND constraint_name = 'pulse_rooms_room_id_route_target_key'
	) THEN
		ALTER TABLE pulse_rooms ADD CONSTRAINT pulse_rooms_room_id_route_target_key UNIQUE (room_id, route_target);
	END IF;
	IF NOT EXISTS (
		SELECT 1
		FROM information_schema.table_constraints
		WHERE table_schema = current_schema()
		  AND table_name = 'pulse_tasks'
		  AND constraint_name = 'pulse_tasks_room_id_route_target_fkey'
	) THEN
		ALTER TABLE pulse_tasks
			ADD CONSTRAINT pulse_tasks_room_id_route_target_fkey
			FOREIGN KEY (room_id, route_target) REFERENCES pulse_rooms(room_id, route_target) ON DELETE CASCADE;
	END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_pulse_rooms_enabled ON pulse_rooms(enabled);
CREATE INDEX IF NOT EXISTS idx_pulse_rooms_claim_until ON pulse_rooms(active_claim_until);
CREATE INDEX IF NOT EXISTS idx_pulse_rooms_bot_room ON pulse_rooms(bot_id, room_id);
CREATE INDEX IF NOT EXISTS idx_pulse_rooms_route_target_room ON pulse_rooms(route_target, room_id);
CREATE INDEX IF NOT EXISTS idx_pulse_tasks_room_id ON pulse_tasks(room_id, route_target);
CREATE INDEX IF NOT EXISTS idx_pulse_tasks_enabled ON pulse_tasks(room_id, route_target, enabled);
`)
	return err
}

func (s *pgPulseStore) EnsureRoom(ctx context.Context, roomID, routeTarget string) (persistence.PulseRoom, error) {
	roomID = strings.TrimSpace(roomID)
	routeTarget = strings.TrimSpace(routeTarget)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO pulse_rooms (room_id, route_target, bot_id, enabled)
VALUES ($1, $2, $2, TRUE)
ON CONFLICT (room_id, route_target) DO UPDATE SET bot_id = EXCLUDED.bot_id
`, roomID, routeTarget)
	if err != nil {
		return persistence.PulseRoom{}, err
	}
	return s.GetRoom(ctx, roomID, routeTarget)
}

func (s *pgPulseStore) GetRoom(ctx context.Context, roomID, routeTarget string) (persistence.PulseRoom, error) {
	var room persistence.PulseRoom
	var projectID, claimToken, summary, pulseErr *string
	var claimUntil, attemptAt, completedAt *time.Time
	err := s.pool.QueryRow(ctx, `
SELECT room_id, route_target, project_id, enabled, revision, active_claim_token, active_claim_until,
       last_pulse_attempt_at, last_pulse_completed_at, last_pulse_summary, last_pulse_error,
       created_at, updated_at
FROM pulse_rooms
WHERE room_id = $1 AND route_target = $2
`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget)).Scan(
		&room.RoomID,
		&room.RouteTarget,
		&projectID,
		&room.Enabled,
		&room.Revision,
		&claimToken,
		&claimUntil,
		&attemptAt,
		&completedAt,
		&summary,
		&pulseErr,
		&room.CreatedAt,
		&room.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return persistence.PulseRoom{}, persistence.ErrNotFound
		}
		return persistence.PulseRoom{}, err
	}
	if projectID != nil {
		room.ProjectID = *projectID
	}
	if claimToken != nil {
		room.ActiveClaimToken = *claimToken
	}
	if claimUntil != nil {
		room.ActiveClaimUntil = claimUntil.UTC()
	}
	if attemptAt != nil {
		room.LastPulseAttemptAt = attemptAt.UTC()
	}
	if completedAt != nil {
		room.LastPulseCompletedAt = completedAt.UTC()
	}
	if summary != nil {
		room.LastPulseSummary = *summary
	}
	if pulseErr != nil {
		room.LastPulseError = *pulseErr
	}
	return room, nil
}

func (s *pgPulseStore) ListRooms(ctx context.Context, routeTarget string) ([]persistence.PulseRoom, error) {
	routeTarget = strings.TrimSpace(routeTarget)
	query := `
SELECT room_id, route_target, project_id, enabled, revision, active_claim_token, active_claim_until,
       last_pulse_attempt_at, last_pulse_completed_at, last_pulse_summary, last_pulse_error,
       created_at, updated_at
FROM pulse_rooms
WHERE ($1 = '' OR route_target = $1)
ORDER BY room_id ASC, route_target ASC
	`
	rows, err := s.pool.Query(ctx, query, routeTarget)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []persistence.PulseRoom
	for rows.Next() {
		room, err := scanPulseRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, room)
	}
	return out, rows.Err()
}

func (s *pgPulseStore) UpsertRoom(ctx context.Context, room persistence.PulseRoom) (persistence.PulseRoom, error) {
	roomID := strings.TrimSpace(room.RoomID)
	routeTarget := strings.TrimSpace(room.RouteTarget)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO pulse_rooms (
	room_id, route_target, bot_id, project_id, enabled, active_claim_token, active_claim_until,
    last_pulse_attempt_at, last_pulse_completed_at, last_pulse_summary, last_pulse_error,
    created_at, updated_at
)
VALUES ($1, $2, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), NOW(), NOW())
ON CONFLICT (room_id, route_target) DO UPDATE SET
    bot_id = EXCLUDED.bot_id,
    project_id = EXCLUDED.project_id,
    enabled = EXCLUDED.enabled,
    active_claim_token = EXCLUDED.active_claim_token,
    active_claim_until = EXCLUDED.active_claim_until,
    last_pulse_attempt_at = COALESCE(EXCLUDED.last_pulse_attempt_at, pulse_rooms.last_pulse_attempt_at),
    last_pulse_completed_at = COALESCE(EXCLUDED.last_pulse_completed_at, pulse_rooms.last_pulse_completed_at),
    last_pulse_summary = COALESCE(EXCLUDED.last_pulse_summary, pulse_rooms.last_pulse_summary),
    last_pulse_error = COALESCE(EXCLUDED.last_pulse_error, pulse_rooms.last_pulse_error),
    updated_at = NOW(),
    revision = pulse_rooms.revision + 1
`, roomID, routeTarget, strings.TrimSpace(room.ProjectID), room.Enabled, strings.TrimSpace(room.ActiveClaimToken), nullTime(room.ActiveClaimUntil), nullTime(room.LastPulseAttemptAt), nullTime(room.LastPulseCompletedAt), emptyToNil(room.LastPulseSummary), emptyToNil(room.LastPulseError))
	if err != nil {
		return persistence.PulseRoom{}, err
	}
	return s.GetRoom(ctx, roomID, routeTarget)
}

func (s *pgPulseStore) ListTasks(ctx context.Context, roomID, routeTarget string) ([]persistence.PulseTask, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, room_id, route_target, title, prompt, schedule_type, interval_seconds, specific_time, specific_at, enabled, last_run_at, last_result_summary, active_durable_task_id, last_durable_task_id, created_at, updated_at
FROM pulse_tasks
WHERE room_id = $1 AND route_target = $2
ORDER BY created_at ASC, id ASC
`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]persistence.PulseTask, 0, 8)
	for rows.Next() {
		task, err := scanPulseTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (s *pgPulseStore) UpsertTask(ctx context.Context, task persistence.PulseTask) (persistence.PulseTask, error) {
	roomID := strings.TrimSpace(task.RoomID)
	routeTarget := strings.TrimSpace(task.RouteTarget)
	if roomID == "" {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	if _, err := s.EnsureRoom(ctx, roomID, routeTarget); err != nil {
		return persistence.PulseTask{}, err
	}
	if strings.TrimSpace(task.ID) == "" {
		task.ID = uuid.NewString()
	}
	var err error
	task, err = pulsecore.NormalizeTaskSchedule(task)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO pulse_tasks (
	id, room_id, route_target, bot_id, title, prompt, schedule_type, interval_seconds, specific_time, specific_at, enabled, last_run_at, last_result_summary, active_durable_task_id, last_durable_task_id, created_at, updated_at
)
VALUES ($1, $2, $3, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET
    room_id = EXCLUDED.room_id,
	route_target = EXCLUDED.route_target,
	bot_id = EXCLUDED.bot_id,
    title = EXCLUDED.title,
    prompt = EXCLUDED.prompt,
    schedule_type = EXCLUDED.schedule_type,
    interval_seconds = EXCLUDED.interval_seconds,
    specific_time = EXCLUDED.specific_time,
    specific_at = EXCLUDED.specific_at,
    enabled = EXCLUDED.enabled,
    last_run_at = COALESCE(EXCLUDED.last_run_at, pulse_tasks.last_run_at),
    last_result_summary = COALESCE(EXCLUDED.last_result_summary, pulse_tasks.last_result_summary),
    active_durable_task_id = COALESCE(EXCLUDED.active_durable_task_id, pulse_tasks.active_durable_task_id),
    last_durable_task_id = COALESCE(EXCLUDED.last_durable_task_id, pulse_tasks.last_durable_task_id),
    updated_at = NOW()
`, task.ID, roomID, routeTarget, strings.TrimSpace(task.Title), strings.TrimSpace(task.Prompt), task.ScheduleType, task.IntervalSeconds, task.SpecificTime, nullTime(task.SpecificAt), task.Enabled, nullTime(task.LastRunAt), emptyToNil(task.LastResultSummary), emptyToNil(task.ActiveDurableTaskID), emptyToNil(task.LastDurableTaskID))
	if err != nil {
		return persistence.PulseTask{}, err
	}
	return s.getTask(ctx, roomID, routeTarget, task.ID)
}

func (s *pgPulseStore) MarkTaskRunQueued(ctx context.Context, roomID, routeTarget, taskID, durableTaskID string) (persistence.PulseTask, error) {
	roomID = strings.TrimSpace(roomID)
	routeTarget = strings.TrimSpace(routeTarget)
	taskID = strings.TrimSpace(taskID)
	durableTaskID = strings.TrimSpace(durableTaskID)
	if roomID == "" || taskID == "" || durableTaskID == "" {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	cmd, err := s.pool.Exec(ctx, `
UPDATE pulse_tasks
SET active_durable_task_id = $4,
    updated_at = NOW()
WHERE room_id = $1 AND route_target = $2 AND id = $3
`, roomID, routeTarget, taskID, durableTaskID)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	return s.getTask(ctx, roomID, routeTarget, taskID)
}

func (s *pgPulseStore) DeleteTask(ctx context.Context, roomID, routeTarget, taskID string) error {
	cmd, err := s.pool.Exec(ctx, `
DELETE FROM pulse_tasks
WHERE room_id = $1 AND route_target = $2 AND id = $3
`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget), strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *pgPulseStore) ClaimRoom(ctx context.Context, roomID, routeTarget, token string, leaseUntil time.Time) (bool, error) {
	cmd, err := s.pool.Exec(ctx, `
UPDATE pulse_rooms
SET active_claim_token = $2,
    active_claim_until = $3,
    last_pulse_attempt_at = NOW(),
    updated_at = NOW(),
    revision = revision + 1
WHERE room_id = $1
	AND route_target = $4
  AND (active_claim_until IS NULL OR active_claim_until <= NOW() OR active_claim_token = $2)
`, strings.TrimSpace(roomID), strings.TrimSpace(token), leaseUntil.UTC(), strings.TrimSpace(routeTarget))
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *pgPulseStore) ClearRoomClaim(ctx context.Context, roomID, routeTarget string) error {
	cmd, err := s.pool.Exec(ctx, `
UPDATE pulse_rooms
SET active_claim_token = NULL,
    active_claim_until = NULL,
    updated_at = NOW(),
    revision = revision + 1
WHERE room_id = $1 AND route_target = $2
`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *pgPulseStore) CompleteRoomPulse(ctx context.Context, completion persistence.RoomPulseCompletion) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cmd, err := tx.Exec(ctx, `
UPDATE pulse_rooms
SET active_claim_token = CASE WHEN $2 = '' THEN active_claim_token ELSE NULL END,
    active_claim_until = CASE WHEN $2 = '' THEN active_claim_until ELSE NULL END,
    last_pulse_completed_at = $3,
    last_pulse_summary = NULLIF($4, ''),
    last_pulse_error = NULLIF($5, ''),
    updated_at = NOW(),
    revision = revision + 1
WHERE room_id = $1 AND route_target = $6 AND ($2 = '' OR active_claim_token = $2)
`, strings.TrimSpace(completion.RoomID), strings.TrimSpace(completion.Token), completion.CompletedAt.UTC(), completion.Summary, completion.Error, strings.TrimSpace(completion.RouteTarget))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrRevisionConflict
	}
	if len(completion.DueTaskIDs) > 0 {
		if _, err := tx.Exec(ctx, `
UPDATE pulse_tasks
SET last_run_at = $3,
    last_result_summary = NULLIF($4, ''),
    active_durable_task_id = CASE
        WHEN $6 = '' OR active_durable_task_id = $6 THEN NULL
        ELSE active_durable_task_id
    END,
    last_durable_task_id = CASE
        WHEN $6 = '' THEN last_durable_task_id
        ELSE $6
    END,
    updated_at = NOW()
WHERE room_id = $1 AND route_target = $5 AND id = ANY($2)
`, strings.TrimSpace(completion.RoomID), completion.DueTaskIDs, completion.CompletedAt.UTC(), completion.Summary, strings.TrimSpace(completion.RouteTarget), strings.TrimSpace(completion.DurableTaskID)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
