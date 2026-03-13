package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	if v := firstNonEmpty("", "foo", "bar"); v != "foo" {
		t.Fatalf("expected 'foo', got %q", v)
	}
	if v := firstNonEmpty(); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestParseInt(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		n, err := parseInt("42")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 42 {
			t.Fatalf("expected 42, got %d", n)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		if _, err := parseInt("notanint"); err == nil {
			t.Fatalf("expected error for invalid int")
		}
	})
}

func TestLoadSpecialists_WrapperAndListAndDisabled(t *testing.T) {
	// Prepare wrapper-style YAML file
	wrapper := `specialists:
  - name: test-wrap
    model: warp-model
    baseURL: https://wrap.example
`
	file1 := "specialists_test_wrap.yaml"
	if err := os.WriteFile(file1, []byte(wrapper), 0o644); err != nil {
		t.Fatalf("write wrapper file: %v", err)
	}
	defer func() {
		_ = os.Remove(file1)
	}()

	old := os.Getenv("SPECIALISTS_CONFIG")
	defer func() {
		_ = os.Setenv("SPECIALISTS_CONFIG", old)
	}()
	_ = os.Setenv("SPECIALISTS_CONFIG", file1)

	var cfg Config
	if err := loadSpecialists(&cfg); err != nil {
		t.Fatalf("loadSpecialists(wrapper) returned error: %v", err)
	}
	if len(cfg.Specialists) != 1 || cfg.Specialists[0].Name != "test-wrap" {
		t.Fatalf("unexpected specialists: %#v", cfg.Specialists)
	}

	// Now test direct-list YAML
	list := `- name: direct-one
  model: direct-model
`
	file2 := "specialists_test_list.yaml"
	if err := os.WriteFile(file2, []byte(list), 0o644); err != nil {
		t.Fatalf("write list file: %v", err)
	}
	defer func() {
		_ = os.Remove(file2)
	}()
	_ = os.Setenv("SPECIALISTS_CONFIG", file2)
	cfg = Config{}
	if err := loadSpecialists(&cfg); err != nil {
		t.Fatalf("loadSpecialists(list) returned error: %v", err)
	}
	if len(cfg.Specialists) != 1 || cfg.Specialists[0].Name != "direct-one" {
		t.Fatalf("unexpected specialists from list: %#v", cfg.Specialists)
	}

	// Test SPECIALISTS_DISABLED
	_ = os.Setenv("SPECIALISTS_DISABLED", "true")
	cfg = Config{}
	if err := loadSpecialists(&cfg); err != nil {
		t.Fatalf("loadSpecialists(disabled) returned error: %v", err)
	}
	if len(cfg.Specialists) != 0 {
		t.Fatalf("expected 0 specialists when disabled, got %d", len(cfg.Specialists))
	}
	_ = os.Unsetenv("SPECIALISTS_DISABLED")
}

