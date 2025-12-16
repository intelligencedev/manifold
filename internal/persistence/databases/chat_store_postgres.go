package databases

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/observability"
	"manifold/internal/persistence"
)

// NewPostgresChatStore returns a Postgres-backed chat history store.
func NewPostgresChatStore(pool *pgxpool.Pool) persistence.ChatStore {
	return &pgChatStore{pool: pool}
}

type pgChatStore struct {
	pool *pgxpool.Pool
}

func (s *pgChatStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *pgChatStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres chat store requires pool")
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS chat_sessions (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    user_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_message_preview TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    summarized_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS chat_messages (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS chat_messages_session_created_idx ON chat_messages(session_id, created_at);

ALTER TABLE chat_sessions
    ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';

ALTER TABLE chat_sessions
    ADD COLUMN IF NOT EXISTS summarized_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE chat_sessions
    ADD COLUMN IF NOT EXISTS user_id BIGINT;

CREATE INDEX IF NOT EXISTS chat_sessions_user_updated_idx ON chat_sessions(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS chat_sessions_user_created_idx ON chat_sessions(user_id, created_at DESC);
`)
	return err
}

func hasAccess(userID *int64, owner *int64) bool {
	if userID == nil {
		return true
	}
	if owner == nil {
		return false
	}
	return *userID == *owner
}

func (s *pgChatStore) scanSession(row pgx.Row) (persistence.ChatSession, error) {
	var cs persistence.ChatSession
	var owner sql.NullInt64
	if err := row.Scan(&cs.ID, &cs.Name, &owner, &cs.CreatedAt, &cs.UpdatedAt, &cs.LastMessagePreview, &cs.Model, &cs.Summary, &cs.SummarizedCount); err != nil {
		return persistence.ChatSession{}, err
	}
	if owner.Valid {
		v := owner.Int64
		cs.UserID = &v
	}
	return cs, nil
}

func (s *pgChatStore) lookupSessionOwner(ctx context.Context, id string) (*int64, error) {
	row := s.pool.QueryRow(ctx, `SELECT user_id FROM chat_sessions WHERE id = $1`, id)
	var owner sql.NullInt64
	if err := row.Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	if !owner.Valid {
		return nil, nil
	}
	v := owner.Int64
	return &v, nil
}

func (s *pgChatStore) EnsureSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(id) == "" {
		return persistence.ChatSession{}, errors.New("id required")
	}
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	var uid any
	if userID != nil {
		uid = *userID
	}
	row := s.pool.QueryRow(ctx, `
WITH ins AS (
  INSERT INTO chat_sessions (id, user_id, name)
  VALUES ($1, $2, $3)
  ON CONFLICT (id) DO NOTHING
  RETURNING id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count
)
SELECT id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count FROM ins
UNION ALL
SELECT id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count FROM chat_sessions WHERE id = $1
LIMIT 1`, id, uid, name)
	cs, err := s.scanSession(row)
	if err != nil {
		return persistence.ChatSession{}, err
	}
	if !hasAccess(userID, cs.UserID) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	return cs, nil
}

func (s *pgChatStore) ListSessions(ctx context.Context, userID *int64) ([]persistence.ChatSession, error) {
	query := `
SELECT id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count
FROM chat_sessions`
	args := []any{}
	if userID != nil {
		query += `
WHERE user_id = $1`
		args = append(args, *userID)
	}
	query += `
ORDER BY updated_at DESC, created_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []persistence.ChatSession
	for rows.Next() {
		cs, err := s.scanSession(rows)
		if err != nil {
			return nil, err
		}
		if !hasAccess(userID, cs.UserID) {
			continue
		}
		out = append(out, cs)
	}
	if out == nil {
		out = make([]persistence.ChatSession, 0)
	}
	return out, rows.Err()
}

func (s *pgChatStore) GetSession(ctx context.Context, userID *int64, id string) (persistence.ChatSession, error) {
	log := observability.LoggerWithTrace(ctx)
	query := `
SELECT id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count
FROM chat_sessions
WHERE id = $1`
	args := []any{id}
	if userID != nil {
		query += ` AND user_id = $2`
		args = append(args, *userID)
		log.Debug().Int64("user_id", *userID).Str("session_id", id).Msg("get_session_with_userid")
	} else {
		log.Debug().Str("session_id", id).Msg("get_session_no_userid")
	}
	row := s.pool.QueryRow(ctx, query, args...)
	cs, err := s.scanSession(row)
	if err == nil {
		log.Debug().Str("session_id", id).Msg("get_session_found")
		return cs, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		log.Error().Err(err).Str("session_id", id).Msg("get_session_error")
		return persistence.ChatSession{}, err
	}
	log.Warn().Str("session_id", id).Msg("get_session_no_rows")
	if userID == nil {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	owner, ownerErr := s.lookupSessionOwner(ctx, id)
	if ownerErr != nil {
		return persistence.ChatSession{}, ownerErr
	}
	if !hasAccess(userID, owner) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	return persistence.ChatSession{}, persistence.ErrNotFound
}

func (s *pgChatStore) CreateSession(ctx context.Context, userID *int64, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	id := uuid.New()
	var uid any
	if userID != nil {
		uid = *userID
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO chat_sessions (id, user_id, name)
VALUES ($1, $2, $3)
RETURNING id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count`, id, uid, name)
	return s.scanSession(row)
}

func (s *pgChatStore) RenameSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		return persistence.ChatSession{}, errors.New("name required")
	}
	query := `
UPDATE chat_sessions
SET name = $2, updated_at = NOW()
WHERE id = $1`
	args := []any{id, name}
	if userID != nil {
		query += ` AND user_id = $3`
		args = append(args, *userID)
	}
	query += `
RETURNING id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count`
	row := s.pool.QueryRow(ctx, query, args...)
	cs, err := s.scanSession(row)
	if err == nil {
		return cs, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return persistence.ChatSession{}, err
	}
	if userID == nil {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	owner, ownerErr := s.lookupSessionOwner(ctx, id)
	if ownerErr != nil {
		return persistence.ChatSession{}, ownerErr
	}
	if !hasAccess(userID, owner) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	return persistence.ChatSession{}, persistence.ErrNotFound
}

func (s *pgChatStore) DeleteSession(ctx context.Context, userID *int64, id string) error {
	query := `DELETE FROM chat_sessions WHERE id = $1`
	args := []any{id}
	if userID != nil {
		query += ` AND user_id = $2`
		args = append(args, *userID)
	}
	cmd, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() > 0 {
		return nil
	}
	if userID == nil {
		return persistence.ErrNotFound
	}
	owner, ownerErr := s.lookupSessionOwner(ctx, id)
	if ownerErr != nil {
		return ownerErr
	}
	if !hasAccess(userID, owner) {
		return persistence.ErrForbidden
	}
	return persistence.ErrNotFound
}

func (s *pgChatStore) ListMessages(ctx context.Context, userID *int64, sessionID string, limit int) ([]persistence.ChatMessage, error) {
	log := observability.LoggerWithTrace(ctx)
	log.Debug().Str("session_id", sessionID).Int("limit", limit).Msg("list_messages_start")
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("list_messages_get_session_failed")
		return nil, err
	}
	log.Debug().Str("session_id", sessionID).Msg("list_messages_session_ok")
	query := `
SELECT id, session_id, role, content, created_at
FROM chat_messages
WHERE session_id = $1
ORDER BY created_at ASC, id ASC`
	args := []any{sessionID}
	if limit > 0 {
		query = `
SELECT id, session_id, role, content, created_at FROM (
    SELECT id, session_id, role, content, created_at
    FROM chat_messages
    WHERE session_id = $1
    ORDER BY created_at DESC, id DESC
    LIMIT $2
) sub
ORDER BY created_at ASC, id ASC`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []persistence.ChatMessage
	for rows.Next() {
		var msg persistence.ChatMessage
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if out == nil {
		out = make([]persistence.ChatMessage, 0)
	}
	log.Debug().Str("session_id", sessionID).Int("message_count", len(out)).Msg("list_messages_complete")
	return out, rows.Err()
}

func (s *pgChatStore) AppendMessages(ctx context.Context, userID *int64, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	if len(messages) == 0 {
		return nil
	}
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, message := range messages {
		id := message.ID
		if id == "" {
			id = uuid.NewString()
		}
		createdAt := message.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO chat_messages (id, session_id, role, content, created_at)
VALUES ($1, $2, $3, $4, $5)`, id, sessionID, message.Role, message.Content, createdAt); err != nil {
			return err
		}
	}

	modelUpdate := strings.TrimSpace(model)
	query := `
UPDATE chat_sessions
SET updated_at = NOW(),
    last_message_preview = $2,
    model = CASE WHEN $3 = '' THEN model ELSE $3 END
WHERE id = $1`
	args := []any{sessionID, preview, modelUpdate}
	if userID != nil {
		query += ` AND user_id = $4`
		args = append(args, *userID)
	}
	cmd, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrForbidden
	}

	return tx.Commit(ctx)
}

func (s *pgChatStore) UpdateSummary(ctx context.Context, userID *int64, sessionID string, summary string, summarizedCount int) error {
	query := `
UPDATE chat_sessions
SET summary = $2, summarized_count = $3, updated_at = NOW()
WHERE id = $1`
	args := []any{sessionID, summary, summarizedCount}
	if userID != nil {
		query += ` AND user_id = $4`
		args = append(args, *userID)
	}
	cmd, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() > 0 {
		return nil
	}
	if userID == nil {
		return persistence.ErrNotFound
	}
	owner, ownerErr := s.lookupSessionOwner(ctx, sessionID)
	if ownerErr != nil {
		return ownerErr
	}
	if !hasAccess(userID, owner) {
		return persistence.ErrForbidden
	}
	return persistence.ErrNotFound
}
