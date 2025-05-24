// Package auth provides authentication mechanisms for the A2A protocol
package auth

import (
	"errors"
	"net/http"
)

// ModelsAuthenticator interface for use with models package
type ModelsAuthenticator interface {
	Authenticate(r *http.Request) (bool, error)
}

// NoopModelsAuthenticator is a no-op authenticator that accepts all requests
type NoopModelsAuthenticator struct{}

func NewNoopModels() *NoopModelsAuthenticator {
	return &NoopModelsAuthenticator{}
}

func (a *NoopModelsAuthenticator) Authenticate(r *http.Request) (bool, error) {
	return true, nil
}

// TokenModelsAuthenticator validates requests using a static bearer token
type TokenModelsAuthenticator struct {
	Token string
	Send  bool
}

// NewTokenModels creates a new token-based authenticator compatible with models.Authenticator
func NewTokenModels(token string) *TokenModelsAuthenticator {
	return &TokenModelsAuthenticator{Token: token}
}

// NewTokenSenderModels creates an authenticator that adds the bearer token to
// outgoing requests, compatible with models.Authenticator
func NewTokenSenderModels(token string) *TokenModelsAuthenticator {
	return &TokenModelsAuthenticator{Token: token, Send: true}
}

// Authenticate checks the Authorization header for the expected token
func (a *TokenModelsAuthenticator) Authenticate(r *http.Request) (bool, error) {
	if a.Token == "" {
		return true, nil
	}
	if a.Send {
		r.Header.Set("Authorization", "Bearer "+a.Token)
		return true, nil
	}

	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false, errors.New("missing authorization header")
	}

	const prefix = "Bearer "
	if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
		return false, errors.New("invalid authorization header format")
	}

	token := auth[len(prefix):]
	if token != a.Token {
		return false, errors.New("invalid token")
	}

	return true, nil
}
