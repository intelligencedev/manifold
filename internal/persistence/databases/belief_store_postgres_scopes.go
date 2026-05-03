package databases

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"manifold/internal/agent/belief"
)

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
