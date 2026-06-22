package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

type sqliteSearch struct {
	db *sql.DB
}

func NewSQLiteSearch(db *sql.DB) FullTextSearch {
	return &sqliteSearch{db: db}
}

func (s *sqliteSearch) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite search store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS manifold_documents (
	id TEXT PRIMARY KEY,
	text TEXT NOT NULL,
	metadata TEXT NOT NULL DEFAULT '{}'
);
CREATE VIRTUAL TABLE IF NOT EXISTS manifold_documents_fts USING fts5(
	id UNINDEXED,
	text,
	metadata UNINDEXED,
	tokenize='porter unicode61'
);
CREATE TABLE IF NOT EXISTS manifold_chunks (
	id TEXT PRIMARY KEY,
	doc_id TEXT NOT NULL,
	idx INTEGER NOT NULL,
	text TEXT NOT NULL,
	metadata TEXT NOT NULL DEFAULT '{}',
	lang TEXT NOT NULL DEFAULT ''
);
CREATE VIRTUAL TABLE IF NOT EXISTS manifold_chunks_fts USING fts5(
	id UNINDEXED,
	doc_id UNINDEXED,
	text,
	metadata UNINDEXED,
	lang UNINDEXED,
	tokenize='porter unicode61'
);
`)
	if err != nil {
		return err
	}
	// Rebuild existing FTS tables if they still use the older unicode61 tokenizer.
	if err := s.migrateTokenizerIfNeeded(ctx); err != nil {
		// Best-effort: migration failure degrades quality but shouldn't block operation.
		_ = err
	}
	return nil
}

// migrateTokenizerIfNeeded checks each fts5 table's tokenizer config and rebuilds
// it with 'porter unicode61' if it was created with the older 'unicode61' tokenizer.
// Data is preserved by re-inserting from the underlying base tables.
func (s *sqliteSearch) migrateTokenizerIfNeeded(ctx context.Context) error {
	if err := s.migrateDocsFTS(ctx); err != nil {
		return err
	}
	return s.migrateChunksFTS(ctx)
}

func (s *sqliteSearch) migrateDocsFTS(ctx context.Context) error {
	tok, err := s.fts5Tokenizer(ctx, "manifold_documents_fts")
	if err != nil || tok == "porter unicode61" {
		return err
	}
	// Rebuild docs FTS table with porter unicode61.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS manifold_documents_fts`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
CREATE VIRTUAL TABLE manifold_documents_fts USING fts5(
	id UNINDEXED,
	text,
	metadata UNINDEXED,
	tokenize='porter unicode61'
)`); err != nil {
		return err
	}
	// Repopulate from base table.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO manifold_documents_fts(id, text, metadata)
SELECT id, text, metadata FROM manifold_documents
`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteSearch) migrateChunksFTS(ctx context.Context) error {
	tok, err := s.fts5Tokenizer(ctx, "manifold_chunks_fts")
	if err != nil || tok == "porter unicode61" {
		return err
	}
	// Rebuild chunks FTS table with porter unicode61.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS manifold_chunks_fts`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
CREATE VIRTUAL TABLE manifold_chunks_fts USING fts5(
	id UNINDEXED,
	doc_id UNINDEXED,
	text,
	metadata UNINDEXED,
	lang UNINDEXED,
	tokenize='porter unicode61'
)`); err != nil {
		return err
	}
	// Repopulate from base table.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO manifold_chunks_fts(id, doc_id, text, metadata, lang)
SELECT id, doc_id, text, metadata, lang FROM manifold_chunks
`); err != nil {
		return err
	}
	return tx.Commit()
}

// fts5Tokenizer returns the tokenizer string for a given fts5 virtual table.
// fts5 stores configuration in a <table>_config shadow table.
// Returns "" when the table does not exist yet.
func (s *sqliteSearch) fts5Tokenizer(ctx context.Context, table string) (string, error) {
	var val string
	err := s.db.QueryRowContext(ctx,
		`SELECT v FROM `+table+`_config WHERE k='tokenize'`,
	).Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		// Table might not exist yet (shadow table missing) — treat as no config.
		return "", nil
	}
	return val, nil
}

func (s *sqliteSearch) Index(ctx context.Context, id, text string, metadata map[string]string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	md := encodeStringMap(metadata)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO manifold_documents(id, text, metadata)
VALUES(?, ?, ?)
ON CONFLICT(id) DO UPDATE SET text=excluded.text, metadata=excluded.metadata
`, id, text, md); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_documents_fts WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO manifold_documents_fts(id, text, metadata) VALUES(?, ?, ?)`, id, text, md); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteSearch) Remove(ctx context.Context, id string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_documents WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_documents_fts WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_chunks WHERE doc_id = ? OR id = ?`, id, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_chunks_fts WHERE doc_id = ? OR id = ?`, id, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteSearch) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	queries := sqliteFTSQueries(query)
	if len(queries) == 0 {
		return nil, nil
	}
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	for _, q := range queries {
		rows, err := s.db.QueryContext(ctx, `
SELECT id, -bm25(manifold_documents_fts) AS score,
       snippet(manifold_documents_fts, 1, '', '', '...', 12) AS snippet,
       text,
       metadata
FROM manifold_documents_fts
WHERE manifold_documents_fts MATCH ?
ORDER BY bm25(manifold_documents_fts)
LIMIT ?`, q, limit)
		if err != nil {
			return nil, err
		}
		results, err := scanSQLiteSearchRows(rows, limit)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results, nil
		}
	}
	return nil, nil
}

