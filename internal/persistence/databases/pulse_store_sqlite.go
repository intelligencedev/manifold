package databases

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/persistence"
	pulsecore "manifold/internal/pulse"
)

type sqlitePulseStore struct {
	db *sql.DB
}

func NewSQLitePulseStore(db *sql.DB) persistence.PulseStore {
	return &sqlitePulseStore{db: db}
}

func (s *sqlitePulseStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite pulse store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS pulse_rooms (
	room_id TEXT NOT NULL,
	route_target TEXT NOT NULL DEFAULT '',
	bot_id TEXT NOT NULL DEFAULT '',
	project_id TEXT,
	enabled INTEGER NOT NULL DEFAULT 1,
	revision INTEGER NOT NULL DEFAULT 1,
	active_claim_token TEXT,
	active_claim_until DATETIME,
	last_pulse_attempt_at DATETIME,
	last_pulse_completed_at DATETIME,
	last_pulse_summary TEXT,
	last_pulse_error TEXT,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	PRIMARY KEY(room_id, route_target)
);
CREATE TABLE IF NOT EXISTS pulse_tasks (
	id TEXT PRIMARY KEY,
	room_id TEXT NOT NULL,
	route_target TEXT NOT NULL DEFAULT '',
	bot_id TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL,
	prompt TEXT NOT NULL,
	schedule_type TEXT NOT NULL DEFAULT 'interval',
	interval_seconds INTEGER NOT NULL,
	specific_time TEXT NOT NULL DEFAULT '',
	specific_at DATETIME,
	enabled INTEGER NOT NULL DEFAULT 1,
	last_run_at DATETIME,
	last_result_summary TEXT,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	FOREIGN KEY(room_id, route_target) REFERENCES pulse_rooms(room_id, route_target) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_pulse_rooms_enabled ON pulse_rooms(enabled);
CREATE INDEX IF NOT EXISTS idx_pulse_rooms_claim_until ON pulse_rooms(active_claim_until);
CREATE INDEX IF NOT EXISTS idx_pulse_rooms_route_target_room ON pulse_rooms(route_target, room_id);
CREATE INDEX IF NOT EXISTS idx_pulse_tasks_room_id ON pulse_tasks(room_id, route_target);
CREATE INDEX IF NOT EXISTS idx_pulse_tasks_enabled ON pulse_tasks(room_id, route_target, enabled);
`)
	return err
}

func (s *sqlitePulseStore) EnsureRoom(ctx context.Context, roomID, routeTarget string) (persistence.PulseRoom, error) {
	if err := s.Init(ctx); err != nil {
		return persistence.PulseRoom{}, err
	}
	roomID = strings.TrimSpace(roomID)
	routeTarget = strings.TrimSpace(routeTarget)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO pulse_rooms(room_id, route_target, bot_id, enabled, created_at, updated_at)
VALUES(?, ?, ?, 1, ?, ?)
ON CONFLICT(room_id, route_target) DO UPDATE SET
	bot_id = excluded.bot_id
`, roomID, routeTarget, routeTarget, now, now)
	if err != nil {
		return persistence.PulseRoom{}, err
	}
	return s.GetRoom(ctx, roomID, routeTarget)
}

func (s *sqlitePulseStore) GetRoom(ctx context.Context, roomID, routeTarget string) (persistence.PulseRoom, error) {
	if err := s.Init(ctx); err != nil {
		return persistence.PulseRoom{}, err
	}
	row := s.db.QueryRowContext(ctx, sqlitePulseRoomSelectSQL+`
WHERE room_id = ? AND route_target = ?`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget))
	room, err := scanPulseRoom(row)
	if errors.Is(err, sql.ErrNoRows) {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	if err != nil {
		return persistence.PulseRoom{}, err
	}
	return room, nil
}

