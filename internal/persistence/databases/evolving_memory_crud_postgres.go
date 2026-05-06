package databases

import (
	"context"
	"errors"
	"fmt"
	"manifold/internal/agent/memory"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *pgEvolvingMemoryStore) Load(ctx context.Context, userID int64, sessionID string) ([]*memory.MemoryEntry, error) {
	if s.pool == nil {
		return nil, errors.New("postgres evolving memory store requires pool")
	}
	sessionID = normalizeMemorySessionID(sessionID)
	rows, err := s.pool.Query(ctx, `
SELECT id, input, output, feedback, summary, raw_trace, embedding, metadata,
       structured_feedback, memory_type, strategy_card, access_count,
			 COALESCE(last_accessed_at, created_at), relevance_score, created_at,
			 scope, expires_at
FROM evolving_memories
WHERE user_id = $1 AND session_id = $2 AND scope = 'session'
	AND (expires_at IS NULL OR expires_at > NOW())
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
			&record.Scope,
			&record.ExpiresAt,
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
	sessionID = normalizeMemorySessionID(sessionID)

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
		if err := upsertStoredMemoryRecord(ctx, tx, userID, sessionID, record); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// Upsert persists changed memory entries without deleting other rows in the session.
func (s *pgEvolvingMemoryStore) Upsert(ctx context.Context, userID int64, sessionID string, entries []*memory.MemoryEntry) error {
	if s.pool == nil {
		return errors.New("postgres evolving memory store requires pool")
	}
	sessionID = normalizeMemorySessionID(sessionID)

	records, _, err := prepareStoredMemoryEntries(entries)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, record := range records {
		if err := upsertStoredMemoryRecord(ctx, tx, userID, sessionID, record); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// Delete removes memory entries by ID for a user/session without rewriting the full session.
func (s *pgEvolvingMemoryStore) Delete(ctx context.Context, userID int64, sessionID string, ids []string) error {
	if s.pool == nil {
		return errors.New("postgres evolving memory store requires pool")
	}
	parsedIDs, err := parseUUIDStrings(ids)
	if err != nil {
		return err
	}
	if len(parsedIDs) == 0 {
		return nil
	}
	sessionID = normalizeMemorySessionID(sessionID)
	_, err = s.pool.Exec(ctx, `
DELETE FROM evolving_memories
WHERE user_id = $1 AND session_id = $2 AND id = ANY($3::uuid[])`, userID, sessionID, parsedIDs)
	return err
}

// TouchAccess increments access counters for memory entries after retrieval.
func (s *pgEvolvingMemoryStore) TouchAccess(ctx context.Context, ids []string, at time.Time) error {
	if s.pool == nil {
		return errors.New("postgres evolving memory store requires pool")
	}
	parsedIDs, err := parseUUIDStrings(ids)
	if err != nil {
		return err
	}
	if len(parsedIDs) == 0 {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err = s.pool.Exec(ctx, `
UPDATE evolving_memories
SET access_count = access_count + 1,
    last_accessed_at = $2
WHERE id = ANY($1::uuid[])`, parsedIDs, at)
	return err
}

// PromoteToUserScope makes a high-quality memory available across sessions for a user.
func (s *pgEvolvingMemoryStore) PromoteToUserScope(ctx context.Context, userID int64, entryID string) error {
	if s.pool == nil {
		return errors.New("postgres evolving memory store requires pool")
	}
	parsedID, err := uuid.Parse(strings.TrimSpace(entryID))
	if err != nil {
		return fmt.Errorf("parse memory id: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
UPDATE evolving_memories
SET scope = 'user'
WHERE user_id = $1 AND id = $2`, userID, parsedID)
	return err
}

// DeleteExpired removes memories whose retention deadline has passed.
func (s *pgEvolvingMemoryStore) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	if s.pool == nil {
		return 0, errors.New("postgres evolving memory store requires pool")
	}
	if before.IsZero() {
		before = time.Now().UTC()
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM evolving_memories WHERE expires_at IS NOT NULL AND expires_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
