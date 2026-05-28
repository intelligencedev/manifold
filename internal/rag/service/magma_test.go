package service

import (
	"context"
	"testing"

	"manifold/internal/persistence/databases"
	"manifold/internal/rag/ingest"
	"manifold/internal/rag/retrieve"
)

func TestService_MagmaOptInIngestAndRetrieve(t *testing.T) {
	t.Parallel()
	mgr := databases.Manager{
		Search: databases.NewMemorySearch(),
		Vector: databases.NewMemoryVector(),
		Graph:  databases.NewMemoryGraph(),
	}
	svc := New(mgr)
	ctx := context.Background()

	resp, err := svc.Ingest(ctx, ingest.IngestRequest{
		ID:     "doc:melanie",
		Text:   "Yesterday Melanie practiced guitar.",
		Tenant: "t1",
		Options: ingest.IngestOptions{
			Magma: ingest.MagmaOptions{Enabled: true, SessionID: "s1"},
		},
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if resp.MagmaEventID == "" {
		t.Fatalf("expected MAGMA event ID")
	}
	if _, err := svc.magma.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}

	got, err := svc.Retrieve(ctx, "When did Melanie practice?", retrieve.RetrieveOptions{
		Tenant:      "t1",
		IncludeText: true,
		Magma:       retrieve.MagmaRetrieveOptions{Enabled: true, IntentHint: "temporal", MaxNodes: 5},
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected one MAGMA result, got %#v", got.Items)
	}
	if got.Items[0].Metadata["source"] != "magma" {
		t.Fatalf("expected MAGMA source metadata, got %#v", got.Items[0].Metadata)
	}
	if magmaDebug, ok := got.Debug["magma"].(map[string]any); !ok || magmaDebug["enabled"] != true {
		t.Fatalf("missing MAGMA debug info: %#v", got.Debug)
	}
}
