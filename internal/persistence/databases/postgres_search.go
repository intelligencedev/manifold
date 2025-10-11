package databases

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgSearch struct{ pool *pgxpool.Pool }

func NewPostgresSearch(pool *pgxpool.Pool) FullTextSearch {
	// best-effort bootstrap
	ctx := context.Background()
	// Create extension if available (no error if not superuser; ignore)
	_, _ = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pg_trgm`)
	// documents table with generated tsvector column (simple config)
	_, _ = pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS documents (
  id TEXT PRIMARY KEY,
  text TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  ts tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce(text,''))) STORED
);
`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS documents_ts_idx ON documents USING GIN (ts)`)
	return &pgSearch{pool: pool}
}

func (p *pgSearch) Index(ctx context.Context, id, text string, metadata map[string]string) error {
	// Ensure metadata is non-nil so the JSONB NOT NULL constraint is not violated.
	md := mapToJSON(metadata)
	_, err := p.pool.Exec(ctx, `
INSERT INTO documents(id, text, metadata) VALUES($1,$2,$3)
ON CONFLICT (id) DO UPDATE SET text=EXCLUDED.text, metadata=EXCLUDED.metadata
`, id, text, md)
	return err
}

func (p *pgSearch) Remove(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM documents WHERE id=$1`, id)
	return err
}

func (p *pgSearch) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	// Use plainto_tsquery over 'simple' dict; sanitize empty query
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	rows, err := p.pool.Query(ctx, `
SELECT id, ts_rank(ts, plainto_tsquery('simple',$1)) AS score,
       left(text, 120) AS snippet,
       text,
       metadata
FROM documents
WHERE ts @@ plainto_tsquery('simple',$1)
ORDER BY score DESC
LIMIT $2
`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SearchResult, 0, limit)
	for rows.Next() {
		var r SearchResult
		var md map[string]string
		if err := rows.Scan(&r.ID, &r.Score, &r.Snippet, &r.Text, &md); err != nil {
			return nil, err
		}
		r.Metadata = md
		out = append(out, r)
	}
	return out, rows.Err()
}

// mapToJSON ensures we never return nil to the database layer; return an empty
// map when callers provide nil so INSERT/UPDATE won't try to write a SQL NULL
// into a NOT NULL JSONB column.
func mapToJSON(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
