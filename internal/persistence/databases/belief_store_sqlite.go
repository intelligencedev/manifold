package databases

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/agent/memory/belief"
)

// NewSQLiteBeliefStore returns a SQLite-backed belief store.
func NewSQLiteBeliefStore(db *sql.DB) belief.Store {
	return &sqliteBeliefStore{db: db}
}

type sqliteBeliefStore struct {
	db *sql.DB
}

func (s *sqliteBeliefStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite belief store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS belief_scopes (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	kind TEXT NOT NULL,
	parent_id TEXT NULL REFERENCES belief_scopes(id),
	path TEXT NOT NULL,
	label TEXT NOT NULL,
	metadata TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	UNIQUE(tenant_id, kind, path),
	CHECK (json_valid(metadata))
);
CREATE INDEX IF NOT EXISTS belief_scopes_tenant_parent_idx
	ON belief_scopes(tenant_id, parent_id);

CREATE TABLE IF NOT EXISTS belief_episodes (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	scope_id TEXT NOT NULL REFERENCES belief_scopes(id),
	project_id TEXT NOT NULL,
	objective_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	agent_role TEXT NOT NULL DEFAULT '',
	user_id INTEGER NOT NULL,
	started_at DATETIME NOT NULL,
	ended_at DATETIME NULL,
	outcome TEXT NOT NULL DEFAULT 'unknown',
	outcome_signal TEXT NOT NULL DEFAULT 'implicit',
	evolving_entry_id TEXT NULL,
	metadata TEXT NOT NULL DEFAULT '{}',
	CHECK (json_valid(metadata))
);
CREATE INDEX IF NOT EXISTS belief_episodes_objective_started_idx
	ON belief_episodes(tenant_id, project_id, objective_id, started_at DESC);
CREATE INDEX IF NOT EXISTS belief_episodes_session_started_idx
	ON belief_episodes(tenant_id, session_id, started_at DESC);
CREATE INDEX IF NOT EXISTS belief_episodes_evolving_entry_idx
	ON belief_episodes(tenant_id, evolving_entry_id)
	WHERE evolving_entry_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS beliefs (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	scope_id TEXT NOT NULL REFERENCES belief_scopes(id),
	statement TEXT NOT NULL,
	statement_hash TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'fact',
	enforcement TEXT NOT NULL DEFAULT 'none',
	source_quality REAL NOT NULL DEFAULT 0,
	review_state TEXT NOT NULL DEFAULT 'auto_active',
	confidence REAL NOT NULL,
	evidence_for INTEGER NOT NULL DEFAULT 0,
	evidence_against INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'active',
	last_observed DATETIME NULL,
	expires_at DATETIME NULL,
	embedding TEXT NOT NULL DEFAULT '[]',
	metadata TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	UNIQUE(tenant_id, scope_id, statement_hash),
	CHECK (json_valid(embedding)),
	CHECK (json_valid(metadata))
);
CREATE INDEX IF NOT EXISTS beliefs_tenant_scope_status_updated_idx
	ON beliefs(tenant_id, scope_id, status, updated_at DESC);
CREATE VIRTUAL TABLE IF NOT EXISTS beliefs_fts USING fts5(
	id UNINDEXED,
	statement,
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS belief_evidence (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	belief_id TEXT NOT NULL REFERENCES beliefs(id),
	episode_id TEXT NULL REFERENCES belief_episodes(id),
	source_kind TEXT NOT NULL,
	source_id TEXT NOT NULL DEFAULT '',
	polarity TEXT NOT NULL,
	weight REAL NOT NULL DEFAULT 1,
	note TEXT NOT NULL DEFAULT '',
	metadata TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME NOT NULL,
	CHECK (json_valid(metadata))
);
CREATE INDEX IF NOT EXISTS belief_evidence_belief_created_idx
	ON belief_evidence(tenant_id, belief_id, created_at DESC);
CREATE INDEX IF NOT EXISTS belief_evidence_source_idx
	ON belief_evidence(tenant_id, source_kind, source_id);

CREATE TABLE IF NOT EXISTS belief_promotions (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	belief_id TEXT NOT NULL REFERENCES beliefs(id),
	from_scope TEXT NOT NULL REFERENCES belief_scopes(id),
	to_scope TEXT NOT NULL REFERENCES belief_scopes(id),
	reason TEXT NOT NULL,
	confidence_before REAL NOT NULL,
	confidence_after REAL NOT NULL,
	actor_user_id INTEGER NULL,
	metadata TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME NOT NULL,
	CHECK (json_valid(metadata))
);
CREATE INDEX IF NOT EXISTS belief_promotions_belief_created_idx
	ON belief_promotions(tenant_id, belief_id, created_at DESC);

CREATE TABLE IF NOT EXISTS belief_candidates (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	episode_id TEXT NULL REFERENCES belief_episodes(id),
	scope_id TEXT NULL REFERENCES belief_scopes(id),
	statement TEXT NOT NULL DEFAULT '',
	statement_hash TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT 'fact',
	enforcement TEXT NOT NULL DEFAULT 'none',
	polarity TEXT NOT NULL DEFAULT 'for',
	confidence REAL NOT NULL DEFAULT 0,
	source_quality REAL NOT NULL DEFAULT 0,
	review_state TEXT NOT NULL DEFAULT 'needs_review',
	evidence_note TEXT NOT NULL DEFAULT '',
	raw_payload TEXT NOT NULL DEFAULT '',
	normalized_payload TEXT NOT NULL DEFAULT '{}',
	validation_status TEXT NOT NULL DEFAULT 'queued',
	rejection_reason TEXT NOT NULL DEFAULT '',
	accepted_belief_id TEXT NULL REFERENCES beliefs(id),
	model TEXT NOT NULL DEFAULT '',
	metadata TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	CHECK (json_valid(normalized_payload)),
	CHECK (json_valid(metadata))
);
CREATE INDEX IF NOT EXISTS belief_candidates_tenant_review_created_idx
	ON belief_candidates(tenant_id, review_state, created_at DESC);
CREATE INDEX IF NOT EXISTS belief_candidates_tenant_episode_idx
	ON belief_candidates(tenant_id, episode_id, created_at DESC);
`)
	return err
}

func (s *sqliteBeliefStore) EnsureScope(ctx context.Context, scope belief.Scope) (belief.Scope, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Scope{}, err
	}
	scope.TenantID = normalizeTenantID(scope.TenantID)
	scope.Kind = normalizeScopeKind(scope.Kind)
	scope.Path = strings.TrimSpace(scope.Path)
	if strings.TrimSpace(scope.ID) == "" {
		scope.ID = uuid.NewString()
	}
	if scope.Label == "" {
		scope.Label = scope.Path
	}
	if scope.Metadata == nil {
		scope.Metadata = map[string]any{}
	}
	now := time.Now().UTC()
	if scope.CreatedAt.IsZero() {
		scope.CreatedAt = now
	}
	scope.UpdatedAt = now
	metadata, err := marshalJSONMap(scope.Metadata)
	if err != nil {
		return belief.Scope{}, err
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO belief_scopes(id, tenant_id, kind, parent_id, path, label, metadata, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(tenant_id, kind, path) DO UPDATE SET
	label = excluded.label,
	parent_id = excluded.parent_id,
	metadata = excluded.metadata,
	updated_at = excluded.updated_at
RETURNING id, tenant_id, kind, parent_id, path, label, metadata, created_at, updated_at
`, scope.ID, scope.TenantID, scope.Kind, nilIfEmpty(scope.ParentID), scope.Path, scope.Label, string(metadata), scope.CreatedAt, scope.UpdatedAt)
	return scanBeliefScope(row)
}

