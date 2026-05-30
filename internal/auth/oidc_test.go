package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestOIDCScopes(t *testing.T) {
	t.Parallel()
	defaults := oidcScopes(nil)
	if strings.Join(defaults, " ") != "openid email profile" {
		t.Fatalf("unexpected default scopes: %v", defaults)
	}
	custom := oidcScopes([]string{"email", "name"})
	if strings.Join(custom, " ") != "openid email name" {
		t.Fatalf("unexpected custom scopes: %v", custom)
	}
}

func TestMergeAppleFormUser(t *testing.T) {
	t.Parallel()
	claims := Claims{}
	mergeAppleFormUser(&claims, `{"email":"user@example.com","name":{"firstName":"Ada","lastName":"Lovelace"}}`)
	if claims.Email != "user@example.com" {
		t.Fatalf("unexpected email: %q", claims.Email)
	}
	if claims.Name != "Ada Lovelace" {
		t.Fatalf("unexpected name: %q", claims.Name)
	}
}

func TestParseOIDCCallbackFormPost(t *testing.T) {
	t.Parallel()
	form := url.Values{"state": {"s"}, "code": {"c"}}
	req := httptest.NewRequest(http.MethodPost, "/auth/callback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := parseOIDCCallbackForm(req); err != nil {
		t.Fatalf("parse callback form: %v", err)
	}
	if req.Form.Get("state") != "s" || req.Form.Get("code") != "c" {
		t.Fatalf("unexpected callback form: %v", req.Form)
	}
}

func TestOIDCLogoutLocalWhenNoLogoutURL(t *testing.T) {
	t.Parallel()
	provider := &OIDC{
		OAuth2Config: &oauth2.Config{ClientID: "client"},
		CookieName:   "sid",
		Issuer:       "https://appleid.apple.com",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/logout?next=/signed-out", nil)
	provider.LogoutHandler(false, "").ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "http://example.com/signed-out" {
		t.Fatalf("unexpected location: %q", got)
	}
}

func TestOIDCLogoutKeycloakFallback(t *testing.T) {
	t.Parallel()
	provider := &OIDC{
		OAuth2Config: &oauth2.Config{ClientID: "client"},
		CookieName:   "sid",
		Issuer:       "https://auth.example.com/realms/manifold",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/logout?next=/auth/login", nil)
	provider.LogoutHandler(false, "").ServeHTTP(rec, req)
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "https://auth.example.com/realms/manifold/protocol/openid-connect/logout?") {
		t.Fatalf("unexpected logout location: %q", location)
	}
	if !strings.Contains(location, "client_id=client") {
		t.Fatalf("logout location missing client_id: %q", location)
	}
}
