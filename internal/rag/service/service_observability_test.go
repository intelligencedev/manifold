package service

import (
	"context"
	"testing"

	"manifold/internal/persistence/databases"
	"manifold/internal/rag/obs"
	"manifold/internal/rag/retrieve"
)

func TestRetrieve_EmitsDiagnosticsAndMetrics(t *testing.T) {
	// Setup memory backends
	mgr := databases.Manager{Search: databases.NewMemorySearch(), Vector: databases.NewMemoryVector(), Graph: databases.NewMemoryGraph()}
	metrics := obs.NewMockMetrics()
	s := New(mgr, WithMetrics(metrics))

	// Seed minimal content
	ctx := context.Background()
	// Index two chunks and corresponding vectors
	_ = mgr.Search.Index(ctx, "chunk:doc:1:0", "hello world", map[string]string{"type": "chunk", "doc_id": "doc:1", "tenant": "t1", "lang": "english"})
	_ = mgr.Search.Index(ctx, "chunk:doc:1:1", "world of golang", map[string]string{"type": "chunk", "doc_id": "doc:1", "tenant": "t1", "lang": "english"})
	// vectors
	_ = mgr.Vector.Upsert(ctx, "chunk:doc:1:0", []float32{0.1, 0.2}, map[string]string{"tenant": "t1", "lang": "english", "doc_id": "doc:1"})
	_ = mgr.Vector.Upsert(ctx, "chunk:doc:1:1", []float32{0.2, 0.1}, map[string]string{"tenant": "t1", "lang": "english", "doc_id": "doc:1"})

	// Run retrieve
	resp, err := s.Retrieve(ctx, "hello world", retrieve.RetrieveOptions{K: 2, UseRRF: true, Tenant: "t1", IncludeSnippet: true})
	if err != nil {
		t.Fatalf("retrieve error: %v", err)
	}
	if len(resp.Items) == 0 {
		t.Fatalf("expected some items")
	}
	// Diagnostics present
	d, ok := resp.Debug["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("missing diagnostics in response")
	}
	for _, key := range []string{"ft_ms", "vec_ms", "package_ms", "fusion_ms", "total_ms"} {
		if _, ok := d[key]; !ok {
			t.Fatalf("missing %s in diagnostics", key)
		}
	}
	// Metrics counters/histograms recorded
	if metrics.Counters["retrieval_results_total"] == 0 {
		t.Fatalf("expected retrieval_results_total > 0")
	}
	if _, ok := metrics.Hists["retrieval_stage_ms"]; !ok {
		t.Fatalf("expected retrieval_stage_ms observations")
	}
}
