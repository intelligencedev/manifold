package databases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"manifold/internal/agent/memory"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type txExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func upsertStoredMemoryRecord(ctx context.Context, tx txExecutor, userID int64, sessionID string, record storedMemoryEntry) error {
	_, err := tx.Exec(ctx, `
INSERT INTO evolving_memories (
    id, user_id, session_id, input, output, feedback, summary, raw_trace,
	embedding, embedding_vec, metadata, structured_feedback, memory_type, strategy_card,
	scope, expires_at, access_count, last_accessed_at, relevance_score, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, '')::vector, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    session_id = EXCLUDED.session_id,
    input = EXCLUDED.input,
    output = EXCLUDED.output,
    feedback = EXCLUDED.feedback,
    summary = EXCLUDED.summary,
    raw_trace = EXCLUDED.raw_trace,
    embedding = EXCLUDED.embedding,
	    embedding_vec = EXCLUDED.embedding_vec,
    metadata = EXCLUDED.metadata,
    structured_feedback = EXCLUDED.structured_feedback,
    memory_type = EXCLUDED.memory_type,
    strategy_card = EXCLUDED.strategy_card,
		scope = EXCLUDED.scope,
		expires_at = EXCLUDED.expires_at,
    access_count = EXCLUDED.access_count,
    last_accessed_at = EXCLUDED.last_accessed_at,
    relevance_score = EXCLUDED.relevance_score,
    created_at = EXCLUDED.created_at`,
		record.ID, userID, sessionID, record.Input, record.Output, record.Feedback, record.Summary, record.RawTrace,
		record.Embedding, record.EmbeddingVector, record.Metadata, record.StructuredFeedback, record.MemoryType, record.StrategyCard,
		record.Scope, record.ExpiresAt, record.AccessCount, record.LastAccessedAt, record.RelevanceScore, record.CreatedAt)
	return err
}

func normalizeMemorySessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "default"
	}
	return sessionID
}

func parseUUIDStrings(ids []string) ([]uuid.UUID, error) {
	parsedIDs := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		parsedID, err := uuid.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("parse memory id: %w", err)
		}
		parsedIDs = append(parsedIDs, parsedID)
	}
	return parsedIDs, nil
}

type storedMemoryEntry struct {
	ID                 uuid.UUID
	Input              string
	Output             string
	Feedback           string
	Summary            string
	RawTrace           string
	Embedding          []byte
	EmbeddingVector    string
	Metadata           []byte
	StructuredFeedback []byte
	MemoryType         string
	StrategyCard       string
	Scope              string
	ExpiresAt          *time.Time
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

	embeddingVector := ""
	if len(entry.Embedding) > 0 {
		embeddingVector = toVectorLiteral(entry.Embedding)
	}

	scope := string(entry.Scope)
	if strings.TrimSpace(scope) == "" {
		scope = string(memory.MemoryScopeSession)
	}

	return storedMemoryEntry{
		ID:                 parsedID,
		Input:              entry.Input,
		Output:             entry.Output,
		Feedback:           entry.Feedback,
		Summary:            entry.Summary,
		RawTrace:           entry.RawTrace,
		Embedding:          embeddingBytes,
		EmbeddingVector:    embeddingVector,
		Metadata:           metadataBytes,
		StructuredFeedback: structuredFeedbackBytes,
		MemoryType:         string(entry.MemoryType),
		StrategyCard:       entry.StrategyCard,
		Scope:              scope,
		ExpiresAt:          entry.ExpiresAt,
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

	metadata := map[string]any{}
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
	scope := memory.MemoryScope(record.Scope)
	if scope == "" {
		scope = memory.MemoryScopeSession
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
		Scope:              scope,
		ExpiresAt:          record.ExpiresAt,
		AccessCount:        record.AccessCount,
		LastAccessedAt:     record.LastAccessedAt,
		RelevanceScore:     record.RelevanceScore,
		CreatedAt:          record.CreatedAt,
	}, nil
}
