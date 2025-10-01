package databases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
`)
	return err
}

func (s *pgChatStore) EnsureSession(ctx context.Context, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(id) == "" {
		return persistence.ChatSession{}, errors.New("id required")
	}
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	row := s.pool.QueryRow(ctx, `
WITH ins AS (
  INSERT INTO chat_sessions (id, name)
  VALUES ($1, $2)
  ON CONFLICT (id) DO NOTHING
  RETURNING id, name, created_at, updated_at, last_message_preview, model, summary, summarized_count
)
SELECT id, name, created_at, updated_at, last_message_preview, model, summary, summarized_count FROM ins
UNION ALL
SELECT id, name, created_at, updated_at, last_message_preview, model, summary, summarized_count FROM chat_sessions WHERE id = $1
LIMIT 1`, id, name)
	var cs persistence.ChatSession
	if err := row.Scan(&cs.ID, &cs.Name, &cs.CreatedAt, &cs.UpdatedAt, &cs.LastMessagePreview, &cs.Model, &cs.Summary, &cs.SummarizedCount); err != nil {
		return persistence.ChatSession{}, err
	}
	return cs, nil
}

func (s *pgChatStore) ListSessions(ctx context.Context) ([]persistence.ChatSession, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, name, created_at, updated_at, last_message_preview, model, summary, summarized_count
FROM chat_sessions
ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []persistence.ChatSession
	for rows.Next() {
		var cs persistence.ChatSession
		if err := rows.Scan(&cs.ID, &cs.Name, &cs.CreatedAt, &cs.UpdatedAt, &cs.LastMessagePreview, &cs.Model, &cs.Summary, &cs.SummarizedCount); err != nil {
			return nil, err
		}
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (s *pgChatStore) GetSession(ctx context.Context, id string) (persistence.ChatSession, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, name, created_at, updated_at, last_message_preview, model, summary, summarized_count
FROM chat_sessions
WHERE id = $1`, id)
	var cs persistence.ChatSession
	if err := row.Scan(&cs.ID, &cs.Name, &cs.CreatedAt, &cs.UpdatedAt, &cs.LastMessagePreview, &cs.Model, &cs.Summary, &cs.SummarizedCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persistence.ChatSession{}, false, nil
		}
		return persistence.ChatSession{}, false, err
	}
	return cs, true, nil
}

func (s *pgChatStore) CreateSession(ctx context.Context, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	id := uuid.New()
	row := s.pool.QueryRow(ctx, `
INSERT INTO chat_sessions (id, name)
VALUES ($1, $2)
RETURNING id, name, created_at, updated_at, last_message_preview, model, summary, summarized_count`, id, name)
	var cs persistence.ChatSession
	if err := row.Scan(&cs.ID, &cs.Name, &cs.CreatedAt, &cs.UpdatedAt, &cs.LastMessagePreview, &cs.Model, &cs.Summary, &cs.SummarizedCount); err != nil {
		return persistence.ChatSession{}, err
	}
	return cs, nil
}

func (s *pgChatStore) RenameSession(ctx context.Context, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		return persistence.ChatSession{}, errors.New("name required")
	}
	row := s.pool.QueryRow(ctx, `
UPDATE chat_sessions
SET name = $2, updated_at = NOW()
WHERE id = $1
RETURNING id, name, created_at, updated_at, last_message_preview, model`, id, name)
	var cs persistence.ChatSession
	if err := row.Scan(&cs.ID, &cs.Name, &cs.CreatedAt, &cs.UpdatedAt, &cs.LastMessagePreview, &cs.Model, &cs.Summary, &cs.SummarizedCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persistence.ChatSession{}, errors.New("session not found")
		}
		return persistence.ChatSession{}, err
	}
	return cs, nil
}

func (s *pgChatStore) DeleteSession(ctx context.Context, id string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM chat_sessions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("session not found")
	}
	return nil
}

func (s *pgChatStore) ListMessages(ctx context.Context, sessionID string, limit int) ([]persistence.ChatMessage, error) {
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
	return out, rows.Err()
}

func (s *pgChatStore) AppendMessages(ctx context.Context, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	if len(messages) == 0 {
		return nil
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

	modelUpdate := model
	if strings.TrimSpace(modelUpdate) == "" {
		modelUpdate = ""
	}
	if _, err := tx.Exec(ctx, `
UPDATE chat_sessions
SET updated_at = NOW(), last_message_preview = $2, model = CASE WHEN $3 = '' THEN model ELSE $3 END
WHERE id = $1`, sessionID, preview, modelUpdate); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *pgChatStore) UpdateSummary(ctx context.Context, sessionID string, summary string, summarizedCount int) error {
	if _, err := s.pool.Exec(ctx, `
UPDATE chat_sessions
SET summary = $2, summarized_count = $3, updated_at = NOW()
WHERE id = $1`, sessionID, summary, summarizedCount); err != nil {
		return err
	}
	return nil
}
