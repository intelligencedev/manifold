package magma

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"manifold/internal/llm"
	"manifold/internal/persistence/databases"
	"manifold/internal/rag/embedder"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type failingEmbedder struct{}

func (failingEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("embed failed")
}

func (failingEmbedder) Name() string               { return "failing" }
func (failingEmbedder) Dimension() int             { return 0 }
func (failingEmbedder) Ping(context.Context) error { return nil }

type fakeConsolidationLLM struct {
	response     string
	model        string
	calls        int
	systemPrompt string
}

func (f *fakeConsolidationLLM) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, model string) (llm.Message, error) {
	f.calls++
	f.model = model
	if len(msgs) > 0 {
		f.systemPrompt = msgs[0].Content
	}
	return llm.Message{Role: "assistant", Content: f.response}, nil
}

func (f *fakeConsolidationLLM) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return nil
}

type recordingVector struct {
	lastK int
}

type recordingObserver struct {
	counters   map[string]int
	histograms map[string][]float64
}

func newRecordingObserver() *recordingObserver {
	return &recordingObserver{
		counters:   map[string]int{},
		histograms: map[string][]float64{},
	}
}

func (r *recordingObserver) IncCounter(name string, _ map[string]string) {
	r.counters[name]++
}

func (r *recordingObserver) ObserveHistogram(name string, value float64, _ map[string]string) {
	r.histograms[name] = append(r.histograms[name], value)
}

func (r *recordingVector) Upsert(context.Context, string, []float32, map[string]string) error {
	return nil
}

func (r *recordingVector) Delete(context.Context, string) error {
	return nil
}

func (r *recordingVector) SimilaritySearch(_ context.Context, _ []float32, k int, _ map[string]string) ([]databases.VectorResult, error) {
	r.lastK = k
	return nil, nil
}

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
	if len(event.EntityMentions) != 1 || event.EntityMentions[0].ID != "entity:t1:melanie" {
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
	plain, err := svc.Query(ctx, "Why did Melanie hike?", QueryOptions{Tenant: "t1", MaxNodes: 5, ContextFormat: "text"})
	if err != nil {
		t.Fatalf("plain Query() error = %v", err)
	}
	if strings.Contains(plain.Text, "Temporal timeline:") || !strings.Contains(plain.Text, "Melanie hiked") {
		t.Fatalf("expected plain context without structured headings, got:\n%s", plain.Text)
	}
}

func TestService_ObservabilityAccessorsExposeStoredGraph(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0))

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-observe",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Melanie practiced guitar with Nora.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}

	events, err := svc.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) != 1 || events[0].ID != resp.EventID {
		t.Fatalf("Events() = %#v, want %q", events, resp.EventID)
	}

	edges, err := svc.Edges(ctx)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}
	if len(edges) == 0 {
		t.Fatalf("Edges() returned no graph edges")
	}
	if !svc.MaintenanceEnabled() {
		t.Fatalf("MaintenanceEnabled() = false, want true")
	}

	var nilSvc *Service
	if events, err := nilSvc.Events(ctx); err != nil || events != nil {
		t.Fatalf("nil Events() = (%#v, %v), want nil, nil", events, err)
	}
	if edges, err := nilSvc.Edges(ctx); err != nil || edges != nil {
		t.Fatalf("nil Edges() = (%#v, %v), want nil, nil", edges, err)
	}
	if nilSvc.MaintenanceEnabled() {
		t.Fatalf("nil MaintenanceEnabled() = true, want false")
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
	if len(mentions) != 1 || mentions[0] != "entity:t1:melanie" {
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

func TestService_EntityCoReferenceConfigControlsRelatedEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		Graphs: GraphConfig{Entity: true, CoReference: false},
	})

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:     "co-ref-disabled",
		Tenant: "t1",
		Text:   "Melanie and Daniel practiced guitar.",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	event, ok := svc.Event(ctx, resp.EventID)
	if !ok || len(event.EntityMentions) != 2 {
		t.Fatalf("expected two entity mentions, got %#v", event.EntityMentions)
	}
	related, err := svc.store.Neighbors(ctx, "entity:t1:melanie", GraphEntity, "RELATED_TO")
	if err != nil {
		t.Fatalf("RELATED_TO neighbors error = %v", err)
	}
	if len(related) != 0 {
		t.Fatalf("expected co-reference disabled to skip RELATED_TO edges, got %#v", related)
	}
	mentions, err := svc.store.Neighbors(ctx, "entity:t1:melanie", GraphEntity, "MENTIONS")
	if err != nil {
		t.Fatalf("MENTIONS neighbors error = %v", err)
	}
	if len(mentions) != 1 || mentions[0] != resp.EventID {
		t.Fatalf("expected MENTIONS edge to remain, got %#v", mentions)
	}
}

