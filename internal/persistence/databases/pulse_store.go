package databases

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"manifold/internal/persistence"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPulseStore returns a Postgres-backed pulse store when a pool is provided,
// otherwise an in-memory implementation.
func NewPulseStore(pool *pgxpool.Pool) persistence.PulseStore {
	if pool == nil {
		return &memPulseStore{
			rooms: map[string]persistence.PulseRoom{},
			tasks: map[string]map[string]persistence.PulseTask{},
		}
	}
	return &pgPulseStore{pool: pool}
}

type memPulseStore struct {
	mu    sync.RWMutex
	rooms map[string]persistence.PulseRoom
	tasks map[string]map[string]persistence.PulseTask
}

func (s *memPulseStore) Init(ctx context.Context) error { return nil }

func (s *memPulseStore) EnsureRoom(ctx context.Context, roomID string) (persistence.PulseRoom, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if room, ok := s.rooms[roomID]; ok {
		return clonePulseRoom(room), nil
	}
	now := time.Now().UTC()
	room := persistence.PulseRoom{
		RoomID:    roomID,
		Enabled:   true,
		Revision:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.rooms[roomID] = room
	return clonePulseRoom(room), nil
}

func (s *memPulseStore) GetRoom(ctx context.Context, roomID string) (persistence.PulseRoom, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, ok := s.rooms[strings.TrimSpace(roomID)]
	if !ok {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	return clonePulseRoom(room), nil
}

func (s *memPulseStore) ListRooms(ctx context.Context) ([]persistence.PulseRoom, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]persistence.PulseRoom, 0, len(s.rooms))
	for _, room := range s.rooms {
		out = append(out, clonePulseRoom(room))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RoomID < out[j].RoomID
	})
	return out, nil
}

func (s *memPulseStore) UpsertRoom(ctx context.Context, room persistence.PulseRoom) (persistence.PulseRoom, error) {
	roomID := strings.TrimSpace(room.RoomID)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	existing, ok := s.rooms[roomID]
	if ok {
		room.CreatedAt = existing.CreatedAt
		room.Revision = existing.Revision + 1
		if room.ActiveClaimToken == "" {
			room.ActiveClaimToken = existing.ActiveClaimToken
			room.ActiveClaimUntil = existing.ActiveClaimUntil
		}
		if room.LastPulseAttemptAt.IsZero() {
			room.LastPulseAttemptAt = existing.LastPulseAttemptAt
		}
		if room.LastPulseCompletedAt.IsZero() {
			room.LastPulseCompletedAt = existing.LastPulseCompletedAt
		}
		if room.LastPulseSummary == "" {
			room.LastPulseSummary = existing.LastPulseSummary
		}
		if room.LastPulseError == "" {
			room.LastPulseError = existing.LastPulseError
		}
	} else {
		room.CreatedAt = now
		room.Revision = 1
	}
	room.RoomID = roomID
	room.UpdatedAt = now
	s.rooms[roomID] = room
	return clonePulseRoom(room), nil
}

