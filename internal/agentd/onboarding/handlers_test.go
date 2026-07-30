package onboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
)

func TestStatusHandler(t *testing.T) {
	cfg := &config.Config{LLMClient: config.LLMClientConfig{Provider: "openai"}}
	h := StatusHandler(Deps{
		Config:       cfg,
		ListenAddr:   ":8080",
		PublicURL:    "https://example.test",
		ConfigPath:   func() string { return "/tmp/manifold.yaml" },
		ResolveModel: func(config.LLMClientConfig) string { return "gpt-test" },
	})
	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	res := httptest.NewRecorder()
	h(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var got StatusResponse
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.NeedsSetup != !got.Ready || got.Model != "gpt-test" || got.ConfigPath != "/tmp/manifold.yaml" {
		t.Fatalf("response = %+v", got)
	}
}

func TestCompleteHandlerWiresCallbacks(t *testing.T) {
	cfg := &config.Config{}
	var got CompleteRequest
	var seeded int64
	h := CompleteHandler(Deps{
		Config:        cfg,
		ApplyComplete: func(req CompleteRequest) error { got = req; return nil },
		SeedPrompt: func(_ context.Context, userID int64) error {
			seeded = userID
			return nil
		},
	})
	body := bytes.NewBufferString(`{"provider":"local","model":"test"}`)
	res := httptest.NewRecorder()
	h(res, httptest.NewRequest(http.MethodPost, "/api/setup/complete", body))
	if res.Code != http.StatusOK || got.Provider != "local" || got.Model != "test" || seeded != 0 {
		t.Fatalf("status=%d request=%+v seeded=%d", res.Code, got, seeded)
	}
}