func TestEmbedApiHeadersEnv_JSONAndCSV(t *testing.T) {
	oldJSON := os.Getenv("EMBED_API_HEADERS")
	defer func() { _ = os.Setenv("EMBED_API_HEADERS", oldJSON) }()

	// Ensure required env for Load()
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	defer func() { _ = os.Setenv("OPENAI_API_KEY", oldOpenAI) }()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")
	oldWorkdir := os.Getenv("WORKDIR")
	defer func() { _ = os.Setenv("WORKDIR", oldWorkdir) }()
	_ = os.Setenv("WORKDIR", ".")

	_ = os.Setenv("EMBED_API_HEADERS", `{"x-api-key":"abc"}`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := cfg.Embedding.Headers["x-api-key"]; got != "abc" {
		t.Fatalf("expected x-api-key abc, got %q", got)
	}

	_ = os.Setenv("EMBED_API_HEADERS", "x-api-key:abc,foo=bar")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := cfg.Embedding.Headers["foo"]; got != "bar" {
		t.Fatalf("expected foo bar, got %q", got)
	}
}

func TestLoad_AllowToolsEnv(t *testing.T) {
	// Ensure required env for Load()
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	defer func() { _ = os.Setenv("OPENAI_API_KEY", oldOpenAI) }()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")
	oldWorkdir := os.Getenv("WORKDIR")
	defer func() { _ = os.Setenv("WORKDIR", oldWorkdir) }()
	_ = os.Setenv("WORKDIR", ".")

	oldAllow := os.Getenv("ALLOW_TOOLS")
	oldSpecialistsConfig := os.Getenv("SPECIALISTS_CONFIG")
	defer func() {
		_ = os.Setenv("ALLOW_TOOLS", oldAllow)
		_ = os.Setenv("SPECIALISTS_CONFIG", oldSpecialistsConfig)
	}()
	_ = os.Setenv("ALLOW_TOOLS", "a, b,,c")
	_ = os.Unsetenv("SPECIALISTS_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	want := []string{"a", "b", "c"}
	if got := cfg.ToolAllowList; len(got) != len(want) {
		t.Fatalf("unexpected allow list: got %#v want %#v", got, want)
	}
	for i := range cfg.ToolAllowList {
		if cfg.ToolAllowList[i] != want[i] {
			t.Fatalf("unexpected allow list: got %#v want %#v", cfg.ToolAllowList, want)
		}
	}
}

func TestLoad_EvolvingMemoryProviderEnv(t *testing.T) {
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	defer func() { _ = os.Setenv("OPENAI_API_KEY", oldOpenAI) }()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")

	oldWorkdir := os.Getenv("WORKDIR")
	defer func() { _ = os.Setenv("WORKDIR", oldWorkdir) }()
	_ = os.Setenv("WORKDIR", ".")

	oldProvider := os.Getenv("EVOLVING_MEMORY_PROVIDER")
	defer func() { _ = os.Setenv("EVOLVING_MEMORY_PROVIDER", oldProvider) }()
	_ = os.Setenv("EVOLVING_MEMORY_PROVIDER", "LOCAL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.EvolvingMemory.Provider != "local" {
		t.Fatalf("expected evolving memory provider local, got %q", cfg.EvolvingMemory.Provider)
	}
}

func TestLoad_EvolvingMemoryProviderEnv_Invalid(t *testing.T) {
	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	defer func() { _ = os.Setenv("OPENAI_API_KEY", oldOpenAI) }()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")

	oldWorkdir := os.Getenv("WORKDIR")
	defer func() { _ = os.Setenv("WORKDIR", oldWorkdir) }()
	_ = os.Setenv("WORKDIR", ".")

	oldProvider := os.Getenv("EVOLVING_MEMORY_PROVIDER")
	defer func() { _ = os.Setenv("EVOLVING_MEMORY_PROVIDER", oldProvider) }()
	_ = os.Setenv("EVOLVING_MEMORY_PROVIDER", "bogus")

	if _, err := Load(); err == nil {
		t.Fatalf("expected invalid evolving memory provider to fail")
	}
}

func TestLoad_EvolvingMemoryLLMClientFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configText := `evolvingMemory:
  enabled: true
  llmClient:
    provider: local
    openai:
      baseURL: http://localhost:11434/v1
      model: qwen-memory
`
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	oldOpenAI := os.Getenv("OPENAI_API_KEY")
	defer func() { _ = os.Setenv("OPENAI_API_KEY", oldOpenAI) }()
	_ = os.Setenv("OPENAI_API_KEY", "dummy")

	oldWorkdir := os.Getenv("WORKDIR")
	defer func() { _ = os.Setenv("WORKDIR", oldWorkdir) }()
	_ = os.Setenv("WORKDIR", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.EvolvingMemory.LLMClient.Provider != "local" {
		t.Fatalf("expected evolving memory llmClient provider local, got %q", cfg.EvolvingMemory.LLMClient.Provider)
	}
	if cfg.EvolvingMemory.LLMClient.OpenAI.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("unexpected evolving memory llmClient baseURL: %q", cfg.EvolvingMemory.LLMClient.OpenAI.BaseURL)
	}
	if cfg.EvolvingMemory.LLMClient.OpenAI.Model != "qwen-memory" {
		t.Fatalf("unexpected evolving memory llmClient model: %q", cfg.EvolvingMemory.LLMClient.OpenAI.Model)
	}
}
