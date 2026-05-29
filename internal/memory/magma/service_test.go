package magma

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"manifold/internal/persistence/databases"
	"manifold/internal/rag/embedder"
)

type failingEmbedder struct{}

func (failingEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embed failed")
}

func (failingEmbedder) Name() string               { return "failing" }
func (failingEmbedder) Dimension() int             { return 0 }
func (failingEmbedder) Ping(context.Context) error { return nil }

func TestService_IngestConsolidateAndQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	graph := databases.NewMemoryGraph()
	vector := databases.NewMemoryVector()
	svc := NewService(graph, vector, embedder.NewDeterministic(32, true, 0))

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-hike",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Yesterday Melanie hiked because the weather improved.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if resp.EventID != "event:t1:melanie-hike" {
		t.Fatalf("unexpected event ID: %q", resp.EventID)
	}

	n, err := svc.DrainConsolidation(ctx, 1)
	if err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("expected one consolidated event, got %d", n)
	}

	event, ok := svc.Event(ctx, resp.EventID)
	if !ok {
		t.Fatalf("expected stored event")
	}
	if event.TemporalAttrs.Date != "2026-05-27" {
		t.Fatalf("expected normalized yesterday date, got %#v", event.TemporalAttrs)
	}
	if len(event.EntityMentions) != 1 || event.EntityMentions[0].ID != "entity:melanie" {
		t.Fatalf("unexpected entities: %#v", event.EntityMentions)
	}

	result, err := svc.Query(ctx, "When did Melanie hike?", QueryOptions{Tenant: "t1", MaxNodes: 5})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.RawEvents) != 1 {
		t.Fatalf("expected one query event, got %#v", result.RawEvents)
	}
	if result.TemporalTimeline[0].Date != "2026-05-27" {
		t.Fatalf("unexpected timeline: %#v", result.TemporalTimeline)
	}
	causal, err := svc.Query(ctx, "Why did Melanie hike?", QueryOptions{Tenant: "t1", MaxNodes: 5})
	if err != nil {
		t.Fatalf("causal Query() error = %v", err)
	}
	if len(causal.CausalChain) != 1 {
		t.Fatalf("expected one causal chain entry, got %#v", causal.CausalChain)
	}
	for _, want := range []string{"Temporal timeline:", "Entity profiles:", "Causal chain:", "the weather improved"} {
		if !strings.Contains(causal.Text, want) {
			t.Fatalf("expected context to contain %q, got:\n%s", want, causal.Text)
		}
	}
}

func TestClassifyIntent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
		want  IntentCategory
	}{
		{name: "temporal", query: "What happened before the hike?", want: IntentTemporal},
		{name: "entity", query: "Who are Melanie's friends?", want: IntentEntity},
		{name: "causal", query: "Why did she leave?", want: IntentCausal},
		{name: "semantic default", query: "Tell me about music", want: IntentSemantic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := ClassifyIntent(tt.query)
			if got&tt.want == 0 {
				t.Fatalf("ClassifyIntent(%q) = %b, want bit %b", tt.query, got, tt.want)
			}
		})
	}
}

func TestExtractCausalStatement(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		text       string
		wantCause  string
		wantEffect string
		wantOK     bool
	}{
		{name: "because", text: "Melanie hiked because the weather improved", wantCause: "the weather improved", wantEffect: "Melanie hiked", wantOK: true},
		{name: "caused", text: "Rain caused the delay", wantCause: "Rain", wantEffect: "the delay", wantOK: true},
		{name: "so", text: "The guitar broke so Melanie switched instruments", wantCause: "The guitar broke", wantEffect: "Melanie switched instruments", wantOK: true},
		{name: "none", text: "Melanie practiced guitar", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCause, gotEffect, gotOK := ExtractCausalStatement(tt.text)
			if gotOK != tt.wantOK || gotCause != tt.wantCause || gotEffect != tt.wantEffect {
				t.Fatalf("ExtractCausalStatement() = (%q, %q, %v), want (%q, %q, %v)", gotCause, gotEffect, gotOK, tt.wantCause, tt.wantEffect, tt.wantOK)
			}
		})
	}
}

func TestService_ConsolidationBuildsTemporalAndEntityEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0))

	first, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-first",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Melanie practiced guitar.",
		CreatedAt: time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("first Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("first DrainConsolidation() error = %v", err)
	}

	second, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-second",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Melanie performed guitar.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("second Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("second DrainConsolidation() error = %v", err)
	}

	before, err := svc.store.Neighbors(ctx, first.EventID, GraphTemporal, "BEFORE")
	if err != nil {
		t.Fatalf("temporal BEFORE neighbors error = %v", err)
	}
	if len(before) != 1 || before[0] != second.EventID {
		t.Fatalf("expected %s BEFORE %s, got %#v", first.EventID, second.EventID, before)
	}
	mentions, err := svc.store.Neighbors(ctx, first.EventID, GraphEntity, "MENTIONS")
	if err != nil {
		t.Fatalf("entity MENTIONS neighbors error = %v", err)
	}
	if len(mentions) != 1 || mentions[0] != "entity:melanie" {
		t.Fatalf("expected event-to-entity mention, got %#v", mentions)
	}
}

