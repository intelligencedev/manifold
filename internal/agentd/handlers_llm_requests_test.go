package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
)

func TestLLMRequestContextHandlerReturnsPayloadObject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-1", "Session"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	requestStore := databases.NewMemoryLLMRequestStore(chatStore)
	if err := requestStore.AppendLLMRequest(ctx, persistence.LLMRequest{
		ID:        "req-1",
		SessionID: "sess-1",
		MessageID: "assistant-1",
		Provider:  "openai",
		Model:     "gpt-test",
		Payload: json.RawMessage(
			`{"messages":[{"role":"user","content":"hello"}],"authorization":"Bearer secret"}`,
		),
		Redacted:  true,
		CreatedAt: time.Date(2026, 7, 6, 21, 30, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendLLMRequest: %v", err)
	}
	app := &app{
		cfg:             &config.Config{},
		chatStore:       chatStore,
		llmRequestStore: requestStore,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/llm-requests/req-1/context", nil)
	app.llmRequestContextHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Authorization string `json:"authorization"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.ID != "req-1" || response.Model != "gpt-test" {
		t.Fatalf("expected request metadata, got %#v", response)
	}
	if len(response.Payload.Messages) != 1 || response.Payload.Messages[0].Content != "hello" {
		t.Fatalf("expected payload messages, got %#v", response.Payload.Messages)
	}
	if response.Payload.Authorization != "[REDACTED]" {
		t.Fatalf("expected authorization redacted, got %q", response.Payload.Authorization)
	}
}
