package databases

import (
	"context"
	"strings"
	"testing"

	"manifold/internal/persistence"
)

func TestMatrixMessageInsertSQLMatchesPartialEventIndex(t *testing.T) {
	t.Parallel()
	if !strings.Contains(matrixMessageInsertSQL, "ON CONFLICT (event_id) WHERE event_id IS NOT NULL AND event_id <> ''") {
		t.Fatalf("matrix message insert must target the partial unique event index: %s", matrixMessageInsertSQL)
	}
}

func TestMemoryMatrixMessageStoreAppendAndPrune(t *testing.T) {
	t.Parallel()
	store := newMemoryMatrixMessageStore()
	ctx := context.Background()

	for idx := range 5 {
		_, err := store.Append(ctx, persistence.MatrixMessage{
			RoomID:    "!room:test",
			EventID:   "$event-" + string(rune('a'+idx)),
			Direction: "inbound",
			Sender:    "@user:test",
			Body:      "message",
			MsgType:   "m.text",
		}, 3)
		if err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	messages, err := store.ListByRoom(ctx, "!room:test", 10, 0)
	if err != nil {
		t.Fatalf("ListByRoom() error = %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 retained messages, got %d", len(messages))
	}
	stats, err := store.RoomStats(ctx, "!room:test")
	if err != nil {
		t.Fatalf("RoomStats() error = %v", err)
	}
	if stats.MessageCount != 3 {
		t.Fatalf("expected message count 3, got %d", stats.MessageCount)
	}
}
