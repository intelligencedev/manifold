package service

import (
	"context"
	"testing"
	"time"

	"manifold/internal/config"
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
	t.Cleanup(svc.Close)
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

	deadline := time.After(2 * time.Second)
	for {
		event, ok := svc.magma.Event(ctx, resp.MagmaEventID)
		if ok && event.TemporalAttrs.Date != "" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for async MAGMA consolidation")
		case <-time.After(10 * time.Millisecond):
		}
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

func TestService_MagmaConfigEnablesDefaultIngestAndRetrieve(t *testing.T) {
	t.Parallel()
	mgr := databases.Manager{
		Search: databases.NewMemorySearch(),
		Vector: databases.NewMemoryVector(),
		Graph:  databases.NewMemoryGraph(),
	}
	svc := New(mgr, WithMagmaConfig(config.MagmaConfig{
		Enabled: true,
		Consolidation: config.MagmaConsolidationConfig{
			WorkerCount: 1,
			Model:       "test-model",
		},
		Graphs: config.MagmaGraphsConfig{
			Semantic: config.MagmaSemanticGraphConfig{Enabled: true, TopK: 20},
			Temporal: config.MagmaTemporalGraphConfig{Enabled: true},
			Causal:   config.MagmaCausalGraphConfig{Enabled: true},
			Entity:   config.MagmaEntityGraphConfig{Enabled: true},
		},
		Retrieval: config.MagmaRetrievalConfig{
			DefaultHops:     2,
			DefaultMaxNodes: 5,
			ContextFormat:   "structured",
		},
	}))
	t.Cleanup(svc.Close)
	ctx := context.Background()

	resp, err := svc.Ingest(ctx, ingest.IngestRequest{
		ID:     "doc:configured-melanie",
		Text:   "Melanie practiced piano.",
		Tenant: "t1",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if resp.MagmaEventID == "" {
		t.Fatalf("expected MAGMA event ID from config default")
	}
	waitForMagmaEvent(t, ctx, svc, resp.MagmaEventID)

	got, err := svc.Retrieve(ctx, "What did Melanie practice?", retrieve.RetrieveOptions{Tenant: "t1", IncludeText: true})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Metadata["source"] != "magma" {
		t.Fatalf("expected config-default MAGMA retrieval, got %#v", got.Items)
	}
}

func TestService_MagmaQueueFullWarning(t *testing.T) {
	t.Parallel()
	mgr := databases.Manager{
		Search: databases.NewMemorySearch(),
		Vector: databases.NewMemoryVector(),
		Graph:  databases.NewMemoryGraph(),
	}
	svc := New(mgr, WithMagmaConfig(config.MagmaConfig{
		Enabled: true,
		Consolidation: config.MagmaConsolidationConfig{
			MaxQueueSize: 1,
			WorkerCount:  -1,
		},
	}))
	ctx := context.Background()

	first, err := svc.Ingest(ctx, ingest.IngestRequest{ID: "doc:first", Text: "Melanie practiced guitar.", Tenant: "t1"})
	if err != nil {
		t.Fatalf("first Ingest() error = %v", err)
	}
	if len(first.Warnings) != 0 {
		t.Fatalf("unexpected first warnings: %#v", first.Warnings)
	}
	second, err := svc.Ingest(ctx, ingest.IngestRequest{ID: "doc:second", Text: "Melanie practiced piano.", Tenant: "t1"})
	if err != nil {
		t.Fatalf("second Ingest() error = %v", err)
	}
	if len(second.Warnings) != 1 {
		t.Fatalf("expected queue full warning, got %#v", second.Warnings)
	}
}

func TestService_MagmaIngestGraphsOptionRestrictsConsolidation(t *testing.T) {
	t.Parallel()
	mgr := databases.Manager{
		Search: databases.NewMemorySearch(),
		Vector: databases.NewMemoryVector(),
		Graph:  databases.NewMemoryGraph(),
	}
	svc := New(mgr)
	ctx := context.Background()

	resp, err := svc.Ingest(ctx, ingest.IngestRequest{
		ID:     "doc:temporal-only",
		Text:   "Yesterday Melanie hiked because the weather improved.",
		Tenant: "t1",
		Options: ingest.IngestOptions{
			Magma: ingest.MagmaOptions{Enabled: true, Graphs: []string{"temporal"}},
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
	event, ok := svc.magma.Event(ctx, resp.MagmaEventID)
	if !ok {
		t.Fatalf("expected event")
	}
	if len(event.EntityMentions) != 0 {
		t.Fatalf("expected per-request graph list to omit entity extraction, got %#v", event.EntityMentions)
	}
}

func waitForMagmaEvent(t *testing.T, ctx context.Context, svc *Service, eventID string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		event, ok := svc.magma.Event(ctx, eventID)
		if ok && event.TemporalAttrs.Date != "" {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for async MAGMA consolidation")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
