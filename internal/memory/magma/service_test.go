package magma

import (
	"context"
	"testing"
	"time"

	"manifold/internal/persistence/databases"
	"manifold/internal/rag/embedder"
)

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
