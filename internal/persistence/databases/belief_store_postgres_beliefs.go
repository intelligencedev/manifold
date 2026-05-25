package databases

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"manifold/internal/agent/belief"
)

func (s *pgBeliefStore) UpsertBelief(ctx context.Context, item belief.Belief) (belief.Belief, error) {
	item.TenantID = normalizeTenantID(item.TenantID)
	item.Status = normalizeBeliefStatus(item.Status)
	item.Kind = belief.NormalizeBeliefKind(item.Kind)
	item.Enforcement = belief.NormalizeEnforcement(item.Enforcement)
	item.ReviewState = belief.NormalizeReviewState(item.ReviewState)
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
    evidence_against, status, last_observed, expires_at, embedding_vec, metadata,
    kind, enforcement, source_quality, review_state
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CASE WHEN $12 = '' THEN NULL ELSE $12::vector END, $13, $14, $15, $16, $17)
ON CONFLICT (tenant_id, scope_id, statement_hash) DO UPDATE SET
    statement = EXCLUDED.statement,
    confidence = EXCLUDED.confidence,
    evidence_for = EXCLUDED.evidence_for,
    evidence_against = EXCLUDED.evidence_against,
    status = EXCLUDED.status,
    kind = EXCLUDED.kind,
    enforcement = EXCLUDED.enforcement,
    source_quality = EXCLUDED.source_quality,
    review_state = EXCLUDED.review_state,
    last_observed = EXCLUDED.last_observed,
    expires_at = EXCLUDED.expires_at,
    embedding_vec = EXCLUDED.embedding_vec,
    metadata = EXCLUDED.metadata,
    updated_at = NOW()
RETURNING id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
    evidence_against, status, kind, enforcement, source_quality, review_state,
    last_observed, expires_at, metadata, created_at, updated_at
`, item.ID, item.TenantID, item.ScopeID, item.Statement, item.StatementHash, item.Confidence, item.EvidenceFor, item.EvidenceAgainst, item.Status, item.LastObserved, item.ExpiresAt, vectorLiteralOrEmpty(item.Embedding), metadata, item.Kind, item.Enforcement, item.SourceQuality, item.ReviewState)
	return scanBelief(row)
}

func (s *pgBeliefStore) GetBelief(ctx context.Context, tenantID int64, id string) (belief.Belief, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, tenant_id, scope_id, statement, statement_hash, confidence, evidence_for,
    evidence_against, status, kind, enforcement, source_quality, review_state,
    last_observed, expires_at, metadata, created_at, updated_at
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
    evidence_against, status, kind, enforcement, source_quality, review_state,
    last_observed, expires_at, metadata, created_at, updated_at,
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
