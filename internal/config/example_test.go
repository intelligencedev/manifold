package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigYAMLExampleLoads(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(repoRoot, "config.yaml.example"))
	if err != nil {
		t.Fatalf("read config.yaml.example: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), data, 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	t.Chdir(tmpDir)

	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("WORKDIR", tmpDir)
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	t.Setenv("GOOGLE_LLM_API_KEY", "test-google-key")
	t.Setenv("EMBED_API_KEY", "test-embed-key")
	t.Setenv("LLAMA_API_KEY", "test-rerank-key")
	t.Setenv("CLICKHOUSE_DSN", "")
	t.Setenv("AUTH_CLIENT_ID", "test-auth-client")
	t.Setenv("AUTH_CLIENT_SECRET", "test-auth-secret")
	t.Setenv("MATRIX_USER_ID", "@manifold:example.com")
	t.Setenv("MATRIX_ACCESS_TOKEN", "test-matrix-token")
	t.Setenv("APPLE_TEAM_ID", "TEAM123456")
	t.Setenv("APPLE_KEY_ID", "KEY123456")
	t.Setenv("APPLE_PRIVATE_KEY_PATH", "")
	t.Setenv("APPLE_PRIVATE_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() config.yaml.example: %v", err)
	}
	if cfg.CodeQA.ArtifactDir == "" {
		t.Fatal("expected codeQA artifactDir from example config")
	}
	if len(cfg.MCP.Servers) != 0 {
		t.Fatalf("expected no inline mcp.servers in config example, got %+v", cfg.MCP.Servers)
	}
}

func TestConfigYAMLBasicLoads(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(repoRoot, "config.yaml.basic"))
	if err != nil {
		t.Fatalf("read config.yaml.basic: %v", err)
	}

	tmpDir := t.TempDir()
	home := filepath.Join(tmpDir, "home")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), data, 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	t.Chdir(tmpDir)

	t.Setenv("HOME", home)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_LLM_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() config.yaml.basic: %v", err)
	}
	if cfg.Workdir != filepath.Join(home, ".manifold", "projects") {
		t.Fatalf("unexpected basic workdir: %q", cfg.Workdir)
	}
	if cfg.Memory.Enabled {
		t.Fatal("expected basic config to leave global memory disabled")
	}
}
