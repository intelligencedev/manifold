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

func TestCompactResponses(t *testing.T) {
	var gotModel string
	var gotInput []any
	var gotAssistantID string
	var gotToolCallID string

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses/compact" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if v, ok := payload["model"].(string); ok {
			gotModel = v
		}
		if v, ok := payload["input"].([]any); ok {
			gotInput = v
			for _, item := range v {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				typ, _ := obj["type"].(string)
				role, _ := obj["role"].(string)
				if gotAssistantID == "" && (typ == "message" || role == "assistant") {
					if id, ok := obj["id"].(string); ok && id != "" {
						gotAssistantID = id
					}
				}
				if gotToolCallID == "" && (typ == "function_call_output" || obj["call_id"] != nil) {
					if callID, ok := obj["call_id"].(string); ok && callID != "" {
						gotToolCallID = callID
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmp_1","object":"response.compaction","created_at":1,"output":[{"type":"compaction","id":"c1","encrypted_content":"enc"}]}`))
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	c := config.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "m", API: "responses"}
	cli := New(c, srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	item, err := cli.Compact(ctx, []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "run", Args: json.RawMessage(`{"cmd":"ls"}`)}}},
		{Role: "tool", Content: "result", ToolID: "call_1"},
	}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.EncryptedContent != "enc" {
		t.Fatalf("expected encrypted content, got %q", item.EncryptedContent)
	}
	if gotModel != "m" {
		t.Fatalf("expected model m, got %q", gotModel)
	}
	if len(gotInput) != 4 {
		t.Fatalf("expected 4 input items, got %d", len(gotInput))
	}
	first, ok := gotInput[0].(map[string]any)
	if !ok {
		t.Fatalf("expected input object, got %#v", gotInput[0])
	}
	if first["role"] != "user" {
		t.Fatalf("expected user role, got %#v", first["role"])
	}
	if gotAssistantID == "" {
		t.Fatalf("expected assistant id in compaction input")
	}
	if !strings.HasPrefix(gotAssistantID, "msg_") {
		t.Fatalf("expected assistant id to start with msg_, got %q", gotAssistantID)
	}
	if gotToolCallID == "" {
		t.Fatalf("expected tool call id in compaction input")
	}
	if !strings.HasPrefix(gotToolCallID, "call_") {
		t.Fatalf("expected tool call id to start with call_, got %q", gotToolCallID)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "a", "b") != "a" {
		t.Fatalf("unexpected firstNonEmpty")
	}
}

func TestExtractReasoningSummary_TopLevelSummaryAliasRemoved(t *testing.T) {
	extra := map[string]any{"summary": "auto", "temperature": 0.2}
	got, ok := extractReasoningSummary(extra)
	if !ok {
		t.Fatalf("expected ok")
	}
	if got != shared.ReasoningSummary("auto") {
		t.Fatalf("expected auto, got %q", got)
	}
	if _, exists := extra["summary"]; exists {
		t.Fatalf("expected summary to be removed from extra")
	}
	if _, exists := extra["temperature"]; !exists {
		t.Fatalf("expected unrelated extra keys to remain")
	}
}

func TestAdaptResponsesInputFiltersOrphanToolOutputs(t *testing.T) {
	input, _ := adaptResponsesInput([]llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "fetch", Args: []byte(`{"url":"https://example.com"}`)}}},
		{Role: "tool", ToolID: "call_1", Content: `{"ok":true}`},
		{Role: "tool", ToolID: "call_orphan", Content: `{"ok":false}`},
	})

	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "call_1") {
		t.Fatalf("expected input to include call_1, got: %s", s)
	}
	if strings.Contains(s, "call_orphan") {
		t.Fatalf("expected input to omit orphan tool output, got: %s", s)
	}
}

func TestBuildCompactionInputFiltersMissingToolOutputs(t *testing.T) {
	items, _ := buildCompactionInput([]llm.Message{
		{Role: "assistant", Content: "hi", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "run", Args: []byte(`{"cmd":"ls"}`)}}},
	}, nil)

	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal compaction input: %v", err)
	}
	if strings.Contains(string(raw), "call_1") {
		t.Fatalf("expected compaction input to omit call_1 without output, got: %s", string(raw))
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

func (h *testStreamHandler) OnThoughtSummary(string) {
}

func (h *testStreamHandler) OnThoughtSignature(string) {
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
