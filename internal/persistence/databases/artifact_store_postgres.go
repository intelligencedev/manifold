package databases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"manifold/internal/agent/memory/artifact"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresArtifactStore returns the postgres artifact store.
func NewPostgresArtifactStore(pool *pgxpool.Pool) artifact.Store {
	if pool == nil {
		return NewMemoryArtifactStore()
	}
	return &pgArtifactStore{pool: pool}
}

type pgArtifactStore struct {
	pool *pgxpool.Pool
}

func (s *pgArtifactStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *pgArtifactStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres artifact store requires pool")
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS artifact_records (
	id TEXT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	kind TEXT NOT NULL,
	external_id TEXT NOT NULL,
	uri TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL DEFAULT '',
	excerpt TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL,
	authored_by TEXT NOT NULL DEFAULT '',
	authored_at TIMESTAMPTZ,
	captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	search_tsv tsvector GENERATED ALWAYS AS (
		to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(excerpt, '') || ' ' || coalesce(external_id, ''))
	) STORED
);
CREATE UNIQUE INDEX IF NOT EXISTS artifact_records_immutable_source
	ON artifact_records(tenant_id, kind, external_id, content_hash);
CREATE INDEX IF NOT EXISTS artifact_records_external_id
	ON artifact_records(tenant_id, kind, external_id);
CREATE INDEX IF NOT EXISTS artifact_records_search_tsv_idx
	ON artifact_records USING GIN(search_tsv);
`)
	return err
}

func (s *pgArtifactStore) UpsertArtifact(ctx context.Context, item artifact.Artifact) (artifact.Artifact, error) {
	item = prepareArtifact(item)
	if existingID, ok, err := s.findBySourceHash(ctx, item); err != nil {
		return artifact.Artifact{}, err
	} else if ok {
		item.ID = existingID
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return artifact.Artifact{}, err
	}
	metadata, err := json.Marshal(item.Metadata)
	if err != nil {
		return artifact.Artifact{}, err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO artifact_records(id, tenant_id, kind, external_id, uri, title, excerpt, content_hash, authored_by, authored_at, captured_at, metadata, payload)
VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb)
ON CONFLICT(id) DO UPDATE SET
	uri=EXCLUDED.uri,
	title=EXCLUDED.title,
	excerpt=EXCLUDED.excerpt,
	authored_by=EXCLUDED.authored_by,
	authored_at=EXCLUDED.authored_at,
	captured_at=EXCLUDED.captured_at,
	metadata=EXCLUDED.metadata,
	payload=EXCLUDED.payload
`, item.ID, item.TenantID, item.Kind, item.ExternalID, item.URI, item.Title, item.Excerpt, item.ContentHash, item.AuthoredBy, item.AuthoredAt, item.CapturedAt, string(metadata), string(payload))
	if err != nil {
		return artifact.Artifact{}, err
	}
	return cloneArtifact(item), nil
}

func (s *pgArtifactStore) GetArtifact(ctx context.Context, tenantID int64, id string) (artifact.Artifact, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT payload FROM artifact_records WHERE tenant_id = $1 AND id = $2`, normalizeTenantID(tenantID), strings.TrimSpace(id))
	item, ok, err := scanPGJSONRow[artifact.Artifact](row)
	if err != nil || !ok {
		return artifact.Artifact{}, ok, err
	}
	return cloneArtifact(item), true, nil
}

func (s *pgArtifactStore) FindByExternalID(ctx context.Context, tenantID int64, kind artifact.ArtifactKind, externalID string) ([]artifact.Artifact, error) {
	rows, err := s.pool.Query(ctx, `
SELECT payload FROM artifact_records
WHERE tenant_id = $1 AND kind = $2 AND external_id = $3
ORDER BY captured_at ASC
`, normalizeTenantID(tenantID), kind, strings.TrimSpace(externalID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanPGJSONRows[artifact.Artifact](rows)
	if err != nil {
		return nil, err
	}
	out := make([]artifact.Artifact, 0, len(items))
	for _, item := range items {
		out = append(out, cloneArtifact(item))
	}
	return out, nil
}

func (s *pgArtifactStore) SearchArtifacts(ctx context.Context, query artifact.SearchQuery) ([]artifact.SearchResult, error) {
	limit := normalizedLimit(query.Limit, 10)
	kinds := make([]string, 0, len(query.Kinds))
	for _, kind := range query.Kinds {
		kinds = append(kinds, string(kind))
	}
	needle := strings.TrimSpace(query.Query)
	rows, err := s.pool.Query(ctx, `
SELECT payload,
	1 + CASE WHEN $3 = '' THEN 0 ELSE ts_rank(search_tsv, plainto_tsquery('simple', $3)) END AS score
FROM artifact_records
WHERE tenant_id = $1
	AND (cardinality($2::text[]) = 0 OR kind = ANY($2::text[]))
	AND ($3 = '' OR search_tsv @@ plainto_tsquery('simple', $3) OR title ILIKE '%' || $3 || '%' OR excerpt ILIKE '%' || $3 || '%' OR external_id ILIKE '%' || $3 || '%')
ORDER BY score DESC, captured_at DESC
LIMIT $4
`, normalizeTenantID(query.TenantID), kinds, needle, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]artifact.SearchResult, 0, limit)
	for rows.Next() {
		item, score, err := scanPGJSONWithScore[artifact.Artifact](rows)
		if err != nil {
			return nil, err
		}
		out = append(out, artifact.SearchResult{Artifact: cloneArtifact(item), Score: score})
	}
	return out, rows.Err()
}

func (s *pgArtifactStore) findBySourceHash(ctx context.Context, item artifact.Artifact) (string, bool, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
SELECT id FROM artifact_records
WHERE tenant_id = $1 AND kind = $2 AND external_id = $3 AND content_hash = $4
`, item.TenantID, item.Kind, item.ExternalID, item.ContentHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}
