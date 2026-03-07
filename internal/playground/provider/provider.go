package provider

import (
	"context"
	"time"
)

// Request captures the information sent to a provider when executing a prompt.
type Request struct {
	Model    string
	Prompt   string
	Inputs   map[string]any
	Params   map[string]any
	Metadata map[string]string
}

// Response wraps the LLM output and metrics returned by the provider.
type Response struct {
	Output       string
	Tokens       int
	Latency      time.Duration
	Cost         float64
	Raw          map[string]any
	ProviderName string
}

// Provider abstracts prompt execution against an LLM.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
}
