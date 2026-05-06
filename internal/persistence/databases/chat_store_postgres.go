package databases

import (
	"context"
	"errors"

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
