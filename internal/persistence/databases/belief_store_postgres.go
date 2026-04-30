package databases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"manifold/internal/agent/belief"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (s *pgBeliefStore) EnsureScope(ctx context.Context, scope belief.Scope) (belief.Scope, error) {
	scope.TenantID = normalizeTenantID(scope.TenantID)
	scope.Kind = normalizeScopeKind(scope.Kind)
	scope.Path = strings.TrimSpace(scope.Path)
	if strings.TrimSpace(scope.ID) == "" {
		scope.ID = uuid.NewString()
	}
	if scope.Label == "" {
		scope.Label = scope.Path
	}
	metadata, err := marshalJSONMap(scope.Metadata)
	if err != nil {
		return belief.Scope{}, err
	}
	parentID := nilIfEmpty(scope.ParentID)
	row := s.pool.QueryRow(ctx, `
INSERT INTO belief_scopes (id, tenant_id, kind, parent_id, path, label, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, kind, path) DO UPDATE SET
    parent_id = COALESCE(EXCLUDED.parent_id, belief_scopes.parent_id),
    label = EXCLUDED.label,
    metadata = EXCLUDED.metadata,
    updated_at = NOW()
RETURNING id, tenant_id, kind, parent_id, path, label, metadata, created_at, updated_at
`, scope.ID, scope.TenantID, scope.Kind, parentID, scope.Path, scope.Label, metadata)
	return scanBeliefScope(row)
}

func (s *pgBeliefStore) GetScope(ctx context.Context, tenantID int64, kind belief.ScopeKind, path string) (belief.Scope, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, tenant_id, kind, parent_id, path, label, metadata, created_at, updated_at
FROM belief_scopes
WHERE tenant_id = $1 AND kind = $2 AND path = $3
`, normalizeTenantID(tenantID), normalizeScopeKind(kind), strings.TrimSpace(path))
	scope, err := scanBeliefScope(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return belief.Scope{}, false, nil
	}
	if err != nil {
		return belief.Scope{}, false, err
	}
	return scope, true, nil
}

func (s *pgBeliefStore) GetScopeByID(ctx context.Context, tenantID int64, id string) (belief.Scope, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, tenant_id, kind, parent_id, path, label, metadata, created_at, updated_at
FROM belief_scopes
WHERE tenant_id = $1 AND id = $2
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	scope, err := scanBeliefScope(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return belief.Scope{}, false, nil
	}
	if err != nil {
		return belief.Scope{}, false, err
	}
	return scope, true, nil
}

func (s *pgBeliefStore) UpsertEpisode(ctx context.Context, episode belief.Episode) (belief.Episode, error) {
	episode.TenantID = normalizeTenantID(episode.TenantID)
	if strings.TrimSpace(episode.ID) == "" {
		episode.ID = uuid.NewString()
	}
	if episode.StartedAt.IsZero() {
		episode.StartedAt = time.Now().UTC()
	}
	if episode.Outcome == "" {
		episode.Outcome = "unknown"
	}
	if episode.OutcomeSignal == "" {
		episode.OutcomeSignal = "implicit"
	}
	metadata, err := marshalJSONMap(episode.Metadata)
	if err != nil {
		return belief.Episode{}, err
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO belief_episodes (
    id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role,
    user_id, started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    scope_id = EXCLUDED.scope_id,
    project_id = EXCLUDED.project_id,
    objective_id = EXCLUDED.objective_id,
    session_id = EXCLUDED.session_id,
    agent_role = EXCLUDED.agent_role,
    user_id = EXCLUDED.user_id,
    started_at = EXCLUDED.started_at,
    ended_at = EXCLUDED.ended_at,
    outcome = EXCLUDED.outcome,
    outcome_signal = EXCLUDED.outcome_signal,
    evolving_entry_id = EXCLUDED.evolving_entry_id,
    metadata = EXCLUDED.metadata
RETURNING id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role,
    user_id, started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
`, episode.ID, episode.TenantID, episode.ScopeID, episode.ProjectID, episode.ObjectiveID, episode.SessionID, episode.AgentRole, episode.UserID, episode.StartedAt, episode.EndedAt, episode.Outcome, episode.OutcomeSignal, nilIfEmpty(episode.EvolvingEntryID), metadata)
	return scanBeliefEpisode(row)
}

