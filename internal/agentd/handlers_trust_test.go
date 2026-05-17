package agentd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
	"manifold/internal/trust"
)

func TestTrustBudgetRefillHandler(t *testing.T) {
	a := &app{cfg: &config.Config{}, trustService: trust.NewService(trust.NewStore(nil))}
	_ = a.trustService.Init(nil)
	body, _ := json.Marshal(map[string]any{"quota": 5})
	req := httptest.NewRequest(http.MethodPost, "/api/trust/budgets/orchestrator/refill", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	a.trustBudgetActionHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
}
