package agentd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"manifold/internal/config"

	yaml "gopkg.in/yaml.v3"
)

func TestHasFocusedSettingsIntent_IgnoresControlFields(t *testing.T) {
	t.Parallel()

	if hasFocusedSettingsIntent([]byte(`{"configSource":"x","serverConfig":{}}`)) {
		t.Fatal("expected pure control-field payload to lack focused intent")
	}
	if !hasFocusedSettingsIntent([]byte(`{"configSource":"x","memoryEnabled":true}`)) {
		t.Fatal("expected focused settings field to signal intent")
	}
	if hasFocusedSettingsIntent([]byte(`{"configPatch":{"memory":{"enabled":true}}}`)) {
		t.Fatal("expected configPatch-only payload to lack focused intent")
	}
}

func TestResolveConfigYAMLPath_PrefersPinnedConfigPath(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{ConfigPath: "/tmp/pinned-manifold-config.yaml"}
	if got := resolveConfigYAMLPath(cfg); got != "/tmp/pinned-manifold-config.yaml" {
		t.Fatalf("expected pinned config path, got %q", got)
	}
}

func TestPersistToConfigYAML_PinnedPathCreatesAndMerges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	seed := "logLevel: info\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg := &config.Config{ConfigPath: path}
	if err := persistToConfigYAML(cfg, agentdSettings{
		MemoryEnabled:    true,
		LexMinifyEnabled: true,
		LogLevel:         "debug",
	}); err != nil {
		t.Fatalf("persistToConfigYAML: %v", err)
	}
	if cfg.ConfigPath != path {
		t.Fatalf("expected ConfigPath to remain pinned, got %q", cfg.ConfigPath)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal persisted config: %v", err)
	}
	if root["logLevel"] != "debug" {
		t.Fatalf("expected logLevel debug, got %#v", root["logLevel"])
	}
	memory, ok := root["memory"].(map[string]any)
	if !ok || memory["enabled"] != true {
		t.Fatalf("expected memory.enabled true, got %#v", root["memory"])
	}
	lex, ok := root["lexMinify"].(map[string]any)
	if !ok || lex["enabled"] != true {
		t.Fatalf("expected lexMinify.enabled true, got %#v", root["lexMinify"])
	}
}

func TestHandleUpdateAgentdConfig_FocusedSaveIgnoresEchoedConfigSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	seed := "memory:\n  enabled: false\nlogLevel: info\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg := &config.Config{ConfigPath: path}
	a := &app{cfg: cfg}
	body := `{"configSource":"x","memoryEnabled":true,"logLevel":"warn"}`
	req := httptest.NewRequest(http.MethodPut, "/api/config/agentd", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.handleUpdateAgentdConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body %s", rec.Code, rec.Body.String())
	}
	if !cfg.Memory.Enabled {
		t.Fatal("expected in-memory memory.enabled to apply")
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("expected in-memory logLevel warn, got %q", cfg.LogLevel)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	memory, ok := root["memory"].(map[string]any)
	if !ok || memory["enabled"] != true {
		t.Fatalf("expected durable memory.enabled true, got %#v", root["memory"])
	}
	if root["logLevel"] != "warn" {
		t.Fatalf("expected durable logLevel warn, got %#v", root["logLevel"])
	}
}

func TestHandleUpdateAgentdConfig_IgnoresRedactedServerConfigEcho(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	seed := "memory:\n  enabled: false\nlogLevel: info\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg := &config.Config{ConfigPath: path}
	a := &app{cfg: cfg}
	/* Redacted serverConfig from GET must not break focused PATCH decode. */
	body := `{"serverConfig":{"auth":"[REDACTED]","workdir":"/tmp"},"configSource":"x","memoryEnabled":true,"logLevel":"debug"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/config/agentd", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.handleUpdateAgentdConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d body %s", rec.Code, rec.Body.String())
	}
	if !cfg.Memory.Enabled {
		t.Fatal("expected memory.enabled to apply despite redacted serverConfig")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected logLevel debug, got %q", cfg.LogLevel)
	}
}
