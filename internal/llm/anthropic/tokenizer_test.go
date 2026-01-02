package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
	"manifold/internal/llm"
)

func TestMessagesTokenizer_CountTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/count_tokens" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		defer r.Body.Close()

		// Verify required fields
		if _, ok := reqBody["model"]; !ok {
			t.Error("request missing model field")
		}
		if _, ok := reqBody["messages"]; !ok {
			t.Error("request missing messages field")
		}

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"input_tokens": 42,
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{
		APIKey:  "test-key",
		Model:   "claude-3-sonnet",
		BaseURL: srv.URL,
	}, srv.Client())

	tokenizer := client.Tokenizer(nil)
	if tokenizer == nil {
		t.Fatal("expected non-nil tokenizer")
	}

	count, err := tokenizer.CountTokens(context.Background(), "Hello, world!")
	if err != nil {
		t.Fatalf("CountTokens returned error: %v", err)
	}
	if count != 42 {
		t.Errorf("expected 42 tokens, got %d", count)
	}
}

func TestMessagesTokenizer_CountMessagesTokens(t *testing.T) {
	var gotMessages []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		defer r.Body.Close()

		gotMessages, _ = reqBody["messages"].([]any)

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"input_tokens": 150,
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{
		APIKey:  "test-key",
		Model:   "claude-3-sonnet",
		BaseURL: srv.URL,
	}, srv.Client())

	tokenizer := client.Tokenizer(nil)

	msgs := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "What is Go?"},
		{Role: "assistant", Content: "Go is a programming language."},
		{Role: "user", Content: "Tell me more."},
	}

	count, err := tokenizer.CountMessagesTokens(context.Background(), msgs)
	if err != nil {
		t.Fatalf("CountMessagesTokens returned error: %v", err)
	}
	if count != 150 {
		t.Errorf("expected 150 tokens, got %d", count)
	}

	// System messages should NOT be in messages array (Anthropic handles them separately)
	// We expect 3 messages (user, assistant, user) - not the system message
	if len(gotMessages) != 3 {
		t.Errorf("expected 3 messages (excluding system), got %d", len(gotMessages))
	}
}

func TestMessagesTokenizer_CountMessagesTokensWithToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"input_tokens": 200,
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{
		APIKey:  "test-key",
		Model:   "claude-3-sonnet",
		BaseURL: srv.URL,
	}, srv.Client())

	tokenizer := client.Tokenizer(nil)

	msgs := []llm.Message{
		{Role: "user", Content: "What's the weather?"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []llm.ToolCall{
				{ID: "call-1", Name: "get_weather", Args: json.RawMessage(`{"city":"NYC"}`)},
			},
		},
		{Role: "tool", ToolID: "call-1", Content: `{"temp": 72}`},
	}

	count, err := tokenizer.CountMessagesTokens(context.Background(), msgs)
	if err != nil {
		t.Fatalf("CountMessagesTokens returned error: %v", err)
	}
	if count != 200 {
		t.Errorf("expected 200 tokens, got %d", count)
	}
}

func TestMessagesTokenizer_EmptyInput(t *testing.T) {
	client := New(config.AnthropicConfig{
		APIKey: "test-key",
		Model:  "claude-3-sonnet",
	}, nil)

	tokenizer := client.Tokenizer(nil)

	// Empty string
	count, err := tokenizer.CountTokens(context.Background(), "")
	if err != nil {
		t.Fatalf("CountTokens returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tokens for empty string, got %d", count)
	}

	// Empty messages
	count, err = tokenizer.CountMessagesTokens(context.Background(), nil)
	if err != nil {
		t.Fatalf("CountMessagesTokens returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tokens for empty messages, got %d", count)
	}
}

func TestMessagesTokenizer_WithCache(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"input_tokens": 25,
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{
		APIKey:  "test-key",
		Model:   "claude-3-sonnet",
		BaseURL: srv.URL,
	}, srv.Client())

	cache := llm.NewTokenCache(llm.TokenCacheConfig{MaxSize: 100})
	tokenizer := client.Tokenizer(cache)

	ctx := context.Background()
	text := "This is a test message"

	// First call - should hit the API
	count1, err := tokenizer.CountTokens(ctx, text)
	if err != nil {
		t.Fatalf("first CountTokens returned error: %v", err)
	}
	if count1 != 25 {
		t.Errorf("expected 25 tokens, got %d", count1)
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}

	// Second call with same text - should use cache
	count2, err := tokenizer.CountTokens(ctx, text)
	if err != nil {
		t.Fatalf("second CountTokens returned error: %v", err)
	}
	if count2 != 25 {
		t.Errorf("expected 25 tokens from cache, got %d", count2)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 API call (cache hit), got %d", callCount)
	}
}

func TestClient_SupportsTokenization(t *testing.T) {
	client := New(config.AnthropicConfig{
		APIKey: "test-key",
		Model:  "claude-3-sonnet",
	}, nil)

	if !client.SupportsTokenization() {
		t.Error("expected Anthropic client to support tokenization")
	}
}
