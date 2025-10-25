package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAbsoluteRedirectURL(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "http://localhost:32180/anything", nil)
	got := absoluteRedirectURL(req, "/next", "/fallback")
	if got != "http://localhost:32180/next" {
		t.Fatalf("unexpected redirect: %s", got)
	}
	reqTLS := httptest.NewRequest(http.MethodGet, "http://localhost:32180/anything", nil)
	reqTLS.TLS = &tls.ConnectionState{}
	got = absoluteRedirectURL(reqTLS, "", "/auth/login")
	if got != "https://localhost:32180/auth/login" {
		t.Fatalf("expected https fallback, got %s", got)
	}
	got = absoluteRedirectURL(reqTLS, "https://example.com/done", "/auth/login")
	if got != "https://example.com/done" {
		t.Fatalf("expected absolute URL untouched, got %s", got)
	}
}