func (s *sqliteSearch) GetByID(ctx context.Context, id string) (SearchResult, bool, error) {
	if err := s.Init(ctx); err != nil {
		return SearchResult{}, false, err
	}
	var text, mdRaw string
	err := s.db.QueryRowContext(ctx, `SELECT text, metadata FROM manifold_documents WHERE id = ?`, id).Scan(&text, &mdRaw)
	if errors.Is(err, sql.ErrNoRows) && strings.HasPrefix(id, "chunk:") {
		err = s.db.QueryRowContext(ctx, `SELECT text, metadata FROM manifold_chunks WHERE id = ?`, id).Scan(&text, &mdRaw)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return SearchResult{}, false, nil
	}
	if err != nil {
		return SearchResult{}, false, err
	}
	return SearchResult{ID: id, Text: text, Metadata: decodeStringMap(mdRaw)}, true, nil
}

func (s *sqliteSearch) SearchChunks(ctx context.Context, query string, _ string, limit int, filter map[string]string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	queries := sqliteFTSQueries(query)
	if len(queries) == 0 {
		return nil, nil
	}
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	chunkFilter := sqliteChunkSearchFilter(filter)
	for _, q := range queries {
		results, err := s.queryChunkFTS(ctx, q, limit, chunkFilter)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results, nil
		}
	}
	docFilter := sqliteDocumentFallbackFilter(filter)
	for _, q := range queries {
		results, err := s.queryDocumentFTS(ctx, q, limit, docFilter)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			return results, nil
		}
	}
	return nil, nil
}

func (s *sqliteSearch) queryChunkFTS(ctx context.Context, q string, limit int, filter map[string]string) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, -bm25(manifold_chunks_fts) AS score,
       snippet(manifold_chunks_fts, 2, '', '', '...', 12) AS snippet,
       text,
       metadata
FROM manifold_chunks_fts
WHERE manifold_chunks_fts MATCH ?
ORDER BY bm25(manifold_chunks_fts)
LIMIT ?`, q, sqliteSearchCandidateLimit(limit))
	if err != nil {
		return nil, err
	}
	results, err := scanSQLiteSearchRows(rows, sqliteSearchCandidateLimit(limit))
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, min(limit, len(results)))
	for _, result := range results {
		if metaMatches(result.Metadata, filter) {
			out = append(out, result)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *sqliteSearch) queryDocumentFTS(ctx context.Context, q string, limit int, filter map[string]string) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, -bm25(manifold_documents_fts) AS score,
       snippet(manifold_documents_fts, 1, '', '', '...', 12) AS snippet,
       text,
       metadata
FROM manifold_documents_fts
WHERE manifold_documents_fts MATCH ?
ORDER BY bm25(manifold_documents_fts)
LIMIT ?`, q, sqliteSearchCandidateLimit(limit))
	if err != nil {
		return nil, err
	}
	results, err := scanSQLiteSearchRows(rows, sqliteSearchCandidateLimit(limit))
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, min(limit, len(results)))
	for _, result := range results {
		if metaMatches(result.Metadata, filter) {
			out = append(out, result)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *sqliteSearch) SnippetForID(ctx context.Context, id, _, query string) (string, bool, error) {
	queries := sqliteFTSQueries(query)
	if len(queries) == 0 {
		return "", false, nil
	}
	if err := s.Init(ctx); err != nil {
		return "", false, err
	}
	stmt := `SELECT snippet(manifold_documents_fts, 1, '', '', '...', 12) FROM manifold_documents_fts WHERE id = ? AND manifold_documents_fts MATCH ? LIMIT 1`
	if strings.HasPrefix(id, "chunk:") {
		stmt = `SELECT snippet(manifold_chunks_fts, 2, '', '', '...', 12) FROM manifold_chunks_fts WHERE id = ? AND manifold_chunks_fts MATCH ? LIMIT 1`
	}
	for _, q := range queries {
		var snippet string
		if err := s.db.QueryRowContext(ctx, stmt, id, q).Scan(&snippet); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return "", false, err
		}
		return snippet, true, nil
	}
	return "", false, nil
}

func (s *sqliteSearch) HasChunksTable(context.Context) (bool, error) {
	return true, nil
}

