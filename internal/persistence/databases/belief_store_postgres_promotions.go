package databases

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"manifold/internal/agent/memory/belief"
)

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
