package databases

import (
	"manifold/internal/persistence"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPulseStore returns a Postgres-backed pulse store when a pool is provided,
// otherwise an in-memory implementation.
func NewPulseStore(pool *pgxpool.Pool) persistence.PulseStore {
	if pool == nil {
		return &memPulseStore{
			rooms: map[string]persistence.PulseRoom{},
			tasks: map[string]map[string]persistence.PulseTask{},
		}
	}
	return &pgPulseStore{pool: pool}
}
