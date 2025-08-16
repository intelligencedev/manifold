package config

import (
	"os"
	"testing"
)

func TestLoad_DatabasesFromYAMLAndEnv(t *testing.T) {
	// Create a temporary YAML including databases section
	yaml := `
openai:
  apiKey: dummy
workdir: .
databases:
  search:
    backend: memory
    index: docs
  vector:
    backend: memory
    index: vectors
    dimensions: 3
    metric: cosine
  graph:
    backend: none
`
	f := "config_db_test.yaml"
	if err := os.WriteFile(f, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	defer func() { _ = os.Remove(f) }()

	// Ensure the loader will read our file
	old := os.Getenv("SPECIALISTS_CONFIG")
	defer func() { _ = os.Setenv("SPECIALISTS_CONFIG", old) }()
	_ = os.Setenv("SPECIALISTS_CONFIG", f)

	// Clear env vars that could interfere
	for _, k := range []string{"OPENAI_API_KEY", "WORKDIR", "SEARCH_BACKEND", "GRAPH_BACKEND"} {
		_ = os.Unsetenv(k)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Databases.Search.Backend != "memory" || cfg.Databases.Search.Index != "docs" {
		t.Fatalf("unexpected search cfg: %+v", cfg.Databases.Search)
	}
	if cfg.Databases.Vector.Dimensions != 3 || cfg.Databases.Vector.Metric != "cosine" {
		t.Fatalf("unexpected vector cfg: %+v", cfg.Databases.Vector)
	}
	if cfg.Databases.Graph.Backend != "none" {
		t.Fatalf("expected graph backend none, got %q", cfg.Databases.Graph.Backend)
	}

	// Now override with env var
	_ = os.Setenv("SEARCH_BACKEND", "none")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load(env override) error: %v", err)
	}
	if cfg.Databases.Search.Backend != "none" {
		t.Fatalf("env override failed; got %q", cfg.Databases.Search.Backend)
	}
}