func (s *sqlitePulseStore) ListRooms(ctx context.Context, routeTarget string) ([]persistence.PulseRoom, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	routeTarget = strings.TrimSpace(routeTarget)
	rows, err := s.db.QueryContext(ctx, sqlitePulseRoomSelectSQL+`
WHERE (? = '' OR route_target = ?)
ORDER BY room_id ASC, route_target ASC`, routeTarget, routeTarget)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persistence.PulseRoom{}
	for rows.Next() {
		room, err := scanPulseRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, room)
	}
	return out, rows.Err()
}

func (s *sqlitePulseStore) UpsertRoom(ctx context.Context, room persistence.PulseRoom) (persistence.PulseRoom, error) {
	if err := s.Init(ctx); err != nil {
		return persistence.PulseRoom{}, err
	}
	roomID := strings.TrimSpace(room.RoomID)
	routeTarget := strings.TrimSpace(room.RouteTarget)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO pulse_rooms (
	room_id, route_target, bot_id, project_id, enabled, active_claim_token, active_claim_until,
	last_pulse_attempt_at, last_pulse_completed_at, last_pulse_summary, last_pulse_error, created_at, updated_at
)
VALUES (?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)
ON CONFLICT(room_id, route_target) DO UPDATE SET
	bot_id = excluded.bot_id,
	project_id = excluded.project_id,
	enabled = excluded.enabled,
	active_claim_token = excluded.active_claim_token,
	active_claim_until = excluded.active_claim_until,
	last_pulse_attempt_at = COALESCE(excluded.last_pulse_attempt_at, pulse_rooms.last_pulse_attempt_at),
	last_pulse_completed_at = COALESCE(excluded.last_pulse_completed_at, pulse_rooms.last_pulse_completed_at),
	last_pulse_summary = COALESCE(excluded.last_pulse_summary, pulse_rooms.last_pulse_summary),
	last_pulse_error = COALESCE(excluded.last_pulse_error, pulse_rooms.last_pulse_error),
	updated_at = excluded.updated_at,
	revision = pulse_rooms.revision + 1
`, roomID, routeTarget, routeTarget, strings.TrimSpace(room.ProjectID), room.Enabled, strings.TrimSpace(room.ActiveClaimToken), nullTime(room.ActiveClaimUntil),
		nullTime(room.LastPulseAttemptAt), nullTime(room.LastPulseCompletedAt), room.LastPulseSummary, room.LastPulseError, now, now)
	if err != nil {
		return persistence.PulseRoom{}, err
	}
	return s.GetRoom(ctx, roomID, routeTarget)
}

func (s *sqlitePulseStore) ListTasks(ctx context.Context, roomID, routeTarget string) ([]persistence.PulseTask, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, sqlitePulseTaskSelectSQL+`
WHERE room_id = ? AND route_target = ?
ORDER BY created_at ASC, id ASC`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persistence.PulseTask{}
	for rows.Next() {
		task, err := scanPulseTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (s *sqlitePulseStore) UpsertTask(ctx context.Context, task persistence.PulseTask) (persistence.PulseTask, error) {
	if err := s.Init(ctx); err != nil {
		return persistence.PulseTask{}, err
	}
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
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO pulse_tasks (
	id, room_id, route_target, bot_id, title, prompt, schedule_type, interval_seconds, specific_time,
	specific_at, enabled, last_run_at, last_result_summary, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?)
ON CONFLICT(id) DO UPDATE SET
	room_id = excluded.room_id,
	route_target = excluded.route_target,
	bot_id = excluded.bot_id,
	title = excluded.title,
	prompt = excluded.prompt,
	schedule_type = excluded.schedule_type,
	interval_seconds = excluded.interval_seconds,
	specific_time = excluded.specific_time,
	specific_at = excluded.specific_at,
	enabled = excluded.enabled,
	last_run_at = COALESCE(excluded.last_run_at, pulse_tasks.last_run_at),
	last_result_summary = COALESCE(excluded.last_result_summary, pulse_tasks.last_result_summary),
	updated_at = excluded.updated_at
`, task.ID, roomID, routeTarget, routeTarget, strings.TrimSpace(task.Title), strings.TrimSpace(task.Prompt), task.ScheduleType, task.IntervalSeconds, task.SpecificTime,
		nullTime(task.SpecificAt), task.Enabled, nullTime(task.LastRunAt), task.LastResultSummary, now, now)
	if err != nil {
		return persistence.PulseTask{}, err
	}
	return s.getTask(ctx, roomID, routeTarget, task.ID)
}

