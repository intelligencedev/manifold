package service

import (
	"context"
	"testing"

	"manifold/internal/config"
	"manifold/internal/embedding"
	"manifold/internal/persistence/databases"
	"manifold/internal/rag/obs"
	"manifold/internal/rag/retrieve"
)

type captureEmbedder struct {
	texts []string
}

func (c *captureEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	c.texts = append(c.texts, texts...)
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

func (c *captureEmbedder) Name() string               { return "capture" }
func (c *captureEmbedder) Dimension() int             { return 2 }
func (c *captureEmbedder) Ping(context.Context) error { return nil }

type captureReranker struct {
	queries []string
	items   [][]retrieve.RetrievedItem
}

func (c *captureReranker) Rerank(_ context.Context, query string, items []retrieve.RetrievedItem) ([]retrieve.RetrievedItem, error) {
	c.queries = append(c.queries, query)
	c.items = append(c.items, append([]retrieve.RetrievedItem(nil), items...))
	out := append([]retrieve.RetrievedItem(nil), items...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func TestRetrieve_EmitsDiagnosticsAndMetrics(t *testing.T) {
	// Setup memory backends
	mgr := databases.Manager{Search: databases.NewMemorySearch(), Vector: databases.NewMemoryVector(), Graph: databases.NewMemoryGraph()}
	metrics := obs.NewMockMetrics()
	s := New(mgr, func(s *Service) { s.metrics = metrics })

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

func TestRetrieve_FormatsQueryEmbeddingInstruction(t *testing.T) {
	mgr := databases.Manager{Search: databases.NewMemorySearch(), Vector: databases.NewMemoryVector()}
	emb := &captureEmbedder{}
	cfg := config.EmbeddingConfig{
		Model: "Qwen3-Embedding-0.6B-f16.gguf",
		Instructions: config.EmbeddingInstructionConfig{
			Mode:   "auto",
			Format: "qwen",
		},
	}
	s := New(mgr, WithEmbedder(emb), WithEmbeddingConfig(cfg))

	resp, err := s.Retrieve(context.Background(), "where is auth configured?", retrieve.RetrieveOptions{K: 1, VecK: 1})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(emb.texts) != 1 {
		t.Fatalf("expected one embedding call, got %d", len(emb.texts))
	}
	want := "Instruct: " + embedding.DefaultRAGQueryInstruction + "\nQuery: where is auth configured?"
	if emb.texts[0] != want {
		t.Fatalf("unexpected embedded text:\n got %q\nwant %q", emb.texts[0], want)
	}
	info, ok := resp.Debug["embedding_instruction"].(map[string]any)
	if !ok || info["applied"] != true || info["useCase"] != embedding.UseCaseRAGQuery {
		t.Fatalf("unexpected embedding instruction debug: %#v", resp.Debug["embedding_instruction"])
	}
}

func TestRetrieve_ExplicitInstructionOverridesDefault(t *testing.T) {
	mgr := databases.Manager{Search: databases.NewMemorySearch(), Vector: databases.NewMemoryVector()}
	emb := &captureEmbedder{}
	cfg := config.EmbeddingConfig{
		Model: "Qwen3-Embedding-0.6B-f16.gguf",
		Instructions: config.EmbeddingInstructionConfig{
			Mode:   "auto",
			Format: "qwen",
		},
	}
	s := New(mgr, WithEmbedder(emb), WithEmbeddingConfig(cfg))

	_, err := s.Retrieve(context.Background(), "where is auth configured?", retrieve.RetrieveOptions{
		K:           1,
		VecK:        1,
		Instruction: "Retrieve implementation details.",
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	want := "Instruct: Retrieve implementation details.\nQuery: where is auth configured?"
	if len(emb.texts) != 1 || emb.texts[0] != want {
		t.Fatalf("unexpected embedded text: %#v", emb.texts)
	}
}

func TestRetrieve_UsesConfiguredRerankerWhenRequested(t *testing.T) {
	mgr := databases.Manager{Search: databases.NewMemorySearch()}
	ctx := context.Background()
	_ = mgr.Search.Index(ctx, "chunk:doc:1:0", "alpha first", map[string]string{"type": "chunk", "tenant": "t1", "lang": "english", "doc_id": "doc:1"})
	_ = mgr.Search.Index(ctx, "chunk:doc:2:0", "alpha second", map[string]string{"type": "chunk", "tenant": "t1", "lang": "english", "doc_id": "doc:2"})
	rerank := &captureReranker{}
	s := New(mgr, WithReranker(rerank))

	resp, err := s.Retrieve(ctx, "alpha", retrieve.RetrieveOptions{K: 2, FtK: 2, VecK: 0, UseRRF: true, Tenant: "t1", Rerank: true})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(rerank.queries) != 1 || rerank.queries[0] != "alpha" {
		t.Fatalf("expected reranker call, got %#v", rerank.queries)
	}
	if len(rerank.items) != 1 || len(rerank.items[0]) != 2 {
		t.Fatalf("unexpected reranker items: %#v", rerank.items)
	}
	if rerank.items[0][0].Text == "" {
		t.Fatalf("expected reranker item text to be hydrated")
	}
	if got, ok := resp.Debug["diagnostics"].(map[string]any)["rerank_ms"]; !ok || got == nil {
		t.Fatalf("missing rerank diagnostics: %#v", resp.Debug)
	}
}

func TestRetrieve_DefaultsToHybridRRFWhenRerankingInactive(t *testing.T) {
	mgr := databases.Manager{Search: databases.NewMemorySearch(), Vector: databases.NewMemoryVector()}
	emb := &captureEmbedder{}
	s := New(mgr, WithEmbedder(emb))
	ctx := context.Background()
	_ = mgr.Search.Index(ctx, "chunk:doc:lexical:0", "rareterm exact lexical match", map[string]string{"type": "chunk", "tenant": "t1", "lang": "english", "doc_id": "doc:lexical"})
	_ = mgr.Vector.Upsert(ctx, "chunk:doc:semantic:0", []float32{1, 0}, map[string]string{"tenant": "t1", "lang": "english", "doc_id": "doc:semantic"})

	resp, err := s.Retrieve(ctx, "rareterm", retrieve.RetrieveOptions{K: 4, Tenant: "t1"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	diag, ok := resp.Debug["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("missing diagnostics: %#v", resp.Debug)
	}
	if _, ok := diag["fusion_ms"]; !ok {
		t.Fatalf("expected RRF fusion diagnostics, got %#v", diag)
	}
	plan, ok := resp.Debug["plan"].(map[string]any)
	if !ok || plan["ftK"] != 2 || plan["vecK"] != 2 {
		t.Fatalf("expected balanced hybrid plan, got %#v", resp.Debug["plan"])
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected lexical and vector candidates, got %#v", resp.Items)
	}
	seen := map[string]bool{}
	for _, item := range resp.Items {
		seen[item.ID] = true
	}
	if !seen["chunk:doc:lexical:0"] || !seen["chunk:doc:semantic:0"] {
		t.Fatalf("expected hybrid lexical/vector results, got %#v", resp.Items)
	}
}
