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

// SearchChunks returns chunk-level search results, preferring the "chunks" table when
// available. It uses websearch_to_tsquery when present, falling back to plainto_tsquery.
// Filters are applied against the metadata JSONB column.
func (p *pgSearch) SearchChunks(ctx context.Context, query string, lang string, limit int, filter map[string]string) ([]SearchResult, error) {
    if limit <= 0 {
        limit = 10
    }
    q := strings.TrimSpace(query)
    if q == "" {
        return nil, nil
    }
    // Ensure type=chunk in filter for both table and fallback paths.
    f := mapToJSON(filter)
    if f == nil {
        f = map[string]string{}
    }
    if _, ok := f["type"]; !ok {
        f["type"] = "chunk"
    }
    // helper to run a query and scan
    run := func(stmt string, args ...any) ([]SearchResult, error) {
        rows, err := p.pool.Query(ctx, stmt, args...)
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
    // Prefer chunks table when present
    useChunks, _ := p.HasChunksTable(ctx)
    // websearch attempt
    if useChunks {
        stmt := `SELECT id, ts_rank(ts, websearch_to_tsquery(to_regconfig($2), $1)) AS score,
                         left(text, 120) AS snippet, text, metadata
                  FROM chunks
                  WHERE ts @@ websearch_to_tsquery(to_regconfig($2), $1)
                    AND metadata @> $3
                  ORDER BY score DESC
                  LIMIT $4`
        res, err := run(stmt, q, lang, f, limit)
        if err == nil {
            return res, nil
        }
        // Fallback to plainto_tsquery
        stmt = `SELECT id, ts_rank(ts, plainto_tsquery(to_regconfig($2), $1)) AS score,
                         left(text, 120) AS snippet, text, metadata
                FROM chunks
                WHERE ts @@ plainto_tsquery(to_regconfig($2), $1)
                  AND metadata @> $3
                ORDER BY score DESC
                LIMIT $4`
        return run(stmt, q, lang, f, limit)
    }
    // Fallback: search documents with chunk prefix/type
    stmt := `SELECT id, ts_rank(ts, websearch_to_tsquery(to_regconfig($2), $1)) AS score,
                     left(text, 120) AS snippet, text, metadata
              FROM documents
              WHERE ts @@ websearch_to_tsquery(to_regconfig($2), $1)
                AND metadata @> $3
                AND id LIKE 'chunk:%'
              ORDER BY score DESC
              LIMIT $4`
    res, err := run(stmt, q, lang, f, limit)
    if err == nil {
        return res, nil
    }
    stmt = `SELECT id, ts_rank(ts, plainto_tsquery(to_regconfig($2), $1)) AS score,
                     left(text, 120) AS snippet, text, metadata
            FROM documents
            WHERE ts @@ plainto_tsquery(to_regconfig($2), $1)
              AND metadata @> $3
              AND id LIKE 'chunk:%'
            ORDER BY score DESC
            LIMIT $4`
    return run(stmt, q, lang, f, limit)
}

func (p *pgSearch) GetByID(ctx context.Context, id string) (SearchResult, bool, error) {
    row := p.pool.QueryRow(ctx, `SELECT id, text, metadata FROM documents WHERE id=$1`, id)
    var r SearchResult
    var md map[string]string
    if err := row.Scan(&r.ID, &r.Text, &md); err != nil {
        if strings.Contains(err.Error(), "no rows") {
            return SearchResult{}, false, nil
        }
        return SearchResult{}, false, err
    }
    r.Metadata = md
    return r, true, nil
}

// SnippetForID returns a highlighted snippet using Postgres ts_headline when available.
// It attempts to select from the chunks table when the id looks like a chunk and the
// table exists; otherwise it selects from documents. Returns (snippet, true, nil) on success.
func (p *pgSearch) SnippetForID(ctx context.Context, id, lang, query string) (string, bool, error) {
    useChunks, _ := p.HasChunksTable(ctx)
    stmt := `SELECT ts_headline(to_regconfig($2), text, websearch_to_tsquery(to_regconfig($2), $3)) FROM documents WHERE id=$1`
    if useChunks && strings.HasPrefix(id, "chunk:") {
        stmt = `SELECT ts_headline(to_regconfig($2), text, websearch_to_tsquery(to_regconfig($2), $3)) FROM chunks WHERE id=$1`
    }
    var snip string
    if err := p.pool.QueryRow(ctx, stmt, id, lang, query).Scan(&snip); err != nil {
        if strings.Contains(err.Error(), "no rows") {
            return "", false, nil
        }
        return "", false, err
    }
    return snip, true, nil
}

// HasChunksTable reports whether a table named "chunks" exists in the current schema.
// This is an optional capability used by higher layers for chunk-level indexing.
func (p *pgSearch) HasChunksTable(ctx context.Context) (bool, error) {
    var exists bool
    // information_schema lookup is portable across Postgres
    err := p.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = current_schema()
    AND table_name = 'chunks'
)
`).Scan(&exists)
    if err != nil {
        return false, err
    }
    return exists, nil
}

// UpsertChunk inserts or updates a row in the chunks table. It assumes the
// table has columns: id TEXT PK, doc_id TEXT, idx INT, text TEXT, metadata JSONB, lang regconfig.
func (p *pgSearch) UpsertChunk(ctx context.Context, chunkID, docID string, idx int, text string, metadata map[string]string, lang string) error {
    md := mapToJSON(metadata)
    _, err := p.pool.Exec(ctx, `
INSERT INTO chunks(id, doc_id, idx, text, metadata, lang)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT (id) DO UPDATE SET text=EXCLUDED.text, metadata=EXCLUDED.metadata, lang=EXCLUDED.lang
`, chunkID, docID, idx, text, md, lang)
    return err
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
