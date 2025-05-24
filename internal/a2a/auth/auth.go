package auth

import (
	"errors"
	"net/http"
)

// Authenticator interface for pluggable authentication
// schemes

type Authenticator interface {
	Authenticate(r *http.Request) error
}

// NoopAuthenticator is a no-op authenticator that accepts all requests

type NoopAuthenticator struct{}

func NewNoop() *NoopAuthenticator {
	return &NoopAuthenticator{}
}

func (a *NoopAuthenticator) Authenticate(r *http.Request) error {
	return nil
}

// TokenAuthenticator validates requests using a static bearer token.
type TokenAuthenticator struct {
	Token string
	Send  bool
}

// NewToken creates a new token-based authenticator.
func NewToken(token string) *TokenAuthenticator {
	return &TokenAuthenticator{Token: token}
}

// NewTokenSender creates an authenticator that adds the bearer token to
// outgoing requests.
func NewTokenSender(token string) *TokenAuthenticator {
	return &TokenAuthenticator{Token: token, Send: true}
}

// Authenticate checks the Authorization header for the expected token.
func (a *TokenAuthenticator) Authenticate(r *http.Request) error {
	if a.Token == "" {
		return nil
	}
	if a.Send {
		r.Header.Set("Authorization", "Bearer "+a.Token)
		return nil
	}
	authz := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(authz) <= len(prefix) || authz[:len(prefix)] != prefix {
		return errors.New("unauthorized")
	}
	if authz[len(prefix):] != a.Token {
		return errors.New("unauthorized")
	}
	return nil
}
