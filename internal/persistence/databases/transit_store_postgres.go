package databases

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/persistence"
	"manifold/internal/transit"
)

type pgTransitStore struct {
	pool *pgxpool.Pool
}

func NewPostgresTransitStore(pool *pgxpool.Pool) transit.Store {
	if pool == nil {
		return NewMemoryTransitStore()
	}
	return &pgTransitStore{pool: pool}
}

func (s *pgTransitStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *pgTransitStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres transit store requires pool")
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS transit_memories (
	id UUID PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	key_name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	value TEXT NOT NULL DEFAULT '',
	base64 BOOLEAN NOT NULL DEFAULT false,
	embed BOOLEAN NOT NULL DEFAULT true,
	embed_source TEXT NOT NULL DEFAULT 'value',
	version BIGINT NOT NULL DEFAULT 1,
	created_by BIGINT NOT NULL,
	updated_by BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	ts tsvector GENERATED ALWAYS AS (
		to_tsvector('simple', coalesce(description, '') || ' ' || coalesce(value, ''))
	) STORED
);

CREATE UNIQUE INDEX IF NOT EXISTS transit_memories_tenant_key_idx ON transit_memories(tenant_id, key_name);
CREATE INDEX IF NOT EXISTS transit_memories_tenant_key_prefix_idx ON transit_memories(tenant_id, key_name text_pattern_ops);
CREATE INDEX IF NOT EXISTS transit_memories_tenant_updated_idx ON transit_memories(tenant_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS transit_memories_ts_idx ON transit_memories USING GIN (ts);
`)
	return err
}

func (s *pgTransitStore) Create(ctx context.Context, tenantID, actorID int64, items []transit.CreateMemoryItem) ([]transit.Record, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	out := make([]transit.Record, 0, len(items))
	for _, item := range items {
		record := transit.Record{
			ID:          uuid.NewString(),
			TenantID:    tenantID,
			KeyName:     item.KeyName,
			Description: item.Description,
			Value:       item.Value,
			Base64:      item.Base64 != nil && *item.Base64,
			Embed:       item.Embed == nil || *item.Embed,
			EmbedSource: transit.NormalizeEmbedSource(item.EmbedSource),
			Version:     1,
			CreatedBy:   actorID,
			UpdatedBy:   actorID,
		}
		row := tx.QueryRow(ctx, `
INSERT INTO transit_memories (
	id, tenant_id, key_name, description, value, base64, embed, embed_source, version, created_by, updated_by
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING created_at, updated_at
`, record.ID, record.TenantID, record.KeyName, record.Description, record.Value, record.Base64, record.Embed, record.EmbedSource, record.Version, record.CreatedBy, record.UpdatedBy)
		if err := row.Scan(&record.CreatedAt, &record.UpdatedAt); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, persistence.ErrRevisionConflict
			}
			return nil, err
		}
		out = append(out, record)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *pgTransitStore) Get(ctx context.Context, tenantID int64, keys []string) ([]transit.Record, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, tenant_id, key_name, description, value, base64, embed, embed_source, version, created_by, updated_by, created_at, updated_at
FROM transit_memories
WHERE tenant_id = $1 AND key_name = ANY($2)
ORDER BY key_name ASC
`, tenantID, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]transit.Record, 0, len(keys))
	for rows.Next() {
		var record transit.Record
		if err := rows.Scan(
			&record.ID,
			&record.TenantID,
			&record.KeyName,
			&record.Description,
			&record.Value,
			&record.Base64,
			&record.Embed,
			&record.EmbedSource,
			&record.Version,
			&record.CreatedBy,
			&record.UpdatedBy,
			&record.CreatedAt,
			&record.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *pgTransitStore) Update(ctx context.Context, tenantID, actorID int64, req transit.UpdateMemoryRequest) (transit.Record, error) {
	args := []any{tenantID, req.KeyName, req.Value, actorID, req.Base64, req.Embed, req.EmbedSource}
	query := `
UPDATE transit_memories
SET value = $3,
	base64 = COALESCE($5, base64),
	embed = COALESCE($6, embed),
	embed_source = COALESCE(NULLIF($7, ''), embed_source),
	updated_by = $4,
	updated_at = NOW(),
	version = version + 1
WHERE tenant_id = $1 AND key_name = $2`
	if req.IfVersion > 0 {
		query += ` AND version = $8`
		args = append(args, req.IfVersion)
	}
	query += ` RETURNING id, tenant_id, key_name, description, value, base64, embed, embed_source, version, created_by, updated_by, created_at, updated_at`
	row := s.pool.QueryRow(ctx, query, args...)
	var record transit.Record
	if err := row.Scan(
		&record.ID,
		&record.TenantID,
		&record.KeyName,
		&record.Description,
		&record.Value,
		&record.Base64,
		&record.Embed,
		&record.EmbedSource,
		&record.Version,
		&record.CreatedBy,
		&record.UpdatedBy,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if req.IfVersion > 0 {
				return transit.Record{}, persistence.ErrRevisionConflict
			}
			return transit.Record{}, transit.NotFoundError(req.KeyName)
		}
		return transit.Record{}, err
	}
	return record, nil
}

func (s *pgTransitStore) Delete(ctx context.Context, tenantID int64, keys []string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM transit_memories WHERE tenant_id = $1 AND key_name = ANY($2)`, tenantID, keys)
	return err
}

func (s *pgTransitStore) ListKeys(ctx context.Context, tenantID int64, req transit.ListRequest) ([]transit.Metadata, error) {
	rows, err := s.pool.Query(ctx, `
SELECT key_name, description, base64, embed, embed_source, version, created_at, updated_at
FROM transit_memories
WHERE tenant_id = $1 AND ($2 = '' OR key_name LIKE $2 || '%')
ORDER BY key_name ASC
LIMIT $3
`, tenantID, req.Prefix, req.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransitMetadata(rows)
}

func (s *pgTransitStore) ListRecent(ctx context.Context, tenantID int64, req transit.ListRequest) ([]transit.Metadata, error) {
	rows, err := s.pool.Query(ctx, `
SELECT key_name, description, base64, embed, embed_source, version, created_at, updated_at
FROM transit_memories
WHERE tenant_id = $1 AND ($2 = '' OR key_name LIKE $2 || '%')
ORDER BY updated_at DESC, key_name ASC
LIMIT $3
`, tenantID, req.Prefix, req.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransitMetadata(rows)
}

func (s *pgTransitStore) SearchText(ctx context.Context, tenantID int64, req transit.SearchRequest) ([]transit.SearchCandidate, error) {
	query := strings.TrimSpace(req.Query)
	rows, err := s.pool.Query(ctx, `
SELECT id, tenant_id, key_name, description, value, base64, embed, embed_source, version, created_by, updated_by, created_at, updated_at,
	CASE WHEN $2 = '' THEN 1.0 ELSE ts_rank(ts, plainto_tsquery('simple', $2)) END AS score,
	left(value, 120) AS snippet
FROM transit_memories
WHERE tenant_id = $1
	AND ($3 = '' OR key_name LIKE $3 || '%')
	AND ($2 = '' OR ts @@ plainto_tsquery('simple', $2))
	AND ($4 = 0 OR updated_at >= NOW() - ($4::text || ' days')::interval)
	AND ($5::timestamptz IS NULL OR created_at >= $5)
	AND ($6::timestamptz IS NULL OR created_at <= $6)
	AND ($7::timestamptz IS NULL OR updated_at >= $7)
	AND ($8::timestamptz IS NULL OR updated_at <= $8)
ORDER BY score DESC, updated_at DESC
LIMIT $9
`, tenantID, query, req.Prefix, req.WithinDays, req.CreatedAfter, req.CreatedBefore, req.UpdatedAfter, req.UpdatedBefore, req.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]transit.SearchCandidate, 0, req.Limit)
	for rows.Next() {
		var candidate transit.SearchCandidate
		if err := rows.Scan(
			&candidate.Record.ID,
			&candidate.Record.TenantID,
			&candidate.Record.KeyName,
			&candidate.Record.Description,
			&candidate.Record.Value,
			&candidate.Record.Base64,
			&candidate.Record.Embed,
			&candidate.Record.EmbedSource,
			&candidate.Record.Version,
			&candidate.Record.CreatedBy,
			&candidate.Record.UpdatedBy,
			&candidate.Record.CreatedAt,
			&candidate.Record.UpdatedAt,
			&candidate.Score,
			&candidate.Snippet,
		); err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

func scanTransitMetadata(rows pgx.Rows) ([]transit.Metadata, error) {
	out := make([]transit.Metadata, 0)
	for rows.Next() {
		var metadata transit.Metadata
		if err := rows.Scan(
			&metadata.KeyName,
			&metadata.Description,
			&metadata.Base64,
			&metadata.Embed,
			&metadata.EmbedSource,
			&metadata.Version,
			&metadata.CreatedAt,
			&metadata.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, metadata)
	}
	return out, rows.Err()
}
