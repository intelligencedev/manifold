package chat

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionsHandlerOptionsDoesNotRequireStore(t *testing.T) {
	called := false
	handler := SessionsHandler(SessionHandlerDeps{
		SetCORS: func(http.ResponseWriter, *http.Request, string) { called = true },
	})
	req := httptest.NewRequest(http.MethodOptions, "/api/chat/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || !called {
		t.Fatalf("OPTIONS status=%d corsCalled=%v", rec.Code, called)
	}
}

func TestSessionsHandlerReportsUnauthorizedAccess(t *testing.T) {
	handler := SessionsHandler(SessionHandlerDeps{AuthEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/api/chat/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