func TestService_EntityCoReferenceEnabledCreatesRelatedEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		Graphs: GraphConfig{Entity: true, CoReference: true},
	})

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:     "co-ref-enabled",
		Tenant: "t1",
		Text:   "Melanie and Daniel practiced guitar.",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	if resp.EventID == "" {
		t.Fatalf("expected event ID")
	}
	related, err := svc.store.Neighbors(ctx, "entity:t1:melanie", GraphEntity, "RELATED_TO")
	if err != nil {
		t.Fatalf("RELATED_TO neighbors error = %v", err)
	}
	if len(related) != 1 || related[0] != "entity:t1:daniel" {
		t.Fatalf("expected co-reference RELATED_TO edge, got %#v", related)
	}
}

func TestService_IngestGraphsOverrideServiceDefaults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0))

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-temporal-request",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Yesterday Melanie hiked because the weather improved.",
		Graphs:    []GraphType{GraphTemporal},
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
	if !slices.Equal(event.Graphs, []GraphType{GraphTemporal}) {
		t.Fatalf("expected stored graph override, got %#v", event.Graphs)
	}
	if len(event.EntityMentions) != 0 {
		t.Fatalf("expected entity graph omitted by per-event override, got %#v", event.EntityMentions)
	}
	causal, err := svc.store.Neighbors(ctx, resp.EventID, GraphCausal, "CAUSES")
	if err != nil {
		t.Fatalf("causal neighbors error = %v", err)
	}
	if len(causal) != 0 {
		t.Fatalf("expected causal graph omitted by per-event override, got %#v", causal)
	}
	temporal, err := svc.store.Neighbors(ctx, resp.EventID, GraphTemporal, "CONCURRENT")
	if err != nil {
		t.Fatalf("temporal neighbors error = %v", err)
	}
	if len(temporal) != 1 || temporal[0] != resp.EventID {
		t.Fatalf("expected temporal graph retained, got %#v", temporal)
	}
}

func TestService_IngestSemanticTopKOverridesServiceDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vector := &recordingVector{}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), vector, embedder.NewDeterministic(32, true, 0), ServiceConfig{
		SemanticTopK: 20,
		Graphs:       GraphConfig{Semantic: true},
	})

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:           "semantic-top-k",
		Tenant:       "t1",
		Text:         "Melanie practiced guitar.",
		SemanticTopK: 3,
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
	if event.SemanticTopK != 3 {
		t.Fatalf("expected semantic top-k to persist, got %d", event.SemanticTopK)
	}
	if vector.lastK != 3 {
		t.Fatalf("expected semantic search top-k override 3, got %d", vector.lastK)
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

func TestService_CollectConsolidationBatchHonorsLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		BatchSize: 3,
		QueueSize: 4,
	})
	for _, id := range []string{"first", "second", "third", "fourth"} {
		resp, err := svc.Ingest(ctx, IngestRequest{ID: id, Tenant: "t1", Text: "Melanie practiced guitar."})
		if err != nil {
			t.Fatalf("Ingest(%q) error = %v", id, err)
		}
		if resp.Status != "queued" {
			t.Fatalf("expected %q queued, got %q", id, resp.Status)
		}
	}

	first := <-svc.queue
	batch := svc.collectConsolidationBatch(first, svc.cfg.BatchSize)
	if len(batch) != 3 {
		t.Fatalf("expected batch of 3, got %#v", batch)
	}
	if got := svc.Stats().QueueDepth; got != 1 {
		t.Fatalf("expected one event left queued, got %d", got)
	}
}

func TestService_DrainConsolidationProcessesBatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		BatchSize: 2,
	})
	eventIDs := make([]string, 0, 3)
	for _, id := range []string{"first", "second", "third"} {
		resp, err := svc.Ingest(ctx, IngestRequest{
			ID:        id,
			Tenant:    "t1",
			Text:      "Yesterday Melanie practiced guitar.",
			CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("Ingest(%q) error = %v", id, err)
		}
		eventIDs = append(eventIDs, resp.EventID)
	}

	processed, err := svc.DrainConsolidation(ctx, 3)
	if err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	if processed != 3 {
		t.Fatalf("expected 3 processed events, got %d", processed)
	}
	stats := svc.Stats()
	if stats.QueueDepth != 0 || stats.ProcessedTotal != 3 || stats.FailedTotal != 0 {
		t.Fatalf("unexpected stats after batch drain: %#v", stats)
	}
	for _, eventID := range eventIDs {
		event, ok := svc.Event(ctx, eventID)
		if !ok || event.TemporalAttrs.Date != "2026-05-27" {
			t.Fatalf("expected consolidated event %q, got %#v ok=%v", eventID, event, ok)
		}
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

func TestService_ConsolidationEmitsQualityMetrics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	observer := newRecordingObserver()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		Observer: observer,
		Graphs:   GraphConfig{Entity: true},
	})

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:     "quality-metrics",
		Tenant: "t1",
		Text:   "Melanie practiced guitar.",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	if resp.EventID == "" {
		t.Fatalf("expected event ID")
	}
	for _, name := range []string{
		"magma_ingestion_consolidation_ms",
		"magma_consolidation_success_rate",
		"magma_entity_resolution_accuracy",
	} {
		if len(observer.histograms[name]) == 0 {
			t.Fatalf("expected histogram %s", name)
		}
	}
	if got := observer.histograms["magma_consolidation_success_rate"][0]; got != 1 {
		t.Fatalf("expected success sample 1, got %f", got)
	}
	if got := observer.histograms["magma_entity_resolution_accuracy"][0]; got != 1 {
		t.Fatalf("expected entity resolution accuracy 1, got %f", got)
	}
}

func TestService_ConsolidationUsesLLMExtractionWhenConfigured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	provider := &fakeConsolidationLLM{response: `{
		"temporal": {"date": "2026-05-20", "offset": "-192h"},
		"entities": [{"name": "Melanie", "type": "Person", "role": "friend"}],
		"causal": [{"cause": "rain", "effect": "Melanie stayed inside", "confidence": 0.91}]
	}`}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		LLM:             provider,
		Model:           "magma-test-model",
		CausalThreshold: 0.8,
		Graphs:          GraphConfig{Temporal: true, Entity: true, Causal: true},
	})

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "llm-extraction",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Melanie stayed inside.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	if provider.model != "magma-test-model" {
		t.Fatalf("expected configured model, got %q", provider.model)
	}
	if !strings.Contains(provider.systemPrompt, "Extract MAGMA memory structure") {
		t.Fatalf("expected default extraction prompt, got %q", provider.systemPrompt)
	}
	event, ok := svc.Event(ctx, resp.EventID)
	if !ok {
		t.Fatalf("expected event")
	}
	if event.TemporalAttrs.Date != "2026-05-20" || event.TemporalAttrs.Offset != "-192h" {
		t.Fatalf("expected LLM temporal attrs, got %#v", event.TemporalAttrs)
	}
	if len(event.EntityMentions) != 1 || event.EntityMentions[0].ID != "entity:t1:melanie" || event.EntityMentions[0].Type != "Person" || event.EntityMentions[0].Role != "friend" {
		t.Fatalf("expected LLM entity mention, got %#v", event.EntityMentions)
	}
	causal, err := svc.store.Neighbors(ctx, resp.EventID, GraphCausal, "CAUSES")
	if err != nil {
		t.Fatalf("causal neighbors error = %v", err)
	}
	if len(causal) != 1 || causal[0] != resp.EventID {
		t.Fatalf("expected LLM causal edge, got %#v", causal)
	}
}

