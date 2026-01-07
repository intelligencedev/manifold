package databases

import (
	"context"
	"encoding/json"
	"errors"
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS evolving_memories_user_created_idx
    ON evolving_memories(user_id, created_at DESC);

ALTER TABLE evolving_memories
    ADD COLUMN IF NOT EXISTS session_id TEXT NOT NULL DEFAULT 'default';

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
SELECT id, input, output, feedback, summary, raw_trace, embedding, metadata, created_at
FROM evolving_memories
WHERE user_id = $1 AND session_id = $2
ORDER BY created_at ASC, id ASC`, userID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*memory.MemoryEntry
	for rows.Next() {
		var (
			id        uuid.UUID
			input     string
			output    string
			feedback  string
			summary   string
			rawTrace  string
			embBytes  []byte
			metaBytes []byte
			createdAt time.Time
		)

		if err := rows.Scan(&id, &input, &output, &feedback, &summary, &rawTrace, &embBytes, &metaBytes, &createdAt); err != nil {
			return nil, err
		}

		var embVec []float32
		if len(embBytes) > 0 {
			_ = json.Unmarshal(embBytes, &embVec)
		}

		md := map[string]interface{}{}
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &md)
		}

		entry := &memory.MemoryEntry{
			ID:        id.String(),
			Input:     input,
			Output:    output,
			Feedback:  feedback,
			Summary:   summary,
			RawTrace:  rawTrace,
			Embedding: embVec,
			Metadata:  md,
			CreatedAt: createdAt,
		}
		out = append(out, entry)
	}
	if out == nil {
		out = make([]*memory.MemoryEntry, 0)
	}
	return out, rows.Err()
}

// Save replaces all memory entries for the given user/session in a single transaction.
// This keeps the database representation aligned with the in-memory slice.
func (s *pgEvolvingMemoryStore) Save(ctx context.Context, userID int64, sessionID string, entries []*memory.MemoryEntry) error {
	if s.pool == nil {
		return errors.New("postgres evolving memory store requires pool")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "default"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM evolving_memories WHERE user_id = $1 AND session_id = $2`, userID, sessionID); err != nil {
		return err
	}

	for _, e := range entries {
		if e == nil {
			continue
		}
		id := e.ID
		if strings.TrimSpace(id) == "" {
			id = uuid.NewString()
		}
		createdAt := e.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		metaBytes, _ := json.Marshal(e.Metadata)
		embBytes, _ := json.Marshal(e.Embedding)
		if _, err := tx.Exec(ctx, `
INSERT INTO evolving_memories (id, user_id, session_id, input, output, feedback, summary, raw_trace, embedding, metadata, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			id, userID, sessionID, e.Input, e.Output, e.Feedback, e.Summary, e.RawTrace, embBytes, metaBytes, createdAt); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
