package databases

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"manifold/internal/agent/memory/decision"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresDecisionStoreWithDimensions returns a Postgres-backed decision store.
func NewPostgresDecisionStoreWithDimensions(pool *pgxpool.Pool, _ int) decision.Store {
	if pool == nil {
		return NewMemoryDecisionStore()
	}
	return &pgDecisionStore{pool: pool}
}

type pgDecisionStore struct {
	pool *pgxpool.Pool
}

func (s *pgDecisionStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *pgDecisionStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres decision store requires pool")
	}
	if _, err := s.pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS decision_records (
	id TEXT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	scope_id TEXT NOT NULL,
	episode_id TEXT,
	title TEXT NOT NULL,
	statement TEXT NOT NULL,
	statement_hash TEXT NOT NULL,
	rationale TEXT NOT NULL DEFAULT '',
	decided_by TEXT NOT NULL DEFAULT '',
	decided_at TIMESTAMPTZ NOT NULL,
	status TEXT NOT NULL,
	review_state TEXT NOT NULL,
	confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
	superseded_by TEXT,
	stale_reason TEXT,
	embedding_vec vector,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	search_tsv tsvector GENERATED ALWAYS AS (
		to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(statement, '') || ' ' || coalesce(rationale, ''))
	) STORED,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS decision_records_tenant_scope_hash
	ON decision_records(tenant_id, scope_id, statement_hash);
CREATE INDEX IF NOT EXISTS decision_records_tenant_status
	ON decision_records(tenant_id, status);
CREATE INDEX IF NOT EXISTS decision_records_search_tsv_idx
	ON decision_records USING GIN(search_tsv);

