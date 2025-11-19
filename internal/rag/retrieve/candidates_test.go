package retrieve

import (
	"context"
	"testing"

	"manifold/internal/persistence/databases"
)

func TestParallelCandidates_Memory(t *testing.T) {
	ctx := context.Background()
	search := databases.NewMemorySearch()
	vector := databases.NewMemoryVector()

	// Seed a tiny corpus: one doc with two chunks
	_ = search.Index(ctx, "doc:acme:alpha", "alpha beta gamma", map[string]string{"tenant": "acme", "lang": "english", "type": "doc"})
	_ = search.Index(ctx, "chunk:doc:acme:alpha:0", "alpha section details", map[string]string{"tenant": "acme", "lang": "english", "type": "chunk", "doc_id": "doc:acme:alpha"})
	_ = search.Index(ctx, "chunk:doc:acme:alpha:1", "beta appendix info", map[string]string{"tenant": "acme", "lang": "english", "type": "chunk", "doc_id": "doc:acme:alpha"})

	// Seed vectors for the two chunks; use small made-up vectors
	_ = vector.Upsert(ctx, "chunk:doc:acme:alpha:0", []float32{1, 0}, map[string]string{"tenant": "acme", "doc_id": "doc:acme:alpha", "type": "chunk"})
	_ = vector.Upsert(ctx, "chunk:doc:acme:alpha:1", []float32{0, 1}, map[string]string{"tenant": "acme", "doc_id": "doc:acme:alpha", "type": "chunk"})

	plan := QueryPlan{Query: "alpha", Lang: "english", FtK: 2, VecK: 2, Filters: map[string]string{"tenant": "acme"}}
	// Query vector close to first chunk
	qvec := []float32{1, 0}
	fts, vrs, diag, err := ParallelCandidates(ctx, search, vector, plan, qvec)
	if err != nil {
		t.Fatalf("ParallelCandidates error: %v", err)
	}
	if len(fts) == 0 {
		t.Fatalf("expected non-empty FTS candidates")
	}
	if len(vrs) == 0 {
		t.Fatalf("expected non-empty vector candidates")
	}
	if diag.FtLatency == 0 && diag.VecLatency == 0 {
		t.Fatalf("expected some latency recorded")
	}
}