func (s *memPulseStore) ListTasks(ctx context.Context, roomID string) ([]persistence.PulseTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roomID = strings.TrimSpace(roomID)
	roomTasks := s.tasks[roomID]
	out := make([]persistence.PulseTask, 0, len(roomTasks))
	for _, task := range roomTasks {
		out = append(out, clonePulseTask(task))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *memPulseStore) UpsertTask(ctx context.Context, task persistence.PulseTask) (persistence.PulseTask, error) {
	roomID := strings.TrimSpace(task.RoomID)
	if roomID == "" {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	if _, err := s.EnsureRoom(ctx, roomID); err != nil {
		return persistence.PulseTask{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[roomID] == nil {
		s.tasks[roomID] = map[string]persistence.PulseTask{}
	}
	now := time.Now().UTC()
	if strings.TrimSpace(task.ID) == "" {
		task.ID = uuid.NewString()
	}
	task.RoomID = roomID
	existing, ok := s.tasks[roomID][task.ID]
	if ok {
		task.CreatedAt = existing.CreatedAt
		if task.LastRunAt.IsZero() {
			task.LastRunAt = existing.LastRunAt
		}
		if task.LastResultSummary == "" {
			task.LastResultSummary = existing.LastResultSummary
		}
	} else {
		task.CreatedAt = now
	}
	if task.IntervalSeconds <= 0 {
		task.IntervalSeconds = 300
	}
	task.UpdatedAt = now
	s.tasks[roomID][task.ID] = task
	return clonePulseTask(task), nil
}

func (s *memPulseStore) DeleteTask(ctx context.Context, roomID, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	roomTasks := s.tasks[strings.TrimSpace(roomID)]
	if roomTasks == nil {
		return persistence.ErrNotFound
	}
	if _, ok := roomTasks[strings.TrimSpace(taskID)]; !ok {
		return persistence.ErrNotFound
	}
	delete(roomTasks, strings.TrimSpace(taskID))
	return nil
}

func (s *memPulseStore) ClaimRoom(ctx context.Context, roomID, token string, leaseUntil time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[strings.TrimSpace(roomID)]
	if !ok {
		return false, persistence.ErrNotFound
	}
	now := time.Now().UTC()
	if room.ActiveClaimToken != "" && room.ActiveClaimToken != token && room.ActiveClaimUntil.After(now) {
		return false, nil
	}
	room.ActiveClaimToken = token
	room.ActiveClaimUntil = leaseUntil.UTC()
	room.LastPulseAttemptAt = now
	room.UpdatedAt = now
	room.Revision++
	s.rooms[room.RoomID] = room
	return true, nil
}

func (s *memPulseStore) ClearRoomClaim(ctx context.Context, roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[strings.TrimSpace(roomID)]
	if !ok {
		return persistence.ErrNotFound
	}
	now := time.Now().UTC()
	room.ActiveClaimToken = ""
	room.ActiveClaimUntil = time.Time{}
	room.UpdatedAt = now
	room.Revision++
	s.rooms[room.RoomID] = room
	return nil
}

func (s *memPulseStore) CompleteRoomPulse(ctx context.Context, roomID, token string, completedAt time.Time, summary, pulseErr string, dueTaskIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[strings.TrimSpace(roomID)]
	if !ok {
		return persistence.ErrNotFound
	}
	if room.ActiveClaimToken != token {
		return persistence.ErrRevisionConflict
	}
	completedAt = completedAt.UTC()
	room.ActiveClaimToken = ""
	room.ActiveClaimUntil = time.Time{}
	room.LastPulseCompletedAt = completedAt
	room.LastPulseSummary = summary
	room.LastPulseError = pulseErr
	room.UpdatedAt = completedAt
	room.Revision++
	s.rooms[room.RoomID] = room
	if len(dueTaskIDs) == 0 {
		return nil
	}
	for _, taskID := range dueTaskIDs {
		task, ok := s.tasks[room.RoomID][taskID]
		if !ok {
			continue
		}
		task.LastRunAt = completedAt
		task.LastResultSummary = summary
		task.UpdatedAt = completedAt
		s.tasks[room.RoomID][taskID] = task
	}
	return nil
}

type pgPulseStore struct {
	pool *pgxpool.Pool
}

func (s *pgPulseStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS pulse_rooms (
    room_id TEXT PRIMARY KEY,
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
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pulse_rooms_enabled ON pulse_rooms(enabled);
CREATE INDEX IF NOT EXISTS idx_pulse_rooms_claim_until ON pulse_rooms(active_claim_until);
CREATE TABLE IF NOT EXISTS pulse_tasks (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL REFERENCES pulse_rooms(room_id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    prompt TEXT NOT NULL,
    interval_seconds INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at TIMESTAMPTZ,
    last_result_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pulse_tasks_room_id ON pulse_tasks(room_id);
CREATE INDEX IF NOT EXISTS idx_pulse_tasks_enabled ON pulse_tasks(room_id, enabled);
`)
	return err
}

func (s *pgPulseStore) EnsureRoom(ctx context.Context, roomID string) (persistence.PulseRoom, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO pulse_rooms (room_id, enabled)
VALUES ($1, TRUE)
ON CONFLICT (room_id) DO NOTHING
`, roomID)
	if err != nil {
		return persistence.PulseRoom{}, err
	}
	return s.GetRoom(ctx, roomID)
}

func (s *pgPulseStore) GetRoom(ctx context.Context, roomID string) (persistence.PulseRoom, error) {
	var room persistence.PulseRoom
	var projectID, claimToken, summary, pulseErr *string
	var claimUntil, attemptAt, completedAt *time.Time
	err := s.pool.QueryRow(ctx, `
SELECT room_id, project_id, enabled, revision, active_claim_token, active_claim_until,
       last_pulse_attempt_at, last_pulse_completed_at, last_pulse_summary, last_pulse_error,
       created_at, updated_at
FROM pulse_rooms
WHERE room_id = $1
`, strings.TrimSpace(roomID)).Scan(
		&room.RoomID,
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

func (s *pgPulseStore) ListRooms(ctx context.Context) ([]persistence.PulseRoom, error) {
	rows, err := s.pool.Query(ctx, `
SELECT room_id, project_id, enabled, revision, active_claim_token, active_claim_until,
       last_pulse_attempt_at, last_pulse_completed_at, last_pulse_summary, last_pulse_error,
       created_at, updated_at
FROM pulse_rooms
ORDER BY room_id ASC
`)
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
	if roomID == "" {
		return persistence.PulseRoom{}, persistence.ErrNotFound
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO pulse_rooms (
    room_id, project_id, enabled, active_claim_token, active_claim_until,
    last_pulse_attempt_at, last_pulse_completed_at, last_pulse_summary, last_pulse_error,
    created_at, updated_at
)
VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, ''), $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), NOW(), NOW())
ON CONFLICT (room_id) DO UPDATE SET
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
`, roomID, strings.TrimSpace(room.ProjectID), room.Enabled, strings.TrimSpace(room.ActiveClaimToken), nullTime(room.ActiveClaimUntil), nullTime(room.LastPulseAttemptAt), nullTime(room.LastPulseCompletedAt), emptyToNil(room.LastPulseSummary), emptyToNil(room.LastPulseError))
	if err != nil {
		return persistence.PulseRoom{}, err
	}
	return s.GetRoom(ctx, roomID)
}

func (s *pgPulseStore) ListTasks(ctx context.Context, roomID string) ([]persistence.PulseTask, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, room_id, title, prompt, interval_seconds, enabled, last_run_at, last_result_summary, created_at, updated_at
FROM pulse_tasks
WHERE room_id = $1
ORDER BY created_at ASC, id ASC
`, strings.TrimSpace(roomID))
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
	if roomID == "" {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	if _, err := s.EnsureRoom(ctx, roomID); err != nil {
		return persistence.PulseTask{}, err
	}
	if strings.TrimSpace(task.ID) == "" {
		task.ID = uuid.NewString()
	}
	if task.IntervalSeconds <= 0 {
		task.IntervalSeconds = 300
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO pulse_tasks (
    id, room_id, title, prompt, interval_seconds, enabled, last_run_at, last_result_summary, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET
    room_id = EXCLUDED.room_id,
    title = EXCLUDED.title,
    prompt = EXCLUDED.prompt,
    interval_seconds = EXCLUDED.interval_seconds,
    enabled = EXCLUDED.enabled,
    last_run_at = COALESCE(EXCLUDED.last_run_at, pulse_tasks.last_run_at),
    last_result_summary = COALESCE(EXCLUDED.last_result_summary, pulse_tasks.last_result_summary),
    updated_at = NOW()
`, task.ID, roomID, strings.TrimSpace(task.Title), strings.TrimSpace(task.Prompt), task.IntervalSeconds, task.Enabled, nullTime(task.LastRunAt), emptyToNil(task.LastResultSummary))
	if err != nil {
		return persistence.PulseTask{}, err
	}
	return s.getTask(ctx, roomID, task.ID)
}

func (s *pgPulseStore) DeleteTask(ctx context.Context, roomID, taskID string) error {
	cmd, err := s.pool.Exec(ctx, `
DELETE FROM pulse_tasks
WHERE room_id = $1 AND id = $2
`, strings.TrimSpace(roomID), strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *pgPulseStore) ClaimRoom(ctx context.Context, roomID, token string, leaseUntil time.Time) (bool, error) {
	cmd, err := s.pool.Exec(ctx, `
UPDATE pulse_rooms
SET active_claim_token = $2,
    active_claim_until = $3,
    last_pulse_attempt_at = NOW(),
    updated_at = NOW(),
    revision = revision + 1
WHERE room_id = $1
  AND (active_claim_until IS NULL OR active_claim_until <= NOW() OR active_claim_token = $2)
`, strings.TrimSpace(roomID), strings.TrimSpace(token), leaseUntil.UTC())
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *pgPulseStore) ClearRoomClaim(ctx context.Context, roomID string) error {
	cmd, err := s.pool.Exec(ctx, `
UPDATE pulse_rooms
SET active_claim_token = NULL,
    active_claim_until = NULL,
    updated_at = NOW(),
    revision = revision + 1
WHERE room_id = $1
`, strings.TrimSpace(roomID))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrNotFound
	}
	return nil
}

func (s *pgPulseStore) CompleteRoomPulse(ctx context.Context, roomID, token string, completedAt time.Time, summary, pulseErr string, dueTaskIDs []string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cmd, err := tx.Exec(ctx, `
UPDATE pulse_rooms
SET active_claim_token = NULL,
    active_claim_until = NULL,
    last_pulse_completed_at = $3,
    last_pulse_summary = NULLIF($4, ''),
    last_pulse_error = NULLIF($5, ''),
    updated_at = NOW(),
    revision = revision + 1
WHERE room_id = $1 AND active_claim_token = $2
`, strings.TrimSpace(roomID), strings.TrimSpace(token), completedAt.UTC(), summary, pulseErr)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrRevisionConflict
	}
	if len(dueTaskIDs) > 0 {
		if _, err := tx.Exec(ctx, `
UPDATE pulse_tasks
SET last_run_at = $3,
    last_result_summary = NULLIF($4, ''),
    updated_at = NOW()
WHERE room_id = $1 AND id = ANY($2)
`, strings.TrimSpace(roomID), dueTaskIDs, completedAt.UTC(), summary); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *pgPulseStore) getTask(ctx context.Context, roomID, taskID string) (persistence.PulseTask, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, room_id, title, prompt, interval_seconds, enabled, last_run_at, last_result_summary, created_at, updated_at
FROM pulse_tasks
WHERE room_id = $1 AND id = $2
`, strings.TrimSpace(roomID), strings.TrimSpace(taskID))
	if err != nil {
		return persistence.PulseTask{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return persistence.PulseTask{}, persistence.ErrNotFound
	}
	return scanPulseTask(rows)
}

func scanPulseRoom(rows interface{ Scan(...any) error }) (persistence.PulseRoom, error) {
	var room persistence.PulseRoom
	var projectID, claimToken, summary, pulseErr *string
	var claimUntil, attemptAt, completedAt *time.Time
	if err := rows.Scan(
		&room.RoomID,
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
	); err != nil {
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

func scanPulseTask(rows interface{ Scan(...any) error }) (persistence.PulseTask, error) {
	var task persistence.PulseTask
	var lastRunAt *time.Time
	var lastSummary *string
	if err := rows.Scan(
		&task.ID,
		&task.RoomID,
		&task.Title,
		&task.Prompt,
		&task.IntervalSeconds,
		&task.Enabled,
		&lastRunAt,
		&lastSummary,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return persistence.PulseTask{}, err
	}
	if lastRunAt != nil {
		task.LastRunAt = lastRunAt.UTC()
	}
	if lastSummary != nil {
		task.LastResultSummary = *lastSummary
	}
	return task, nil
}

func clonePulseRoom(room persistence.PulseRoom) persistence.PulseRoom {
	return room
}

func clonePulseTask(task persistence.PulseTask) persistence.PulseTask {
	return task
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func emptyToNil(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}
