package databases

import (
	"context"
	"errors"
	"fmt"

	"manifold/internal/agent/belief"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewBeliefStore returns a Postgres-backed belief store when a pool is provided,
// otherwise an in-memory implementation.
func NewBeliefStore(pool *pgxpool.Pool) belief.Store {
	return NewBeliefStoreWithDimensions(pool, 0)
}

// NewBeliefStoreWithDimensions returns a belief store with optional pgvector dimensions.
func NewBeliefStoreWithDimensions(pool *pgxpool.Pool, dimensions int) belief.Store {
	if pool == nil {
		return NewMemoryBeliefStore()
	}
	return &pgBeliefStore{pool: pool, dimensions: dimensions}
}

type pgBeliefStore struct {
	pool       *pgxpool.Pool
	dimensions int
}

func (s *pgBeliefStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *pgBeliefStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres belief store requires pool")
	}
	if _, err := s.pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS belief_scopes (
    id UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    kind TEXT NOT NULL,
    parent_id UUID NULL REFERENCES belief_scopes(id),
    path TEXT NOT NULL,
    label TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS belief_scopes_tenant_kind_path_idx
    ON belief_scopes(tenant_id, kind, path);

CREATE INDEX IF NOT EXISTS belief_scopes_tenant_parent_idx
    ON belief_scopes(tenant_id, parent_id);

CREATE TABLE IF NOT EXISTS belief_episodes (
    id UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    scope_id UUID NOT NULL REFERENCES belief_scopes(id),
    project_id TEXT NOT NULL,
    objective_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    agent_role TEXT NOT NULL DEFAULT '',
    user_id BIGINT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NULL,
    outcome TEXT NOT NULL DEFAULT 'unknown',
    outcome_signal TEXT NOT NULL DEFAULT 'implicit',
    evolving_entry_id TEXT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS belief_episodes_objective_started_idx
    ON belief_episodes(tenant_id, project_id, objective_id, started_at DESC);

CREATE INDEX IF NOT EXISTS belief_episodes_session_started_idx
    ON belief_episodes(tenant_id, session_id, started_at DESC);

CREATE INDEX IF NOT EXISTS belief_episodes_evolving_entry_idx
    ON belief_episodes(tenant_id, evolving_entry_id)
    WHERE evolving_entry_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS beliefs (
    id UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    scope_id UUID NOT NULL REFERENCES belief_scopes(id),
    statement TEXT NOT NULL,
    statement_hash TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL,
    evidence_for INTEGER NOT NULL DEFAULT 0,
    evidence_against INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    last_observed TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS beliefs_tenant_scope_statement_hash_idx
    ON beliefs(tenant_id, scope_id, statement_hash);

CREATE INDEX IF NOT EXISTS beliefs_tenant_scope_status_updated_idx
    ON beliefs(tenant_id, scope_id, status, updated_at DESC);

ALTER TABLE beliefs
    ADD COLUMN IF NOT EXISTS search_tsv tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(statement, ''))
    ) STORED;

CREATE INDEX IF NOT EXISTS beliefs_search_tsv_idx
    ON beliefs USING GIN (search_tsv);

CREATE TABLE IF NOT EXISTS belief_evidence (
    id UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    belief_id UUID NOT NULL REFERENCES beliefs(id),
    episode_id UUID NULL REFERENCES belief_episodes(id),
    source_kind TEXT NOT NULL,
    source_id TEXT NOT NULL DEFAULT '',
    polarity TEXT NOT NULL,
    weight DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    note TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS belief_evidence_belief_created_idx
    ON belief_evidence(tenant_id, belief_id, created_at DESC);

CREATE INDEX IF NOT EXISTS belief_evidence_source_idx
    ON belief_evidence(tenant_id, source_kind, source_id);

CREATE TABLE IF NOT EXISTS belief_promotions (
    id UUID PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    belief_id UUID NOT NULL REFERENCES beliefs(id),
    from_scope UUID NOT NULL REFERENCES belief_scopes(id),
    to_scope UUID NOT NULL REFERENCES belief_scopes(id),
    reason TEXT NOT NULL,
    confidence_before DOUBLE PRECISION NOT NULL,
    confidence_after DOUBLE PRECISION NOT NULL,
    actor_user_id BIGINT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS belief_promotions_belief_created_idx
    ON belief_promotions(tenant_id, belief_id, created_at DESC);
`)
	if err != nil {
		return err
	}
	return s.initVectorSchema(ctx)
}

func (s *pgBeliefStore) initVectorSchema(ctx context.Context) error {
	vecType := "vector"
	if s.dimensions > 0 {
		vecType = fmt.Sprintf("vector(%d)", s.dimensions)
	}
	if _, err := s.pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE beliefs ADD COLUMN IF NOT EXISTS embedding_vec %s`, vecType)); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
CREATE INDEX IF NOT EXISTS beliefs_embedding_hnsw_idx
    ON beliefs USING hnsw (embedding_vec vector_cosine_ops)
    WHERE embedding_vec IS NOT NULL`)
	return err
}
