package databases

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/agent/memory"
)

// NewPostgresEvolvingMemoryStore returns a Postgres-backed EvolvingMemoryStore.
//
// It mirrors the style of the ChatStore implementation and is intended to be
// constructed by higher-level factories (e.g., databases.Manager or agentd).
func NewPostgresEvolvingMemoryStore(pool *pgxpool.Pool) memory.EvolvingMemoryStore {
	return NewPostgresEvolvingMemoryStoreWithDimensions(pool, 0)
}

// NewPostgresEvolvingMemoryStoreWithDimensions returns a Postgres-backed
// evolving memory store with an optional pgvector dimensionality.
func NewPostgresEvolvingMemoryStoreWithDimensions(pool *pgxpool.Pool, dimensions int) memory.EvolvingMemoryStore {
	return &pgEvolvingMemoryStore{pool: pool, dimensions: dimensions}
}

type pgEvolvingMemoryStore struct {
	pool       *pgxpool.Pool
	dimensions int
}

// Close closes the underlying pool if present.
func (s *pgEvolvingMemoryStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}
