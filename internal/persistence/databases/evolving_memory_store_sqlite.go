package databases

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/agent/memory"
)

type sqliteEvolvingMemoryStore struct {
	db         *sql.DB
	dimensions int
}

func NewSQLiteEvolvingMemoryStore(db *sql.DB) memory.EvolvingMemoryStore {
	return NewSQLiteEvolvingMemoryStoreWithDimensions(db, 0)
}

func NewSQLiteEvolvingMemoryStoreWithDimensions(db *sql.DB, dimensions int) memory.EvolvingMemoryStore {
	return &sqliteEvolvingMemoryStore{db: db, dimensions: dimensions}
}

func (s *sqliteEvolvingMemoryStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite evolving memory store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS evolving_memories (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	session_id TEXT NOT NULL,
	input TEXT NOT NULL DEFAULT '',
	output TEXT NOT NULL DEFAULT '',
	feedback TEXT NOT NULL DEFAULT '',
	summary TEXT NOT NULL DEFAULT '',
	raw_trace TEXT NOT NULL DEFAULT '',
	embedding TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(embedding)),
	metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata)),
	structured_feedback TEXT CHECK (structured_feedback IS NULL OR json_valid(structured_feedback)),
	memory_type TEXT NOT NULL DEFAULT '',
	strategy_card TEXT NOT NULL DEFAULT '',
	scope TEXT NOT NULL DEFAULT 'session',
	expires_at DATETIME,
	access_count INTEGER NOT NULL DEFAULT 0,
	last_accessed_at DATETIME,
	relevance_score REAL NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS evolving_memories_user_session_created_idx
	ON evolving_memories(user_id, session_id, created_at);
CREATE INDEX IF NOT EXISTS evolving_memories_user_scope_idx
	ON evolving_memories(user_id, scope);
CREATE INDEX IF NOT EXISTS evolving_memories_expires_idx
	ON evolving_memories(expires_at);
`)
	return err
}

func (s *sqliteEvolvingMemoryStore) Load(ctx context.Context, userID int64, sessionID string) ([]*memory.MemoryEntry, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	sessionID = normalizeMemorySessionID(sessionID)
	rows, err := s.db.QueryContext(ctx, sqliteEvolvingSelectSQL+`
WHERE user_id = ? AND session_id = ? AND scope = 'session'
	AND (expires_at IS NULL OR expires_at > ?)
ORDER BY created_at ASC, id ASC`, userID, sessionID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanSQLiteEvolvingRows(rows)
}

func (s *sqliteEvolvingMemoryStore) ListSessions(ctx context.Context, userID int64) ([]string, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT session_id
FROM evolving_memories
WHERE user_id = ?
GROUP BY session_id
ORDER BY MAX(created_at) DESC, session_id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(sessionID) != "" {
			out = append(out, sessionID)
		}
	}
	return out, rows.Err()
}

func (s *sqliteEvolvingMemoryStore) Save(ctx context.Context, userID int64, sessionID string, entries []*memory.MemoryEntry) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	sessionID = normalizeMemorySessionID(sessionID)
	records, activeIDs, err := prepareStoredMemoryEntries(entries)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)

	if len(activeIDs) == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM evolving_memories WHERE user_id = ? AND session_id = ?`, userID, sessionID); err != nil {
			return err
		}
		return tx.Commit()
	}
	idValues := make([]string, 0, len(activeIDs))
	for _, id := range activeIDs {
		idValues = append(idValues, id.String())
	}
	inSQL, args := sqliteStringInClause(idValues)
	args = append([]any{userID, sessionID}, args...)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM evolving_memories
