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
	tokenize='unicode61'
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
	tokenize='unicode61'
);
`)
	return err
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

func sqliteFTSQueries(query string) []string {
	terms := sqliteFTSTerms(query)
	if len(terms) == 0 {
		return nil
	}
	exact := strings.Join(terms, " ")
	prefixTerms := make([]string, 0, len(terms))
	for _, term := range terms {
		if len(term) < 3 {
			prefixTerms = append(prefixTerms, term)
			continue
		}
		prefixTerms = append(prefixTerms, term+"*")
	}
	prefix := strings.Join(prefixTerms, " ")
	if prefix == exact {
		return []string{exact}
	}
	return []string{exact, prefix}
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
