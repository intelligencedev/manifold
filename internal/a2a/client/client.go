package client

import (
	"net/http"
)

type Authenticator interface {
	Authenticate(r *http.Request) error
}

type A2AClient struct {
	baseURL string
	http    *http.Client
	auth    Authenticator
}

func New(baseURL string, client *http.Client, auth Authenticator) *A2AClient {
	return &A2AClient{
		baseURL: baseURL,
		http:    client,
		auth:    auth,
	}
}
