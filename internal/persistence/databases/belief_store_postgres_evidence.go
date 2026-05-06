package databases

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"manifold/internal/agent/belief"
)

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
