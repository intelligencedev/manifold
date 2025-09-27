package databases

import (
	"context"

	"intelligence.dev/internal/persistence"
)

// SearchResult represents a single hit from the full-text search backend.
type SearchResult struct {
	ID       string
	Score    float64
	Snippet  string
	Metadata map[string]string
}

// FullTextSearch defines the minimum interface for a pluggable FTS backend.
type FullTextSearch interface {
	Index(ctx context.Context, id string, text string, metadata map[string]string) error
	Remove(ctx context.Context, id string) error
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
}

// VectorResult represents a single nearest neighbor lookup result.
type VectorResult struct {
	ID       string
	Score    float64 // Higher is closer by default
	Metadata map[string]string
}

// VectorStore defines the minimum interface for a pluggable vector store.
type VectorStore interface {
	Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error
	Delete(ctx context.Context, id string) error
	SimilaritySearch(ctx context.Context, vector []float32, k int, filter map[string]string) ([]VectorResult, error)
}

// Node is a minimal in-memory representation of a graph node.
type Node struct {
	ID     string
	Labels []string
	Props  map[string]any
}

// GraphDB defines a portable interface for minimal graph operations.
type GraphDB interface {
	UpsertNode(ctx context.Context, id string, labels []string, props map[string]any) error
	UpsertEdge(ctx context.Context, srcID, rel, dstID string, props map[string]any) error
	Neighbors(ctx context.Context, id string, rel string) ([]string, error)
	GetNode(ctx context.Context, id string) (Node, bool)
}

// Manager holds concrete database backends resolved from configuration.
type Manager struct {
	Search     FullTextSearch
	Vector     VectorStore
	Graph      GraphDB
	Chat       persistence.ChatStore
	Playground *PlaygroundStore
}

// Close attempts to close any underlying pools. It's a no-op for memory backends.
func (m Manager) Close() {
	if c, ok := any(m.Search).(interface{ Close() }); ok {
		c.Close()
	}
	if c, ok := any(m.Vector).(interface{ Close() }); ok {
		c.Close()
	}
	if c, ok := any(m.Graph).(interface{ Close() }); ok {
		c.Close()
	}
	if c, ok := any(m.Chat).(interface{ Close() }); ok {
		c.Close()
	}
	if c, ok := any(m.Playground).(interface{ Close() }); ok {
		c.Close()
	}
}
