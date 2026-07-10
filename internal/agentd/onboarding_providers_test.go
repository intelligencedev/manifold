package agentd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"manifold/internal/config"
)

func newSetupApp(t *testing.T) (*app, string, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		ConfigPath: cfgPath,
		LLMClient: config.LLMClientConfig{
			Provider: "openai",
			OpenAI:   config.OpenAIConfig{Model: "gpt-5-mini", BaseURL: config.OpenAIAPIV1BaseURL},
		},
	}
	a := &app{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/setup/complete", a.setupCompleteHandler())
	return a, cfgPath, mux
}

func postSetup(t *testing.T, mux http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSetupCompleteOpenRouterAppliesDefaults(t *testing.T) {
	a, cfgPath, mux := newSetupApp(t)

	rec := postSetup(t, mux, `{"provider":"openrouter","apiKey":"sk-or-test","model":"anthropic/claude-3.5-sonnet"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if a.cfg.LLMClient.Provider != "openrouter" {
		t.Errorf("provider = %q, want openrouter", a.cfg.LLMClient.Provider)
	}
	// OpenRouter is Anthropic-backed: settings land in the Anthropic sub-config.
	if got := a.cfg.LLMClient.Anthropic.BaseURL; got != "https://openrouter.ai/api" {
		t.Errorf("Anthropic.BaseURL = %q, want openrouter endpoint", got)
	}
	if got := a.cfg.LLMClient.Anthropic.APIKey; got != "sk-or-test" {
		t.Errorf("Anthropic.APIKey = %q, want sk-or-test", got)
	}
	if got := a.cfg.LLMClient.Anthropic.ExtraParams["max_tokens"]; got != 16384 {
		t.Errorf("Anthropic.ExtraParams[max_tokens] = %v, want 16384", got)
	}
	raw, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(raw), "max_tokens") || !strings.Contains(string(raw), "openrouter.ai") {
		t.Fatalf("expected extraParams + baseURL persisted, got:\n%s", raw)
	}
	if !strings.Contains(string(raw), "anthropic") {
		t.Fatalf("expected openrouter persisted under anthropic block, got:\n%s", raw)
	}
}

func TestSetupCompleteLlamaCppRequiresEndpoint(t *testing.T) {
	_, _, mux := newSetupApp(t)
	rec := postSetup(t, mux, `{"provider":"llamacpp","model":"local-model"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when llamacpp endpoint missing, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSetupCompleteLlamaCppAppliesDefaults(t *testing.T) {
	a, _, mux := newSetupApp(t)
	rec := postSetup(t, mux, `{"provider":"llamacpp","model":"local-model","baseUrl":"http://192.168.1.9:8080/v1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if got := a.cfg.LLMClient.OpenAI.BaseURL; got != "http://192.168.1.9:8080/v1" {
		t.Errorf("baseURL = %q, want user-provided", got)
	}
	if got := a.cfg.LLMClient.OpenAI.ExtraParams["cache_prompt"]; got != true {
		t.Errorf("ExtraParams[cache_prompt] = %v, want true", got)
	}
}
