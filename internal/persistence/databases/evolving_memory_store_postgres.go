package databases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/agent/memory"
)

// NewPostgresEvolvingMemoryStore returns a Postgres-backed EvolvingMemoryStore.
//
// It mirrors the style of the ChatStore implementation and is intended to be
// constructed by higher-level factories (e.g., databases.Manager or agentd).
func NewPostgresEvolvingMemoryStore(pool *pgxpool.Pool) memory.EvolvingMemoryStore {
	return &pgEvolvingMemoryStore{pool: pool}
}

type pgEvolvingMemoryStore struct {
	pool *pgxpool.Pool
}

// Close closes the underlying pool if present.
func (s *pgEvolvingMemoryStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Init ensures the evolving_memories table exists.
func (s *pgEvolvingMemoryStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres evolving memory store requires pool")
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS evolving_memories (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL,
    session_id TEXT NOT NULL DEFAULT 'default',
    input TEXT NOT NULL,
    output TEXT NOT NULL,
    feedback TEXT NOT NULL,
    summary TEXT NOT NULL,
    raw_trace TEXT NOT NULL DEFAULT '',
    embedding BYTEA,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	structured_feedback JSONB,
	memory_type TEXT NOT NULL DEFAULT '',
	strategy_card TEXT NOT NULL DEFAULT '',
	access_count INTEGER NOT NULL DEFAULT 0,
	last_accessed_at TIMESTAMPTZ,
	relevance_score DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS evolving_memories_user_created_idx
    ON evolving_memories(user_id, created_at DESC);

ALTER TABLE evolving_memories
    ADD COLUMN IF NOT EXISTS session_id TEXT NOT NULL DEFAULT 'default';

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS structured_feedback JSONB;

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS memory_type TEXT NOT NULL DEFAULT '';

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS strategy_card TEXT NOT NULL DEFAULT '';

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS access_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS last_accessed_at TIMESTAMPTZ;

ALTER TABLE evolving_memories
	ADD COLUMN IF NOT EXISTS relevance_score DOUBLE PRECISION NOT NULL DEFAULT 1.0;

CREATE INDEX IF NOT EXISTS evolving_memories_user_session_created_idx
    ON evolving_memories(user_id, session_id, created_at DESC);
`)
	return err
}

// Load returns all memory entries for a given user/session ordered by creation time.
func (s *pgEvolvingMemoryStore) Load(ctx context.Context, userID int64, sessionID string) ([]*memory.MemoryEntry, error) {
	if s.pool == nil {
		return nil, errors.New("postgres evolving memory store requires pool")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "default"
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, input, output, feedback, summary, raw_trace, embedding, metadata,
       structured_feedback, memory_type, strategy_card, access_count,
       COALESCE(last_accessed_at, created_at), relevance_score, created_at
FROM evolving_memories
WHERE user_id = $1 AND session_id = $2
ORDER BY created_at ASC, id ASC`, userID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*memory.MemoryEntry
	for rows.Next() {
		var record storedMemoryEntry

		if err := rows.Scan(
			&record.ID,
			&record.Input,
			&record.Output,
			&record.Feedback,
			&record.Summary,
			&record.RawTrace,
			&record.Embedding,
			&record.Metadata,
			&record.StructuredFeedback,
			&record.MemoryType,
			&record.StrategyCard,
			&record.AccessCount,
			&record.LastAccessedAt,
			&record.RelevanceScore,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}

		entry, err := decodeStoredMemoryEntry(record)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if out == nil {
		out = make([]*memory.MemoryEntry, 0)
	}
	return out, rows.Err()
}

// ListSessions returns session IDs that currently have persisted evolving memory.
func (s *pgEvolvingMemoryStore) ListSessions(ctx context.Context, userID int64) ([]string, error) {
	if s.pool == nil {
		return nil, errors.New("postgres evolving memory store requires pool")
	}

	rows, err := s.pool.Query(ctx, `
SELECT session_id
FROM evolving_memories
WHERE user_id = $1
GROUP BY session_id
ORDER BY MAX(created_at) DESC, session_id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []string
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, err
		}
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			continue
		}
		sessions = append(sessions, sessionID)
	}
	if sessions == nil {
		sessions = make([]string, 0)
	}
	return sessions, rows.Err()
}

// Save upserts the current memory entries for the given user/session and removes
// rows that are no longer present in the in-memory snapshot.
func (s *pgEvolvingMemoryStore) Save(ctx context.Context, userID int64, sessionID string, entries []*memory.MemoryEntry) error {
	if s.pool == nil {
		return errors.New("postgres evolving memory store requires pool")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	records, activeIDs, err := prepareStoredMemoryEntries(entries)
	if err != nil {
		return err
	}

	if len(activeIDs) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM evolving_memories WHERE user_id = $1 AND session_id = $2`, userID, sessionID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM evolving_memories
WHERE user_id = $1 AND session_id = $2 AND NOT (id = ANY($3::uuid[]))`, userID, sessionID, activeIDs); err != nil {
		return err
	}

	for _, record := range records {
		if _, err := tx.Exec(ctx, `
INSERT INTO evolving_memories (
    id, user_id, session_id, input, output, feedback, summary, raw_trace,
    embedding, metadata, structured_feedback, memory_type, strategy_card,
    access_count, last_accessed_at, relevance_score, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    session_id = EXCLUDED.session_id,
    input = EXCLUDED.input,
    output = EXCLUDED.output,
    feedback = EXCLUDED.feedback,
    summary = EXCLUDED.summary,
    raw_trace = EXCLUDED.raw_trace,
    embedding = EXCLUDED.embedding,
    metadata = EXCLUDED.metadata,
    structured_feedback = EXCLUDED.structured_feedback,
    memory_type = EXCLUDED.memory_type,
    strategy_card = EXCLUDED.strategy_card,
    access_count = EXCLUDED.access_count,
    last_accessed_at = EXCLUDED.last_accessed_at,
    relevance_score = EXCLUDED.relevance_score,
    created_at = EXCLUDED.created_at`,
			record.ID, userID, sessionID, record.Input, record.Output, record.Feedback, record.Summary, record.RawTrace,
			record.Embedding, record.Metadata, record.StructuredFeedback, record.MemoryType, record.StrategyCard,
			record.AccessCount, record.LastAccessedAt, record.RelevanceScore, record.CreatedAt); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

type storedMemoryEntry struct {
	ID                 uuid.UUID
	Input              string
	Output             string
	Feedback           string
	Summary            string
	RawTrace           string
	Embedding          []byte
	Metadata           []byte
	StructuredFeedback []byte
	MemoryType         string
	StrategyCard       string
	AccessCount        int
	LastAccessedAt     time.Time
	RelevanceScore     float64
	CreatedAt          time.Time
}

func encodeStoredMemoryEntry(entry *memory.MemoryEntry) (storedMemoryEntry, error) {
	if entry == nil {
		return storedMemoryEntry{}, errors.New("memory entry cannot be nil")
	}

	metadataBytes, err := json.Marshal(entry.Metadata)
	if err != nil {
		return storedMemoryEntry{}, fmt.Errorf("marshal metadata: %w", err)
	}
	embeddingBytes, err := json.Marshal(entry.Embedding)
	if err != nil {
		return storedMemoryEntry{}, fmt.Errorf("marshal embedding: %w", err)
	}

	var structuredFeedbackBytes []byte
	if entry.StructuredFeedback != nil {
		structuredFeedbackBytes, err = json.Marshal(entry.StructuredFeedback)
		if err != nil {
			return storedMemoryEntry{}, fmt.Errorf("marshal structured feedback: %w", err)
		}
	}

	lastAccessedAt := entry.LastAccessedAt
	if lastAccessedAt.IsZero() {
		lastAccessedAt = entry.CreatedAt
	}
	createdAt := entry.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	id := strings.TrimSpace(entry.ID)
	if id == "" {
		id = uuid.NewString()
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return storedMemoryEntry{}, fmt.Errorf("parse memory id: %w", err)
	}

	return storedMemoryEntry{
		ID:                 parsedID,
		Input:              entry.Input,
		Output:             entry.Output,
		Feedback:           entry.Feedback,
		Summary:            entry.Summary,
		RawTrace:           entry.RawTrace,
		Embedding:          embeddingBytes,
		Metadata:           metadataBytes,
		StructuredFeedback: structuredFeedbackBytes,
		MemoryType:         string(entry.MemoryType),
		StrategyCard:       entry.StrategyCard,
		AccessCount:        entry.AccessCount,
		LastAccessedAt:     lastAccessedAt,
		RelevanceScore:     entry.RelevanceScore,
		CreatedAt:          createdAt,
	}, nil
}

func prepareStoredMemoryEntries(entries []*memory.MemoryEntry) ([]storedMemoryEntry, []uuid.UUID, error) {
	records := make([]storedMemoryEntry, 0, len(entries))
	activeIDs := make([]uuid.UUID, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		record, err := encodeStoredMemoryEntry(entry)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, record)
		activeIDs = append(activeIDs, record.ID)
	}
	return records, activeIDs, nil
}

func decodeStoredMemoryEntry(record storedMemoryEntry) (*memory.MemoryEntry, error) {
	var embedding []float32
	if len(record.Embedding) > 0 {
		if err := json.Unmarshal(record.Embedding, &embedding); err != nil {
			return nil, fmt.Errorf("unmarshal embedding: %w", err)
		}
	}

	metadata := map[string]interface{}{}
	if len(record.Metadata) > 0 {
		if err := json.Unmarshal(record.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	var structuredFeedback *memory.StructuredFeedback
	if len(record.StructuredFeedback) > 0 && string(record.StructuredFeedback) != "null" {
		structuredFeedback = &memory.StructuredFeedback{}
		if err := json.Unmarshal(record.StructuredFeedback, structuredFeedback); err != nil {
			return nil, fmt.Errorf("unmarshal structured feedback: %w", err)
		}
	}

	return &memory.MemoryEntry{
		ID:                 record.ID.String(),
		Input:              record.Input,
		Output:             record.Output,
		Feedback:           record.Feedback,
		Summary:            record.Summary,
		RawTrace:           record.RawTrace,
		Embedding:          embedding,
		Metadata:           metadata,
		StructuredFeedback: structuredFeedback,
		MemoryType:         memory.MemoryType(record.MemoryType),
		StrategyCard:       record.StrategyCard,
		AccessCount:        record.AccessCount,
		LastAccessedAt:     record.LastAccessedAt,
		RelevanceScore:     record.RelevanceScore,
		CreatedAt:          record.CreatedAt,
	}, nil
}
