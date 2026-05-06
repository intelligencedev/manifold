package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/codeqa"
)

const postgresSchema = `
CREATE TABLE IF NOT EXISTS codeqa_runs (
	id           TEXT PRIMARY KEY,
	user_id      BIGINT NOT NULL,
	status       TEXT NOT NULL,
	mode         TEXT NOT NULL,
	repository   TEXT NOT NULL,
	error        TEXT NOT NULL DEFAULT '',
	started_at   TIMESTAMPTZ NOT NULL,
	completed_at TIMESTAMPTZ,
	result       JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS codeqa_runs_user_started_idx
	ON codeqa_runs (user_id, started_at DESC);

CREATE TABLE IF NOT EXISTS codeqa_run_events (
	run_id       TEXT NOT NULL REFERENCES codeqa_runs(id) ON DELETE CASCADE,
	user_id      BIGINT NOT NULL,
	sequence     BIGINT NOT NULL,
	type         TEXT NOT NULL,
	payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
	occurred_at  TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (run_id, sequence)
);

CREATE INDEX IF NOT EXISTS codeqa_run_events_user_run_idx
	ON codeqa_run_events (user_id, run_id, sequence);
`

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres codeqa store requires pool")
	}
	_, err := s.pool.Exec(ctx, postgresSchema)
	if err != nil {
		return fmt.Errorf("init codeqa schema: %w", err)
	}
	return nil
}

func (s *PostgresStore) Save(ctx context.Context, userID int64, run codeqa.RunResult) error {
	if s.pool == nil {
		return errors.New("postgres codeqa store requires pool")
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal run: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO codeqa_runs (id, user_id, status, mode, repository, error, started_at, completed_at, result)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, TIMESTAMPTZ '0001-01-01 00:00:00+00'), $9::jsonb)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			mode = EXCLUDED.mode,
			repository = EXCLUDED.repository,
			error = EXCLUDED.error,
			started_at = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at,
			result = EXCLUDED.result,
			user_id = EXCLUDED.user_id
	`, run.RunID, userID, string(run.Status), string(run.Mode), run.Repository, run.Error, run.StartedAt, run.CompletedAt, payload)
	if err != nil {
		return fmt.Errorf("save run: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, userID int64, runID string) (codeqa.RunResult, bool, error) {
	if s.pool == nil {
		return codeqa.RunResult{}, false, errors.New("postgres codeqa store requires pool")
	}
	var payload []byte
	err := s.pool.QueryRow(ctx, `SELECT result FROM codeqa_runs WHERE id = $1 AND user_id = $2`, runID, userID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
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

func (s *PostgresStore) List(ctx context.Context, userID int64, limit int) ([]codeqa.RunResult, error) {
	if s.pool == nil {
		return nil, errors.New("postgres codeqa store requires pool")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT result FROM codeqa_runs WHERE user_id = $1 ORDER BY started_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
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

func (s *PostgresStore) AppendEvent(ctx context.Context, userID int64, event codeqa.RunEvent) (codeqa.RunEvent, error) {
	if s.pool == nil {
		return codeqa.RunEvent{}, errors.New("postgres codeqa store requires pool")
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("marshal event payload: %w", err)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM codeqa_run_events WHERE run_id = $1 AND user_id = $2`, event.RunID, userID).Scan(&event.Sequence); err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("next event sequence: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO codeqa_run_events (run_id, user_id, sequence, type, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
	`, event.RunID, userID, event.Sequence, string(event.Type), payload, event.OccurredAt)
	if err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("insert event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return codeqa.RunEvent{}, fmt.Errorf("commit event: %w", err)
	}
	return event, nil
}

func (s *PostgresStore) ListEvents(ctx context.Context, userID int64, runID string) ([]codeqa.RunEvent, error) {
	if s.pool == nil {
		return nil, errors.New("postgres codeqa store requires pool")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT sequence, type, payload, occurred_at
		FROM codeqa_run_events
		WHERE run_id = $1 AND user_id = $2
		ORDER BY sequence ASC
	`, runID, userID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
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
