package retrieve

import "context"

// Reranker optionally reorders retrieved items (e.g., via a cross-encoder).
// Implementations should not drop items and should preserve Metadata fields.
type Reranker interface {
    Rerank(ctx context.Context, query string, items []RetrievedItem) ([]RetrievedItem, error)
}

// NoopReranker is the default implementation that leaves ordering unchanged.
type NoopReranker struct{}

func (NoopReranker) Rerank(_ context.Context, _ string, items []RetrievedItem) ([]RetrievedItem, error) {
    return items, nil
}

