package transit

import "context"

type Store interface {
	Init(ctx context.Context) error
	Create(ctx context.Context, tenantID, actorID int64, items []CreateMemoryItem) ([]Record, error)
	Get(ctx context.Context, tenantID int64, keys []string) ([]Record, error)
	Update(ctx context.Context, tenantID, actorID int64, req UpdateMemoryRequest) (Record, error)
	Delete(ctx context.Context, tenantID int64, keys []string) error
	ListKeys(ctx context.Context, tenantID int64, req ListRequest) ([]Metadata, error)
	ListRecent(ctx context.Context, tenantID int64, req ListRequest) ([]Metadata, error)
	SearchText(ctx context.Context, tenantID int64, req SearchRequest) ([]SearchCandidate, error)
}

type SearchIndexer interface {
	Index(ctx context.Context, id string, text string, metadata map[string]string) error
	Remove(ctx context.Context, id string) error
	Search(ctx context.Context, query string, limit int) ([]SearchIndexResult, error)
}

type SearchIndexResult struct {
	ID       string
	Score    float64
	Snippet  string
	Text     string
	Metadata map[string]string
}

type VectorIndexer interface {
	Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error
	Delete(ctx context.Context, id string) error
	SimilaritySearch(ctx context.Context, vector []float32, k int, filter map[string]string) ([]VectorIndexResult, error)
}

type VectorIndexResult struct {
	ID       string
	Score    float64
	Metadata map[string]string
}