func (s *pgBeliefStore) GetEpisode(ctx context.Context, tenantID int64, id string) (belief.Episode, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role,
    user_id, started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
FROM belief_episodes
WHERE tenant_id = $1 AND id = $2
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	episode, err := scanBeliefEpisode(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return belief.Episode{}, false, nil
	}
	if err != nil {
		return belief.Episode{}, false, err
	}
	return episode, true, nil
}

func (s *pgBeliefStore) UpsertBelief(ctx context.Context, item belief.Belief) (belief.Belief, error) {
	item.TenantID = normalizeTenantID(item.TenantID)
	item.Status = normalizeBeliefStatus(item.Status)
	if strings.TrimSpace(item.ID) == "" {
		item.ID = uuid.NewString()
	}
	metadata, err := marshalJSONMap(item.Metadata)
	if err != nil {
		return belief.Belief{}, err
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO beliefs (
    id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
    evidence_against, status, last_observed, expires_at, embedding_vec, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CASE WHEN $12 = '' THEN NULL ELSE $12::vector END, $13)
ON CONFLICT (tenant_id, scope_id, statement_hash) DO UPDATE SET
    statement = EXCLUDED.statement,
    confidence = EXCLUDED.confidence,
    evidence_for = EXCLUDED.evidence_for,
    evidence_against = EXCLUDED.evidence_against,
    status = EXCLUDED.status,
    last_observed = EXCLUDED.last_observed,
    expires_at = EXCLUDED.expires_at,
    embedding_vec = EXCLUDED.embedding_vec,
    metadata = EXCLUDED.metadata,
    updated_at = NOW()
RETURNING id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
    evidence_against, status, last_observed, expires_at, metadata, created_at, updated_at
`, item.ID, item.TenantID, item.ScopeID, item.Statement, item.StatementHash, item.Confidence, item.EvidenceFor, item.EvidenceAgainst, item.Status, item.LastObserved, item.ExpiresAt, vectorLiteralOrEmpty(item.Embedding), metadata)
	return scanBelief(row)
}

func (s *pgBeliefStore) GetBelief(ctx context.Context, tenantID int64, id string) (belief.Belief, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
    evidence_against, status, last_observed, expires_at, metadata, created_at, updated_at
FROM beliefs
WHERE tenant_id = $1 AND id = $2
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	item, err := scanBelief(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return belief.Belief{}, false, nil
	}
	if err != nil {
		return belief.Belief{}, false, err
	}
	return item, true, nil
}

func (s *pgBeliefStore) SearchBeliefs(ctx context.Context, query belief.SearchQuery) ([]belief.SearchResult, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	statuses := make([]string, 0, len(query.Statuses))
	for _, status := range query.Statuses {
		statuses = append(statuses, string(normalizeBeliefStatus(status)))
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
    evidence_against, status, last_observed, expires_at, metadata, created_at, updated_at,
    confidence + CASE WHEN $4 = '' THEN 0 ELSE ts_rank(search_tsv, plainto_tsquery('simple', $4)) END AS score
FROM beliefs
WHERE tenant_id = $1
	AND (cardinality($2::text[]) = 0 OR scope_id::text = ANY($2::text[]))
    AND (cardinality($3::text[]) = 0 OR status = ANY($3::text[]))
    AND ($4 = '' OR search_tsv @@ plainto_tsquery('simple', $4) OR statement ILIKE '%' || $4 || '%')
ORDER BY score DESC, updated_at DESC
LIMIT $5
`, normalizeTenantID(query.TenantID), query.ScopeIDs, statuses, strings.TrimSpace(query.Query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]belief.SearchResult, 0, limit)
	for rows.Next() {
		item, score, err := scanBeliefWithScore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, belief.SearchResult{Belief: item, Score: score})
	}
	return out, rows.Err()
}

