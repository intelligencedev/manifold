package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"manifold/internal/codeqa"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS codeqa_runs (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	status TEXT NOT NULL,
	mode TEXT NOT NULL,
	repository TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	started_at DATETIME NOT NULL,
	completed_at DATETIME,
	result TEXT NOT NULL CHECK (json_valid(result))
);
CREATE INDEX IF NOT EXISTS codeqa_runs_user_started_idx
	ON codeqa_runs(user_id, started_at DESC);

CREATE TABLE IF NOT EXISTS codeqa_run_events (
	run_id TEXT NOT NULL REFERENCES codeqa_runs(id) ON DELETE CASCADE,
	user_id INTEGER NOT NULL,
	sequence INTEGER NOT NULL,
	type TEXT NOT NULL,
	payload TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
	occurred_at DATETIME NOT NULL,
	PRIMARY KEY(run_id, sequence)
);
CREATE INDEX IF NOT EXISTS codeqa_run_events_user_run_idx
	ON codeqa_run_events(user_id, run_id, sequence);
`

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite codeqa store requires db")
	}
	if _, err := s.db.ExecContext(ctx, sqliteSchema); err != nil {
		return fmt.Errorf("init codeqa sqlite schema: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Save(ctx context.Context, userID int64, run codeqa.RunResult) error {
	if s.db == nil {
		return errors.New("sqlite codeqa store requires db")
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal run: %w", err)
	}
	var completedAt any
	if !run.CompletedAt.IsZero() {
		completedAt = run.CompletedAt.UTC()
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO codeqa_runs(id, user_id, status, mode, repository, error, started_at, completed_at, result)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	user_id = excluded.user_id,
	status = excluded.status,
	mode = excluded.mode,
	repository = excluded.repository,
	error = excluded.error,
	started_at = excluded.started_at,
	completed_at = excluded.completed_at,
	result = excluded.result
`, run.RunID, userID, string(run.Status), string(run.Mode), run.Repository, run.Error, run.StartedAt.UTC(), completedAt, string(payload))
	if err != nil {
		return fmt.Errorf("save run: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Get(ctx context.Context, userID int64, runID string) (codeqa.RunResult, bool, error) {
	if s.db == nil {
		return codeqa.RunResult{}, false, errors.New("sqlite codeqa store requires db")
	}
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT result FROM codeqa_runs WHERE id = ? AND user_id = ?`, runID, userID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return codeqa.RunResult{}, false, nil
	}
	if err != nil {
		return codeqa.RunResult{}, false, fmt.Errorf("get run: %w", err)
	}
	var run codeqa.RunResult
	if err := json.Unmarshal(payload, &run); err != nil {
		return codeqa.RunResult{}, false, fmt.Errorf("decode run: %w", err)
	}
	return run, true, nil
}

func (s *SQLiteStore) List(ctx context.Context, userID int64, limit int) ([]codeqa.RunResult, error) {
	if s.db == nil {
		return nil, errors.New("sqlite codeqa store requires db")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT result FROM codeqa_runs WHERE user_id = ? ORDER BY started_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]codeqa.RunResult, 0, limit)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		var run codeqa.RunResult
		if err := json.Unmarshal(payload, &run); err != nil {
			return nil, fmt.Errorf("decode run: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}
	sort.Slice(out, func(i int, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

func (s *SQLiteStore) AppendEvent(ctx context.Context, userID int64, event codeqa.RunEvent) (codeqa.RunEvent, error) {
	if s.db == nil {
		return codeqa.RunEvent{}, errors.New("sqlite codeqa store requires db")
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("marshal event payload: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM codeqa_run_events WHERE run_id = ? AND user_id = ?`, event.RunID, userID).Scan(&event.Sequence); err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("next event sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO codeqa_run_events(run_id, user_id, sequence, type, payload, occurred_at)
VALUES(?, ?, ?, ?, ?, ?)
`, event.RunID, userID, event.Sequence, string(event.Type), string(payload), event.OccurredAt.UTC()); err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("insert event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("commit event: %w", err)
	}
	return event, nil
}

func (s *SQLiteStore) ListEvents(ctx context.Context, userID int64, runID string) ([]codeqa.RunEvent, error) {
	if s.db == nil {
		return nil, errors.New("sqlite codeqa store requires db")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT sequence, type, payload, occurred_at
FROM codeqa_run_events
WHERE run_id = ? AND user_id = ?
ORDER BY sequence ASC`, runID, userID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]codeqa.RunEvent, 0, 32)
	for rows.Next() {
		var (
			event   codeqa.RunEvent
			payload []byte
			typ     string
		)
		if err := rows.Scan(&event.Sequence, &typ, &payload, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.RunID = runID
		event.Type = codeqa.RunEventType(typ)
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &event.Payload); err != nil {
				return nil, fmt.Errorf("decode event payload: %w", err)
			}
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return out, nil
}
