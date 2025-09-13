package databases

import (
	"context"
	"testing"

	"intelligence.dev/internal/config"
)

func TestMemorySearch_IndexAndSearch(t *testing.T) {
	t.Parallel()
	s := NewMemorySearch()
	ctx := context.Background()
	_ = s.Index(ctx, "1", "The quick brown fox jumps over the lazy dog", map[string]string{"type": "doc"})
	_ = s.Index(ctx, "2", "Foxes are swift and quick", nil)
	_ = s.Index(ctx, "3", "Completely unrelated text", nil)
	hits, err := s.Search(ctx, "quick fox", 5)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected at least one hit")
	}
	if hits[0].ID != "1" && hits[0].ID != "2" {
		t.Fatalf("unexpected top hit: %#v", hits[0])
	}
}

func TestMemoryVector_UpsertAndQuery(t *testing.T) {
	t.Parallel()
	v := NewMemoryVector()
	ctx := context.Background()
	// 2D vectors for simplicity
	_ = v.Upsert(ctx, "a", []float32{1, 0}, map[string]string{"label": "A"})
	_ = v.Upsert(ctx, "b", []float32{0, 1}, nil)
	_ = v.Upsert(ctx, "c", []float32{1, 1}, nil)
	q := []float32{0.9, 0.1}
	res, err := v.SimilaritySearch(ctx, q, 2, nil)
	if err != nil {
		t.Fatalf("sim search error: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].ID != "a" {
		t.Fatalf("expected 'a' to be nearest, got %q", res[0].ID)
	}
}

func TestMemoryGraph_Basics(t *testing.T) {
	t.Parallel()
	g := NewMemoryGraph()
	ctx := context.Background()
	_ = g.UpsertNode(ctx, "n1", []string{"User"}, map[string]any{"name": "Alice"})
	_ = g.UpsertNode(ctx, "n2", []string{"User"}, map[string]any{"name": "Bob"})
	_ = g.UpsertEdge(ctx, "n1", "KNOWS", "n2", map[string]any{"since": 2020})
	neigh, err := g.Neighbors(ctx, "n1", "KNOWS")
	if err != nil {
		t.Fatalf("neighbors error: %v", err)
	}
	if len(neigh) != 1 || neigh[0] != "n2" {
		t.Fatalf("unexpected neighbors: %#v", neigh)
	}
	if n, ok := g.GetNode(ctx, "n1"); !ok || n.Props["name"] != "Alice" {
		t.Fatalf("unexpected node: %#v exists=%v", n, ok)
	}
}

func TestFactory_DefaultsAndNone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Defaults should create memory backends
	mgr, err := NewManager(ctx, config.DBConfig{})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if mgr.Search == nil || mgr.Vector == nil || mgr.Graph == nil {
		t.Fatalf("expected non-nil backends by default")
	}
	// None should create no-op backends
	mgr, err = NewManager(ctx, config.DBConfig{Search: config.SearchConfig{Backend: "none"}, Vector: config.VectorConfig{Backend: "none"}, Graph: config.GraphConfig{Backend: "none"}})
	if err != nil {
		t.Fatalf("NewManager error (none): %v", err)
	}
	// Calls should not error
	_ = mgr.Search.Index(ctx, "x", "y", nil)
	_, _ = mgr.Search.Search(ctx, "z", 1)
	_ = mgr.Vector.Upsert(ctx, "x", []float32{1}, nil)
	_, _ = mgr.Vector.SimilaritySearch(ctx, []float32{1}, 1, nil)
	_ = mgr.Graph.UpsertNode(ctx, "n", nil, nil)
}
