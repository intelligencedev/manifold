package mcpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServersHandlerRequiresUser(t *testing.T) {
	handler := ServersHandler(ServerHandlerDeps{})
	req := httptest.NewRequest(http.MethodGet, "/api/mcp/servers", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestServerDetailHandlerRequiresUser(t *testing.T) {
	handler := ServerDetailHandler(ServerHandlerDeps{})
	req := httptest.NewRequest(http.MethodDelete, "/api/mcp/servers/demo", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
