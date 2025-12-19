package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v2/shared"

	"manifold/internal/config"
	"manifold/internal/llm"
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

func (h *testStreamHandler) OnImage(llm.GeneratedImage) {
}

func TestChatImageGeneration(t *testing.T) {
	t.Parallel()
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aGVsbG8="}]}`))
	}))
	t.Cleanup(srv.Close)

	client := New(config.OpenAIConfig{
		APIKey:  "k",
		Model:   "gpt-image-1.5",
		BaseURL: srv.URL,
	}, srv.Client())

	ctx := llm.WithImagePrompt(context.Background(), llm.ImagePromptOptions{Size: "1K"})
	msg, err := client.Chat(ctx, []llm.Message{{Role: "user", Content: "draw a cat"}}, nil, "")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if gotPath != "/images/generations" {
		t.Fatalf("expected image generation path, got %q", gotPath)
	}
	if len(msg.Images) != 1 || string(msg.Images[0].Data) != "hello" {
		t.Fatalf("unexpected images: %+v", msg.Images)
	}
	if msg.Content == "" {
		t.Fatalf("expected content hint for image generation")
	}
	if prompt, ok := gotBody["prompt"].(string); !ok || !strings.Contains(prompt, "cat") {
		t.Fatalf("expected prompt forwarded, got %#v", gotBody["prompt"])
	}
}

func TestExtractReasoningEffort(t *testing.T) {
	t.Parallel()
	t.Run("extracts and strips string values", func(t *testing.T) {
		extra := map[string]any{
			"reasoning_effort": "medium",
			"other":            "keep",
		}
		val, ok := extractReasoningEffort(extra)
		if !ok {
			t.Fatal("expected reasoning effort to be extracted")
		}
		if val != shared.ReasoningEffort("medium") {
			t.Fatalf("unexpected effort value: %v", val)
		}
		if _, exists := extra["reasoning_effort"]; exists {
			t.Fatal("reasoning_effort should have been removed from extra params")
		}
		if extra["other"] != "keep" {
			t.Fatal("other fields should remain untouched")
		}
	})

	t.Run("removes invalid types without setting field", func(t *testing.T) {
		extra := map[string]any{"reasoning_effort": 123}
		if _, ok := extractReasoningEffort(extra); ok {
			t.Fatal("expected invalid type to be ignored")
		}
		if _, exists := extra["reasoning_effort"]; exists {
			t.Fatal("invalid reasoning_effort entries should still be removed")
		}
	})

	t.Run("ignores when not provided", func(t *testing.T) {
		extra := map[string]any{"foo": "bar"}
		if _, ok := extractReasoningEffort(extra); ok {
			t.Fatal("unexpected extraction when key is missing")
		}
	})
}
