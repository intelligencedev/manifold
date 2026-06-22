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

	// Add a weighted tsvector so title and URL rank above body text.
	_, _ = pool.Exec(ctx, `
	ALTER TABLE documents ADD COLUMN IF NOT EXISTS ts_weighted tsvector
	  GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(metadata->>'title', '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(metadata->>'url',   '')), 'B') ||
    setweight(to_tsvector('simple', coalesce(text,               '')), 'C')
  ) STORED
`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS documents_ts_weighted_idx ON documents USING GIN (ts_weighted)`)

	// Trigram index on text for fuzzy fallback (pg_trgm must be loaded above).
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS documents_text_trgm_idx ON documents USING GIN (text gin_trgm_ops)`)

	// chunks table: ts is stored as a plain column and computed at write-time in
	// UpsertChunk so each chunk can use its own per-document language regconfig.
	_, _ = pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS chunks (
  id       TEXT PRIMARY KEY,
  doc_id   TEXT NOT NULL,
  idx      INT  NOT NULL,
  text     TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  lang     regconfig NOT NULL DEFAULT 'simple',
  ts       tsvector NOT NULL DEFAULT ''::tsvector
);
`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS chunks_ts_idx ON chunks USING GIN (ts)`)
	// Trigram index on chunk text for fuzzy fallback.
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS chunks_text_trgm_idx ON chunks USING GIN (text gin_trgm_ops)`)

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
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}

	// Primary path: rank against ts_weighted (title/url weighted above body).
	// Fall back to ts (plain body) if ts_weighted column does not yet exist.
	// Use ts_headline for rich snippet generation.
	rows, err := p.pool.Query(ctx, `
SELECT id,
       ts_rank(ts_weighted, plainto_tsquery('simple',$1)) AS score,
       ts_headline('simple', text, plainto_tsquery('simple',$1),
                   'MaxWords=35,MinWords=15,StartSel="",StopSel="",MaxFragments=1') AS snippet,
       text,
       metadata
FROM documents
WHERE ts_weighted @@ plainto_tsquery('simple',$1)
ORDER BY score DESC
LIMIT $2
`, q, limit)
	if err == nil {
		results, scanErr := p.scanSearchRows(rows, limit)
		if scanErr != nil {
			return nil, scanErr
		}
		if len(results) > 0 {
			return results, nil
		}
	}

	// Fallback: plain ts column (e.g. ts_weighted column not yet migrated).
	rows, err = p.pool.Query(ctx, `
SELECT id,
       ts_rank(ts, plainto_tsquery('simple',$1)) AS score,
       ts_headline('simple', text, plainto_tsquery('simple',$1),
                   'MaxWords=35,MinWords=15,StartSel="",StopSel="",MaxFragments=1') AS snippet,
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
	results, err := p.scanSearchRows(rows, limit)
	if err != nil {
		return nil, err
	}
	if len(results) > 0 {
		return results, nil
	}

	// Trigram fuzzy fallback when FTS returns nothing (handles misspellings /
	// morphological variants not covered by 'simple' stemming).
	return p.trigramFallbackDocuments(ctx, q, limit)
}

