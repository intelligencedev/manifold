package databases

import (
	"testing"
	"time"

	"manifold/internal/agent/memory"

	"github.com/google/uuid"
)

func TestStoredMemoryEntryRoundTrip(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	lastAccessedAt := createdAt.Add(5 * time.Minute)
	entry := &memory.MemoryEntry{
		ID:             uuid.NewString(),
		Input:          "input",
		Output:         "output",
		Feedback:       "success",
		Summary:        "summary",
		RawTrace:       "trace",
		Embedding:      []float32{1, 2, 3},
		Metadata:       map[string]interface{}{"domain": "test"},
		CreatedAt:      createdAt,
		MemoryType:     memory.MemoryProcedural,
		StrategyCard:   "When confronted with X, do Y.",
		AccessCount:    4,
		LastAccessedAt: lastAccessedAt,
		RelevanceScore: 1.25,
		StructuredFeedback: &memory.StructuredFeedback{
			Type:         memory.FeedbackSuccess,
			Correct:      true,
			ProgressRate: 0.75,
			StepsUsed:    3,
			StepsOptimal: 2,
			Message:      "solid result",
		},
	}

	record, err := encodeStoredMemoryEntry(entry)
	if err != nil {
		t.Fatalf("encodeStoredMemoryEntry failed: %v", err)
	}
	record.ID = uuid.MustParse(entry.ID)
	record.Input = entry.Input
	record.Output = entry.Output
	record.Feedback = entry.Feedback
	record.Summary = entry.Summary
	record.RawTrace = entry.RawTrace
	record.MemoryType = string(entry.MemoryType)
	record.StrategyCard = entry.StrategyCard
	record.AccessCount = entry.AccessCount
	record.RelevanceScore = entry.RelevanceScore
	record.CreatedAt = entry.CreatedAt

	decoded, err := decodeStoredMemoryEntry(record)
	if err != nil {
		t.Fatalf("decodeStoredMemoryEntry failed: %v", err)
	}

	if decoded.ID != entry.ID {
		t.Fatalf("expected ID %q, got %q", entry.ID, decoded.ID)
	}
	if decoded.MemoryType != entry.MemoryType {
		t.Fatalf("expected memory type %q, got %q", entry.MemoryType, decoded.MemoryType)
	}
	if decoded.StrategyCard != entry.StrategyCard {
		t.Fatalf("expected strategy card %q, got %q", entry.StrategyCard, decoded.StrategyCard)
	}
	if decoded.AccessCount != entry.AccessCount {
		t.Fatalf("expected access count %d, got %d", entry.AccessCount, decoded.AccessCount)
	}
	if !decoded.LastAccessedAt.Equal(entry.LastAccessedAt) {
		t.Fatalf("expected last accessed %v, got %v", entry.LastAccessedAt, decoded.LastAccessedAt)
	}
	if decoded.RelevanceScore != entry.RelevanceScore {
		t.Fatalf("expected relevance score %f, got %f", entry.RelevanceScore, decoded.RelevanceScore)
	}
	if decoded.StructuredFeedback == nil {
		t.Fatal("expected structured feedback to round-trip")
	}
	if decoded.StructuredFeedback.Message != entry.StructuredFeedback.Message {
		t.Fatalf("expected structured feedback message %q, got %q", entry.StructuredFeedback.Message, decoded.StructuredFeedback.Message)
	}
	if decoded.Metadata["domain"] != entry.Metadata["domain"] {
		t.Fatalf("expected metadata to round-trip, got %#v", decoded.Metadata)
	}
}

func TestEncodeStoredMemoryEntryDefaultsLastAccessedAt(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	record, err := encodeStoredMemoryEntry(&memory.MemoryEntry{CreatedAt: createdAt})
	if err != nil {
		t.Fatalf("encodeStoredMemoryEntry failed: %v", err)
	}
	if !record.LastAccessedAt.Equal(createdAt) {
		t.Fatalf("expected last accessed default to created_at, got %v want %v", record.LastAccessedAt, createdAt)
	}
}

func TestPrepareStoredMemoryEntriesSkipsNilAndNormalizesIDs(t *testing.T) {
	t.Parallel()

	records, ids, err := prepareStoredMemoryEntries([]*memory.MemoryEntry{
		nil,
		{Input: "a", CreatedAt: time.Now().UTC()},
		{ID: uuid.NewString(), Input: "b", CreatedAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("prepareStoredMemoryEntries failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	if records[0].ID == uuid.Nil {
		t.Fatal("expected generated ID for entry without one")
	}
	if records[1].ID != ids[1] {
		t.Fatalf("expected IDs slice to match record IDs, got %v and %v", records[1].ID, ids[1])
	}
}

func TestPrepareStoredMemoryEntriesRejectsInvalidID(t *testing.T) {
	t.Parallel()

	_, _, err := prepareStoredMemoryEntries([]*memory.MemoryEntry{{ID: "not-a-uuid"}})
	if err == nil {
		t.Fatal("expected invalid UUID error")
	}
}
