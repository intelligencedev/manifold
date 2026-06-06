package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"manifold/internal/persistence"
)

func NewSQLiteSpecialistActivityStore(db *sql.DB) persistence.SpecialistActivityStore {
	return &sqliteSpecialistActivityStore{db: db}
}

type sqliteSpecialistActivityStore struct {
	db *sql.DB
}

func (s *sqliteSpecialistActivityStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite specialist activity store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS chat_agent_activities (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
	user_id INTEGER,
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
	entries TEXT NOT NULL DEFAULT '[]',
	thought_summaries TEXT NOT NULL DEFAULT '[]',
	error TEXT NOT NULL DEFAULT '',
	started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	finished_at DATETIME
);
CREATE INDEX IF NOT EXISTS chat_agent_activities_session_updated_idx ON chat_agent_activities(session_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS chat_agent_activities_run_idx ON chat_agent_activities(session_id, run_id);
`)
	return err
}

func (s *sqliteSpecialistActivityStore) ListSessionActivities(ctx context.Context, userID *int64, sessionID string) ([]persistence.SpecialistActivityRecord, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	query := `
SELECT id, session_id, user_id, run_id, assistant_message_id, call_id, parent_call_id, agent, team, model, prompt, depth, status, content, entries, thought_summaries, error, started_at, updated_at, finished_at
FROM chat_agent_activities
WHERE session_id = ?`
	args := []any{sessionID}
	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}
	query += ` ORDER BY updated_at ASC, started_at ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persistence.SpecialistActivityRecord{}
	for rows.Next() {
		record, err := scanSQLiteSpecialistActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *sqliteSpecialistActivityStore) UpsertSessionActivities(ctx context.Context, userID *int64, sessionID string, activities []persistence.SpecialistActivityRecord) error {
	if len(activities) == 0 {
		return nil
	}
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	for _, activity := range activities {
		if strings.TrimSpace(activity.ID) == "" {
			continue
		}
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
		if _, err := tx.ExecContext(ctx, `
INSERT INTO chat_agent_activities (
	id, session_id, user_id, run_id, assistant_message_id, call_id, parent_call_id, agent, team, model, prompt, depth, status, content, entries, thought_summaries, error, started_at, updated_at, finished_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	user_id=excluded.user_id,
	run_id=excluded.run_id,
	assistant_message_id=excluded.assistant_message_id,
	call_id=excluded.call_id,
	parent_call_id=excluded.parent_call_id,
	agent=excluded.agent,
	team=excluded.team,
	model=excluded.model,
	prompt=excluded.prompt,
	depth=excluded.depth,
	status=excluded.status,
	content=excluded.content,
	entries=excluded.entries,
	thought_summaries=excluded.thought_summaries,
	error=excluded.error,
	started_at=excluded.started_at,
	updated_at=excluded.updated_at,
	finished_at=excluded.finished_at
`, activity.ID, sessionID, owner, activity.RunID, strings.TrimSpace(activity.AssistantMessageID), activity.CallID, strings.TrimSpace(activity.ParentCallID), activity.Agent, strings.TrimSpace(activity.Team), activity.Model, activity.Prompt, activity.Depth, activity.Status, activity.Content, string(entriesJSON), string(thoughtJSON), activity.Error, activity.StartedAt, activity.UpdatedAt, activity.FinishedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteSpecialistActivityStore) DeleteSessionActivities(ctx context.Context, userID *int64, sessionID string) error {
	return s.deleteActivities(ctx, `session_id = ?`, []any{sessionID}, userID)
}

func (s *sqliteSpecialistActivityStore) DeleteRunActivities(ctx context.Context, userID *int64, sessionID string, runID string) error {
	return s.deleteActivities(ctx, `session_id = ? AND run_id = ?`, []any{sessionID, runID}, userID)
}

func (s *sqliteSpecialistActivityStore) deleteActivities(ctx context.Context, where string, args []any, userID *int64) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	query := `DELETE FROM chat_agent_activities WHERE ` + where
	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

type sqliteScanner interface {
	Scan(dest ...any) error
}

func scanSQLiteSpecialistActivity(row sqliteScanner) (persistence.SpecialistActivityRecord, error) {
	var record persistence.SpecialistActivityRecord
	var owner sql.NullInt64
	var entriesJSON, thoughtsJSON []byte
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
