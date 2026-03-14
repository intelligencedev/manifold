package databases

import (
	"context"
	"reflect"

	"manifold/internal/agent/memory"
	"manifold/internal/persistence"
	"manifold/internal/transit"
)

// SearchResult represents a single hit from the full-text search backend.
type SearchResult struct {
	ID      string
	Score   float64
	Snippet string
	// Text may contain the full document text when available.
	Text     string `json:"text,omitempty"`
	Metadata map[string]string
}

// FullTextSearch defines the minimum interface for a pluggable FTS backend.
type FullTextSearch interface {
	Index(ctx context.Context, id string, text string, metadata map[string]string) error
	Remove(ctx context.Context, id string) error
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
	// GetByID returns a document by its exact ID if present.
	GetByID(ctx context.Context, id string) (SearchResult, bool, error)
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
	Search          FullTextSearch
	Vector          VectorStore
	Graph           GraphDB
	Chat            persistence.ChatStore
	EvolvingMemory  memory.EvolvingMemoryStore
	Playground      *PlaygroundStore
	Warpp           persistence.WarppWorkflowStore
	MCP             persistence.MCPStore
	Projects        persistence.ProjectsStore
	UserPreferences persistence.UserPreferencesStore
	Pulse           persistence.PulseStore
	Transit         transit.Store
}

// Close attempts to close any underlying pools. It's a no-op for memory backends.
func (m Manager) Close() {
	closeIfPossible(m.Search)
	closeIfPossible(m.Vector)
	closeIfPossible(m.Graph)
	closeIfPossible(m.Chat)
	closeIfPossible(m.EvolvingMemory)
	closeIfPossible(m.Playground)
	closeIfPossible(m.MCP)
	closeIfPossible(m.Projects)
	closeIfPossible(m.UserPreferences)
	closeIfPossible(m.Pulse)
	closeIfPossible(m.Transit)
}

func closeIfPossible(value any) {
	if value == nil {
		return
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if rv.IsNil() {
			return
		}
	}

	if closer, ok := value.(interface{ Close() }); ok {
		closer.Close()
		return
	}
	if closer, ok := value.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}
