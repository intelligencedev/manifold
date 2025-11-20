package google

import (
	"context"
	"errors"
	"testing"

	"manifold/internal/config"
)

func TestClientStubs(t *testing.T) {
	c := New(config.GoogleConfig{APIKey: "k"}, nil)
	if c.httpClient == nil {
		t.Fatalf("expected default http client to be set")
	}
	if _, err := c.Chat(context.Background(), nil, nil, ""); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("chat should be stubbed: %v", err)
	}
	if err := c.ChatStream(context.Background(), nil, nil, "", nil); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("chat stream should be stubbed: %v", err)
	}
}
