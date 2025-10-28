package openai

import (
	"context"
	"manifold/internal/config"
	"manifold/internal/llm"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestSelfHostedSSEHeaderInjection verifies that streaming requests to self-hosted
// mlx_lm.server backends receive the Accept: text/event-stream header.
func TestSelfHostedSSEHeaderInjection(t *testing.T) {
	var completionsAcceptHeader string
	var requestMade bool

	// Create a test server that records the Accept header from streaming requests
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMade = true
		// Capture the Accept header specifically for /chat/completions endpoint
		if strings.Contains(r.URL.Path, "/chat/completions") {
			completionsAcceptHeader = r.Header.Get("Accept")
			t.Logf("Chat completions Accept header: %q", completionsAcceptHeader)
		}

		if strings.Contains(r.URL.Path, "/tokenize") {
			// Return a mock tokenize response
			_, _ = w.Write([]byte(`{"tokens": [1, 2, 3]}`))
			return
		}

		// Return a mock streaming response for chat completions
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"test"},"finish_reason":null}]}`))
		_, _ = w.Write([]byte("\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		_, _ = w.Write([]byte("\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	// Create a custom HTTP client with our wrapped transport
	httpClient := &http.Client{
		Transport: &http.Transport{},
	}

	t.Logf("Test server URL: %s", srv.URL)

	// Create client with self-hosted baseURL
	c := config.OpenAIConfig{
		APIKey:  "test",
		BaseURL: srv.URL, // This should trigger SSE header injection
		Model:   "test-model",
	}
	cli := New(c, httpClient)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Make a streaming request
	handler := &testStreamHandler{}
	err := cli.ChatStream(ctx, []llm.Message{{Role: "user", Content: "test"}}, nil, "", handler)
	if err != nil {
		t.Logf("Stream error (may be expected for mock server): %v", err)
	}

	if !requestMade {
		t.Fatal("No request was made to the test server")
	}

	// Verify Accept header was injected for chat completions
	if completionsAcceptHeader != "text/event-stream" {
		t.Errorf("Expected Accept: text/event-stream header on /chat/completions, got %q", completionsAcceptHeader)
	}
}

type testStreamHandler struct {
	deltas []string
}

func (h *testStreamHandler) OnDelta(content string) {
	h.deltas = append(h.deltas, content)
}

func (h *testStreamHandler) OnToolCall(tc llm.ToolCall) {
}