func TestService_ConsolidationUsesConfiguredExtractionPrompt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	provider := &fakeConsolidationLLM{response: `{"entities":[{"name":"Melanie"}]}`}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		LLM: provider,
		Prompts: PromptConfig{
			ConsolidationExtraction: "custom extraction prompt",
		},
		Graphs: GraphConfig{Entity: true},
	})

	if _, err := svc.Ingest(ctx, IngestRequest{ID: "custom-prompt", Tenant: "t1", Text: "Melanie practiced guitar."}); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	if provider.systemPrompt != "custom extraction prompt" {
		t.Fatalf("expected custom extraction prompt, got %q", provider.systemPrompt)
	}
}

func TestService_ConsolidationFiltersLowConfidenceLLMCausalEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	provider := &fakeConsolidationLLM{response: `{
		"entities": [{"name": "Melanie"}],
		"causal": [{"cause": "rain", "effect": "Melanie stayed inside", "confidence": 0.2}]
	}`}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		LLM:             provider,
		CausalThreshold: 0.8,
		Graphs:          GraphConfig{Entity: true, Causal: true},
	})

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:     "low-confidence-causal",
		Tenant: "t1",
		Text:   "Melanie stayed inside.",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	causal, err := svc.store.Neighbors(ctx, resp.EventID, GraphCausal, "CAUSES")
	if err != nil {
		t.Fatalf("causal neighbors error = %v", err)
	}
	if len(causal) != 0 {
		t.Fatalf("expected low-confidence LLM causal edge to be filtered, got %#v", causal)
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
	profile, ok := result.EntityProfile["entity:t1:melanie"]
	if !ok || len(profile.Events) != 1 {
		t.Fatalf("expected Melanie entity profile, got %#v", result.EntityProfile)
	}
}

func TestService_QueryEntityAnchorsAreTenantScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(databases.NewMemoryGraph(), nil, embedder.NewDeterministic(32, true, 0))

	t1, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-t1",
		Tenant:    "t1",
		SessionID: "s1",
		Text:      "Melanie practiced guitar.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("t1 Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("t1 DrainConsolidation() error = %v", err)
	}
	t2, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-t2",
		Tenant:    "t2",
		SessionID: "s1",
		Text:      "Melanie practiced piano.",
		CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("t2 Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("t2 DrainConsolidation() error = %v", err)
	}

	result, err := svc.Query(ctx, "What did Melanie practice?", QueryOptions{Tenant: "t1", MaxNodes: 5})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.RawEvents) != 1 || result.RawEvents[0].ID != t1.EventID {
		t.Fatalf("expected only tenant t1 event %s, got %#v; t2=%s", t1.EventID, result.RawEvents, t2.EventID)
	}
	if _, ok := result.EntityProfile["entity:t2:melanie"]; ok {
		t.Fatalf("unexpected tenant t2 entity profile: %#v", result.EntityProfile)
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

func TestQueryIntentClassificationSemanticMode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(databases.NewMemoryGraph(), nil, embedder.NewDeterministic(32, true, 0))

	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "melanie-entity-only",
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

	entityResult, err := svc.Query(ctx, "What do you know about Melanie?", QueryOptions{Tenant: "t1", MaxNodes: 5})
	if err != nil {
		t.Fatalf("entity Query() error = %v", err)
	}
	if len(entityResult.RawEvents) != 1 || entityResult.RawEvents[0].ID != resp.EventID {
		t.Fatalf("expected rules mode to use entity anchors, got %#v", entityResult.RawEvents)
	}
	semanticResult, err := svc.Query(ctx, "What do you know about Melanie?", QueryOptions{Tenant: "t1", MaxNodes: 5, IntentClassification: "semantic"})
	if err != nil {
		t.Fatalf("semantic Query() error = %v", err)
	}
	if len(semanticResult.RawEvents) != 0 {
		t.Fatalf("expected semantic mode without vector anchors to return no entity-only events, got %#v", semanticResult.RawEvents)
	}
	hintedResult, err := svc.Query(ctx, "What do you know about Melanie?", QueryOptions{Tenant: "t1", MaxNodes: 5, IntentClassification: "semantic", IntentHint: IntentEntity})
	if err != nil {
		t.Fatalf("hinted Query() error = %v", err)
	}
	if len(hintedResult.RawEvents) != 1 || hintedResult.RawEvents[0].ID != resp.EventID {
		t.Fatalf("expected explicit hint to override semantic mode, got %#v", hintedResult.RawEvents)
	}
}

