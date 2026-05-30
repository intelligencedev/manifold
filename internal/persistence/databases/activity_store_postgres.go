package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/persistence"
)

func NewPostgresSpecialistActivityStore(pool *pgxpool.Pool) persistence.SpecialistActivityStore {
	if pool == nil {
		return NewMemorySpecialistActivityStore()
	}
	return &pgSpecialistActivityStore{pool: pool}
}

type pgSpecialistActivityStore struct {
	pool *pgxpool.Pool
}

func (s *pgSpecialistActivityStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *pgSpecialistActivityStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres specialist activity store requires pool")
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS chat_agent_activities (
    id TEXT PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    user_id BIGINT,
    run_id TEXT NOT NULL DEFAULT '',
    assistant_message_id TEXT NOT NULL DEFAULT '',
    call_id TEXT NOT NULL,
    parent_call_id TEXT NOT NULL DEFAULT '',
    agent TEXT NOT NULL DEFAULT '',
    team TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    depth INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running',
    content TEXT NOT NULL DEFAULT '',
    entries JSONB NOT NULL DEFAULT '[]'::jsonb,
    thought_summaries JSONB NOT NULL DEFAULT '[]'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS chat_agent_activities_session_updated_idx ON chat_agent_activities(session_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS chat_agent_activities_run_idx ON chat_agent_activities(session_id, run_id);

ALTER TABLE chat_agent_activities ADD COLUMN IF NOT EXISTS team TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_agent_activities ADD COLUMN IF NOT EXISTS assistant_message_id TEXT NOT NULL DEFAULT '';
`)
	return err
}

func (s *pgSpecialistActivityStore) ListSessionActivities(ctx context.Context, userID *int64, sessionID string) ([]persistence.SpecialistActivityRecord, error) {
	query := `
SELECT id, session_id, user_id, run_id, assistant_message_id, call_id, parent_call_id, agent, team, model, prompt, depth, status, content, entries, thought_summaries, error, started_at, updated_at, finished_at
FROM chat_agent_activities
WHERE session_id = $1`
	args := []any{sessionID}
	if userID != nil {
		query += ` AND user_id = $2`
		args = append(args, *userID)
	}
	query += ` ORDER BY updated_at ASC, started_at ASC, id ASC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]persistence.SpecialistActivityRecord, 0)
	for rows.Next() {
		record, err := scanSpecialistActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *pgSpecialistActivityStore) UpsertSessionActivities(ctx context.Context, userID *int64, sessionID string, activities []persistence.SpecialistActivityRecord) error {
	if len(activities) == 0 {
		return nil
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, activity := range activities {
		entriesJSON, err := json.Marshal(activity.Entries)
		if err != nil {
			return err
		}
		thoughtJSON, err := json.Marshal(activity.ThoughtSummaries)
		if err != nil {
			return err
		}
		var owner any
		if activity.UserID != nil {
			owner = *activity.UserID
		} else if userID != nil {
			owner = *userID
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO chat_agent_activities (
    id, session_id, user_id, run_id, assistant_message_id, call_id, parent_call_id, agent, team, model, prompt, depth, status, content, entries, thought_summaries, error, started_at, updated_at, finished_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    run_id = EXCLUDED.run_id,
    assistant_message_id = EXCLUDED.assistant_message_id,
    call_id = EXCLUDED.call_id,
    parent_call_id = EXCLUDED.parent_call_id,
    agent = EXCLUDED.agent,
    team = EXCLUDED.team,
    model = EXCLUDED.model,
    prompt = EXCLUDED.prompt,
    depth = EXCLUDED.depth,
    status = EXCLUDED.status,
    content = EXCLUDED.content,
    entries = EXCLUDED.entries,
    thought_summaries = EXCLUDED.thought_summaries,
    error = EXCLUDED.error,
    started_at = EXCLUDED.started_at,
    updated_at = EXCLUDED.updated_at,
    finished_at = EXCLUDED.finished_at
`, activity.ID, sessionID, owner, activity.RunID, strings.TrimSpace(activity.AssistantMessageID), activity.CallID, strings.TrimSpace(activity.ParentCallID), activity.Agent, strings.TrimSpace(activity.Team), activity.Model, activity.Prompt, activity.Depth, activity.Status, activity.Content, entriesJSON, thoughtJSON, activity.Error, activity.StartedAt, activity.UpdatedAt, activity.FinishedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *pgSpecialistActivityStore) DeleteSessionActivities(ctx context.Context, userID *int64, sessionID string) error {
	query := `DELETE FROM chat_agent_activities WHERE session_id = $1`
	args := []any{sessionID}
	if userID != nil {
		query += ` AND user_id = $2`
		args = append(args, *userID)
	}
	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

func (s *pgSpecialistActivityStore) DeleteRunActivities(ctx context.Context, userID *int64, sessionID string, runID string) error {
	query := `DELETE FROM chat_agent_activities WHERE session_id = $1 AND run_id = $2`
	args := []any{sessionID, runID}
	if userID != nil {
		query += ` AND user_id = $3`
		args = append(args, *userID)
	}
	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

func scanSpecialistActivity(row pgx.Row) (persistence.SpecialistActivityRecord, error) {
	var record persistence.SpecialistActivityRecord
	var owner sql.NullInt64
	var entriesJSON []byte
	var thoughtsJSON []byte
	var finishedAt sql.NullTime
	if err := row.Scan(
		&record.ID,
		&record.SessionID,
		&owner,
		&record.RunID,
		&record.AssistantMessageID,
		&record.CallID,
		&record.ParentCallID,
		&record.Agent,
		&record.Team,
		&record.Model,
		&record.Prompt,
		&record.Depth,
		&record.Status,
		&record.Content,
		&entriesJSON,
		&thoughtsJSON,
		&record.Error,
		&record.StartedAt,
		&record.UpdatedAt,
		&finishedAt,
	); err != nil {
		return persistence.SpecialistActivityRecord{}, err
	}
	if owner.Valid {
		v := owner.Int64
		record.UserID = &v
	}
	if len(entriesJSON) > 0 {
		if err := json.Unmarshal(entriesJSON, &record.Entries); err != nil {
			return persistence.SpecialistActivityRecord{}, err
		}
	}
	if len(thoughtsJSON) > 0 {
		if err := json.Unmarshal(thoughtsJSON, &record.ThoughtSummaries); err != nil {
			return persistence.SpecialistActivityRecord{}, err
		}
	}
	if finishedAt.Valid {
		finished := finishedAt.Time
		record.FinishedAt = &finished
	}
	return record, nil
}