WHERE user_id = ? AND session_id = ? AND id NOT IN (%s)`, inSQL), args...); err != nil {
		return err
	}
	for _, record := range records {
		if err := upsertSQLiteStoredMemoryRecord(ctx, tx, userID, sessionID, record); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteEvolvingMemoryStore) Upsert(ctx context.Context, userID int64, sessionID string, entries []*memory.MemoryEntry) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	sessionID = normalizeMemorySessionID(sessionID)
	records, _, err := prepareStoredMemoryEntries(entries)
	if err != nil || len(records) == 0 {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	for _, record := range records {
		if err := upsertSQLiteStoredMemoryRecord(ctx, tx, userID, sessionID, record); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteEvolvingMemoryStore) Delete(ctx context.Context, userID int64, sessionID string, ids []string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	idValues, err := normalizeMemoryIDs(ids)
	if err != nil || len(idValues) == 0 {
		return err
	}
	sessionID = normalizeMemorySessionID(sessionID)
	inSQL, args := sqliteStringInClause(idValues)
	args = append([]any{userID, sessionID}, args...)
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM evolving_memories
WHERE user_id = ? AND session_id = ? AND id IN (%s)`, inSQL), args...)
	return err
}

func (s *sqliteEvolvingMemoryStore) TouchAccess(ctx context.Context, ids []string, at time.Time) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	idValues, err := normalizeMemoryIDs(ids)
	if err != nil || len(idValues) == 0 {
		return err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	inSQL, args := sqliteStringInClause(idValues)
	args = append([]any{at.UTC()}, args...)
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`
UPDATE evolving_memories
SET access_count = access_count + 1,
	last_accessed_at = ?
WHERE id IN (%s)`, inSQL), args...)
	return err
}

