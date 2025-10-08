package auth

import "testing"

func requireRole(t *testing.T, roles []string, want string) {
	t.Helper()
	for _, r := range roles {
		if r == want {
			return
		}
	}
	t.Fatalf("expected role %q in %v", want, roles)
}

func TestRolesFromClaims(t *testing.T) {
	claims := Claims{}
	out := rolesFromClaims(claims)
	if len(out) != 1 || out[0] != "user" {
		t.Fatalf("expected default user role, got %v", out)
	}

	claims = Claims{RealmAccess: struct {
		Roles []string `json:"roles"`
	}{Roles: []string{"Admin"}}}
	out = rolesFromClaims(claims)
	requireRole(t, out, "admin")
	requireRole(t, out, "user")

	claims = Claims{Groups: []string{"/Admin"}}
	out = rolesFromClaims(claims)
	requireRole(t, out, "admin")
	requireRole(t, out, "user")
}
