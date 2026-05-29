package agentd

import (
	"context"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent/memory"
	"manifold/internal/memory/magma"
	"manifold/internal/persistence/databases"
	"manifold/internal/rag/embedder"
)

func TestEvolvingMagmaSinkIngestsMemoryEntry(t *testing.T) {
	t.Parallel()

	svc := magma.NewService(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0))
	defer svc.Close()
	sink := evolvingMagmaSink{service: svc, workerCount: 0}
	entry := &memory.MemoryEntry{
		ID:           "entry-1",
		Input:        "deploy the API",
		Output:       "deployment succeeded",
		Feedback:     "success",
		Summary:      "Use the release checklist.",
		StrategyCard: "Run tests before rollout.",
		MemoryType:   memory.MemoryProcedural,
		Scope:        memory.MemoryScopeSession,
		CreatedAt:    time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	}

	eventID, err := sink.IngestEvolvingMemory(context.Background(), 42, "session-a", entry)
	if err != nil {
		t.Fatalf("IngestEvolvingMemory() error = %v", err)
	}
	if eventID == "" {
		t.Fatal("expected MAGMA event ID")
	}
	event, ok := svc.Event(context.Background(), eventID)
	if !ok {
		t.Fatalf("expected MAGMA event %q", eventID)
	}
	if event.Tenant != "user:42" || event.Session != "session-a" {
		t.Fatalf("unexpected event scope: tenant=%q session=%q", event.Tenant, event.Session)
	}
	for _, want := range []string{"Input: deploy the API", "Summary: Use the release checklist.", "Strategy: Run tests before rollout."} {
		if !strings.Contains(event.Text, want) {
			t.Fatalf("event text missing %q:\n%s", want, event.Text)
		}
	}
}
