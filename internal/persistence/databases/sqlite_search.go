package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
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
	q := sqliteFTSQuery(query)
	if q == "" {
		return nil, nil
	}
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
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
	defer func() { _ = rows.Close() }()
	return scanSQLiteSearchRows(rows, limit)
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
	q := sqliteFTSQuery(query)
	if q == "" {
		return nil, nil
	}
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, -bm25(manifold_chunks_fts) AS score,
       snippet(manifold_chunks_fts, 2, '', '', '...', 12) AS snippet,
       text,
       metadata
FROM manifold_chunks_fts
WHERE manifold_chunks_fts MATCH ?
ORDER BY bm25(manifold_chunks_fts)
LIMIT ?`, q, limit*10)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results, err := scanSQLiteSearchRows(rows, limit*10)
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
	q := sqliteFTSQuery(query)
	if q == "" {
		return "", false, nil
	}
	if err := s.Init(ctx); err != nil {
		return "", false, err
	}
	stmt := `SELECT snippet(manifold_documents_fts, 1, '', '', '...', 12) FROM manifold_documents_fts WHERE id = ? AND manifold_documents_fts MATCH ? LIMIT 1`
	if strings.HasPrefix(id, "chunk:") {
		stmt = `SELECT snippet(manifold_chunks_fts, 2, '', '', '...', 12) FROM manifold_chunks_fts WHERE id = ? AND manifold_chunks_fts MATCH ? LIMIT 1`
	}
	var snippet string
	if err := s.db.QueryRowContext(ctx, stmt, id, q).Scan(&snippet); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return snippet, true, nil
}

func (s *sqliteSearch) HasChunksTable(context.Context) (bool, error) {
	return true, nil
}

func (s *sqliteSearch) UpsertChunk(ctx context.Context, chunk ChunkSearchRow) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	md := encodeStringMap(chunk.Metadata)
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

func sqliteFTSQuery(query string) string {
	terms := strings.Fields(strings.TrimSpace(query))
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.Trim(term, `"`)
		if term == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " ")
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
