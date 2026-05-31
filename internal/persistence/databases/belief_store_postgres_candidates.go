package databases

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"manifold/internal/agent/memory/belief"
)

func (s *pgBeliefStore) RecordCandidate(ctx context.Context, candidate belief.CandidateRecord) (belief.CandidateRecord, error) {
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
	normalized, err := marshalJSONMap(candidate.NormalizedPayload)
	if err != nil {
		return belief.CandidateRecord{}, err
	}
	metadata, err := marshalJSONMap(candidate.Metadata)
	if err != nil {
		return belief.CandidateRecord{}, err
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO belief_candidates (
    id, tenant_id, episode_id, scope_id, statement, statement_hash, kind, enforcement,
    polarity, confidence, source_quality, review_state, evidence_note, raw_payload,
    normalized_payload, validation_status, rejection_reason, accepted_belief_id, model, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (id) DO UPDATE SET
    statement = EXCLUDED.statement,
    statement_hash = EXCLUDED.statement_hash,
    kind = EXCLUDED.kind,
    enforcement = EXCLUDED.enforcement,
    polarity = EXCLUDED.polarity,
    confidence = EXCLUDED.confidence,
    source_quality = EXCLUDED.source_quality,
    review_state = EXCLUDED.review_state,
    evidence_note = EXCLUDED.evidence_note,
    raw_payload = EXCLUDED.raw_payload,
    normalized_payload = EXCLUDED.normalized_payload,
    validation_status = EXCLUDED.validation_status,
    rejection_reason = EXCLUDED.rejection_reason,
    accepted_belief_id = EXCLUDED.accepted_belief_id,
    model = EXCLUDED.model,
    metadata = EXCLUDED.metadata,
    updated_at = NOW()
RETURNING id, tenant_id, episode_id, scope_id, statement, statement_hash, kind, enforcement,
    polarity, confidence, source_quality, review_state, evidence_note, raw_payload,
    normalized_payload, validation_status, rejection_reason, accepted_belief_id, model,
    metadata, created_at, updated_at
`, candidate.ID, candidate.TenantID, nilIfEmpty(candidate.EpisodeID), nilIfEmpty(candidate.ScopeID), candidate.Statement, candidate.StatementHash, candidate.Kind, candidate.Enforcement, candidate.Polarity, candidate.Confidence, candidate.SourceQuality, candidate.ReviewState, candidate.EvidenceNote, candidate.RawPayload, normalized, candidate.ValidationStatus, candidate.RejectionReason, nilIfEmpty(candidate.AcceptedBeliefID), candidate.Model, metadata)
	return scanBeliefCandidate(row)
}

func (s *pgBeliefStore) GetCandidate(ctx context.Context, tenantID int64, id string) (belief.CandidateRecord, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, tenant_id, episode_id, scope_id, statement, statement_hash, kind, enforcement,
    polarity, confidence, source_quality, review_state, evidence_note, raw_payload,
    normalized_payload, validation_status, rejection_reason, accepted_belief_id, model,
    metadata, created_at, updated_at
FROM belief_candidates
WHERE tenant_id = $1 AND id = $2
`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	candidate, err := scanBeliefCandidate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return belief.CandidateRecord{}, false, nil
	}
	if err != nil {
		return belief.CandidateRecord{}, false, err
	}
	return candidate, true, nil
}

func (s *pgBeliefStore) ListCandidates(ctx context.Context, query belief.CandidateQuery) ([]belief.CandidateRecord, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, tenant_id, episode_id, scope_id, statement, statement_hash, kind, enforcement,
    polarity, confidence, source_quality, review_state, evidence_note, raw_payload,
    normalized_payload, validation_status, rejection_reason, accepted_belief_id, model,
    metadata, created_at, updated_at
FROM belief_candidates
WHERE tenant_id = $1
    AND ($2 = '' OR episode_id::text = $2)
    AND ($3 = '' OR review_state = $3)
    AND ($4 = '' OR validation_status = $4)
ORDER BY created_at DESC
LIMIT $5
`, normalizeTenantID(query.TenantID), strings.TrimSpace(query.EpisodeID), strings.TrimSpace(string(query.ReviewState)), strings.TrimSpace(string(query.ValidationStatus)), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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
