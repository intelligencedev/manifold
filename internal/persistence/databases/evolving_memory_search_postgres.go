package databases

import (
	"context"
	"errors"
	"manifold/internal/agent/memory"
	"strings"
)

func (s *pgEvolvingMemoryStore) SearchTopK(ctx context.Context, userID int64, sessionID string, queryVec []float32, k int) ([]memory.ScoredMemoryEntry, error) {
	if s.pool == nil {
		return nil, errors.New("postgres evolving memory store requires pool")
	}
	if len(queryVec) == 0 {
		return nil, nil
	}
	if k <= 0 {
		k = 4
	}
	sessionID = normalizeMemorySessionID(sessionID)
	vecLit := toVectorLiteral(queryVec)
	rows, err := s.pool.Query(ctx, `
SELECT id, input, output, feedback, summary, raw_trace, embedding, metadata,
       structured_feedback, memory_type, strategy_card, access_count,
       COALESCE(last_accessed_at, created_at), relevance_score, created_at,
			 scope, expires_at, 1 - (embedding_vec <=> $3::vector) AS score
FROM evolving_memories
WHERE user_id = $1
	AND ((session_id = $2 AND scope = 'session') OR scope = 'user')
	AND embedding_vec IS NOT NULL
	AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY embedding_vec <=> $3::vector
LIMIT $4`, userID, sessionID, vecLit, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]memory.ScoredMemoryEntry, 0, k)
	for rows.Next() {
		var record storedMemoryEntry
		var score float64
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
			&score,
		); err != nil {
			return nil, err
		}
		entry, err := decodeStoredMemoryEntry(record)
		if err != nil {
			return nil, err
		}
		out = append(out, memory.ScoredMemoryEntry{Entry: entry, Score: score})
	}
	return out, rows.Err()
}

// KeywordSearch retrieves memories by exact lexical match over task, summary,
// and strategy-card text. It is intended to be fused with dense retrieval.
func (s *pgEvolvingMemoryStore) KeywordSearch(ctx context.Context, userID int64, sessionID string, query string, k int) ([]memory.ScoredMemoryEntry, error) {
	if s.pool == nil {
		return nil, errors.New("postgres evolving memory store requires pool")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if k <= 0 {
		k = 4
	}
	sessionID = normalizeMemorySessionID(sessionID)
	rows, err := s.pool.Query(ctx, `
SELECT id, input, output, feedback, summary, raw_trace, embedding, metadata,
       structured_feedback, memory_type, strategy_card, access_count,
       COALESCE(last_accessed_at, created_at), relevance_score, created_at,
			 scope, expires_at,
       ts_rank(search_tsv, plainto_tsquery('simple', $3)) AS score
FROM evolving_memories
WHERE user_id = $1
	AND ((session_id = $2 AND scope = 'session') OR scope = 'user')
	AND search_tsv @@ plainto_tsquery('simple', $3)
	AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY score DESC, created_at DESC
LIMIT $4`, userID, sessionID, query, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]memory.ScoredMemoryEntry, 0, k)
	for rows.Next() {
		var record storedMemoryEntry
		var score float64
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
			&score,
		); err != nil {
			return nil, err
		}
		entry, err := decodeStoredMemoryEntry(record)
		if err != nil {
			return nil, err
		}
		out = append(out, memory.ScoredMemoryEntry{Entry: entry, Score: score})
	}
	return out, rows.Err()
}
