package agentd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
)

func TestMeRouteTerminatesWhenAuthDisabled(t *testing.T) {
	t.Setenv("FRONTEND_DEV_PROXY", "http://127.0.0.1:1")

	app := &app{cfg: &config.Config{}}
	mux := newRouter(app)
	if err := app.registerFrontend(mux); err != nil {
		t.Fatalf("register frontend: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	app.wrapWithMiddleware(mux).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}
}