func (s *sqliteEvolvingMemoryStore) PromoteToUserScope(ctx context.Context, userID int64, entryID string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	idValues, err := normalizeMemoryIDs([]string{entryID})
	if err != nil || len(idValues) == 0 {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE evolving_memories SET scope = 'user' WHERE user_id = ? AND id = ?`, userID, idValues[0])
	return err
}

func (s *sqliteEvolvingMemoryStore) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	if err := s.Init(ctx); err != nil {
		return 0, err
	}
	if before.IsZero() {
		before = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM evolving_memories WHERE expires_at IS NOT NULL AND expires_at < ?`, before.UTC())
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

func (s *sqliteEvolvingMemoryStore) SearchTopK(ctx context.Context, userID int64, sessionID string, queryVec []float32, k int) ([]memory.ScoredMemoryEntry, error) {
	if len(queryVec) == 0 {
		return nil, nil
	}
	entries, err := s.loadSearchEntries(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if k <= 0 {
		k = 4
	}
	out := make([]memory.ScoredMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || len(entry.Embedding) == 0 {
			continue
		}
		out = append(out, memory.ScoredMemoryEntry{Entry: entry, Score: cosineMemoryVectors(queryVec, entry.Embedding)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}

func (s *sqliteEvolvingMemoryStore) KeywordSearch(ctx context.Context, userID int64, sessionID string, query string, k int) ([]memory.ScoredMemoryEntry, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	entries, err := s.loadSearchEntries(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if k <= 0 {
		k = 4
	}
	terms := strings.Fields(strings.ToLower(query))
	out := make([]memory.ScoredMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		text := strings.ToLower(strings.Join([]string{entry.Input, entry.Output, entry.Feedback, entry.Summary, entry.RawTrace, entry.StrategyCard}, "\n"))
		matches := 0
		for _, term := range terms {
			matches += strings.Count(text, term)
		}
		if matches > 0 {
			out = append(out, memory.ScoredMemoryEntry{Entry: entry, Score: float64(matches) / float64(len(terms))})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Entry.CreatedAt.After(out[j].Entry.CreatedAt)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}

func (s *sqliteEvolvingMemoryStore) loadSearchEntries(ctx context.Context, userID int64, sessionID string) ([]*memory.MemoryEntry, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	sessionID = normalizeMemorySessionID(sessionID)
	rows, err := s.db.QueryContext(ctx, sqliteEvolvingSelectSQL+`
WHERE user_id = ?
	AND ((session_id = ? AND scope = 'session') OR scope = 'user')
	AND (expires_at IS NULL OR expires_at > ?)
ORDER BY created_at ASC, id ASC`, userID, sessionID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanSQLiteEvolvingRows(rows)
}

func upsertSQLiteStoredMemoryRecord(ctx context.Context, tx *sql.Tx, userID int64, sessionID string, record storedMemoryEntry) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO evolving_memories (
	id, user_id, session_id, input, output, feedback, summary, raw_trace, embedding,
	metadata, structured_feedback, memory_type, strategy_card, scope, expires_at,
	access_count, last_accessed_at, relevance_score, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	user_id = excluded.user_id,
	session_id = excluded.session_id,
	input = excluded.input,
	output = excluded.output,
	feedback = excluded.feedback,
	summary = excluded.summary,
	raw_trace = excluded.raw_trace,
	embedding = excluded.embedding,
	metadata = excluded.metadata,
	structured_feedback = excluded.structured_feedback,
	memory_type = excluded.memory_type,
	strategy_card = excluded.strategy_card,
	scope = excluded.scope,
	expires_at = excluded.expires_at,
	access_count = excluded.access_count,
	last_accessed_at = excluded.last_accessed_at,
	relevance_score = excluded.relevance_score,
	created_at = excluded.created_at
`, record.ID.String(), userID, sessionID, record.Input, record.Output, record.Feedback, record.Summary, record.RawTrace, string(record.Embedding),
		string(record.Metadata), nullableJSONText(record.StructuredFeedback), record.MemoryType, record.StrategyCard, record.Scope, record.ExpiresAt,
		record.AccessCount, record.LastAccessedAt, record.RelevanceScore, record.CreatedAt)
	return err
}

func scanSQLiteEvolvingRows(rows *sql.Rows) ([]*memory.MemoryEntry, error) {
	out := []*memory.MemoryEntry{}
	for rows.Next() {
		record, err := scanSQLiteStoredMemoryRecord(rows)
		if err != nil {
			return nil, err
		}
		entry, err := decodeStoredMemoryEntry(record)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func scanSQLiteStoredMemoryRecord(row interface{ Scan(dest ...any) error }) (storedMemoryEntry, error) {
	var record storedMemoryEntry
	var id string
	var embedding, metadata string
	var structuredFeedback sql.NullString
	var expiresAt sql.NullTime
	if err := row.Scan(
		&id,
		&record.Input,
		&record.Output,
		&record.Feedback,
		&record.Summary,
		&record.RawTrace,
		&embedding,
		&metadata,
		&structuredFeedback,
		&record.MemoryType,
		&record.StrategyCard,
		&record.AccessCount,
		&record.LastAccessedAt,
		&record.RelevanceScore,
		&record.CreatedAt,
		&record.Scope,
		&expiresAt,
	); err != nil {
		return storedMemoryEntry{}, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return storedMemoryEntry{}, fmt.Errorf("parse memory id: %w", err)
	}
	record.ID = parsedID
	record.Embedding = []byte(embedding)
	record.Metadata = []byte(metadata)
	if structuredFeedback.Valid {
		record.StructuredFeedback = []byte(structuredFeedback.String)
	}
	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		record.ExpiresAt = &t
	}
	return record, nil
}

const sqliteEvolvingSelectSQL = `
SELECT id, input, output, feedback, summary, raw_trace, embedding, metadata,
	structured_feedback, memory_type, strategy_card, access_count,
	COALESCE(last_accessed_at, created_at), relevance_score, created_at, scope, expires_at
FROM evolving_memories
`

func normalizeMemoryIDs(ids []string) ([]string, error) {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		parsedID, err := uuid.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("parse memory id: %w", err)
		}
		out = append(out, parsedID.String())
	}
	return out, nil
}

func sqliteStringInClause(values []string) (string, []any) {
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, value := range values {
		placeholders[i] = "?"
		args[i] = value
	}
	return strings.Join(placeholders, ","), args
}

func nullableJSONText(raw []byte) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}

func cosineMemoryVectors(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		normA += af * af
		normB += bf * bf
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