func TestQueryIntentClassificationLLMMode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	provider := &fakeConsolidationLLM{response: `{"intents":["causal"]}`}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		LLM:   provider,
		Model: "intent-model",
	})

	result, err := svc.Query(ctx, "What led to the schedule change?", QueryOptions{IntentClassification: "llm"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.Intent != IntentCausal {
		t.Fatalf("expected LLM causal intent, got %s", result.Intent)
	}
	if provider.model != "intent-model" || provider.calls != 1 {
		t.Fatalf("expected one LLM intent call with model, got model=%q calls=%d", provider.model, provider.calls)
	}
	if !strings.Contains(provider.systemPrompt, "Classify the memory retrieval query") {
		t.Fatalf("expected default intent prompt, got %q", provider.systemPrompt)
	}
}

func TestQueryIntentClassificationUsesConfiguredPrompt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	provider := &fakeConsolidationLLM{response: `{"intents":["semantic"]}`}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		LLM: provider,
		Prompts: PromptConfig{
			IntentClassification: "custom intent prompt",
		},
	})

	if _, err := svc.Query(ctx, "What were we discussing?", QueryOptions{IntentClassification: "llm"}); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if provider.systemPrompt != "custom intent prompt" {
		t.Fatalf("expected custom intent prompt, got %q", provider.systemPrompt)
	}
}

func TestQueryIntentClassificationHybridUsesLLMForSemanticOnlyRules(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	provider := &fakeConsolidationLLM{response: `{"intents":["causal","temporal"]}`}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), nil, embedder.NewDeterministic(32, true, 0), ServiceConfig{LLM: provider})

	result, err := svc.Query(ctx, "What led to the change?", QueryOptions{IntentClassification: "hybrid"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.Intent != IntentTemporal|IntentCausal {
		t.Fatalf("expected hybrid LLM temporal+causal intent, got %s", result.Intent)
	}
	if provider.calls != 1 {
		t.Fatalf("expected one hybrid LLM call, got %d", provider.calls)
	}
}

func TestQueryIntentClassificationHybridKeepsSpecificRules(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	provider := &fakeConsolidationLLM{response: `{"intents":["causal"]}`}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), nil, embedder.NewDeterministic(32, true, 0), ServiceConfig{LLM: provider})

	result, err := svc.Query(ctx, "When did Melanie practice?", QueryOptions{IntentClassification: "hybrid"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.Intent != IntentTemporal|IntentEntity {
		t.Fatalf("expected rules temporal+entity intent, got %s", result.Intent)
	}
	if provider.calls != 0 {
		t.Fatalf("expected hybrid to skip LLM for specific rules, got %d calls", provider.calls)
	}
}

func TestQueryIntentClassificationLLMFallsBackToRules(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	provider := &fakeConsolidationLLM{response: `not json`}
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), nil, embedder.NewDeterministic(32, true, 0), ServiceConfig{LLM: provider})

	result, err := svc.Query(ctx, "When did Melanie practice?", QueryOptions{IntentClassification: "llm"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.Intent != IntentTemporal|IntentEntity {
		t.Fatalf("expected fallback rules temporal+entity intent, got %s", result.Intent)
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

func TestService_PruneExpiresEventsAndDeletesVector(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vector := databases.NewMemoryVector()
	svc := NewService(databases.NewMemoryGraph(), vector, embedder.NewDeterministic(32, true, 0))

	oldResp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "old",
		Tenant:    "t1",
		Text:      "Melanie practiced guitar.",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("old Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("old DrainConsolidation() error = %v", err)
	}
	newResp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "new",
		Tenant:    "t1",
		Text:      "Melanie practiced piano.",
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("new Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("new DrainConsolidation() error = %v", err)
	}

	stats, err := svc.Prune(ctx, LifecyclePolicy{EventTTL: 24 * time.Hour})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if stats.EventsDeleted != 1 {
		t.Fatalf("expected one deleted event, got %#v", stats)
	}
	if _, ok := svc.Event(ctx, oldResp.EventID); ok {
		t.Fatalf("expected old event to be deleted")
	}
	if _, ok := svc.Event(ctx, newResp.EventID); !ok {
		t.Fatalf("expected new event to remain")
	}
	results, err := vector.SimilaritySearch(ctx, []float32{1, 0, 0}, 10, map[string]string{"tenant": "t1"})
	if err != nil {
		t.Fatalf("SimilaritySearch() error = %v", err)
	}
	for _, result := range results {
		if result.ID == oldResp.EventID {
			t.Fatalf("expected old vector to be deleted, got %#v", results)
		}
	}
}

func TestService_PruneEdgesByWeightAndFanout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), nil, embedder.NewDeterministic(32, true, 0), ServiceConfig{
		Graphs: GraphConfig{Semantic: true},
	})
	for _, id := range []string{"source", "keep", "drop_weight", "drop_fanout"} {
		if err := svc.store.StoreEvent(ctx, EventNode{ID: "event:t1:" + id, Tenant: "t1", Text: id, CreatedAt: time.Now()}); err != nil {
			t.Fatalf("StoreEvent(%s) error = %v", id, err)
		}
	}
	edges := []Edge{
		{Source: "event:t1:source", GraphType: GraphSemantic, Rel: "SIMILAR_TO", Target: "event:t1:keep", Weight: 0.95},
		{Source: "event:t1:source", GraphType: GraphSemantic, Rel: "SIMILAR_TO", Target: "event:t1:drop_weight", Weight: 0.25},
		{Source: "event:t1:source", GraphType: GraphSemantic, Rel: "SIMILAR_TO", Target: "event:t1:drop_fanout", Weight: 0.8},
	}
	for _, edge := range edges {
		if err := svc.store.UpsertEdge(ctx, edge); err != nil {
			t.Fatalf("UpsertEdge() error = %v", err)
		}
	}
	stats, err := svc.Prune(ctx, LifecyclePolicy{MinSemanticWeight: 0.5, MaxEdgesPerSourceRel: 1})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if stats.EdgesDeleted != 2 {
		t.Fatalf("expected two deleted edges, got %#v", stats)
	}
	neighbors, err := svc.store.Neighbors(ctx, "event:t1:source", GraphSemantic, "SIMILAR_TO")
	if err != nil {
		t.Fatalf("Neighbors() error = %v", err)
	}
	if len(neighbors) != 1 || neighbors[0] != "event:t1:keep" {
		t.Fatalf("expected only strongest edge to remain, got %#v", neighbors)
	}
}

