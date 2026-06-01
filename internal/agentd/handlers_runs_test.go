package agentd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"manifold/internal/config"
)

func TestRunsHandlerFiltersInMemoryRunsByWindow(t *testing.T) {
	store := newRunStore()
	now := time.Now().UTC()
	store.createWithID("recent", "recent prompt", now.Add(-30*time.Minute))
	store.createWithID("old", "old prompt", now.Add(-2*time.Hour))

	a := &app{cfg: &config.Config{}, runs: store}
	req := httptest.NewRequest(http.MethodGet, "/api/runs?window=1h", nil)
	rr := httptest.NewRecorder()

	a.runsHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var runs []AgentRun
	if err := json.Unmarshal(rr.Body.Bytes(), &runs); err != nil {
		t.Fatalf("unmarshal runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run in window, got %d: %#v", len(runs), runs)
	}
	if runs[0].ID != "recent" {
		t.Fatalf("expected recent run, got %q", runs[0].ID)
	}
}

func TestRunsHandlerRejectsInvalidWindow(t *testing.T) {
	a := &app{cfg: &config.Config{}, runs: newRunStore()}
	req := httptest.NewRequest(http.MethodGet, "/api/runs?window=soon", nil)
	rr := httptest.NewRecorder()

	a.runsHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}
