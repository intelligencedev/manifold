package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeConfigExpandsHomeAndDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := NormalizeConfig(Config{Path: "~/.manifold/test.db"})
	if err != nil {
		t.Fatalf("NormalizeConfig: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if cfg.Path != filepath.Join(home, ".manifold", "test.db") {
		t.Fatalf("unexpected path: %s", cfg.Path)
	}
	if cfg.BusyTimeoutMs != DefaultBusyTimeoutMs {
		t.Fatalf("expected default busy timeout, got %d", cfg.BusyTimeoutMs)
	}
	if cfg.MaxOpenConns != DefaultMaxOpenConns {
		t.Fatalf("expected default max open conns, got %d", cfg.MaxOpenConns)
	}
}

func TestRunMigrationRunsOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := Open(ctx, Config{Path: filepath.Join(t.TempDir(), "migrations.db"), WAL: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	calls := 0
	migration := func(ctx context.Context, tx *sql.Tx) error {
		calls++
		_, err := tx.ExecContext(ctx, `CREATE TABLE migration_test(id INTEGER PRIMARY KEY)`)
		return err
	}
	if err := RunMigration(ctx, db, "001_test", migration); err != nil {
		t.Fatalf("RunMigration first: %v", err)
	}
	if err := RunMigration(ctx, db, "001_test", migration); err != nil {
		t.Fatalf("RunMigration second: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected migration to run once, got %d", calls)
	}
}
