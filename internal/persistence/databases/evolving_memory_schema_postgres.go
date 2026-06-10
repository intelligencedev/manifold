package databases

import (
	"context"
	"errors"
	"fmt"
)

func (s *pgEvolvingMemoryStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres evolving memory store requires pool")
	}
	if _, err := s.pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS evolving_memories (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL,
    session_id TEXT NOT NULL DEFAULT 'default',
    input TEXT NOT NULL,
    output TEXT NOT NULL,
    feedback TEXT NOT NULL,
    summary TEXT NOT NULL,
    raw_trace TEXT NOT NULL DEFAULT '',
    embedding BYTEA,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	structured_feedback JSONB,
	memory_type TEXT NOT NULL DEFAULT '',
	strategy_card TEXT NOT NULL DEFAULT '',
	scope TEXT NOT NULL DEFAULT 'session',
	expires_at TIMESTAMPTZ,
	access_count INTEGER NOT NULL DEFAULT 0,
	last_accessed_at TIMESTAMPTZ,
	relevance_score DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS evolving_memories_user_created_idx
    ON evolving_memories(user_id, created_at DESC);

ALTER TABLE evolving_memories
    ADD COLUMN IF NOT EXISTS session_id TEXT NOT NULL DEFAULT 'default';

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS structured_feedback JSONB;

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS memory_type TEXT NOT NULL DEFAULT '';

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS strategy_card TEXT NOT NULL DEFAULT '';

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'session';

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS access_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS last_accessed_at TIMESTAMPTZ;

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS relevance_score DOUBLE PRECISION NOT NULL DEFAULT 1.0;

CREATE INDEX IF NOT EXISTS evolving_memories_user_session_created_idx
    ON evolving_memories(user_id, session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS evolving_memories_user_scope_created_idx
	ON evolving_memories(user_id, scope, created_at DESC);

CREATE INDEX IF NOT EXISTS evolving_memories_expires_at_idx
	ON evolving_memories(expires_at)
	WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS evolving_memory_archive (
	id UUID PRIMARY KEY,
	original_id TEXT NOT NULL,
	user_id BIGINT NOT NULL,
	session_id TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS evolving_memory_archive_user_session_idx
	ON evolving_memory_archive(user_id, session_id, archived_at DESC);
`)
	if err != nil {
		return err
	}
	if err := s.initVectorSchema(ctx); err != nil {
		return err
	}
	return s.initKeywordSchema(ctx)
}

func (s *pgEvolvingMemoryStore) initVectorSchema(ctx context.Context) error {
	vecType := "vector"
	if s.dimensions > 0 {
		vecType = fmt.Sprintf("vector(%d)", s.dimensions)
	}
	if _, err := s.pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE evolving_memories ADD COLUMN IF NOT EXISTS embedding_vec %s`, vecType)); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
CREATE INDEX IF NOT EXISTS evolving_memories_embedding_hnsw_idx
    ON evolving_memories USING hnsw (embedding_vec vector_cosine_ops)
    WHERE embedding_vec IS NOT NULL`); err != nil {
		return err
	}
	return nil
}

func (s *pgEvolvingMemoryStore) initKeywordSchema(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `
ALTER TABLE evolving_memories
    ADD COLUMN IF NOT EXISTS search_tsv tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(input, '') || ' ' || coalesce(output, '') || ' ' || coalesce(feedback, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(strategy_card, ''))
    ) STORED`); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
CREATE INDEX IF NOT EXISTS evolving_memories_search_tsv_idx
    ON evolving_memories USING GIN (search_tsv)`); err != nil {
		return err
	}
	return nil
}
