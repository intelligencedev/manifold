package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"manifold/internal/config"
	"manifold/internal/llm"
)

func TestResponsesTokenizer_BuildInputItems_AssistantToolCallWithoutContent(t *testing.T) {
	tokenizer := &ResponsesTokenizer{}
	params := tokenizer.countParams([]llm.Message{
		{
			Role:      "assistant",
			Content:   "",
			ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "run", Args: json.RawMessage(`{"cmd":"ls"}`)}},
		},
		{Role: "tool", Content: `{"ok":true}`, ToolID: "call_1"},
	})

	for _, item := range params.Input.OfResponseInputItemArray {
		itemType := item.GetType()
		role := item.GetRole()
		if itemType != nil && role != nil && *itemType == "message" && *role == "assistant" {
			t.Fatalf("unexpected assistant message without content in input_tokens payload")
		}
	}
}

func TestResponsesTokenizer_CountMessagesTokensCachesUnsupportedEndpoint(t *testing.T) {
	t.Parallel()

	var inputTokensCount atomic.Int32
	var tokenizeCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses/input_tokens":
			inputTokensCount.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"message":"File Not Found","type":"not_found_error","code":404}}`)
		case "/apply-template":
			t.Fatal("local tokenizer should not call /apply-template")
		case "/tokenize":
			tokenizeCount.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"message":"File Not Found","type":"not_found_error","code":404}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(config.OpenAIConfig{APIKey: "test", BaseURL: server.URL + "/v1", Model: "model", API: "responses"}, server.Client())
	tokenizer := NewResponsesTokenizer(client, "model", nil, 0)
	msgs := []llm.Message{{Role: "user", Content: "hello world"}}

	count1, err := tokenizer.CountMessagesTokens(context.Background(), msgs)
	if err != nil {
		t.Fatalf("first CountMessagesTokens error: %v", err)
	}
	if count1 != llm.EstimateTokensForMessages(msgs) {
		t.Fatalf("expected heuristic count %d, got %d", llm.EstimateTokensForMessages(msgs), count1)
	}

	count2, err := tokenizer.CountMessagesTokens(context.Background(), msgs)
	if err != nil {
		t.Fatalf("second CountMessagesTokens error: %v", err)
	}
	if count2 != llm.EstimateTokensForMessages(msgs) {
		t.Fatalf("expected heuristic count %d on second call, got %d", llm.EstimateTokensForMessages(msgs), count2)
	}
	if got := inputTokensCount.Load(); got != 1 {
		t.Fatalf("expected one input_tokens HTTP attempt, got %d", got)
	}
	if got := tokenizeCount.Load(); got == 0 {
		t.Fatal("expected local tokenize fallback to be attempted")
	}
}

func TestLocalTokenizerCountMessagesUsesTokenizeOnly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		switch r.URL.Path {
		case "/apply-template":
			t.Fatal("local tokenizer should not call /apply-template")
		case "/tokenize":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode tokenize: %v", err)
			}
			content := body["content"].(string)
			if !strings.Contains(content, "user") || !strings.Contains(content, "hello") {
				t.Fatalf("expected flattened prompt to be tokenized, got %#v", content)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"tokens":[1,2,3,4]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL + "/v1", httpClient: server.Client(), model: "model", api: "completions"}
	tokenizer := NewLocalTokenizer(client, "model", nil)
	count, err := tokenizer.CountMessagesTokens(context.Background(), []llm.Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("CountMessagesTokens returned error: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 tokens, got %d", count)
	}
}

func TestResponsesTokenizerFallsBackToLocalTokenizerOnInputTokens404(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses/input_tokens":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"message":"File Not Found","type":"not_found_error","code":404}}`)
		case "/apply-template":
			t.Fatal("responses local tokenizer fallback should not call /apply-template")
		case "/tokenize":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"tokens":[1,2,3,4,5]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(config.OpenAIConfig{APIKey: "test", BaseURL: server.URL + "/v1", Model: "model", API: "responses"}, server.Client())
	tokenizer := NewResponsesTokenizer(client, "model", nil, 0)
	count, err := tokenizer.CountMessagesTokens(context.Background(), []llm.Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("CountMessagesTokens returned error: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected local tokenizer count 5, got %d", count)
	}
}
