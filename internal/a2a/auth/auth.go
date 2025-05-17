package auth

import (
	"net/http"
)

// Authenticator interface for pluggable authentication
// schemes

type Authenticator interface {
	Authenticate(r *http.Request) error
}

// NoopAuthenticator is a no-op authenticator that accepts all requests

type NoopAuthenticator struct {}

func NewNoop() *NoopAuthenticator {
	return &NoopAuthenticator{}
}

func (a *NoopAuthenticator) Authenticate(r *http.Request) error {
	return nil
}