func (s *sqliteBeliefStore) GetScope(ctx context.Context, tenantID int64, kind belief.ScopeKind, path string) (belief.Scope, bool, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Scope{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, kind, parent_id, path, label, metadata, created_at, updated_at
FROM belief_scopes
WHERE tenant_id = ? AND kind = ? AND path = ?
`, normalizeTenantID(tenantID), normalizeScopeKind(kind), strings.TrimSpace(path))
	scope, err := scanBeliefScope(row)
	if errors.Is(err, sql.ErrNoRows) {
		return belief.Scope{}, false, nil
	}
	if err != nil {
		return belief.Scope{}, false, err
	}
	return scope, true, nil
}

func (s *sqliteBeliefStore) GetScopeByID(ctx context.Context, tenantID int64, id string) (belief.Scope, bool, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Scope{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, kind, parent_id, path, label, metadata, created_at, updated_at
FROM belief_scopes
WHERE tenant_id = ? AND id = ?
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	scope, err := scanBeliefScope(row)
	if errors.Is(err, sql.ErrNoRows) {
		return belief.Scope{}, false, nil
	}
	if err != nil {
		return belief.Scope{}, false, err
	}
	return scope, true, nil
}

func (s *sqliteBeliefStore) UpsertEpisode(ctx context.Context, episode belief.Episode) (belief.Episode, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Episode{}, err
	}
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
	if episode.Metadata == nil {
		episode.Metadata = map[string]any{}
	}
	metadata, err := marshalJSONMap(episode.Metadata)
	if err != nil {
		return belief.Episode{}, err
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO belief_episodes(
	id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role, user_id,
	started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	scope_id = excluded.scope_id,
	project_id = excluded.project_id,
	objective_id = excluded.objective_id,
	session_id = excluded.session_id,
	agent_role = excluded.agent_role,
	user_id = excluded.user_id,
	started_at = excluded.started_at,
	ended_at = excluded.ended_at,
	outcome = excluded.outcome,
	outcome_signal = excluded.outcome_signal,
	evolving_entry_id = excluded.evolving_entry_id,
	metadata = excluded.metadata
RETURNING id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role, user_id,
	started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
`, episode.ID, episode.TenantID, episode.ScopeID, episode.ProjectID, episode.ObjectiveID, episode.SessionID, episode.AgentRole, episode.UserID, episode.StartedAt, episode.EndedAt, episode.Outcome, episode.OutcomeSignal, nilIfEmpty(episode.EvolvingEntryID), string(metadata))
	return scanBeliefEpisode(row)
}

