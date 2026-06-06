package databases

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PlaygroundStore persists playground entities into SQLite or Postgres.
type PlaygroundStore struct {
	pool *pgxpool.Pool
	db   *sql.DB
}

// NewPlaygroundStore creates the store and ensures schema exists.
func NewPlaygroundStore(ctx context.Context, pool *pgxpool.Pool) (*PlaygroundStore, error) {
	store := &PlaygroundStore{pool: pool}
	if err := store.initSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

// NewSQLitePlaygroundStore creates the SQLite playground store and ensures schema exists.
func NewSQLitePlaygroundStore(ctx context.Context, db *sql.DB) (*PlaygroundStore, error) {
	store := &PlaygroundStore{db: db}
	if err := store.initSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

// NewPlaygroundStoreFromDSN constructs a new store using its own connection pool.
func NewPlaygroundStoreFromDSN(ctx context.Context, dsn string) (*PlaygroundStore, error) {
	pool, err := newPgPool(ctx, dsn)
	if err != nil {
		return nil, err
	}
	store, err := NewPlaygroundStore(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *PlaygroundStore) initSchema(ctx context.Context) error {
	if s.db != nil {
		return s.initSQLiteSchema(ctx)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS playground_prompts (
			id TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL DEFAULT 0,
			payload JSONB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS playground_prompt_versions (
			id TEXT PRIMARY KEY,
			prompt_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			user_id BIGINT NOT NULL DEFAULT 0,
			payload JSONB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS playground_datasets (
			id TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL DEFAULT 0,
			payload JSONB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS playground_snapshots (
			id TEXT NOT NULL,
			dataset_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			user_id BIGINT NOT NULL DEFAULT 0,
			payload JSONB NOT NULL,
			PRIMARY KEY (dataset_id, id)
		);`,
		`CREATE TABLE IF NOT EXISTS playground_rows (
			dataset_id TEXT NOT NULL,
			snapshot_id TEXT NOT NULL,
			row_id TEXT NOT NULL,
			user_id BIGINT NOT NULL DEFAULT 0,
			payload JSONB NOT NULL,
			PRIMARY KEY (dataset_id, snapshot_id, row_id)
		);`,
		`CREATE TABLE IF NOT EXISTS playground_experiments (
			id TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL DEFAULT 0,
			payload JSONB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS playground_runs (
			id TEXT PRIMARY KEY,
			experiment_id TEXT NOT NULL,
			user_id BIGINT NOT NULL DEFAULT 0,
			payload JSONB NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS playground_run_results (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			user_id BIGINT NOT NULL DEFAULT 0,
			payload JSONB NOT NULL
		);`,
		// Backfill for existing deployments: ensure user_id columns and indexes exist.
		`ALTER TABLE playground_prompts ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;`,
		`ALTER TABLE playground_prompt_versions ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;`,
		`ALTER TABLE playground_datasets ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;`,
		`ALTER TABLE playground_snapshots ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;`,
		`ALTER TABLE playground_rows ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;`,
		`ALTER TABLE playground_experiments ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;`,
		`ALTER TABLE playground_runs ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;`,
		`ALTER TABLE playground_run_results ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;`,
		`CREATE INDEX IF NOT EXISTS idx_pg_prompts_user ON playground_prompts(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_prompt_versions_user ON playground_prompt_versions(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_datasets_user ON playground_datasets(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_snapshots_user ON playground_snapshots(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_rows_user ON playground_rows(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_experiments_user ON playground_experiments(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_runs_user ON playground_runs(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_run_results_user ON playground_run_results(user_id);`,
	}
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("init playground schema: %w", err)
		}
	}
	return nil
}

func (s *PlaygroundStore) initSQLiteSchema(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("sqlite playground store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS playground_prompts (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL DEFAULT 0,
	payload TEXT NOT NULL,
	CHECK (json_valid(payload))
);
CREATE TABLE IF NOT EXISTS playground_prompt_versions (
	id TEXT PRIMARY KEY,
	prompt_id TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	user_id INTEGER NOT NULL DEFAULT 0,
	payload TEXT NOT NULL,
	CHECK (json_valid(payload))
);
CREATE TABLE IF NOT EXISTS playground_datasets (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL DEFAULT 0,
	payload TEXT NOT NULL,
	CHECK (json_valid(payload))
);
CREATE TABLE IF NOT EXISTS playground_snapshots (
	id TEXT NOT NULL,
	dataset_id TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	user_id INTEGER NOT NULL DEFAULT 0,
	payload TEXT NOT NULL,
	PRIMARY KEY (dataset_id, id),
	CHECK (json_valid(payload))
);
CREATE TABLE IF NOT EXISTS playground_rows (
	dataset_id TEXT NOT NULL,
	snapshot_id TEXT NOT NULL,
	row_id TEXT NOT NULL,
	user_id INTEGER NOT NULL DEFAULT 0,
	payload TEXT NOT NULL,
	PRIMARY KEY (dataset_id, snapshot_id, row_id),
	CHECK (json_valid(payload))
);
CREATE TABLE IF NOT EXISTS playground_experiments (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL DEFAULT 0,
	payload TEXT NOT NULL,
	CHECK (json_valid(payload))
);
CREATE TABLE IF NOT EXISTS playground_runs (
	id TEXT PRIMARY KEY,
	experiment_id TEXT NOT NULL,
	user_id INTEGER NOT NULL DEFAULT 0,
	payload TEXT NOT NULL,
	CHECK (json_valid(payload))
);
CREATE TABLE IF NOT EXISTS playground_run_results (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	user_id INTEGER NOT NULL DEFAULT 0,
	payload TEXT NOT NULL,
	CHECK (json_valid(payload))
);
CREATE INDEX IF NOT EXISTS idx_pg_prompts_user ON playground_prompts(user_id);
CREATE INDEX IF NOT EXISTS idx_pg_prompt_versions_user ON playground_prompt_versions(user_id);
CREATE INDEX IF NOT EXISTS idx_pg_datasets_user ON playground_datasets(user_id);
CREATE INDEX IF NOT EXISTS idx_pg_snapshots_user ON playground_snapshots(user_id);
CREATE INDEX IF NOT EXISTS idx_pg_rows_user ON playground_rows(user_id);
CREATE INDEX IF NOT EXISTS idx_pg_experiments_user ON playground_experiments(user_id);
CREATE INDEX IF NOT EXISTS idx_pg_runs_user ON playground_runs(user_id);
CREATE INDEX IF NOT EXISTS idx_pg_run_results_user ON playground_run_results(user_id);
`)
	return err
}