func (s *pgBeliefStore) AddEvidence(ctx context.Context, evidence belief.Evidence) (belief.Evidence, error) {
	evidence.TenantID = normalizeTenantID(evidence.TenantID)
	if strings.TrimSpace(evidence.ID) == "" {
		evidence.ID = uuid.NewString()
	}
	if evidence.SourceKind == "" {
		evidence.SourceKind = belief.SourceKindEpisode
	}
	if evidence.Polarity == "" {
		evidence.Polarity = belief.EvidencePolarityFor
	}
	if evidence.Weight == 0 {
		evidence.Weight = 1
	}
	metadata, err := marshalJSONMap(evidence.Metadata)
	if err != nil {
		return belief.Evidence{}, err
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO belief_evidence (
    id, tenant_id, belief_id, episode_id, source_kind, source_id, polarity, weight, note, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, tenant_id, belief_id, episode_id, source_kind, source_id, polarity, weight, note, metadata, created_at
`, evidence.ID, evidence.TenantID, evidence.BeliefID, nilIfEmpty(evidence.EpisodeID), evidence.SourceKind, evidence.SourceID, evidence.Polarity, evidence.Weight, evidence.Note, metadata)
	return scanBeliefEvidence(row)
}

func (s *pgBeliefStore) ListEvidence(ctx context.Context, query belief.EvidenceQuery) ([]belief.Evidence, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, tenant_id, belief_id, episode_id, source_kind, source_id, polarity, weight, note, metadata, created_at
FROM belief_evidence
WHERE tenant_id = $1
    AND ($2 = '' OR belief_id::text = $2)
    AND ($3 = '' OR episode_id::text = $3)
    AND ($4 = '' OR source_id = $4)
    AND ($5 = '' OR source_kind = $5)
    AND ($6 = '' OR polarity = $6)
ORDER BY created_at DESC
LIMIT $7
`, normalizeTenantID(query.TenantID), strings.TrimSpace(query.BeliefID), strings.TrimSpace(query.EpisodeID), strings.TrimSpace(query.SourceID), strings.TrimSpace(string(query.Source)), strings.TrimSpace(string(query.Polarity)), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]belief.Evidence, 0, limit)
	for rows.Next() {
		evidence, err := scanBeliefEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, evidence)
	}
	return out, rows.Err()
}

