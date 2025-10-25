package auth

import (
	"context"
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

func TestAppendLogoutRedirect(t *testing.T) {
	t.Parallel()
	out := appendLogoutRedirect("https://example.com/logout?foo=bar", "redirect_uri", "https://app.local/auth/login")
	want := "https://example.com/logout?foo=bar&redirect_uri=https%3A%2F%2Fapp.local%2Fauth%2Flogin"
	if out != want {
		t.Fatalf("expected %s, got %s", want, out)
	}
}
