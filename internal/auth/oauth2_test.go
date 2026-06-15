package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewOAuth2Validation(t *testing.T) {
	t.Parallel()
	_, err := NewOAuth2(context.Background(), &Store{}, OAuth2Options{})
	if err == nil {
		t.Fatalf("expected error when oauth2 endpoints missing")
	}
}

func TestNormalizeDefaultRoles(t *testing.T) {
	t.Parallel()
	roles := normalizeDefaultRoles([]string{"Admin", "user", "  "})
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %v", roles)
	}
	if roles[0] != "admin" || roles[1] != "user" {
		t.Fatalf("unexpected role ordering/content: %v", roles)
	}
}

func TestExtractRoles(t *testing.T) {
	t.Parallel()
	payload := map[string]any{
		"groups": []any{"Admin", "dev", "admin"},
	}
	roles := extractRoles(payload, "groups")
	if len(roles) != 2 {
		t.Fatalf("expected deduped roles, got %v", roles)
	}
	if roles[0] != "admin" || roles[1] != "dev" {
		t.Fatalf("unexpected roles: %v", roles)
	}
	if out := extractRoles(payload, "missing"); len(out) != 0 {
		t.Fatalf("expected empty slice for missing path, got %v", out)
	}
}

func TestDig(t *testing.T) {
	t.Parallel()
	payload := map[string]any{
		"profile": map[string]any{
			"email": "user@example.com",
		},
	}
	val, ok := dig(payload, "profile.email")
	if !ok {
		t.Fatalf("expected to find nested field")
	}
	if val.(string) != "user@example.com" {
		t.Fatalf("unexpected value: %v", val)
	}
	if _, ok := dig(payload, "profile.missing"); ok {
		t.Fatalf("expected missing path to be false")
	}
}

func TestOAuth2MeHandlerEscapesUserFields(t *testing.T) {
	t.Parallel()
	user := &User{
		Email:   `mike+test@example.com`,
		Name:    `O'Brien "the Magnificent"`,
		Picture: `https://cdn.example.com/p.jpg?q="x"`,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req = req.WithContext(WithUser(req.Context(), user))
	rec := httptest.NewRecorder()

	(&OAuth2{}).MeHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("unexpected content-type: %q", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, rec.Body.String())
	}
	if got["email"] != user.Email {
		t.Fatalf("email round-trip failed: want %q got %q", user.Email, got["email"])
	}
	if got["name"] != user.Name {
		t.Fatalf("name round-trip failed: want %q got %q", user.Name, got["name"])
	}
	if got["picture"] != user.Picture {
		t.Fatalf("picture round-trip failed: want %q got %q", user.Picture, got["picture"])
	}
}

func TestOAuth2MeHandlerUnauthorized(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()

	(&OAuth2{}).MeHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, rec.Body.String())
	}
	if got["error"] != "unauthorized" {
		t.Fatalf("expected error=unauthorized, got %v", got)
	}
}

func TestAppendLogoutRedirect(t *testing.T) {
	t.Parallel()
	out := appendLogoutRedirect("https://example.com/logout?foo=bar", "redirect_uri", "https://app.local/auth/login")
	want := "https://example.com/logout?foo=bar&redirect_uri=https%3A%2F%2Fapp.local%2Fauth%2Flogin"
	if out != want {
		t.Fatalf("expected %s, got %s", want, out)
	}
}
