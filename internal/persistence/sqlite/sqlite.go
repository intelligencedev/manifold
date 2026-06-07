package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/vec1"
)

const (
	DefaultBusyTimeoutMs = 10000
	DefaultMaxOpenConns  = 1
)

type Config struct {
	Path          string
	BusyTimeoutMs int
	WAL           bool
	MaxOpenConns  int
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".manifold", "manifold.db"), nil
}

func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	cfg, err := NormalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	path := cfg.Path

	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
	}

	db, err := driver.Open(dsn(path, cfg.BusyTimeoutMs), vec1.Register)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)

	if err := configure(ctx, db, cfg.WAL && path != ":memory:"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func NormalizeConfig(cfg Config) (Config, error) {
	cfg.Path = strings.TrimSpace(cfg.Path)
	if cfg.Path == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return Config{}, err
		}
		cfg.Path = defaultPath
	} else if cfg.Path == "~" || strings.HasPrefix(cfg.Path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve user home: %w", err)
		}
		if cfg.Path == "~" {
			cfg.Path = home
		} else {
			cfg.Path = filepath.Join(home, strings.TrimPrefix(cfg.Path, "~/"))
		}
	}
	if cfg.BusyTimeoutMs <= 0 {
		cfg.BusyTimeoutMs = DefaultBusyTimeoutMs
	}
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = DefaultMaxOpenConns
	}
	return cfg, nil
}

func dsn(path string, busyTimeoutMs int) string {
	if strings.HasPrefix(path, "file:") {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return fmt.Sprintf("%s%s_txlock=immediate&_timefmt=rfc3339&_pragma=busy_timeout(%d)", path, sep, busyTimeoutMs)
	}
	if path == ":memory:" {
		return fmt.Sprintf("file:%s?mode=memory&cache=shared&_txlock=immediate&_timefmt=rfc3339&_pragma=busy_timeout(%d)", url.QueryEscape("manifold"), busyTimeoutMs)
	}
	u := url.URL{Scheme: "file", Path: path}
	return fmt.Sprintf("%s?_txlock=immediate&_timefmt=rfc3339&_pragma=busy_timeout(%d)", u.String(), busyTimeoutMs)
}

func configure(ctx context.Context, db *sql.DB, wal bool) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}
	if wal {
		if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
			return fmt.Errorf("enable sqlite WAL: %w", err)
		}
		if _, err := db.ExecContext(ctx, `PRAGMA synchronous = NORMAL`); err != nil {
			return fmt.Errorf("set sqlite synchronous mode: %w", err)
		}
	}
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return err
	}
	if err := Verify(ctx, db); err != nil {
		return err
	}
	return nil
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS manifold_schema_migrations (
	name TEXT PRIMARY KEY,
	applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`)
	if err != nil {
		return fmt.Errorf("initialize sqlite schema migrations table: %w", err)
	}
	return nil
}

func RunMigration(ctx context.Context, db *sql.DB, name string, fn func(context.Context, *sql.Tx) error) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("sqlite migration name is required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM manifold_schema_migrations WHERE name = ?)`, name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO manifold_schema_migrations(name) VALUES(?)`, name); err != nil {
		return err
	}
	return tx.Commit()
}

func Verify(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS temp.manifold_fts5_check USING fts5(value)`); err != nil {
		return fmt.Errorf("verify sqlite FTS5: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS temp.manifold_fts5_check`); err != nil {
		return fmt.Errorf("cleanup sqlite FTS5 check: %w", err)
	}
	var nprobe float64
	if err := db.QueryRowContext(ctx, `SELECT vec1_config('nprobe')`).Scan(&nprobe); err != nil {
		return fmt.Errorf("verify sqlite Vec1: %w", err)
	}
	return nil
}