func TestService_ReviewWorkflowFlagsApprovesAndRetractsEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		Graphs: GraphConfig{Causal: true},
		Lifecycle: LifecyclePolicy{
			RequireReviewApproval: true,
		},
	})
	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:     "causal-low-confidence",
		Tenant: "t1",
		Text:   "Melanie stayed inside because rain started.",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	stats, err := svc.Prune(ctx, LifecyclePolicy{LowConfidenceThreshold: 0.6})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if stats.EdgesFlaggedReview != 1 {
		t.Fatalf("expected one flagged edge, got %#v", stats)
	}
	review, err := svc.ReviewEdges(ctx)
	if err != nil {
		t.Fatalf("ReviewEdges() error = %v", err)
	}
	if len(review) != 1 || review[0].ReviewState != "needs_review" {
		t.Fatalf("expected one review edge, got %#v", review)
	}
	result, err := svc.Query(ctx, "Why did Melanie stay inside?", QueryOptions{Tenant: "t1", IntentHint: IntentCausal, MaxNodes: 5})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.CausalChain) != 0 {
		t.Fatalf("expected review-gated causal edge to be filtered, got %#v", result.CausalChain)
	}
	selector := selectorForEdge(review[0].Edge)
	if err := svc.ApproveEdge(ctx, selector, "tester"); err != nil {
		t.Fatalf("ApproveEdge() error = %v", err)
	}
	result, err = svc.Query(ctx, "Why did Melanie stay inside?", QueryOptions{Tenant: "t1", IntentHint: IntentCausal, MaxNodes: 5})
	if err != nil {
		t.Fatalf("approved Query() error = %v", err)
	}
	if len(result.CausalChain) != 1 {
		t.Fatalf("expected approved causal edge to be used, got %#v", result.CausalChain)
	}
	if err := svc.RetractEdge(ctx, selector, "incorrect"); err != nil {
		t.Fatalf("RetractEdge() error = %v", err)
	}
	neighbors, err := svc.store.Neighbors(ctx, resp.EventID, GraphCausal, "CAUSES")
	if err != nil {
		t.Fatalf("Neighbors() error = %v", err)
	}
	if len(neighbors) != 0 {
		t.Fatalf("expected retracted edge deleted, got %#v", neighbors)
	}
}

