package agentd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
	"manifold/internal/constitution"
)

func TestConstitutionCreateAndList(t *testing.T) {
	a := &app{cfg: &config.Config{}, constitutionSvc: constitution.NewService(constitution.NewStore(nil))}
	_ = a.constitutionSvc.Init(nil)
	body, _ := json.Marshal(map[string]string{"body": "Rule 1"})
	req := httptest.NewRequest(http.MethodPost, "/api/constitution/versions", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	a.constitutionVersionsHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/constitution/versions", nil)
	rr = httptest.NewRecorder()
	a.constitutionVersionsHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
}