func (s *sqlitePulseStore) DeleteTask(ctx context.Context, roomID, routeTarget, taskID string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
DELETE FROM pulse_tasks
WHERE room_id = ? AND route_target = ? AND id = ?`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget), strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *sqlitePulseStore) ClaimRoom(ctx context.Context, roomID, routeTarget, token string, leaseUntil time.Time) (bool, error) {
	if err := s.Init(ctx); err != nil {
		return false, err
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE pulse_rooms
SET active_claim_token = ?,
	active_claim_until = ?,
	last_pulse_attempt_at = ?,
	updated_at = ?,
	revision = revision + 1
WHERE room_id = ?
  AND route_target = ?
  AND (active_claim_until IS NULL OR active_claim_until <= ? OR active_claim_token = ?)
`, strings.TrimSpace(token), leaseUntil.UTC(), now, now, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget), now, strings.TrimSpace(token))
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (s *sqlitePulseStore) ClearRoomClaim(ctx context.Context, roomID, routeTarget string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE pulse_rooms
SET active_claim_token = NULL,
	active_claim_until = NULL,
	updated_at = ?,
	revision = revision + 1
WHERE room_id = ? AND route_target = ?`, time.Now().UTC(), strings.TrimSpace(roomID), strings.TrimSpace(routeTarget))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *sqlitePulseStore) CompleteRoomPulse(ctx context.Context, completion persistence.RoomPulseCompletion) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)

	completedAt := completion.CompletedAt.UTC()
	result, err := tx.ExecContext(ctx, `
UPDATE pulse_rooms
SET active_claim_token = NULL,
	active_claim_until = NULL,
	last_pulse_completed_at = ?,
	last_pulse_summary = NULLIF(?, ''),
	last_pulse_error = NULLIF(?, ''),
	updated_at = ?,
	revision = revision + 1
WHERE room_id = ? AND route_target = ? AND active_claim_token = ?
`, completedAt, completion.Summary, completion.Error, time.Now().UTC(), strings.TrimSpace(completion.RoomID), strings.TrimSpace(completion.RouteTarget), strings.TrimSpace(completion.Token))
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return persistence.ErrRevisionConflict
	}
	for _, taskID := range completion.DueTaskIDs {
		if _, err := tx.ExecContext(ctx, `
UPDATE pulse_tasks
SET last_run_at = ?,
	last_result_summary = NULLIF(?, ''),
	updated_at = ?
WHERE room_id = ? AND route_target = ? AND id = ?
`, completedAt, completion.Summary, completedAt, strings.TrimSpace(completion.RoomID), strings.TrimSpace(completion.RouteTarget), strings.TrimSpace(taskID)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqlitePulseStore) getTask(ctx context.Context, roomID, routeTarget, taskID string) (persistence.PulseTask, error) {
	row := s.db.QueryRowContext(ctx, sqlitePulseTaskSelectSQL+`
WHERE room_id = ? AND route_target = ? AND id = ?`, strings.TrimSpace(roomID), strings.TrimSpace(routeTarget), strings.TrimSpace(taskID))
	task, err := scanPulseTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	if err != nil {
		return persistence.PulseTask{}, err
	}
	return task, nil
}

const sqlitePulseRoomSelectSQL = `
SELECT room_id, route_target, project_id, enabled, revision, active_claim_token, active_claim_until,
	last_pulse_attempt_at, last_pulse_completed_at, last_pulse_summary, last_pulse_error, created_at, updated_at
FROM pulse_rooms
`

const sqlitePulseTaskSelectSQL = `
SELECT id, room_id, route_target, title, prompt, schedule_type, interval_seconds, specific_time, specific_at,
	enabled, last_run_at, last_result_summary, created_at, updated_at
FROM pulse_tasks
`
