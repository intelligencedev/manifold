package databases

import (
	"context"
	"fmt"

	"singularityio/internal/config"
)

// NewManager constructs database backends based on configuration.
// Supported backends: memory (default) and none.
func NewManager(_ context.Context, cfg config.DBConfig) (Manager, error) {
	var m Manager
	// Full-text search
	switch cfg.Search.Backend {
	case "", "memory":
		m.Search = NewMemorySearch()
	case "none", "disabled":
		m.Search = noopSearch{}
	default:
		return Manager{}, fmt.Errorf("unsupported search backend: %s", cfg.Search.Backend)
	}
	// Vector store
	switch cfg.Vector.Backend {
	case "", "memory":
		m.Vector = NewMemoryVector()
	case "none", "disabled":
		m.Vector = noopVector{}
	default:
		return Manager{}, fmt.Errorf("unsupported vector backend: %s", cfg.Vector.Backend)
	}
	// Graph DB
	switch cfg.Graph.Backend {
	case "", "memory":
		m.Graph = NewMemoryGraph()
	case "none", "disabled":
		m.Graph = noopGraph{}
	default:
		return Manager{}, fmt.Errorf("unsupported graph backend: %s", cfg.Graph.Backend)
	}
	return m, nil
}

// no-op backends for "none" configuration
type noopSearch struct{}

func (noopSearch) Index(context.Context, string, string, map[string]string) error { return nil }
func (noopSearch) Remove(context.Context, string) error                           { return nil }
func (noopSearch) Search(context.Context, string, int) ([]SearchResult, error)    { return nil, nil }

type noopVector struct{}

func (noopVector) Upsert(context.Context, string, []float32, map[string]string) error { return nil }
func (noopVector) Delete(context.Context, string) error                               { return nil }
func (noopVector) SimilaritySearch(context.Context, []float32, int, map[string]string) ([]VectorResult, error) {
	return nil, nil
}

type noopGraph struct{}

func (noopGraph) UpsertNode(context.Context, string, []string, map[string]any) error { return nil }
func (noopGraph) UpsertEdge(context.Context, string, string, string, map[string]any) error {
	return nil
}
func (noopGraph) Neighbors(context.Context, string, string) ([]string, error) { return nil, nil }
func (noopGraph) GetNode(context.Context, string) (Node, bool)                { return Node{}, false }
