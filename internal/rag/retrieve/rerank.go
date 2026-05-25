package retrieve

import "context"

// Reranker optionally reorders retrieved items using an external scoring model.
// Implementations should not drop items and should preserve Metadata fields.
type Reranker interface {
	Rerank(ctx context.Context, query string, items []RetrievedItem) ([]RetrievedItem, error)
}
