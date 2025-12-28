package databases

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPool creates a Postgres connection pool using the standard defaults.
func OpenPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return newPgPool(ctx, dsn)
}
