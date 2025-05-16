package server

import (
	"net/http"
)

type Authenticator interface {
	Authenticate(r *http.Request) error
}

func Authenticate(next http.Handler, auth Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.Authenticate(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}