func (s *sqliteSearch) UpsertChunk(ctx context.Context, chunk ChunkSearchRow) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	md := encodeStringMap(chunkSearchMetadata(chunk.Metadata, chunk.Lang))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO manifold_chunks(id, doc_id, idx, text, metadata, lang)
VALUES(?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET doc_id=excluded.doc_id, idx=excluded.idx, text=excluded.text, metadata=excluded.metadata, lang=excluded.lang
`, chunk.ID, chunk.DocID, chunk.Index, chunk.Text, md, chunk.Lang); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manifold_chunks_fts WHERE id = ?`, chunk.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO manifold_chunks_fts(id, doc_id, text, metadata, lang) VALUES(?, ?, ?, ?, ?)`, chunk.ID, chunk.DocID, chunk.Text, md, chunk.Lang); err != nil {
		return err
	}
	return tx.Commit()
}

func scanSQLiteSearchRows(rows *sql.Rows, limit int) ([]SearchResult, error) {
	defer func() { _ = rows.Close() }()
	out := make([]SearchResult, 0, limit)
	for rows.Next() {
		var result SearchResult
		var mdRaw string
		if err := rows.Scan(&result.ID, &result.Score, &result.Snippet, &result.Text, &mdRaw); err != nil {
			return nil, err
		}
		result.Metadata = decodeStringMap(mdRaw)
		out = append(out, result)
	}
	return out, rows.Err()
}

// sqliteFTSQueries builds a ranked list of fts5 MATCH expressions to try in order.
//
// Priority:
//  1. Exact AND of all terms (highest precision).
//  2. AND with prefix wildcards on terms ≥3 chars (handles morphological variants
//     not caught by the porter stemmer, and bypasses stemmer for prefix tokens).
//  3. OR of prefix wildcards (maximum recall — any term prefix is sufficient).
//     Note: we use prefix terms in the OR, not exact, because the porter stemmer
//     can change query-term stems in ways that don't match compound words (e.g.
//     "key" stems to "kei" and no longer prefix-matches "keyphrase"). Prefix
//     queries bypass the stemmer and match raw index tokens directly.
func sqliteFTSQueries(query string) []string {
	terms := sqliteFTSTerms(query)
	if len(terms) == 0 {
		return nil
	}

	// 1. Exact AND: "term1 term2"
	exact := strings.Join(terms, " ")

	// 2. Prefix AND: "term1* term2*" (short terms left as-is)
	prefixTerms := make([]string, 0, len(terms))
	for _, term := range terms {
		if len(term) < 3 {
			prefixTerms = append(prefixTerms, term)
			continue
		}
		prefixTerms = append(prefixTerms, term+"*")
	}
	prefix := strings.Join(prefixTerms, " ")

	// 3. OR of prefix terms: "term1* OR term2* OR term3*"
	// Using prefix terms (not exact) in the OR so that the porter-stemmer
	// bypass applies here too — giving recall even when stems diverge.
	orQuery := strings.Join(prefixTerms, " OR ")

	// Deduplicate while preserving order.
	seen := make(map[string]bool, 3)
	var out []string
	for _, q := range []string{exact, prefix, orQuery} {
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	return out
}

func sqliteFTSQuery(query string) string {
	return strings.Join(sqliteFTSTerms(query), " ")
}

func sqliteFTSTerms(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	terms := make([]string, 0, 4)
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		terms = append(terms, b.String())
		b.Reset()
	}
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return terms
}

func sqliteSearchCandidateLimit(limit int) int {
	if limit <= 0 {
		limit = 10
	}
	candidateLimit := limit * 10
	if candidateLimit < limit {
		return limit
	}
	if candidateLimit > maxSearchLimit {
		return maxSearchLimit
	}
	return candidateLimit
}

func sqliteChunkSearchFilter(filter map[string]string) map[string]string {
	if len(filter) == 0 {
		return nil
	}
	out := make(map[string]string, len(filter))
	for key, value := range filter {
		if key == "lang" {
			continue
		}
		out[key] = value
	}
	return out
}

func sqliteDocumentFallbackFilter(filter map[string]string) map[string]string {
	if len(filter) == 0 {
		return nil
	}
	out := make(map[string]string, len(filter))
	for key, value := range filter {
		if key == "lang" || key == "type" {
			continue
		}
		out[key] = value
	}
	return out
}

func chunkSearchMetadata(metadata map[string]string, lang string) map[string]string {
	out := copyMap(metadata)
	if out == nil {
		out = map[string]string{}
	}
	lang = strings.TrimSpace(lang)
	if lang != "" && out["lang"] == "" {
		out["lang"] = lang
	}
	return out
}

func encodeStringMap(m map[string]string) string {
	if m == nil {
		return "{}"
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodeStringMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]string{}
	}
	return out
}

func rollbackQuietly(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}
