package agentd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"manifold/internal/config"
)

func TestWrapWithMiddlewarePreflight(t *testing.T) {
	t.Parallel()

	called := false
	a := &app{cfg: &config.Config{}}
	handler := a.wrapWithMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/agent/run", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Accept")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if called {
		t.Fatal("preflight request should not reach wrapped handler")
	}
	if res.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, res.Code)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allow-origin header to echo request origin, got %q", got)
	}
	if got := res.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected allow-credentials header, got %q", got)
	}
	if got := res.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Accept" {
		t.Fatalf("expected allow-headers to reflect requested headers, got %q", got)
	}
	if got := res.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPost) || !strings.Contains(got, http.MethodOptions) {
		t.Fatalf("expected allow-methods to include POST and OPTIONS, got %q", got)
	}
	if vary := strings.Join(res.Header().Values("Vary"), ","); !strings.Contains(vary, "Origin") {
		t.Fatalf("expected Vary header to include Origin, got %q", vary)
	}
}

func TestWrapWithMiddlewareAddsCORSHeadersToResponses(t *testing.T) {
	t.Parallel()

	a := &app{cfg: &config.Config{}}
	handler := a.wrapWithMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config/agentd", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allow-origin header to echo request origin, got %q", got)
	}
	if got := res.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected allow-credentials header, got %q", got)
	}
	if got := res.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodGet) {
		t.Fatalf("expected allow-methods to include GET, got %q", got)
	}
}