// SearchChunks returns chunk-level search results, preferring the "chunks" table when
// available. Falls back to searching the documents table filtered to chunk-prefixed rows.
// All tsvector comparisons use the same text-search configuration ('simple') that was
// used when the index was built, preventing stemming/stopword mismatches.
// Per-chunk language configs are honoured for the chunks table path.
func (p *pgSearch) SearchChunks(ctx context.Context, query string, lang string, limit int, filter map[string]string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	// Ensure type=chunk in filter for the document-fallback path.
	f := mapToJSON(filter)
	if f == nil {
		f = map[string]string{}
	}
	if _, ok := f["type"]; !ok {
		f["type"] = "chunk"
	}

	// Prefer chunks table when present.
	useChunks, _ := p.HasChunksTable(ctx)
	if useChunks {
		// Each chunk row stores the regconfig it was indexed with in the lang column.
		// Use the caller-supplied lang when provided; fall back to 'simple'.
		chunkLang := pgLang(lang)
		stmt := `SELECT id,
                         ts_rank(ts, websearch_to_tsquery($2::regconfig, $1)) AS score,
                         ts_headline($2::regconfig, text, websearch_to_tsquery($2::regconfig, $1),
                                     'MaxWords=35,MinWords=15,StartSel="",StopSel="",MaxFragments=1') AS snippet,
                         text, metadata
                  FROM chunks
                  WHERE ts @@ websearch_to_tsquery($2::regconfig, $1)
                    AND metadata @> $3
                  ORDER BY score DESC
                  LIMIT $4`
		res, err := p.runQuery(ctx, stmt, q, chunkLang, f, limit)
		if err == nil && len(res) > 0 {
			return res, nil
		}
		if err == nil {
			// FTS returned nothing — try trigram fuzzy fallback on chunks.
			return p.trigramFallbackChunks(ctx, q, limit, f)
		}
		// Fallback to plainto_tsquery (websearch_to_tsquery not available on older PG)
		stmt = `SELECT id,
                         ts_rank(ts, plainto_tsquery($2::regconfig, $1)) AS score,
                         ts_headline($2::regconfig, text, plainto_tsquery($2::regconfig, $1),
                                     'MaxWords=35,MinWords=15,StartSel="",StopSel="",MaxFragments=1') AS snippet,
                         text, metadata
                FROM chunks
                WHERE ts @@ plainto_tsquery($2::regconfig, $1)
                  AND metadata @> $3
                ORDER BY score DESC
                LIMIT $4`
		res, err = p.runQuery(ctx, stmt, q, chunkLang, f, limit)
		if err != nil {
			return nil, err
		}
		if len(res) > 0 {
			return res, nil
		}
		return p.trigramFallbackChunks(ctx, q, limit, f)
	}

	// Document-fallback path: documents.ts is indexed with 'simple' — use 'simple'
	// here to avoid a stemming/stopword mismatch between the stored tsvector and the
	// query. websearch_to_tsquery gives richer query parsing when available.
	stmt := `SELECT id,
                     ts_rank(ts, websearch_to_tsquery('simple', $1)) AS score,
                     ts_headline('simple', text, websearch_to_tsquery('simple', $1),
                                 'MaxWords=35,MinWords=15,StartSel="",StopSel="",MaxFragments=1') AS snippet,
                     text, metadata
              FROM documents
              WHERE ts @@ websearch_to_tsquery('simple', $1)
                AND metadata @> $2
                AND id LIKE 'chunk:%'
              ORDER BY score DESC
              LIMIT $3`
	res, err := p.runQuery(ctx, stmt, q, f, limit)
	if err == nil && len(res) > 0 {
		return res, nil
	}
	if err == nil {
		return p.trigramFallbackDocuments(ctx, q, limit)
	}
	// Fallback to plainto_tsquery
	stmt = `SELECT id,
                     ts_rank(ts, plainto_tsquery('simple', $1)) AS score,
                     ts_headline('simple', text, plainto_tsquery('simple', $1),
                                 'MaxWords=35,MinWords=15,StartSel="",StopSel="",MaxFragments=1') AS snippet,
                     text, metadata
            FROM documents
            WHERE ts @@ plainto_tsquery('simple', $1)
              AND metadata @> $2
              AND id LIKE 'chunk:%'
            ORDER BY score DESC
            LIMIT $3`
	res, err = p.runQuery(ctx, stmt, q, f, limit)
	if err != nil {
		return nil, err
	}
	if len(res) > 0 {
		return res, nil
	}
	return p.trigramFallbackDocuments(ctx, q, limit)
}