func (s *sqliteBeliefStore) GetEpisode(ctx context.Context, tenantID int64, id string) (belief.Episode, bool, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Episode{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, scope_id, project_id, objective_id, session_id, agent_role, user_id,
	started_at, ended_at, outcome, outcome_signal, evolving_entry_id, metadata
FROM belief_episodes
WHERE tenant_id = ? AND id = ?
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	episode, err := scanBeliefEpisode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return belief.Episode{}, false, nil
	}
	if err != nil {
		return belief.Episode{}, false, err
	}
	return episode, true, nil
}

func (s *sqliteBeliefStore) UpsertBelief(ctx context.Context, item belief.Belief) (belief.Belief, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Belief{}, err
	}
	item.TenantID = normalizeTenantID(item.TenantID)
	item.Status = normalizeBeliefStatus(item.Status)
	item.Kind = belief.NormalizeBeliefKind(item.Kind)
	item.Enforcement = belief.NormalizeEnforcement(item.Enforcement)
	item.ReviewState = belief.NormalizeReviewState(item.ReviewState)
	if strings.TrimSpace(item.ID) == "" {
		item.ID = uuid.NewString()
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	metadata, err := marshalJSONMap(item.Metadata)
	if err != nil {
		return belief.Belief{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return belief.Belief{}, err
	}
	defer rollbackQuietly(tx)

	row := tx.QueryRowContext(ctx, `
INSERT INTO beliefs(
	id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
	evidence_against, status, last_observed, expires_at, embedding, metadata,
	kind, enforcement, source_quality, review_state, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(tenant_id, scope_id, statement_hash) DO UPDATE SET
	statement = excluded.statement,
	confidence = excluded.confidence,
	evidence_for = excluded.evidence_for,
	evidence_against = excluded.evidence_against,
	status = excluded.status,
	kind = excluded.kind,
	enforcement = excluded.enforcement,
	source_quality = excluded.source_quality,
	review_state = excluded.review_state,
	last_observed = excluded.last_observed,
	expires_at = excluded.expires_at,
	embedding = excluded.embedding,
	metadata = excluded.metadata,
	updated_at = excluded.updated_at
RETURNING id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
	evidence_against, status, kind, enforcement, source_quality, review_state,
	last_observed, expires_at, metadata, created_at, updated_at
`, item.ID, item.TenantID, item.ScopeID, item.Statement, item.StatementHash, item.Confidence, item.EvidenceFor, item.EvidenceAgainst, item.Status, item.LastObserved, item.ExpiresAt, encodeJSON(item.Embedding, "[]"), string(metadata), item.Kind, item.Enforcement, item.SourceQuality, item.ReviewState, item.CreatedAt, item.UpdatedAt)
	stored, err := scanBelief(row)
	if err != nil {
		return belief.Belief{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM beliefs_fts WHERE id = ?`, stored.ID); err != nil {
		return belief.Belief{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO beliefs_fts(id, statement) VALUES(?, ?)`, stored.ID, stored.Statement); err != nil {
		return belief.Belief{}, err
	}
	if err := tx.Commit(); err != nil {
		return belief.Belief{}, err
	}
	stored.Embedding = append([]float32(nil), item.Embedding...)
	return stored, nil
}

func (s *sqliteBeliefStore) GetBelief(ctx context.Context, tenantID int64, id string) (belief.Belief, bool, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Belief{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
	evidence_against, status, kind, enforcement, source_quality, review_state,
	last_observed, expires_at, metadata, created_at, updated_at
FROM beliefs
WHERE tenant_id = ? AND id = ?
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	item, err := scanBelief(row)
	if errors.Is(err, sql.ErrNoRows) {
		return belief.Belief{}, false, nil
	}
	if err != nil {
		return belief.Belief{}, false, err
	}
	return item, true, nil
}

func (s *sqliteBeliefStore) SearchBeliefs(ctx context.Context, query belief.SearchQuery) ([]belief.SearchResult, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	whereArgs := []any{normalizeTenantID(query.TenantID)}
	clauses := []string{"tenant_id = ?"}
	if len(query.ScopeIDs) > 0 {
		scopeIDs := make([]string, 0, len(query.ScopeIDs))
		for _, scopeID := range query.ScopeIDs {
			if trimmed := strings.TrimSpace(scopeID); trimmed != "" {
				scopeIDs = append(scopeIDs, trimmed)
			}
		}
		if len(scopeIDs) > 0 {
			inSQL, inArgs := sqliteStringInClause(scopeIDs)
			clauses = append(clauses, "scope_id IN ("+inSQL+")")
			whereArgs = append(whereArgs, inArgs...)
		}
	}
	if len(query.Statuses) > 0 {
		statuses := make([]string, 0, len(query.Statuses))
		for _, status := range query.Statuses {
			statuses = append(statuses, string(normalizeBeliefStatus(status)))
		}
		inSQL, inArgs := sqliteStringInClause(statuses)
		clauses = append(clauses, "status IN ("+inSQL+")")
		whereArgs = append(whereArgs, inArgs...)
	}
	needle := strings.TrimSpace(query.Query)
	scoreExpr := "confidence"
	scoreArgs := []any{}
	if needle != "" {
		ftsQuery := sqliteFTSQuery(needle)
		like := "%" + strings.ToLower(needle) + "%"
		if ftsQuery != "" {
			clauses = append(clauses, "(id IN (SELECT id FROM beliefs_fts WHERE beliefs_fts MATCH ?) OR lower(statement) LIKE ?)")
			whereArgs = append(whereArgs, ftsQuery, like)
		} else {
			clauses = append(clauses, "lower(statement) LIKE ?")
			whereArgs = append(whereArgs, like)
		}
		scoreExpr = "confidence + CASE WHEN lower(statement) LIKE ? THEN 0.1 ELSE 0 END"
		scoreArgs = append(scoreArgs, like)
	}
	args := append(scoreArgs, whereArgs...)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
	evidence_against, status, kind, enforcement, source_quality, review_state,
	last_observed, expires_at, metadata, created_at, updated_at,
	%s AS score
FROM beliefs
WHERE %s
ORDER BY score DESC, updated_at DESC
LIMIT ?`, scoreExpr, strings.Join(clauses, " AND ")), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

func (s *sqliteBeliefStore) AddEvidence(ctx context.Context, evidence belief.Evidence) (belief.Evidence, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Evidence{}, err
	}
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
	if evidence.Metadata == nil {
		evidence.Metadata = map[string]any{}
	}
	if evidence.CreatedAt.IsZero() {
		evidence.CreatedAt = time.Now().UTC()
	}
	metadata, err := marshalJSONMap(evidence.Metadata)
	if err != nil {
		return belief.Evidence{}, err
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO belief_evidence(id, tenant_id, belief_id, episode_id, source_kind, source_id, polarity, weight, note, metadata, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	belief_id = excluded.belief_id,
	episode_id = excluded.episode_id,
	source_kind = excluded.source_kind,
	source_id = excluded.source_id,
	polarity = excluded.polarity,
	weight = excluded.weight,
	note = excluded.note,
	metadata = excluded.metadata,
	created_at = excluded.created_at
RETURNING id, tenant_id, belief_id, episode_id, source_kind, source_id, polarity, weight, note, metadata, created_at
`, evidence.ID, evidence.TenantID, evidence.BeliefID, nilIfEmpty(evidence.EpisodeID), evidence.SourceKind, evidence.SourceID, evidence.Polarity, evidence.Weight, evidence.Note, string(metadata), evidence.CreatedAt)
	return scanBeliefEvidence(row)
}

func (s *sqliteBeliefStore) ListEvidence(ctx context.Context, query belief.EvidenceQuery) ([]belief.Evidence, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	clauses := []string{"tenant_id = ?"}
	args := []any{normalizeTenantID(query.TenantID)}
	addStringClause := func(column, value string) {
		if strings.TrimSpace(value) != "" {
			clauses = append(clauses, column+" = ?")
			args = append(args, strings.TrimSpace(value))
		}
	}
	addStringClause("belief_id", query.BeliefID)
	addStringClause("episode_id", query.EpisodeID)
	addStringClause("source_id", query.SourceID)
	if query.Source != "" {
		clauses = append(clauses, "source_kind = ?")
		args = append(args, query.Source)
	}
	if query.Polarity != "" {
		clauses = append(clauses, "polarity = ?")
		args = append(args, query.Polarity)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT id, tenant_id, belief_id, episode_id, source_kind, source_id, polarity, weight, note, metadata, created_at
FROM belief_evidence
WHERE %s
ORDER BY created_at DESC
LIMIT ?`, strings.Join(clauses, " AND ")), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

func (s *sqliteBeliefStore) RecordPromotion(ctx context.Context, promotion belief.Promotion) (belief.Promotion, error) {
	if err := s.Init(ctx); err != nil {
		return belief.Promotion{}, err
	}
	promotion.TenantID = normalizeTenantID(promotion.TenantID)
	if strings.TrimSpace(promotion.ID) == "" {
		promotion.ID = uuid.NewString()
	}
	if promotion.Metadata == nil {
		promotion.Metadata = map[string]any{}
	}
	if promotion.CreatedAt.IsZero() {
		promotion.CreatedAt = time.Now().UTC()
	}
	metadata, err := marshalJSONMap(promotion.Metadata)
	if err != nil {
		return belief.Promotion{}, err
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO belief_promotions(id, tenant_id, belief_id, from_scope, to_scope, reason, confidence_before, confidence_after, actor_user_id, metadata, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	belief_id = excluded.belief_id,
	from_scope = excluded.from_scope,
	to_scope = excluded.to_scope,
	reason = excluded.reason,
	confidence_before = excluded.confidence_before,
	confidence_after = excluded.confidence_after,
	actor_user_id = excluded.actor_user_id,
	metadata = excluded.metadata,
	created_at = excluded.created_at
RETURNING id, tenant_id, belief_id, from_scope, to_scope, reason, confidence_before, confidence_after, actor_user_id, metadata, created_at
`, promotion.ID, promotion.TenantID, promotion.BeliefID, promotion.FromScope, promotion.ToScope, promotion.Reason, promotion.ConfidenceBefore, promotion.ConfidenceAfter, promotion.ActorUserID, string(metadata), promotion.CreatedAt)
	return scanBeliefPromotion(row)
}

func (s *sqliteBeliefStore) ListPromotions(ctx context.Context, query belief.PromotionQuery) ([]belief.Promotion, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	clauses := []string{"tenant_id = ?"}
	args := []any{normalizeTenantID(query.TenantID)}
	addStringClause := func(column, value string) {
		if strings.TrimSpace(value) != "" {
			clauses = append(clauses, column+" = ?")
			args = append(args, strings.TrimSpace(value))
		}
	}
	addStringClause("belief_id", query.BeliefID)
	addStringClause("from_scope", query.FromScope)
	addStringClause("to_scope", query.ToScope)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT id, tenant_id, belief_id, from_scope, to_scope, reason, confidence_before, confidence_after, actor_user_id, metadata, created_at
FROM belief_promotions
WHERE %s
ORDER BY created_at DESC
LIMIT ?`, strings.Join(clauses, " AND ")), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

func (s *sqliteBeliefStore) RecordCandidate(ctx context.Context, candidate belief.CandidateRecord) (belief.CandidateRecord, error) {
	if err := s.Init(ctx); err != nil {
		return belief.CandidateRecord{}, err
	}
	candidate.TenantID = normalizeTenantID(candidate.TenantID)
	candidate.Kind = belief.NormalizeBeliefKind(candidate.Kind)
	candidate.Enforcement = belief.NormalizeEnforcement(candidate.Enforcement)
	candidate.ReviewState = belief.NormalizeReviewState(candidate.ReviewState)
	candidate.ValidationStatus = belief.NormalizeCandidateValidationStatus(candidate.ValidationStatus)
	if strings.TrimSpace(candidate.ID) == "" {
		candidate.ID = uuid.NewString()
	}
	if strings.TrimSpace(candidate.StatementHash) == "" && strings.TrimSpace(candidate.Statement) != "" {
		candidate.StatementHash = belief.StatementHash(candidate.Statement)
	}
	if candidate.NormalizedPayload == nil {
		candidate.NormalizedPayload = map[string]any{}
	}
	if candidate.Metadata == nil {
		candidate.Metadata = map[string]any{}
	}
	now := time.Now().UTC()
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = now
	}
	candidate.UpdatedAt = now
	normalizedPayload, err := marshalJSONMap(candidate.NormalizedPayload)
	if err != nil {
		return belief.CandidateRecord{}, err
	}
	metadata, err := marshalJSONMap(candidate.Metadata)
	if err != nil {
		return belief.CandidateRecord{}, err
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO belief_candidates(
	id, tenant_id, episode_id, scope_id, statement, statement_hash, kind, enforcement,
	polarity, confidence, source_quality, review_state, evidence_note, raw_payload,
	normalized_payload, validation_status, rejection_reason, accepted_belief_id, model,
	metadata, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	episode_id = excluded.episode_id,
	scope_id = excluded.scope_id,
	statement = excluded.statement,
	statement_hash = excluded.statement_hash,
	kind = excluded.kind,
	enforcement = excluded.enforcement,
	polarity = excluded.polarity,
	confidence = excluded.confidence,
	source_quality = excluded.source_quality,
	review_state = excluded.review_state,
	evidence_note = excluded.evidence_note,
	raw_payload = excluded.raw_payload,
	normalized_payload = excluded.normalized_payload,
	validation_status = excluded.validation_status,
	rejection_reason = excluded.rejection_reason,
	accepted_belief_id = excluded.accepted_belief_id,
	model = excluded.model,
	metadata = excluded.metadata,
	updated_at = excluded.updated_at
RETURNING id, tenant_id, episode_id, scope_id, statement, statement_hash, kind, enforcement,
	polarity, confidence, source_quality, review_state, evidence_note, raw_payload,
	normalized_payload, validation_status, rejection_reason, accepted_belief_id, model,
	metadata, created_at, updated_at
`, candidate.ID, candidate.TenantID, nilIfEmpty(candidate.EpisodeID), nilIfEmpty(candidate.ScopeID), candidate.Statement, candidate.StatementHash, candidate.Kind, candidate.Enforcement, candidate.Polarity, candidate.Confidence, candidate.SourceQuality, candidate.ReviewState, candidate.EvidenceNote, candidate.RawPayload, string(normalizedPayload), candidate.ValidationStatus, candidate.RejectionReason, nilIfEmpty(candidate.AcceptedBeliefID), candidate.Model, string(metadata), candidate.CreatedAt, candidate.UpdatedAt)
	return scanBeliefCandidate(row)
}

func (s *sqliteBeliefStore) GetCandidate(ctx context.Context, tenantID int64, id string) (belief.CandidateRecord, bool, error) {
	if err := s.Init(ctx); err != nil {
		return belief.CandidateRecord{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, episode_id, scope_id, statement, statement_hash, kind, enforcement,
	polarity, confidence, source_quality, review_state, evidence_note, raw_payload,
	normalized_payload, validation_status, rejection_reason, accepted_belief_id, model,
	metadata, created_at, updated_at
FROM belief_candidates
WHERE tenant_id = ? AND id = ?
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	candidate, err := scanBeliefCandidate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return belief.CandidateRecord{}, false, nil
	}
	if err != nil {
		return belief.CandidateRecord{}, false, err
	}
	return candidate, true, nil
}

func (s *sqliteBeliefStore) ListCandidates(ctx context.Context, query belief.CandidateQuery) ([]belief.CandidateRecord, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	clauses := []string{"tenant_id = ?"}
	args := []any{normalizeTenantID(query.TenantID)}
	if strings.TrimSpace(query.EpisodeID) != "" {
		clauses = append(clauses, "episode_id = ?")
		args = append(args, strings.TrimSpace(query.EpisodeID))
	}
	if query.ReviewState != "" {
		clauses = append(clauses, "review_state = ?")
		args = append(args, belief.NormalizeReviewState(query.ReviewState))
	}
	if query.ValidationStatus != "" {
		clauses = append(clauses, "validation_status = ?")
		args = append(args, belief.NormalizeCandidateValidationStatus(query.ValidationStatus))
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT id, tenant_id, episode_id, scope_id, statement, statement_hash, kind, enforcement,
	polarity, confidence, source_quality, review_state, evidence_note, raw_payload,
	normalized_payload, validation_status, rejection_reason, accepted_belief_id, model,
	metadata, created_at, updated_at
FROM belief_candidates
WHERE %s
ORDER BY created_at DESC
LIMIT ?`, strings.Join(clauses, " AND ")), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]belief.CandidateRecord, 0, limit)
	for rows.Next() {
		candidate, err := scanBeliefCandidate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}
