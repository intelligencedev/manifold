package agentd

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"manifold/internal/config"
)

func TestListenHTTPFallsBackWhenPreferredBusy(t *testing.T) {
	blocker, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen blocker: %v", err)
	}
	defer blocker.Close()
	port := blocker.Addr().(*net.TCPAddr).Port

	ln, bindAddr, err := listenHTTP(port)
	if err != nil {
		t.Fatalf("listenHTTP: %v", err)
	}
	defer ln.Close()

	boundPort := ln.Addr().(*net.TCPAddr).Port
	if boundPort == port {
		t.Fatalf("expected fallback free port, still on %d", port)
	}
	if !strings.Contains(bindAddr, ":") {
		t.Fatalf("unexpected bind addr %q", bindAddr)
	}
	base := publicBaseURL(bindAddr)
	if !strings.HasPrefix(base, "http://") {
		t.Fatalf("unexpected base URL %q", base)
	}
}

func TestSetupStatusAndComplete(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("HOME", home)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_LLM_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("MANIFOLD_CONFIG", filepath.Join(home, ".manifold", "config.yaml"))

	cfgPath := filepath.Join(home, ".manifold", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := &config.Config{
		ConfigPath: cfgPath,
		LLMClient: config.LLMClientConfig{
			Provider: "openai",
			OpenAI: config.OpenAIConfig{
				Model:   "gpt-5-mini",
				BaseURL: config.OpenAIAPIV1BaseURL,
			},
		},
	}

	a := &app{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/setup/status", a.setupStatusHandler())
	mux.HandleFunc("/api/setup/complete", a.setupCompleteHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code %d body %s", rec.Code, rec.Body.String())
	}
	var status setupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Ready || !status.NeedsSetup {
		t.Fatalf("expected needs setup, got %+v", status)
	}

	body := "{\"provider\":\"openai\",\"apiKey\":\"sk-test-key\",\"model\":\"gpt-test\"}"
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete code %d body %s", rec.Code, rec.Body.String())
	}
	var done setupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &done); err != nil {
		t.Fatalf("decode complete: %v", err)
	}
	if !done.Ready {
		t.Fatalf("expected ready after complete, got %+v", done)
	}
	if cfg.LLMClient.OpenAI.APIKey != "sk-test-key" {
		t.Fatalf("expected api key applied, got %q", cfg.LLMClient.OpenAI.APIKey)
	}
	if cfg.LLMClient.OpenAI.Model != "gpt-test" {
		t.Fatalf("expected model applied, got %q", cfg.LLMClient.OpenAI.Model)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if !strings.Contains(string(raw), "sk-test-key") {
		t.Fatalf("expected api key persisted, content=%s", string(raw))
	}
}

func TestFindConfigYAMLPathDefault(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("HOME", home)
	t.Setenv("MANIFOLD_CONFIG", "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	path := findConfigYAMLPath()
	if !strings.HasSuffix(path, "config.yaml") {
		t.Fatalf("unexpected path %q", path)
	}
}