func TestService_ConfigDisablesGraphViews(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		Graphs: GraphConfig{Temporal: true},
	})

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-temporal-only",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Yesterday Melanie hiked because the weather improved.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	event, ok := svc.Event(ctx, resp.EventID)
	if !ok {
		t.Fatalf("expected event")
	}
	if event.TemporalAttrs.Date != "2026-05-27" {
		t.Fatalf("expected temporal normalization, got %#v", event.TemporalAttrs)
	}
	if len(event.EntityMentions) != 0 {
		t.Fatalf("expected entity graph disabled, got entities %#v", event.EntityMentions)
	}
	causal, err := svc.store.Neighbors(ctx, resp.EventID, GraphCausal, "CAUSES")
	if err != nil {
		t.Fatalf("causal neighbors error = %v", err)
	}
	if len(causal) != 0 {
		t.Fatalf("expected causal graph disabled, got %#v", causal)
	}
}

func TestService_ConfigControlsSemanticThreshold(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vector := databases.NewMemoryVector()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), vector, embedder.NewDeterministic(32, true, 0), ServiceConfig{
		SemanticTopK:        5,
		SimilarityThreshold: 1.1,
		Graphs:              GraphConfig{Semantic: true},
	})

	first, err := svc.Ingest(ctx, IngestRequest{ID: "first", Tenant: "t1", Text: "Melanie practiced guitar."})
	if err != nil {
		t.Fatalf("first Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("first DrainConsolidation() error = %v", err)
	}
	second, err := svc.Ingest(ctx, IngestRequest{ID: "second", Tenant: "t1", Text: "Melanie practiced guitar."})
	if err != nil {
		t.Fatalf("second Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("second DrainConsolidation() error = %v", err)
	}
	neighbors, err := svc.store.Neighbors(ctx, second.EventID, GraphSemantic, "SIMILAR_TO")
	if err != nil {
		t.Fatalf("semantic neighbors error = %v", err)
	}
	if len(neighbors) != 0 {
		t.Fatalf("expected semantic threshold to prune links between %s and %s, got %#v", first.EventID, second.EventID, neighbors)
	}
}

func TestService_ReportsQueueFull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		QueueSize: 1,
	})

	first, err := svc.Ingest(ctx, IngestRequest{ID: "first", Tenant: "t1", Text: "Melanie practiced guitar."})
	if err != nil {
		t.Fatalf("first Ingest() error = %v", err)
	}
	if first.Status != "queued" {
		t.Fatalf("expected first event queued, got %q", first.Status)
	}
	second, err := svc.Ingest(ctx, IngestRequest{ID: "second", Tenant: "t1", Text: "Melanie practiced piano."})
	if err != nil {
		t.Fatalf("second Ingest() error = %v", err)
	}
	if second.Status != "queue_full" {
		t.Fatalf("expected queue_full status, got %q", second.Status)
	}
	stats := svc.Stats()
	if stats.QueueDepth != 1 || stats.DroppedTotal != 1 {
		t.Fatalf("unexpected stats after queue full: %#v", stats)
	}
}

func TestService_StatsTrackConsolidationFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), nil, failingEmbedder{}, ServiceConfig{})
	if err := svc.store.StoreEvent(ctx, EventNode{ID: "event:t1:broken", Tenant: "t1", Text: "Broken event"}); err != nil {
		t.Fatalf("StoreEvent() error = %v", err)
	}
	if !svc.enqueue("event:t1:broken") {
		t.Fatalf("expected enqueue")
	}
	processed, err := svc.DrainConsolidation(ctx, 1)
	if err == nil {
		t.Fatalf("expected consolidation error")
	}
	if processed != 0 {
		t.Fatalf("expected no successful events, got %d", processed)
	}
	stats := svc.Stats()
	if stats.FailedTotal != 1 || stats.ProcessedTotal != 0 || !strings.Contains(stats.LastError, "embed failed") {
		t.Fatalf("unexpected failure stats: %#v", stats)
	}
}

func TestService_QueryUsesEntityAnchorsWithoutVectorStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(databases.NewMemoryGraph(), nil, embedder.NewDeterministic(32, true, 0))

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-guitar",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Melanie practiced guitar.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}

	result, err := svc.Query(ctx, "What do you know about Melanie?", QueryOptions{Tenant: "t1", MaxNodes: 5})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.RawEvents) != 1 || result.RawEvents[0].ID != resp.EventID {
		t.Fatalf("expected entity-anchored event %s, got %#v", resp.EventID, result.RawEvents)
	}
	profile, ok := result.EntityProfile["entity:melanie"]
	if !ok || len(profile.Events) != 1 {
		t.Fatalf("expected Melanie entity profile, got %#v", result.EntityProfile)
	}
}

func TestSelectPolicy_MixedIntentUsesUnionOfGraphViews(t *testing.T) {
	t.Parallel()
	policy := SelectPolicy(IntentTemporal | IntentEntity)
	if !slices.Contains(policy.GraphViews, GraphTemporal) || !slices.Contains(policy.GraphViews, GraphEntity) || !slices.Contains(policy.GraphViews, GraphSemantic) {
		t.Fatalf("expected temporal/entity/semantic graph views, got %#v", policy.GraphViews)
	}
	if policy.MaxHops < 2 {
		t.Fatalf("expected mixed policy to allow entity traversal, got %d hops", policy.MaxHops)
	}
	if policy.AnchorStrategy != AnchorEntity {
		t.Fatalf("expected entity anchor strategy, got %q", policy.AnchorStrategy)
	}
}

func TestService_StartConsolidationWorkersProcessesQueue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0))
	t.Cleanup(svc.Close)
	svc.StartConsolidationWorkers(ctx, 1)

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-practice",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Yesterday Melanie practiced guitar.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		event, ok := svc.Event(ctx, resp.EventID)
		if ok && event.TemporalAttrs.Date == "2026-05-27" {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for async consolidation")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
