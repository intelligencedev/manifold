package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"manifold/internal/agent/memory/decision"

	"github.com/google/uuid"
)

type sqliteDecisionStore struct {
	db *sql.DB
}

// NewSQLiteDecisionStore returns a SQLite-backed decision store.
func NewSQLiteDecisionStore(db *sql.DB) decision.Store {
	if db == nil {
		return NewMemoryDecisionStore()
	}
	return &sqliteDecisionStore{db: db}
}

func (s *sqliteDecisionStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite decision store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS decision_records (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	scope_id TEXT NOT NULL,
	episode_id TEXT,
	title TEXT NOT NULL,
	statement TEXT NOT NULL,
	statement_hash TEXT NOT NULL,
	rationale TEXT NOT NULL DEFAULT '',
	decided_by TEXT NOT NULL DEFAULT '',
	decided_at DATETIME NOT NULL,
	status TEXT NOT NULL,
	review_state TEXT NOT NULL,
	confidence REAL NOT NULL DEFAULT 0,
	superseded_by TEXT,
	stale_reason TEXT,
	embedding BLOB,
	metadata TEXT,
	payload TEXT NOT NULL CHECK(json_valid(payload)),
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS decision_records_tenant_scope_hash
	ON decision_records(tenant_id, scope_id, statement_hash);
CREATE INDEX IF NOT EXISTS decision_records_tenant_status
	ON decision_records(tenant_id, status);

CREATE TABLE IF NOT EXISTS decision_assumptions (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	decision_id TEXT NOT NULL,
	belief_id TEXT NOT NULL,
	criticality TEXT NOT NULL,
	belief_statement_at_link TEXT NOT NULL DEFAULT '',
	belief_confidence_at_link REAL NOT NULL DEFAULT 0,
	note TEXT NOT NULL DEFAULT '',
	metadata TEXT,
	payload TEXT NOT NULL CHECK(json_valid(payload)),
	created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS decision_assumptions_belief
	ON decision_assumptions(tenant_id, belief_id);
CREATE INDEX IF NOT EXISTS decision_assumptions_decision
	ON decision_assumptions(tenant_id, decision_id);

CREATE TABLE IF NOT EXISTS decision_alternatives (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	decision_id TEXT NOT NULL,
	statement TEXT NOT NULL,
	rejection_reason TEXT NOT NULL DEFAULT '',
	metadata TEXT,
	payload TEXT NOT NULL CHECK(json_valid(payload)),
	created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS decision_alternatives_decision
	ON decision_alternatives(tenant_id, decision_id);

CREATE TABLE IF NOT EXISTS decision_evidence (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	decision_id TEXT NOT NULL,
	source_kind TEXT NOT NULL,
	source_id TEXT NOT NULL,
	polarity TEXT NOT NULL,
	weight REAL NOT NULL DEFAULT 1,
	note TEXT NOT NULL DEFAULT '',
	metadata TEXT,
	payload TEXT NOT NULL CHECK(json_valid(payload)),
	created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS decision_evidence_source
	ON decision_evidence(tenant_id, source_kind, source_id);
CREATE INDEX IF NOT EXISTS decision_evidence_decision
	ON decision_evidence(tenant_id, decision_id);

CREATE TABLE IF NOT EXISTS decision_transitions (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	decision_id TEXT NOT NULL,
	from_status TEXT NOT NULL,
	to_status TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	trigger_kind TEXT NOT NULL DEFAULT '',
	trigger_id TEXT NOT NULL DEFAULT '',
	actor_user_id INTEGER,
	payload TEXT NOT NULL CHECK(json_valid(payload)),
	created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS decision_transitions_decision
	ON decision_transitions(tenant_id, decision_id, created_at);

CREATE TABLE IF NOT EXISTS decision_candidates (
	id TEXT PRIMARY KEY,
	tenant_id INTEGER NOT NULL,
	episode_id TEXT NOT NULL DEFAULT '',
	scope_id TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL,
	statement TEXT NOT NULL,
	statement_hash TEXT NOT NULL,
	rationale TEXT NOT NULL DEFAULT '',
	payload TEXT NOT NULL CHECK(json_valid(payload)),
	confidence REAL NOT NULL DEFAULT 0,
	review_state TEXT NOT NULL,
	validation_status TEXT NOT NULL,
	rejection_reason TEXT,
	accepted_decision_id TEXT,
	model TEXT,
	raw_payload TEXT,
	metadata TEXT,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
`)
	return err
}

func (s *sqliteDecisionStore) UpsertDecision(ctx context.Context, item decision.Decision) (decision.Decision, error) {
	if err := s.Init(ctx); err != nil {
		return decision.Decision{}, err
	}
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
	metadata, _ := json.Marshal(item.Metadata)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO decision_records(id, tenant_id, scope_id, episode_id, title, statement, statement_hash, rationale, decided_by, decided_at, status, review_state, confidence, superseded_by, stale_reason, metadata, payload, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	scope_id=excluded.scope_id,
	episode_id=excluded.episode_id,
	title=excluded.title,
	statement=excluded.statement,
	statement_hash=excluded.statement_hash,
	rationale=excluded.rationale,
	decided_by=excluded.decided_by,
	decided_at=excluded.decided_at,
	status=excluded.status,
	review_state=excluded.review_state,
	confidence=excluded.confidence,
	superseded_by=excluded.superseded_by,
	stale_reason=excluded.stale_reason,
	metadata=excluded.metadata,
	payload=excluded.payload,
	updated_at=excluded.updated_at
`, item.ID, item.TenantID, item.ScopeID, item.EpisodeID, item.Title, item.Statement, item.StatementHash, item.Rationale, item.DecidedBy, sqliteTime{Time: item.DecidedAt}, item.Status, item.ReviewState, item.Confidence, nullableString(item.SupersededBy), nullableString(item.StaleReason), string(metadata), string(payload), sqliteTime{Time: item.CreatedAt}, sqliteTime{Time: item.UpdatedAt})
	if err != nil {
		return decision.Decision{}, err
	}
	return cloneDecision(item), nil
}

func (s *sqliteDecisionStore) GetDecision(ctx context.Context, tenantID int64, id string) (decision.Decision, bool, error) {
	if err := s.Init(ctx); err != nil {
		return decision.Decision{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT payload FROM decision_records WHERE tenant_id = ? AND id = ?`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	item, ok, err := scanJSONRow[decision.Decision](row)
	if err != nil || !ok {
		return decision.Decision{}, ok, err
	}
	return cloneDecision(item), true, nil
}

func (s *sqliteDecisionStore) SearchDecisions(ctx context.Context, query decision.SearchQuery) ([]decision.SearchResult, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM decision_records WHERE tenant_id = ?`, normalizeTenantID(query.TenantID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	scopeSet := stringSet(query.ScopeIDs)
	statusSet := map[decision.DecisionStatus]bool{}
	for _, status := range query.Statuses {
		statusSet[decision.NormalizeDecisionStatus(status)] = true
	}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	out := []decision.SearchResult{}
	for rows.Next() {
		item, err := scanJSONRows[decision.Decision](rows)
		if err != nil {
			return nil, err
		}
		if len(scopeSet) > 0 && !scopeSet[item.ScopeID] {
			continue
		}
		if len(statusSet) > 0 && !statusSet[decision.NormalizeDecisionStatus(item.Status)] {
			continue
		}
		text := strings.ToLower(item.Title + " " + item.Statement + " " + item.Rationale)
		if needle != "" && !strings.Contains(text, needle) && item.StatementHash != needle {
			continue
		}
		score := item.Confidence
		if needle != "" {
			score += 0.1
		}
		out = append(out, decision.SearchResult{Decision: cloneDecision(item), Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Decision.UpdatedAt.After(out[j].Decision.UpdatedAt)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *sqliteDecisionStore) AddAssumption(ctx context.Context, link decision.AssumptionLink) (decision.AssumptionLink, error) {
	if err := s.Init(ctx); err != nil {
		return decision.AssumptionLink{}, err
	}
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
	metadata, _ := json.Marshal(link.Metadata)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO decision_assumptions(id, tenant_id, decision_id, belief_id, criticality, belief_statement_at_link, belief_confidence_at_link, note, metadata, payload, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET criticality=excluded.criticality, belief_statement_at_link=excluded.belief_statement_at_link, belief_confidence_at_link=excluded.belief_confidence_at_link, note=excluded.note, metadata=excluded.metadata, payload=excluded.payload
`, link.ID, link.TenantID, link.DecisionID, link.BeliefID, link.Criticality, link.BeliefStatementAtLink, link.BeliefConfidenceAtLink, link.Note, string(metadata), string(payload), sqliteTime{Time: link.CreatedAt})
	return cloneAssumption(link), err
}

func (s *sqliteDecisionStore) ListAssumptions(ctx context.Context, query decision.AssumptionQuery) ([]decision.AssumptionLink, error) {
	return listDecisionJSON[decision.AssumptionLink](ctx, s.db, s.Init, `SELECT payload FROM decision_assumptions WHERE tenant_id = ? ORDER BY created_at ASC`, normalizeTenantID(query.TenantID), query.Limit, func(link decision.AssumptionLink) bool {
		return (query.DecisionID == "" || link.DecisionID == strings.TrimSpace(query.DecisionID)) &&
			(query.BeliefID == "" || link.BeliefID == strings.TrimSpace(query.BeliefID))
	})
}

func (s *sqliteDecisionStore) AddAlternative(ctx context.Context, alt decision.Alternative) (decision.Alternative, error) {
	if err := s.Init(ctx); err != nil {
		return decision.Alternative{}, err
	}
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
	metadata, _ := json.Marshal(alt.Metadata)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO decision_alternatives(id, tenant_id, decision_id, statement, rejection_reason, metadata, payload, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET statement=excluded.statement, rejection_reason=excluded.rejection_reason, metadata=excluded.metadata, payload=excluded.payload
`, alt.ID, alt.TenantID, alt.DecisionID, alt.Statement, alt.RejectionReason, string(metadata), string(payload), sqliteTime{Time: alt.CreatedAt})
	return cloneAlternative(alt), err
}

func (s *sqliteDecisionStore) ListAlternatives(ctx context.Context, tenantID int64, decisionID string) ([]decision.Alternative, error) {
	return listDecisionJSON[decision.Alternative](ctx, s.db, s.Init, `SELECT payload FROM decision_alternatives WHERE tenant_id = ? ORDER BY created_at ASC`, normalizeTenantID(tenantID), 0, func(alt decision.Alternative) bool {
		return alt.DecisionID == strings.TrimSpace(decisionID)
	})
}

func (s *sqliteDecisionStore) AddEvidence(ctx context.Context, ev decision.DecisionEvidence) (decision.DecisionEvidence, error) {
	if err := s.Init(ctx); err != nil {
		return decision.DecisionEvidence{}, err
	}
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
	metadata, _ := json.Marshal(ev.Metadata)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO decision_evidence(id, tenant_id, decision_id, source_kind, source_id, polarity, weight, note, metadata, payload, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET source_kind=excluded.source_kind, source_id=excluded.source_id, polarity=excluded.polarity, weight=excluded.weight, note=excluded.note, metadata=excluded.metadata, payload=excluded.payload
`, ev.ID, ev.TenantID, ev.DecisionID, ev.SourceKind, ev.SourceID, ev.Polarity, ev.Weight, ev.Note, string(metadata), string(payload), sqliteTime{Time: ev.CreatedAt})
	return cloneDecisionEvidence(ev), err
}

func (s *sqliteDecisionStore) ListEvidence(ctx context.Context, query decision.EvidenceQuery) ([]decision.DecisionEvidence, error) {
	return listDecisionJSON[decision.DecisionEvidence](ctx, s.db, s.Init, `SELECT payload FROM decision_evidence WHERE tenant_id = ? ORDER BY created_at ASC`, normalizeTenantID(query.TenantID), query.Limit, func(ev decision.DecisionEvidence) bool {
		return (query.DecisionID == "" || ev.DecisionID == strings.TrimSpace(query.DecisionID)) &&
			(query.SourceKind == "" || ev.SourceKind == strings.TrimSpace(query.SourceKind)) &&
			(query.SourceID == "" || ev.SourceID == strings.TrimSpace(query.SourceID)) &&
			(query.Polarity == "" || ev.Polarity == decision.NormalizeEvidencePolarity(query.Polarity))
	})
}

func (s *sqliteDecisionStore) RecordTransition(ctx context.Context, tr decision.Transition) (decision.Transition, error) {
	if err := s.Init(ctx); err != nil {
		return decision.Transition{}, err
	}
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
	_, err = s.db.ExecContext(ctx, `
INSERT INTO decision_transitions(id, tenant_id, decision_id, from_status, to_status, reason, trigger_kind, trigger_id, actor_user_id, payload, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET payload=excluded.payload
`, tr.ID, tr.TenantID, tr.DecisionID, tr.FromStatus, tr.ToStatus, tr.Reason, tr.TriggerKind, tr.TriggerID, actor, string(payload), sqliteTime{Time: tr.CreatedAt})
	return cloneTransition(tr), err
}

func (s *sqliteDecisionStore) ListTransitions(ctx context.Context, query decision.TransitionQuery) ([]decision.Transition, error) {
	return listDecisionJSON[decision.Transition](ctx, s.db, s.Init, `SELECT payload FROM decision_transitions WHERE tenant_id = ? ORDER BY created_at ASC`, normalizeTenantID(query.TenantID), query.Limit, func(tr decision.Transition) bool {
		return query.DecisionID == "" || tr.DecisionID == strings.TrimSpace(query.DecisionID)
	})
}

func (s *sqliteDecisionStore) RecordCandidate(ctx context.Context, c decision.Candidate) (decision.Candidate, error) {
	if err := s.Init(ctx); err != nil {
		return decision.Candidate{}, err
	}
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
	metadata, _ := json.Marshal(c.Metadata)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO decision_candidates(id, tenant_id, episode_id, scope_id, title, statement, statement_hash, rationale, payload, confidence, review_state, validation_status, rejection_reason, accepted_decision_id, model, raw_payload, metadata, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	title=excluded.title,
	statement=excluded.statement,
	statement_hash=excluded.statement_hash,
	rationale=excluded.rationale,
	payload=excluded.payload,
	confidence=excluded.confidence,
	review_state=excluded.review_state,
	validation_status=excluded.validation_status,
	rejection_reason=excluded.rejection_reason,
	accepted_decision_id=excluded.accepted_decision_id,
	model=excluded.model,
	raw_payload=excluded.raw_payload,
	metadata=excluded.metadata,
	updated_at=excluded.updated_at
`, c.ID, c.TenantID, c.EpisodeID, c.ScopeID, c.Title, c.Statement, c.StatementHash, c.Rationale, string(payload), c.Confidence, c.ReviewState, c.ValidationStatus, nullableString(c.RejectionReason), nullableString(c.AcceptedDecisionID), nullableString(c.Model), nullableString(c.RawPayload), string(metadata), sqliteTime{Time: c.CreatedAt}, sqliteTime{Time: c.UpdatedAt})
	return cloneCandidate(c), err
}

func (s *sqliteDecisionStore) GetCandidate(ctx context.Context, tenantID int64, id string) (decision.Candidate, bool, error) {
	if err := s.Init(ctx); err != nil {
		return decision.Candidate{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT payload FROM decision_candidates WHERE tenant_id = ? AND id = ?`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	c, ok, err := scanJSONRow[decision.Candidate](row)
	if err != nil || !ok {
		return decision.Candidate{}, ok, err
	}
	return cloneCandidate(c), true, nil
}

func (s *sqliteDecisionStore) ListCandidates(ctx context.Context, query decision.CandidateQuery) ([]decision.Candidate, error) {
	return listDecisionJSON[decision.Candidate](ctx, s.db, s.Init, `SELECT payload FROM decision_candidates WHERE tenant_id = ? ORDER BY created_at ASC`, normalizeTenantID(query.TenantID), query.Limit, func(c decision.Candidate) bool {
		return (query.EpisodeID == "" || c.EpisodeID == strings.TrimSpace(query.EpisodeID)) &&
			(query.ReviewState == "" || c.ReviewState == decision.NormalizeReviewState(query.ReviewState)) &&
			(query.ValidationStatus == "" || c.ValidationStatus == decision.NormalizeCandidateValidationStatus(query.ValidationStatus))
	})
}

func (s *sqliteDecisionStore) findDecisionIDByHash(ctx context.Context, tenantID int64, scopeID, hash string) (string, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM decision_records WHERE tenant_id = ? AND scope_id = ? AND statement_hash = ?`, normalizeTenantID(tenantID), strings.TrimSpace(scopeID), strings.TrimSpace(hash)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

type payloadScanner interface {
	Scan(dest ...any) error
}

func scanJSONRow[T any](row payloadScanner) (T, bool, error) {
	var zero T
	var payload string
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return zero, false, nil
		}
		return zero, false, err
	}
	var out T
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return zero, false, err
	}
	return out, true, nil
}

func scanJSONRows[T any](rows *sql.Rows) (T, error) {
	var zero T
	var payload string
	if err := rows.Scan(&payload); err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return zero, err
	}
	return out, nil
}

func listDecisionJSON[T any](ctx context.Context, db *sql.DB, init func(context.Context) error, query string, tenantID int64, limit int, keep func(T) bool) ([]T, error) {
	if err := init(ctx); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if limit <= 0 {
		limit = 100
	}
	out := make([]T, 0, limit)
	for rows.Next() {
		item, err := scanJSONRows[T](rows)
		if err != nil {
			return nil, err
		}
		if keep != nil && !keep(item) {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out[trimmed] = true
		}
	}
	return out
}