func (s *pgBeliefStore) RecordPromotion(ctx context.Context, promotion belief.Promotion) (belief.Promotion, error) {
	promotion.TenantID = normalizeTenantID(promotion.TenantID)
	if strings.TrimSpace(promotion.ID) == "" {
		promotion.ID = uuid.NewString()
	}
	metadata, err := marshalJSONMap(promotion.Metadata)
	if err != nil {
		return belief.Promotion{}, err
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO belief_promotions (
    id, tenant_id, belief_id, from_scope, to_scope, reason, confidence_before,
    confidence_after, actor_user_id, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, tenant_id, belief_id, from_scope, to_scope, reason, confidence_before,
    confidence_after, actor_user_id, metadata, created_at
`, promotion.ID, promotion.TenantID, promotion.BeliefID, promotion.FromScope, promotion.ToScope, promotion.Reason, promotion.ConfidenceBefore, promotion.ConfidenceAfter, promotion.ActorUserID, metadata)
	return scanBeliefPromotion(row)
}

func (s *pgBeliefStore) ListPromotions(ctx context.Context, query belief.PromotionQuery) ([]belief.Promotion, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, tenant_id, belief_id, from_scope, to_scope, reason, confidence_before,
    confidence_after, actor_user_id, metadata, created_at
FROM belief_promotions
WHERE tenant_id = $1
    AND ($2 = '' OR belief_id::text = $2)
    AND ($3 = '' OR from_scope::text = $3)
    AND ($4 = '' OR to_scope::text = $4)
ORDER BY created_at DESC
LIMIT $5
`, normalizeTenantID(query.TenantID), strings.TrimSpace(query.BeliefID), strings.TrimSpace(query.FromScope), strings.TrimSpace(query.ToScope), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]belief.Promotion, 0, limit)
	for rows.Next() {
		promotion, err := scanBeliefPromotion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, promotion)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBeliefScope(row scanner) (belief.Scope, error) {
	var scope belief.Scope
	var kind string
	var parentID *string
	var metadata []byte
	if err := row.Scan(&scope.ID, &scope.TenantID, &kind, &parentID, &scope.Path, &scope.Label, &metadata, &scope.CreatedAt, &scope.UpdatedAt); err != nil {
		return belief.Scope{}, err
	}
	scope.Kind = belief.ScopeKind(kind)
	if parentID != nil {
		scope.ParentID = *parentID
	}
	scope.Metadata = unmarshalJSONMap(metadata)
	return scope, nil
}

func scanBeliefEpisode(row scanner) (belief.Episode, error) {
	var episode belief.Episode
	var endedAt *time.Time
	var evolvingEntryID *string
	var metadata []byte
	if err := row.Scan(&episode.ID, &episode.TenantID, &episode.ScopeID, &episode.ProjectID, &episode.ObjectiveID, &episode.SessionID, &episode.AgentRole, &episode.UserID, &episode.StartedAt, &endedAt, &episode.Outcome, &episode.OutcomeSignal, &evolvingEntryID, &metadata); err != nil {
		return belief.Episode{}, err
	}
	episode.EndedAt = endedAt
	if evolvingEntryID != nil {
		episode.EvolvingEntryID = *evolvingEntryID
	}
	episode.Metadata = unmarshalJSONMap(metadata)
	return episode, nil
}

func scanBelief(row scanner) (belief.Belief, error) {
	var item belief.Belief
	var status string
	var lastObserved *time.Time
	var expiresAt *time.Time
	var metadata []byte
	if err := row.Scan(&item.ID, &item.TenantID, &item.ScopeID, &item.Statement, &item.StatementHash, &item.Confidence, &item.EvidenceFor, &item.EvidenceAgainst, &status, &lastObserved, &expiresAt, &metadata, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return belief.Belief{}, err
	}
	item.Status = belief.BeliefStatus(status)
	item.LastObserved = lastObserved
	item.ExpiresAt = expiresAt
	item.Metadata = unmarshalJSONMap(metadata)
	return item, nil
}

func scanBeliefWithScore(row scanner) (belief.Belief, float64, error) {
	var item belief.Belief
	var status string
	var lastObserved *time.Time
	var expiresAt *time.Time
	var metadata []byte
	var score float64
	if err := row.Scan(&item.ID, &item.TenantID, &item.ScopeID, &item.Statement, &item.StatementHash, &item.Confidence, &item.EvidenceFor, &item.EvidenceAgainst, &status, &lastObserved, &expiresAt, &metadata, &item.CreatedAt, &item.UpdatedAt, &score); err != nil {
		return belief.Belief{}, 0, err
	}
	item.Status = belief.BeliefStatus(status)
	item.LastObserved = lastObserved
	item.ExpiresAt = expiresAt
	item.Metadata = unmarshalJSONMap(metadata)
	return item, score, nil
}

func scanBeliefEvidence(row scanner) (belief.Evidence, error) {
	var evidence belief.Evidence
	var episodeID *string
	var sourceKind string
	var polarity string
	var metadata []byte
	if err := row.Scan(&evidence.ID, &evidence.TenantID, &evidence.BeliefID, &episodeID, &sourceKind, &evidence.SourceID, &polarity, &evidence.Weight, &evidence.Note, &metadata, &evidence.CreatedAt); err != nil {
		return belief.Evidence{}, err
	}
	if episodeID != nil {
		evidence.EpisodeID = *episodeID
	}
	evidence.SourceKind = belief.SourceKind(sourceKind)
	evidence.Polarity = belief.EvidencePolarity(polarity)
	evidence.Metadata = unmarshalJSONMap(metadata)
	return evidence, nil
}

func scanBeliefPromotion(row scanner) (belief.Promotion, error) {
	var promotion belief.Promotion
	var actorUserID *int64
	var metadata []byte
	if err := row.Scan(&promotion.ID, &promotion.TenantID, &promotion.BeliefID, &promotion.FromScope, &promotion.ToScope, &promotion.Reason, &promotion.ConfidenceBefore, &promotion.ConfidenceAfter, &actorUserID, &metadata, &promotion.CreatedAt); err != nil {
		return belief.Promotion{}, err
	}
	promotion.ActorUserID = actorUserID
	promotion.Metadata = unmarshalJSONMap(metadata)
	return promotion, nil
}

func marshalJSONMap(metadata map[string]any) ([]byte, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return json.Marshal(metadata)
}

func unmarshalJSONMap(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func nilIfEmpty(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func vectorLiteralOrEmpty(vector []float32) string {
	if len(vector) == 0 {
		return ""
	}
	return toVectorLiteral(vector)
}
