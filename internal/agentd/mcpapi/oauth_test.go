package mcpapi

import (
	"crypto/tls"
	"errors"
	"net/http"
	"testing"

	"manifold/internal/persistence"
)

func TestOAuthCookieNameIsStableAndScoped(t *testing.T) {
	t.Parallel()

	got := OAuthCookieName("state_", "opaque-state")
	if got == "state_" || got != OAuthCookieName("state_", "opaque-state") {
		t.Fatalf("OAuthCookieName() = %q, want stable non-empty suffix", got)
	}
	if got == OAuthCookieName("state_", "different-state") {
		t.Fatalf("cookie names collide for different state values: %q", got)
	}
}

func TestMCPConversionAndOAuthHelpers(t *testing.T) {
	server := &persistence.MCPServer{Name: "demo", BearerToken: "bearer", OAuthAccessToken: "oauth"}
	converted := ConvertToConfig(*server)
	if converted.Name != "demo" || converted.BearerToken != "oauth" {
		t.Fatalf("converted = %+v", converted)
	}
	if got := OAuthScopes(nil, server); len(got) != 2 || got[0] != "openid" {
		t.Fatalf("default scopes = %#v", got)
	}
	if !RequiresOAuthPrompt(errors.New("unauthorized")) || RequiresOAuthPrompt(nil) {
		t.Fatal("OAuth prompt classification is incorrect")
	}
}

func TestUsesSecureCookiesChecksTLSAndForwardedProtocol(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		req    *http.Request
		secure bool
	}{
		{name: "plain", req: &http.Request{}, secure: false},
		{name: "tls", req: &http.Request{TLS: &tls.ConnectionState{}}, secure: true},
		{name: "forwarded", req: &http.Request{Header: http.Header{"X-Forwarded-Proto": []string{"https"}}}, secure: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UsesSecureCookies(tc.req); got != tc.secure {
				t.Fatalf("UsesSecureCookies() = %v, want %v", got, tc.secure)
			}
		})
	}
}

func TestValidateOAuthTargetURLRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		url  string
		code int
	}{
		{name: "empty", code: http.StatusBadRequest},
		{name: "missing host", url: "https:///path", code: http.StatusBadRequest},
		{name: "unsupported scheme", url: "ftp://example.com", code: http.StatusBadRequest},
		{name: "invalid host", url: "https://example..com", code: http.StatusBadRequest},
		{name: "valid", url: "https://example.com/mcp", code: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, err := ValidateOAuthTargetURL(tc.url)
			if code != tc.code {
				t.Fatalf("status = %d, want %d", code, tc.code)
			}
			if (err != nil) == (code == http.StatusOK) {
				t.Fatalf("error = %v for status %d", err, code)
			}
		})
	}
}