CREATE TABLE IF NOT EXISTS decision_assumptions (
	id TEXT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	decision_id TEXT NOT NULL REFERENCES decision_records(id),
	belief_id TEXT NOT NULL,
	criticality TEXT NOT NULL,
	belief_statement_at_link TEXT NOT NULL DEFAULT '',
	belief_confidence_at_link DOUBLE PRECISION NOT NULL DEFAULT 0,
	note TEXT NOT NULL DEFAULT '',
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS decision_assumptions_belief
	ON decision_assumptions(tenant_id, belief_id);
CREATE INDEX IF NOT EXISTS decision_assumptions_decision
	ON decision_assumptions(tenant_id, decision_id);

CREATE TABLE IF NOT EXISTS decision_alternatives (
	id TEXT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	decision_id TEXT NOT NULL REFERENCES decision_records(id),
	statement TEXT NOT NULL,
	rejection_reason TEXT NOT NULL DEFAULT '',
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS decision_alternatives_decision
	ON decision_alternatives(tenant_id, decision_id);

CREATE TABLE IF NOT EXISTS decision_evidence (
	id TEXT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	decision_id TEXT NOT NULL REFERENCES decision_records(id),
	source_kind TEXT NOT NULL,
	source_id TEXT NOT NULL,
	polarity TEXT NOT NULL,
	weight DOUBLE PRECISION NOT NULL DEFAULT 1,
	note TEXT NOT NULL DEFAULT '',
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS decision_evidence_source
	ON decision_evidence(tenant_id, source_kind, source_id);
CREATE INDEX IF NOT EXISTS decision_evidence_decision
	ON decision_evidence(tenant_id, decision_id);

CREATE TABLE IF NOT EXISTS decision_transitions (
	id TEXT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	decision_id TEXT NOT NULL REFERENCES decision_records(id),
	from_status TEXT NOT NULL,
	to_status TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	trigger_kind TEXT NOT NULL DEFAULT '',
	trigger_id TEXT NOT NULL DEFAULT '',
	actor_user_id BIGINT,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS decision_transitions_decision
	ON decision_transitions(tenant_id, decision_id, created_at);

CREATE TABLE IF NOT EXISTS decision_candidates (
	id TEXT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	episode_id TEXT NOT NULL DEFAULT '',
	scope_id TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL,
	statement TEXT NOT NULL,
	statement_hash TEXT NOT NULL,
	rationale TEXT NOT NULL DEFAULT '',
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
	review_state TEXT NOT NULL,
	validation_status TEXT NOT NULL,
	rejection_reason TEXT,
	accepted_decision_id TEXT,
	model TEXT,
	raw_payload TEXT,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS decision_candidates_tenant_review_created
	ON decision_candidates(tenant_id, review_state, created_at DESC);
CREATE INDEX IF NOT EXISTS decision_candidates_tenant_episode_created
	ON decision_candidates(tenant_id, episode_id, created_at DESC);
`)
	return err
}

func (s *pgDecisionStore) UpsertDecision(ctx context.Context, item decision.Decision) (decision.Decision, error) {
	item = prepareDecision(item)
	if existingID, ok, err := s.findDecisionIDByHash(ctx, item.TenantID, item.ScopeID, item.StatementHash); err != nil {
		return decision.Decision{}, err
	} else if ok && existingID != item.ID {
		item.ID = existingID
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return decision.Decision{}, err
	}
	metadata, err := json.Marshal(item.Metadata)
	if err != nil {
		return decision.Decision{}, err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO decision_records(id, tenant_id, scope_id, episode_id, title, statement, statement_hash, rationale, decided_by, decided_at, status, review_state, confidence, superseded_by, stale_reason, metadata, payload, created_at, updated_at)
VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16::jsonb, $17::jsonb, $18, $19)
ON CONFLICT(id) DO UPDATE SET
	scope_id=EXCLUDED.scope_id,
	episode_id=EXCLUDED.episode_id,
	title=EXCLUDED.title,
	statement=EXCLUDED.statement,
	statement_hash=EXCLUDED.statement_hash,
	rationale=EXCLUDED.rationale,
	decided_by=EXCLUDED.decided_by,
	decided_at=EXCLUDED.decided_at,
	status=EXCLUDED.status,
	review_state=EXCLUDED.review_state,
	confidence=EXCLUDED.confidence,
	superseded_by=EXCLUDED.superseded_by,
	stale_reason=EXCLUDED.stale_reason,
	metadata=EXCLUDED.metadata,
	payload=EXCLUDED.payload,
	updated_at=EXCLUDED.updated_at
`, item.ID, item.TenantID, item.ScopeID, item.EpisodeID, item.Title, item.Statement, item.StatementHash, item.Rationale, item.DecidedBy, item.DecidedAt, item.Status, item.ReviewState, item.Confidence, nullableString(item.SupersededBy), nullableString(item.StaleReason), string(metadata), string(payload), item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return decision.Decision{}, err
	}
	return cloneDecision(item), nil
}

func (s *pgDecisionStore) GetDecision(ctx context.Context, tenantID int64, id string) (decision.Decision, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT payload FROM decision_records WHERE tenant_id = $1 AND id = $2`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	item, ok, err := scanPGJSONRow[decision.Decision](row)
	if err != nil || !ok {
		return decision.Decision{}, ok, err
	}
	return cloneDecision(item), true, nil
}

func (s *pgDecisionStore) SearchDecisions(ctx context.Context, query decision.SearchQuery) ([]decision.SearchResult, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	statuses := make([]string, 0, len(query.Statuses))
	for _, status := range query.Statuses {
		statuses = append(statuses, string(decision.NormalizeDecisionStatus(status)))
	}
	scopeIDs := query.ScopeIDs
	if scopeIDs == nil {
		scopeIDs = []string{}
	}
	scopePrefixes := query.ScopePrefixes
	if scopePrefixes == nil {
		scopePrefixes = []string{}
	}
	rows, err := s.pool.Query(ctx, `
SELECT payload,
	confidence + CASE WHEN $5 = '' THEN 0 ELSE ts_rank(search_tsv, plainto_tsquery('simple', $5)) END AS score
FROM decision_records
WHERE tenant_id = $1
	AND (
		(cardinality($2::text[]) = 0 AND cardinality($3::text[]) = 0)
		OR scope_id = ANY($2::text[])
		OR EXISTS (SELECT 1 FROM unnest($3::text[]) AS prefix WHERE left(scope_id, length(prefix)) = prefix)
	)
	AND (cardinality($4::text[]) = 0 OR status = ANY($4::text[]))
	AND ($5 = '' OR search_tsv @@ plainto_tsquery('simple', $5) OR statement ILIKE '%' || $5 || '%' OR title ILIKE '%' || $5 || '%' OR rationale ILIKE '%' || $5 || '%')
ORDER BY score DESC, updated_at DESC
LIMIT $6
`, normalizeTenantID(query.TenantID), scopeIDs, scopePrefixes, statuses, strings.TrimSpace(query.Query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]decision.SearchResult, 0, limit)
	for rows.Next() {
		item, score, err := scanPGJSONWithScore[decision.Decision](rows)
		if err != nil {
			return nil, err
		}
		out = append(out, decision.SearchResult{Decision: cloneDecision(item), Score: score})
	}
	return out, rows.Err()
}
func (s *pgDecisionStore) AddAssumption(ctx context.Context, link decision.AssumptionLink) (decision.AssumptionLink, error) {
	link.TenantID = normalizeTenantID(link.TenantID)
	if strings.TrimSpace(link.ID) == "" {
		link.ID = uuid.NewString()
	}
	link.Criticality = decision.NormalizeAssumptionCriticality(link.Criticality)
	if link.Metadata == nil {
		link.Metadata = map[string]any{}
	}
	if link.CreatedAt.IsZero() {
		link.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(link)
	if err != nil {
		return decision.AssumptionLink{}, err
	}
	metadata, err := json.Marshal(link.Metadata)
	if err != nil {
		return decision.AssumptionLink{}, err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO decision_assumptions(id, tenant_id, decision_id, belief_id, criticality, belief_statement_at_link, belief_confidence_at_link, note, metadata, payload, created_at)
VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11)
ON CONFLICT(id) DO UPDATE SET
	criticality=EXCLUDED.criticality,
	belief_statement_at_link=EXCLUDED.belief_statement_at_link,
	belief_confidence_at_link=EXCLUDED.belief_confidence_at_link,
	note=EXCLUDED.note,
	metadata=EXCLUDED.metadata,
	payload=EXCLUDED.payload
`, link.ID, link.TenantID, link.DecisionID, link.BeliefID, link.Criticality, link.BeliefStatementAtLink, link.BeliefConfidenceAtLink, link.Note, string(metadata), string(payload), link.CreatedAt)
	return cloneAssumption(link), err
}

func (s *pgDecisionStore) ListAssumptions(ctx context.Context, query decision.AssumptionQuery) ([]decision.AssumptionLink, error) {
	rows, err := s.pool.Query(ctx, `
SELECT payload FROM decision_assumptions
WHERE tenant_id = $1
	AND ($2 = '' OR decision_id = $2)
	AND ($3 = '' OR belief_id = $3)
ORDER BY created_at ASC
LIMIT $4
`, normalizeTenantID(query.TenantID), strings.TrimSpace(query.DecisionID), strings.TrimSpace(query.BeliefID), normalizedLimit(query.Limit, 100))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPGJSONRows[decision.AssumptionLink](rows)
}

func (s *pgDecisionStore) AddAlternative(ctx context.Context, alt decision.Alternative) (decision.Alternative, error) {
	alt.TenantID = normalizeTenantID(alt.TenantID)
	if strings.TrimSpace(alt.ID) == "" {
		alt.ID = uuid.NewString()
	}
	alt.Statement = decision.NormalizeStatement(alt.Statement)
	if alt.Metadata == nil {
		alt.Metadata = map[string]any{}
	}
	if alt.CreatedAt.IsZero() {
		alt.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(alt)
	if err != nil {
		return decision.Alternative{}, err
	}
	metadata, err := json.Marshal(alt.Metadata)
	if err != nil {
		return decision.Alternative{}, err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO decision_alternatives(id, tenant_id, decision_id, statement, rejection_reason, metadata, payload, created_at)
VALUES($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)
ON CONFLICT(id) DO UPDATE SET statement=EXCLUDED.statement, rejection_reason=EXCLUDED.rejection_reason, metadata=EXCLUDED.metadata, payload=EXCLUDED.payload
`, alt.ID, alt.TenantID, alt.DecisionID, alt.Statement, alt.RejectionReason, string(metadata), string(payload), alt.CreatedAt)
	return cloneAlternative(alt), err
}

func (s *pgDecisionStore) ListAlternatives(ctx context.Context, tenantID int64, decisionID string) ([]decision.Alternative, error) {
	rows, err := s.pool.Query(ctx, `
SELECT payload FROM decision_alternatives
WHERE tenant_id = $1 AND decision_id = $2
ORDER BY created_at ASC
`, normalizeTenantID(tenantID), strings.TrimSpace(decisionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPGJSONRows[decision.Alternative](rows)
}

func (s *pgDecisionStore) AddEvidence(ctx context.Context, ev decision.DecisionEvidence) (decision.DecisionEvidence, error) {
	ev.TenantID = normalizeTenantID(ev.TenantID)
	if strings.TrimSpace(ev.ID) == "" {
		ev.ID = uuid.NewString()
	}
	ev.Polarity = decision.NormalizeEvidencePolarity(ev.Polarity)
	if ev.Weight == 0 {
		ev.Weight = 1
	}
	if ev.Metadata == nil {
		ev.Metadata = map[string]any{}
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return decision.DecisionEvidence{}, err
	}
	metadata, err := json.Marshal(ev.Metadata)
	if err != nil {
		return decision.DecisionEvidence{}, err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO decision_evidence(id, tenant_id, decision_id, source_kind, source_id, polarity, weight, note, metadata, payload, created_at)
VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11)
ON CONFLICT(id) DO UPDATE SET source_kind=EXCLUDED.source_kind, source_id=EXCLUDED.source_id, polarity=EXCLUDED.polarity, weight=EXCLUDED.weight, note=EXCLUDED.note, metadata=EXCLUDED.metadata, payload=EXCLUDED.payload
`, ev.ID, ev.TenantID, ev.DecisionID, ev.SourceKind, ev.SourceID, ev.Polarity, ev.Weight, ev.Note, string(metadata), string(payload), ev.CreatedAt)
	return cloneDecisionEvidence(ev), err
}

func (s *pgDecisionStore) ListEvidence(ctx context.Context, query decision.EvidenceQuery) ([]decision.DecisionEvidence, error) {
	rows, err := s.pool.Query(ctx, `
SELECT payload FROM decision_evidence
WHERE tenant_id = $1
	AND ($2 = '' OR decision_id = $2)
	AND ($3 = '' OR source_kind = $3)
	AND ($4 = '' OR source_id = $4)
	AND ($5 = '' OR polarity = $5)
ORDER BY created_at ASC
LIMIT $6
`, normalizeTenantID(query.TenantID), strings.TrimSpace(query.DecisionID), strings.TrimSpace(query.SourceKind), strings.TrimSpace(query.SourceID), string(query.Polarity), normalizedLimit(query.Limit, 100))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPGJSONRows[decision.DecisionEvidence](rows)
}

func (s *pgDecisionStore) RecordTransition(ctx context.Context, tr decision.Transition) (decision.Transition, error) {
	tr.TenantID = normalizeTenantID(tr.TenantID)
	if strings.TrimSpace(tr.ID) == "" {
		tr.ID = uuid.NewString()
	}
	if tr.CreatedAt.IsZero() {
		tr.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(tr)
	if err != nil {
		return decision.Transition{}, err
	}
	var actor any
	if tr.ActorUserID != nil {
		actor = *tr.ActorUserID
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO decision_transitions(id, tenant_id, decision_id, from_status, to_status, reason, trigger_kind, trigger_id, actor_user_id, payload, created_at)
VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11)
ON CONFLICT(id) DO UPDATE SET payload=EXCLUDED.payload
`, tr.ID, tr.TenantID, tr.DecisionID, tr.FromStatus, tr.ToStatus, tr.Reason, tr.TriggerKind, tr.TriggerID, actor, string(payload), tr.CreatedAt)
	return cloneTransition(tr), err
}

func (s *pgDecisionStore) ListTransitions(ctx context.Context, query decision.TransitionQuery) ([]decision.Transition, error) {
	rows, err := s.pool.Query(ctx, `
SELECT payload FROM decision_transitions
WHERE tenant_id = $1 AND ($2 = '' OR decision_id = $2)
ORDER BY created_at ASC
LIMIT $3
`, normalizeTenantID(query.TenantID), strings.TrimSpace(query.DecisionID), normalizedLimit(query.Limit, 100))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPGJSONRows[decision.Transition](rows)
}

func (s *pgDecisionStore) RecordCandidate(ctx context.Context, c decision.Candidate) (decision.Candidate, error) {
	c.TenantID = normalizeTenantID(c.TenantID)
	if strings.TrimSpace(c.ID) == "" {
		c.ID = uuid.NewString()
	}
	c.Statement = decision.NormalizeStatement(c.Statement)
	if strings.TrimSpace(c.StatementHash) == "" {
		c.StatementHash = decision.StatementHash(c.Statement)
	}
	c.ReviewState = decision.NormalizeReviewState(c.ReviewState)
	c.ValidationStatus = decision.NormalizeCandidateValidationStatus(c.ValidationStatus)
	c.Confidence = clampDecisionConfidence(c.Confidence)
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.Metadata == nil {
		c.Metadata = map[string]any{}
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return decision.Candidate{}, err
	}
	metadata, err := json.Marshal(c.Metadata)
	if err != nil {
		return decision.Candidate{}, err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO decision_candidates(id, tenant_id, episode_id, scope_id, title, statement, statement_hash, rationale, payload, confidence, review_state, validation_status, rejection_reason, accepted_decision_id, model, raw_payload, metadata, created_at, updated_at)
VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, $15, $16, $17::jsonb, $18, $19)
ON CONFLICT(id) DO UPDATE SET
	title=EXCLUDED.title,
	statement=EXCLUDED.statement,
	statement_hash=EXCLUDED.statement_hash,
	rationale=EXCLUDED.rationale,
	payload=EXCLUDED.payload,
	confidence=EXCLUDED.confidence,
	review_state=EXCLUDED.review_state,
	validation_status=EXCLUDED.validation_status,
	rejection_reason=EXCLUDED.rejection_reason,
	accepted_decision_id=EXCLUDED.accepted_decision_id,
	model=EXCLUDED.model,
	raw_payload=EXCLUDED.raw_payload,
	metadata=EXCLUDED.metadata,
	updated_at=EXCLUDED.updated_at
`, c.ID, c.TenantID, c.EpisodeID, c.ScopeID, c.Title, c.Statement, c.StatementHash, c.Rationale, string(payload), c.Confidence, c.ReviewState, c.ValidationStatus, nullableString(c.RejectionReason), nullableString(c.AcceptedDecisionID), nullableString(c.Model), nullableString(c.RawPayload), string(metadata), c.CreatedAt, c.UpdatedAt)
	return cloneCandidate(c), err
}

func (s *pgDecisionStore) GetCandidate(ctx context.Context, tenantID int64, id string) (decision.Candidate, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT payload FROM decision_candidates WHERE tenant_id = $1 AND id = $2`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	item, ok, err := scanPGJSONRow[decision.Candidate](row)
	if err != nil || !ok {
		return decision.Candidate{}, ok, err
	}
	return cloneCandidate(item), true, nil
}

func (s *pgDecisionStore) ListCandidates(ctx context.Context, query decision.CandidateQuery) ([]decision.Candidate, error) {
	rows, err := s.pool.Query(ctx, `
SELECT payload FROM decision_candidates
WHERE tenant_id = $1
	AND ($2 = '' OR episode_id = $2)
	AND ($3 = '' OR review_state = $3)
	AND ($4 = '' OR validation_status = $4)
ORDER BY created_at ASC
LIMIT $5
`, normalizeTenantID(query.TenantID), strings.TrimSpace(query.EpisodeID), string(query.ReviewState), string(query.ValidationStatus), normalizedLimit(query.Limit, 100))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPGJSONRows[decision.Candidate](rows)
}

func (s *pgDecisionStore) findDecisionIDByHash(ctx context.Context, tenantID int64, scopeID, hash string) (string, bool, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM decision_records WHERE tenant_id = $1 AND scope_id = $2 AND statement_hash = $3`, normalizeTenantID(tenantID), strings.TrimSpace(scopeID), strings.TrimSpace(hash)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

func scanPGJSONRow[T any](row pgx.Row) (T, bool, error) {
	var zero T
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, false, nil
		}
		return zero, false, err
	}
	var out T
	if err := json.Unmarshal(payload, &out); err != nil {
		return zero, false, err
	}
	return out, true, nil
}

func scanPGJSONWithScore[T any](rows pgx.Rows) (T, float64, error) {
	var zero T
	var payload []byte
	var score float64
	if err := rows.Scan(&payload, &score); err != nil {
		return zero, 0, err
	}
	var out T
	if err := json.Unmarshal(payload, &out); err != nil {
		return zero, 0, err
	}
	return out, score, nil
}

func scanPGJSONRows[T any](rows pgx.Rows) ([]T, error) {
	out := []T{}
	for rows.Next() {
		item, ok, err := scanPGJSONRow[T](rows)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func normalizedLimit(limit, fallback int) int {
	if limit <= 0 {
		return fallback
	}
	return limit
}

func sortDecisionResults(out []decision.SearchResult) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Decision.UpdatedAt.After(out[j].Decision.UpdatedAt)
		}
		return out[i].Score > out[j].Score
	})
}
