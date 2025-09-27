package provider

import (
	"context"
	"fmt"
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

// MockProvider is a deterministic provider for tests.
type MockProvider struct {
	NameField    string
	Latency      time.Duration
	FailureAfter int
	calls        int
}

// NewMockProvider constructs a mock provider with defaults.
func NewMockProvider(name string) *MockProvider {
	return &MockProvider{NameField: name, Latency: 5 * time.Millisecond}
}

// Name returns the configured provider name.
func (m *MockProvider) Name() string {
	if m.NameField == "" {
		return "mock"
	}
	return m.NameField
}

// Complete returns a fabricated response; it errors after FailureAfter calls when set.
func (m *MockProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if m.FailureAfter > 0 && m.calls >= m.FailureAfter {
		return Response{}, fmt.Errorf("mock provider forced failure")
	}
	m.calls++
	select {
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case <-time.After(m.Latency):
	}
	output := fmt.Sprintf("model=%s name=%s prompt=%s", req.Model, m.Name(), req.Prompt)
	return Response{
		Output:       output,
		Tokens:       len(req.Prompt) / 4,
		Latency:      m.Latency,
		Cost:         float64(len(req.Prompt)) * 0.00001,
		Raw:          map[string]any{"inputs": req.Inputs},
		ProviderName: m.Name(),
	}, nil
}