func TestService_DeleteEventRemovesEventEdgesAndVectorEntry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	vector := databases.NewMemoryVector()
	emb := embedder.NewDeterministic(32, true, 0)
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), vector, emb, ServiceConfig{
		Graphs: GraphConfig{Causal: true},
	})
	text := "Melanie stayed inside because rain started."
	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:     "delete-node",
		Tenant: "t1",
		Text:   text,
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}

	deleted, err := svc.DeleteEvent(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("DeleteEvent() error = %v", err)
	}
	if !deleted {
		t.Fatal("expected event to be deleted")
	}

	events, err := svc.Events(ctx)
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	for _, event := range events {
		if event.ID == resp.EventID {
			t.Fatalf("expected event %q to be removed, got %#v", resp.EventID, events)
		}
	}

	neighbors, err := svc.store.Neighbors(ctx, resp.EventID, GraphCausal, "CAUSES")
	if err != nil {
		t.Fatalf("Neighbors() error = %v", err)
	}
	if len(neighbors) != 0 {
		t.Fatalf("expected incident graph edges to be removed, got %#v", neighbors)
	}

	queryVecs, err := emb.EmbedBatch(ctx, []string{text})
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	results, err := vector.SimilaritySearch(ctx, queryVecs[0], 10, map[string]string{"tenant": "t1"})
	if err != nil {
		t.Fatalf("SimilaritySearch() error = %v", err)
	}
	for _, result := range results {
		if result.ID == resp.EventID {
			t.Fatalf("expected vector entry %q to be removed, got %#v", resp.EventID, results)
		}
	}

	deleted, err = svc.DeleteEvent(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("second DeleteEvent() error = %v", err)
	}
	if deleted {
		t.Fatal("expected second delete to report not found")
	}
}

func TestService_EmitsMagmaTraceSpans(t *testing.T) {
	ctx := context.Background()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	svc := NewService(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0))
	resp, err := svc.Ingest(ctx, IngestRequest{ID: "trace", Tenant: "t1", Text: "Yesterday Melanie practiced guitar."})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if _, err := svc.DrainConsolidation(ctx, 1); err != nil {
		t.Fatalf("DrainConsolidation() error = %v", err)
	}
	if _, err := svc.Query(ctx, "When did Melanie practice?", QueryOptions{Tenant: "t1", MaxNodes: 5}); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if _, err := svc.Prune(ctx, LifecyclePolicy{LowConfidenceThreshold: 0.6}); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if resp.EventID == "" {
		t.Fatalf("expected event ID")
	}
	names := map[string]bool{}
	for _, span := range recorder.Ended() {
		names[span.Name()] = true
	}
	for _, want := range []string{"magma.ingest", "magma.consolidate", "magma.query", "magma.query.classify_intent", "magma.query.anchors", "magma.query.traverse", "magma.prune"} {
		if !names[want] {
			t.Fatalf("expected span %q in %#v", want, names)
		}
	}
}

func TestService_StartLifecycleWorkerPrunesWithConfiguredPolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewServiceWithConfig(databases.NewMemoryGraph(), databases.NewMemoryVector(), embedder.NewDeterministic(32, true, 0), ServiceConfig{
		Lifecycle: LifecyclePolicy{EventTTL: time.Nanosecond},
	})
	t.Cleanup(svc.Close)
	resp, err := svc.Ingest(ctx, IngestRequest{
		ID:        "lifecycle-worker",
		Tenant:    "t1",
		Text:      "Melanie practiced guitar.",
		CreatedAt: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	svc.StartLifecycleWorker(ctx, time.Millisecond)
	deadline := time.After(time.Second)
	for {
		if _, ok := svc.Event(ctx, resp.EventID); !ok {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for lifecycle worker to prune event")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
