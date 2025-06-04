package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "cfgtest")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgContent := `host: "localhost"
port: 8080
database:
  connection_string: "user:pass@/dbname"
completions:
  default_host: "api.example.com"
  completions_model: "model"
  api_key: "key"
embeddings:
  host: "host"
  api_key: "key"
  dimensions: 128
  embed_prefix: "e"
  search_prefix: "s"
  reranker:
  host: "rhost"
`
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Host != "localhost" || cfg.Port != 8080 {
		t.Errorf("unexpected host/port: %v:%v", cfg.Host, cfg.Port)
	}
	if cfg.Database.ConnectionString != "user:pass@/dbname" {
		t.Errorf("database connection incorrect: %v", cfg.Database.ConnectionString)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	// Create bad YAML
	tmpFile, err := os.CreateTemp("", "bad.*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString("not: [invalid yaml"); err != nil {
		t.Fatalf("failed to write bad yaml: %v", err)
	}
	tmpFile.Close()

	_, err = LoadConfig(tmpFile.Name())
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
