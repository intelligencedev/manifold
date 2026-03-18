package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DatabasesFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("OPENAI_API_KEY", "dummy")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/manifold?sslmode=disable")

	configText := `workdir: .
llm_client:
  provider: openai
  openai:
    apiKey: "${OPENAI_API_KEY}"
databases:
  defaultDSN: "${DATABASE_URL}"
  search:
    backend: postgres
    dsn: "${DATABASE_URL}"
    index: docs
  vector:
    backend: postgres
    dsn: "${DATABASE_URL}"
    index: vectors
    dimensions: 3
    metric: cosine
  graph:
    backend: postgres
    dsn: "${DATABASE_URL}"
  chat:
    backend: postgres
    dsn: "${DATABASE_URL}"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Databases.Search.Backend != "postgres" || cfg.Databases.Search.Index != "docs" {
		t.Fatalf("unexpected search cfg: %+v", cfg.Databases.Search)
	}
	if cfg.Databases.Vector.Dimensions != 3 || cfg.Databases.Vector.Metric != "cosine" {
		t.Fatalf("unexpected vector cfg: %+v", cfg.Databases.Vector)
	}
	if cfg.Databases.Graph.Backend != "postgres" {
		t.Fatalf("expected graph backend postgres, got %q", cfg.Databases.Graph.Backend)
	}
	if cfg.Databases.Chat.DSN != "postgres://user:pass@localhost:5432/manifold?sslmode=disable" {
		t.Fatalf("unexpected chat dsn: %q", cfg.Databases.Chat.DSN)
	}
}
