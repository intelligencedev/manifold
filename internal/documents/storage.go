package documents

import "context"

// VectorStore stores and searches vectorised chunks.
type VectorStore interface {
	UpsertChunks(ctx context.Context, chunks []PersistReq) error
	Search(ctx context.Context, query []float32, k int) ([]Chunk, error)
}

// PersistReq represents a chunk to persist with its embedding.
type PersistReq struct {
	Chunk     Chunk
	Embedding []float32
}

// mockstore provides an in-memory store for tests.
type mockstore struct {
	data []PersistReq
}

func newMockStore() *mockstore { return &mockstore{} }

func (m *mockstore) UpsertChunks(_ context.Context, c []PersistReq) error {
	m.data = append(m.data, c...)
	return nil
}

func (m *mockstore) Search(_ context.Context, _ []float32, k int) ([]Chunk, error) {
	var res []Chunk
	for i := 0; i < k && i < len(m.data); i++ {
		res = append(res, m.data[i].Chunk)
	}
	return res, nil
}

// PgVectorStore is a placeholder for a Postgres vector store.
type PgVectorStore struct{}

// TODO: implement database backed methods.
func (PgVectorStore) UpsertChunks(ctx context.Context, chunks []PersistReq) error { return nil }
func (PgVectorStore) Search(ctx context.Context, query []float32, k int) ([]Chunk, error) {
	return nil, nil
}
