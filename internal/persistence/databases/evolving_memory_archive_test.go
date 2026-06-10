package databases

import (
	"context"
	"testing"
	"time"

	"manifold/internal/agent/memory"
)

func TestSQLiteEvolvingMemoryArchive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewSQLiteEvolvingMemoryStore(openTestSQLite(t))
	archive, ok := store.(memory.EvolvingMemoryArchiveStore)
	if !ok {
		t.Fatalf("sqlite evolving store does not implement archive store")
	}
	entry := &memory.MemoryEntry{
		ID:        "11111111-1111-1111-1111-111111111111",
		Input:     "why postgres?",
		Output:    "jsonb preserves payloads",
		CreatedAt: time.Now().UTC(),
	}
	if err := archive.Archive(ctx, 7, "session-1", []*memory.MemoryEntry{entry}, "relevance"); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	var count int
	if err := openCountQuery(store, &count); err != nil {
		t.Fatalf("count archive rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one archive row, got %d", count)
	}
}

func openCountQuery(store memory.EvolvingMemoryStore, dest *int) error {
	sqliteStore := store.(*sqliteEvolvingMemoryStore)
	return sqliteStore.db.QueryRow(`SELECT COUNT(*) FROM evolving_memory_archive WHERE user_id = 7 AND session_id = 'session-1' AND reason = 'relevance'`).Scan(dest)
}
