package google

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"manifold/internal/config"
	"manifold/internal/llm"
)

type streamRecorder struct {
	deltas []string
	calls  []llm.ToolCall
}

func (s *streamRecorder) OnDelta(content string) { s.deltas = append(s.deltas, content) }
func (s *streamRecorder) OnToolCall(tc llm.ToolCall) {
	s.calls = append(s.calls, tc)
}

func TestChatSuccess(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		defer r.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]}}]}`))
	}))
	t.Cleanup(srv.Close)

	cfg := config.GoogleConfig{
		APIKey:  "k",
		Model:   "test-model",
		BaseURL: srv.URL,
	}
	client, err := New(cfg, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	msg, err := client.Chat(context.Background(), []llm.Message{
		{Role: "system", Content: "do"},
		{Role: "user", Content: "hi"},
	}, nil, "")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if msg.Content != "hello" {
		t.Fatalf("expected hello, got %q", msg.Content)
	}
	if gotPath != "/v1beta/models/test-model:generateContent" {
		t.Fatalf("unexpected path %q", gotPath)
	}
}

func TestChatStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]}}]}`,
			`{"candidates":[{"content":{"role":"model","parts":[{"text":" world"}]}}]}`,
		}
		for _, c := range chunks {
			_, _ = w.Write([]byte("data: " + c + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)

	cfg := config.GoogleConfig{
		APIKey:  "k",
		Model:   "test-model",
		BaseURL: srv.URL,
	}
	client, err := New(cfg, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	rec := &streamRecorder{}
	err = client.ChatStream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, "", rec)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}
	got := strings.Join(rec.deltas, "")
	if got != "hello world" {
		t.Fatalf("unexpected deltas %q", got)
	}
}

func TestToolsNotSupported(t *testing.T) {
	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m"}, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	_, err = client.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, []llm.ToolSchema{{Name: "x"}}, "")
	if !errors.Is(err, ErrToolsNotSupported) {
		t.Fatalf("expected tools error, got %v", err)
	}
	err = client.ChatStream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, []llm.ToolSchema{{Name: "x"}}, "", &streamRecorder{})
	if !errors.Is(err, ErrToolsNotSupported) {
		t.Fatalf("expected tools error, got %v", err)
	}
}
