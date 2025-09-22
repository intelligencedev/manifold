package auth

import (
	"net/http"
	"strings"
)

// Middleware returns an http.Handler that attaches the current user to the request context
// if a valid session cookie is present. When require is true, unauthenticated requests get 401.
func Middleware(store *Store, cookieName string, require bool) func(http.Handler) http.Handler {
	if cookieName == "" {
		cookieName = "sio_session"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err == nil && c != nil && c.Value != "" {
				if sess, user, err := store.GetSession(r.Context(), c.Value); err == nil && sess != nil && user != nil {
					r = r.WithContext(WithUser(r.Context(), user))
				}
			}
			if require {
				if _, ok := CurrentUser(r.Context()); !ok {
					w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRoles wraps a handler and ensures the current user has at least one of the provided roles.
func RequireRoles(store *Store, roles ...string) func(http.Handler) http.Handler {
	want := make([]string, 0, len(roles))
	for _, r := range roles {
		if strings.TrimSpace(r) != "" {
			want = append(want, strings.TrimSpace(r))
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := CurrentUser(r.Context())
			if !ok || u == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			for _, rn := range want {
				okRole, err := store.HasRole(r.Context(), u.ID, rn)
				if err == nil && okRole {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "forbidden", http.StatusForbidden)
		})
	}
}