// trigramFallbackDocuments returns results from pg_trgm similarity search on
// documents when exact FTS produces no matches. Requires the pg_trgm extension
// and the GIN trigram index created during bootstrap.
func (p *pgSearch) trigramFallbackDocuments(ctx context.Context, q string, limit int) ([]SearchResult, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id,
       similarity(text, $1) AS score,
       left(text, 120) AS snippet,
       text,
       metadata
FROM documents
WHERE text % $1
ORDER BY score DESC
LIMIT $2
`, q, limit)
	if err != nil {
		// pg_trgm not available or index missing — swallow and return empty.
		return nil, nil
	}
	return p.scanSearchRows(rows, limit)
}

// trigramFallbackChunks returns results from pg_trgm similarity search on
// chunks when exact FTS produces no matches.
func (p *pgSearch) trigramFallbackChunks(ctx context.Context, q string, limit int, filter map[string]string) ([]SearchResult, error) {
	rows, err := p.pool.Query(ctx, `
SELECT id,
       similarity(text, $1) AS score,
       left(text, 120) AS snippet,
       text,
       metadata
FROM chunks
WHERE text % $1
  AND metadata @> $2
ORDER BY score DESC
LIMIT $3
`, q, filter, limit)
	if err != nil {
		return nil, nil
	}
	return p.scanSearchRows(rows, limit)
}

func (p *pgSearch) GetByID(ctx context.Context, id string) (SearchResult, bool, error) {
	// If we have a chunks table and the ID looks like a chunk, read from chunks; otherwise read from documents.
	useChunks, _ := p.HasChunksTable(ctx)
	stmt := `SELECT id, text, metadata FROM documents WHERE id=$1`
	if useChunks && strings.HasPrefix(id, "chunk:") {
		stmt = `SELECT id, text, metadata FROM chunks WHERE id=$1`
	}
	row := p.pool.QueryRow(ctx, stmt, id)
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

// SnippetForID returns a highlighted snippet using Postgres ts_headline.
// For the documents table, 'simple' is used (matching the stored tsvector).
// For the chunks table, the per-chunk lang stored in the row is used.
func (p *pgSearch) SnippetForID(ctx context.Context, id, lang, query string) (string, bool, error) {
	useChunks, _ := p.HasChunksTable(ctx)

	var snip string
	var err error

	if useChunks && strings.HasPrefix(id, "chunk:") {
		// Chunks are indexed with their own per-row lang; use the caller-supplied lang
		// (falling back to 'simple') so ts_headline highlights correctly.
		chunkLang := pgLang(lang)
		err = p.pool.QueryRow(ctx, `
SELECT ts_headline($2::regconfig, text, websearch_to_tsquery($2::regconfig, $3))
FROM chunks WHERE id=$1`, id, chunkLang, query).Scan(&snip)
	} else {
		// documents.ts is indexed with 'simple' — ts_headline must use the same config.
		err = p.pool.QueryRow(ctx, `
SELECT ts_headline('simple', text, websearch_to_tsquery('simple', $2))
FROM documents WHERE id=$1`, id, query).Scan(&snip)
	}

	if err != nil {
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

type ChunkSearchRow struct {
	ID       string
	DocID    string
	Index    int
	Text     string
	Metadata map[string]string
	Lang     string
}

// UpsertChunk inserts or updates a row in the chunks table. The tsvector column ts
// is computed at write-time using the chunk's own lang regconfig so that subsequent
// SearchChunks queries use a consistent configuration for that row.
func (p *pgSearch) UpsertChunk(ctx context.Context, chunk ChunkSearchRow) error {
	md := mapToJSON(chunkSearchMetadata(chunk.Metadata, chunk.Lang))
	lang := pgLang(chunk.Lang)
	_, err := p.pool.Exec(ctx, `
INSERT INTO chunks(id, doc_id, idx, text, metadata, lang, ts)
VALUES($1,$2,$3,$4,$5,$6::regconfig, to_tsvector($6::regconfig, coalesce($4,'')))
ON CONFLICT (id) DO UPDATE
  SET text=EXCLUDED.text,
      metadata=EXCLUDED.metadata,
      lang=EXCLUDED.lang,
      ts=EXCLUDED.ts
`, chunk.ID, chunk.DocID, chunk.Index, chunk.Text, md, lang)
	return err
}

// scanSearchRows scans a pgx Rows result set into []SearchResult.
func (p *pgSearch) scanSearchRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}, limit int) ([]SearchResult, error) {
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

// runQuery executes a query and scans results using scanSearchRows.
func (p *pgSearch) runQuery(ctx context.Context, stmt string, args ...any) ([]SearchResult, error) {
	rows, err := p.pool.Query(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	return p.scanSearchRows(rows, 100)
}

// pgLang returns a safe Postgres regconfig name. When lang is empty or blank it
// returns 'simple', which matches the configuration used for the documents table
// and avoids stemming/stopword mismatches.
func pgLang(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return "simple"
	}
	return lang
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
