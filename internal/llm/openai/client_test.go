package openai

import (
	"context"
	"manifold/internal/config"
	"manifold/internal/llm"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChatWithOptions_ServerReturnsChoice(t *testing.T) {
	// Start a test server that mimics minimal OpenAI Chat Completion response.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Minimal response: one choice with a message containing content
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello","tool_calls":[]}}]}`))
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	c := config.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "m"}
	cli := New(c, srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg, err := cli.ChatWithOptions(ctx, []llm.Message{{Role: "user", Content: "hi"}}, nil, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "hello" {
		t.Fatalf("expected hello, got %q", msg.Content)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "a", "b") != "a" {
		t.Fatalf("unexpected firstNonEmpty")
	}
}
