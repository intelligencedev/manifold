package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/persistence/databases"
	"manifold/internal/rag/ingest"
	"manifold/internal/rag/obs"
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

func TestService_MagmaIngestTopSemanticKOptionPersists(t *testing.T) {
	t.Parallel()
	mgr := databases.Manager{
		Search: databases.NewMemorySearch(),
		Vector: databases.NewMemoryVector(),
		Graph:  databases.NewMemoryGraph(),
	}
	svc := New(mgr)
	ctx := context.Background()

	resp, err := svc.Ingest(ctx, ingest.IngestRequest{
		ID:     "doc:top-k",
		Text:   "Melanie practiced guitar.",
		Tenant: "t1",
		Options: ingest.IngestOptions{
			Magma: ingest.MagmaOptions{Enabled: true, Graphs: []string{"semantic"}, TopSemanticK: 4},
		},
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.magma.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	event, ok := svc.magma.Event(ctx, resp.MagmaEventID)
	if !ok {
		t.Fatalf("expected MAGMA event")
	}
	if event.SemanticTopK != 4 {
		t.Fatalf("expected top semantic k from request, got %d", event.SemanticTopK)
	}
}

func TestService_MagmaRetrieveContextFormatText(t *testing.T) {
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
		ID:     "doc:context-format",
		Text:   "Yesterday Melanie practiced guitar.",
		Tenant: "t1",
		Options: ingest.IngestOptions{
			Magma: ingest.MagmaOptions{Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.magma.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	got, err := svc.Retrieve(ctx, "When did Melanie practice?", retrieve.RetrieveOptions{
		Tenant: "t1",
		Magma:  retrieve.MagmaRetrieveOptions{Enabled: true, IntentHint: "temporal", ContextFormat: "text"},
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	magmaDebug, ok := got.Debug["magma"].(map[string]any)
	if !ok {
		t.Fatalf("missing MAGMA debug info: %#v", got.Debug)
	}
	contextText, _ := magmaDebug["context"].(string)
	if strings.Contains(contextText, "Temporal timeline:") || !strings.Contains(contextText, "Melanie practiced guitar") {
		t.Fatalf("expected plain MAGMA context, got:\n%s", contextText)
	}
	if resp.MagmaEventID == "" {
		t.Fatalf("expected MAGMA event ID")
	}
}

func TestService_GraphAugmentAddsMagmaContext(t *testing.T) {
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
		ID:     "doc:graph-augment",
		Text:   "Yesterday Melanie practiced guitar.",
		Tenant: "t1",
		Options: ingest.IngestOptions{
			Magma: ingest.MagmaOptions{Enabled: true, SessionID: "s1"},
		},
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.magma.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}

	got, err := svc.Retrieve(ctx, "When did Melanie practice?", retrieve.RetrieveOptions{
		Tenant:       "t1",
		K:            5,
		FtK:          5,
		VecK:         0,
		IncludeText:  true,
		GraphAugment: true,
		Magma: retrieve.MagmaRetrieveOptions{
			IntentHint: "temporal",
			MaxNodes:   5,
		},
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if resp.MagmaEventID == "" {
		t.Fatal("expected MAGMA event ID")
	}
	foundMagma := false
	for _, item := range got.Items {
		if item.ID == resp.MagmaEventID && item.Metadata["source"] == "magma" {
			foundMagma = true
			break
		}
	}
	if !foundMagma {
		t.Fatalf("expected graph augmentation to append MAGMA event %q, got %#v", resp.MagmaEventID, got.Items)
	}
	magmaGraph, ok := got.Debug["magma_graph"].(map[string]any)
	if !ok || magmaGraph["expanded"] == nil || magmaGraph["intent"] == "" {
		t.Fatalf("expected MAGMA graph augmentation debug, got %#v", got.Debug)
	}
}

func TestService_MagmaRetrieveIntentClassificationFromConfig(t *testing.T) {
	t.Parallel()
	mgr := databases.Manager{
		Search: databases.NewMemorySearch(),
		Graph:  databases.NewMemoryGraph(),
	}
	svc := New(mgr, WithMagmaConfig(config.MagmaConfig{
		Enabled: true,
		Consolidation: config.MagmaConsolidationConfig{
			WorkerCount: -1,
		},
		Graphs: config.MagmaGraphsConfig{
			Semantic: config.MagmaSemanticGraphConfig{Enabled: true, TopK: 20},
			Temporal: config.MagmaTemporalGraphConfig{Enabled: true},
			Causal:   config.MagmaCausalGraphConfig{Enabled: true},
			Entity:   config.MagmaEntityGraphConfig{Enabled: true},
		},
		Retrieval: config.MagmaRetrievalConfig{
			DefaultHops:          2,
			DefaultMaxNodes:      5,
			IntentClassification: "semantic",
			ContextFormat:        "structured",
		},
	}))
	ctx := context.Background()

	resp, err := svc.Ingest(ctx, ingest.IngestRequest{ID: "doc:intent-mode", Text: "Melanie practiced guitar.", Tenant: "t1"})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.magma.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	if resp.MagmaEventID == "" {
		t.Fatalf("expected MAGMA event")
	}

	got, err := svc.Retrieve(ctx, "What do you know about Melanie?", retrieve.RetrieveOptions{Tenant: "t1"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got.Items) != 0 {
		t.Fatalf("expected semantic config mode not to use entity anchors, got %#v", got.Items)
	}
}

func TestService_MagmaEmitsSpecificMetrics(t *testing.T) {
	t.Parallel()
	mgr := databases.Manager{
		Search: databases.NewMemorySearch(),
		Vector: databases.NewMemoryVector(),
		Graph:  databases.NewMemoryGraph(),
	}
	metrics := obs.NewMockMetrics()
	svc := New(mgr, WithMagmaConfig(config.MagmaConfig{
		Enabled: true,
		Consolidation: config.MagmaConsolidationConfig{
			WorkerCount: -1,
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
	}), func(s *Service) { s.metrics = metrics })
	ctx := context.Background()

	resp, err := svc.Ingest(ctx, ingest.IngestRequest{
		ID:     "doc:metrics",
		Text:   "Yesterday Melanie practiced guitar.",
		Tenant: "t1",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.magma.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	got, err := svc.Retrieve(ctx, "When did Melanie practice?", retrieve.RetrieveOptions{Tenant: "t1"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(got.Items) == 0 || resp.MagmaEventID == "" {
		t.Fatalf("expected MAGMA ingest and retrieve to produce results")
	}
	for _, name := range []string{
		"magma_ingestion_fast_ms",
		"magma_ingestion_consolidation_ms",
		"magma_query_ms",
		"magma_traversal_hops",
		"magma_context_tokens",
	} {
		if _, ok := metrics.Hists[name]; !ok {
			t.Fatalf("expected histogram %s", name)
		}
	}
	if metrics.Counters["magma_events_total"] == 0 {
		t.Fatalf("expected magma_events_total counter")
	}
	if metrics.Counters["magma_intent_distribution"] == 0 {
		t.Fatalf("expected magma_intent_distribution counter")
	}
	magmaDebug, ok := got.Debug["magma"].(map[string]any)
	if !ok || magmaDebug["intent"] == "" || magmaDebug["graphs"] == "" {
		t.Fatalf("expected MAGMA debug policy metadata, got %#v", got.Debug["magma"])
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
