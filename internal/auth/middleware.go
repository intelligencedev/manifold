package auth

import "net/http"

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
