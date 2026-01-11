package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"manifold/internal/config"
	"manifold/internal/llm"
)

type streamRecorder struct {
	deltas     []string
	calls      []llm.ToolCall
	summaries  []string
	signatures []string
}

func (s *streamRecorder) OnDelta(content string)     { s.deltas = append(s.deltas, content) }
func (s *streamRecorder) OnToolCall(tc llm.ToolCall) { s.calls = append(s.calls, tc) }
func (s *streamRecorder) OnImage(llm.GeneratedImage) {}
func (s *streamRecorder) OnThoughtSummary(summary string) {
	s.summaries = append(s.summaries, summary)
}
func (s *streamRecorder) OnThoughtSignature(sig string) {
	s.signatures = append(s.signatures, sig)
}

func TestChatReturnsText(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		defer r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		resp := sdk.Message{
			ID:           "msg_1",
			Type:         constant.Message("message"),
			Role:         constant.Assistant("assistant"),
			Model:        sdk.ModelClaude3_7SonnetLatest,
			StopReason:   sdk.StopReasonEndTurn,
			StopSequence: "",
			Content: []sdk.ContentBlockUnion{
				{Type: "text", Text: "hello"},
			},
			Usage: minimalUsage(),
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	msg, err := client.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, "")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if msg.Content != "hello" {
		t.Fatalf("unexpected content %q", msg.Content)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("unexpected path %q", gotPath)
	}
}

func TestChatToolCall(t *testing.T) {
	var reqBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")
		resp := sdk.Message{
			ID:           "msg_2",
			Type:         constant.Message("message"),
			Role:         constant.Assistant("assistant"),
			Model:        sdk.ModelClaude3_7SonnetLatest,
			StopReason:   sdk.StopReasonToolUse,
			StopSequence: "",
			Content: []sdk.ContentBlockUnion{
				{Type: "tool_use", Name: "lookup", ID: "", Input: json.RawMessage(`{"x":2}`)},
			},
			Usage: minimalUsage(),
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{APIKey: "k", BaseURL: srv.URL}, srv.Client())
	msg, err := client.Chat(context.Background(), []llm.Message{{Role: "user", Content: "go"}}, []llm.ToolSchema{
		{Name: "lookup", Description: "desc", Parameters: map[string]any{"type": "object"}},
	}, "")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "lookup" {
		t.Fatalf("expected tool call, got %+v", msg.ToolCalls)
	}
	if msg.ToolCalls[0].ID == "" {
		t.Fatalf("expected generated tool call id")
	}
	tools, ok := reqBody["tools"]
	if !ok || tools == nil {
		t.Fatalf("expected tools to be sent in request, got %#v", reqBody)
	}
}

func TestChatPromptCacheAddsCacheControlToSystemAndTools(t *testing.T) {
	var reqBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")
		resp := sdk.Message{
			ID:           "msg_cache",
			Type:         constant.Message("message"),
			Role:         constant.Assistant("assistant"),
			Model:        sdk.ModelClaude3_7SonnetLatest,
			StopReason:   sdk.StopReasonEndTurn,
			StopSequence: "",
			Content:      []sdk.ContentBlockUnion{{Type: "text", Text: "ok"}},
			Usage:        minimalUsage(),
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	cfg := config.AnthropicConfig{
		APIKey:  "k",
		BaseURL: srv.URL,
		PromptCache: config.AnthropicPromptCacheConfig{
			Enabled: true,
			// Intentionally leave CacheSystem/CacheTools unset to verify defaults.
		},
	}
	client := New(cfg, srv.Client())
	_, err := client.Chat(
		context.Background(),
		[]llm.Message{{Role: "system", Content: "static system"}, {Role: "user", Content: "hi"}},
		[]llm.ToolSchema{{Name: "lookup", Parameters: map[string]any{"type": "object"}}},
		"",
	)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	sysAny, ok := reqBody["system"]
	if !ok {
		t.Fatalf("expected system in request, got %#v", reqBody)
	}
	sysList, ok := sysAny.([]any)
	if !ok || len(sysList) == 0 {
		t.Fatalf("expected system blocks array, got %#v", sysAny)
	}
	sys0, ok := sysList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected system block object, got %#v", sysList[0])
	}
	if _, ok := sys0["cache_control"]; !ok {
		t.Fatalf("expected system cache_control, got %#v", sys0)
	}

	toolsAny, ok := reqBody["tools"]
	if !ok {
		t.Fatalf("expected tools in request, got %#v", reqBody)
	}
	toolsList, ok := toolsAny.([]any)
	if !ok || len(toolsList) == 0 {
		t.Fatalf("expected tools array, got %#v", toolsAny)
	}
	tool0, ok := toolsList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool object, got %#v", toolsList[0])
	}
	if _, ok := tool0["cache_control"]; !ok {
		t.Fatalf("expected tool cache_control, got %#v", tool0)
	}
}

func TestChatStreamText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		writeEvent(w, flusher, "message_start", map[string]any{
			"message": minimalMessage(),
		})
		writeEvent(w, flusher, "content_block_start", map[string]any{
			"index":         0,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
		writeEvent(w, flusher, "content_block_delta", map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": "hello"},
		})
		writeEvent(w, flusher, "content_block_delta", map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": " world"},
		})
		writeEvent(w, flusher, "message_delta", map[string]any{
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": ""},
			"usage": minimalDeltaUsage(),
		})
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{APIKey: "k", Model: "claude-3-7-sonnet-latest", BaseURL: srv.URL}, srv.Client())
	rec := &streamRecorder{}
	if err := client.ChatStream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, "", rec); err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}
	got := strings.Join(rec.deltas, "")
	if got != "hello world" {
		t.Fatalf("unexpected delta content %q", got)
	}
}

func TestChatStreamThoughtSummaries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		writeEvent(w, flusher, "message_start", map[string]any{"message": minimalMessage()})
		writeEvent(w, flusher, "content_block_start", map[string]any{
			"index":         0,
			"content_block": map[string]any{"type": "thinking", "thinking": ""},
		})
		writeEvent(w, flusher, "content_block_delta", map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "thinking_delta", "thinking": "first"},
		})
		writeEvent(w, flusher, "content_block_delta", map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "thinking_delta", "thinking": " second"},
		})
		writeEvent(w, flusher, "message_delta", map[string]any{
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": ""},
			"usage": minimalDeltaUsage(),
		})
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{APIKey: "k", Model: "claude-3-7-sonnet-latest", BaseURL: srv.URL}, srv.Client())
	rec := &streamRecorder{}
	if err := client.ChatStream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, "", rec); err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}
	if len(rec.summaries) < 2 {
		t.Fatalf("expected at least 2 thought summary updates, got %d: %#v", len(rec.summaries), rec.summaries)
	}
	if rec.summaries[len(rec.summaries)-1] != "first second" {
		t.Fatalf("unexpected final thought summary: %q", rec.summaries[len(rec.summaries)-1])
	}
}

func TestChatStreamEmitsToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		writeEvent(w, flusher, "message_start", map[string]any{"message": minimalMessage()})
		writeEvent(w, flusher, "content_block_start", map[string]any{
			"index": 0,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    "tool-1",
				"name":  "lookup",
				"input": map[string]any{},
			},
		})
		writeEvent(w, flusher, "content_block_delta", map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"x":3}`},
		})
		writeEvent(w, flusher, "message_delta", map[string]any{
			"delta": map[string]any{"stop_reason": "tool_use", "stop_sequence": ""},
			"usage": minimalDeltaUsage(),
		})
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{APIKey: "k", BaseURL: srv.URL}, srv.Client())
	rec := &streamRecorder{}
	err := client.ChatStream(context.Background(), []llm.Message{{Role: "user", Content: "go"}}, []llm.ToolSchema{
		{Name: "lookup", Parameters: map[string]any{"type": "object"}},
	}, "", rec)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected one tool call, got %+v", rec.calls)
	}
	if rec.calls[0].Name != "lookup" || rec.calls[0].ID != "tool-1" {
		t.Fatalf("unexpected tool call %+v", rec.calls[0])
	}
	if string(rec.calls[0].Args) != `{"x":3}` {
		t.Fatalf("unexpected args %s", string(rec.calls[0].Args))
	}
}

func minimalUsage() sdk.Usage {
	return sdk.Usage{
		CacheCreation: sdk.CacheCreation{
			Ephemeral1hInputTokens: 0,
			Ephemeral5mInputTokens: 0,
		},
		CacheCreationInputTokens: 0,
		CacheReadInputTokens:     0,
		InputTokens:              0,
		OutputTokens:             0,
		ServerToolUse:            sdk.ServerToolUsage{WebSearchRequests: 0},
		ServiceTier:              sdk.UsageServiceTierStandard,
	}
}

func writeEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, payload map[string]any) {
	if _, ok := payload["type"]; !ok {
		payload["type"] = eventType
	}
	b, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", b)
	if flusher != nil {
		flusher.Flush()
	}
}

func minimalMessage() sdk.Message {
	return sdk.Message{
		ID:           "msg",
		Type:         constant.Message("message"),
		Role:         constant.Assistant("assistant"),
		Model:        sdk.ModelClaude3_7SonnetLatest,
		StopReason:   sdk.StopReasonEndTurn,
		StopSequence: "",
		Content:      []sdk.ContentBlockUnion{},
		Usage:        minimalUsage(),
	}
}

func minimalDeltaUsage() map[string]any {
	return map[string]any{
		"cache_creation_input_tokens": 0,
		"cache_read_input_tokens":     0,
		"input_tokens":                0,
		"output_tokens":               0,
		"server_tool_use":             map[string]any{"web_search_requests": 0},
	}
}

func TestThinkingBlockPreservation(t *testing.T) {
	// Test that thinking blocks are captured from responses and included in subsequent requests
	var reqBodies []map[string]any
	reqCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		reqBodies = append(reqBodies, body)
		reqCount++

		w.Header().Set("Content-Type", "application/json")
		resp := sdk.Message{
			ID:           fmt.Sprintf("msg_%d", reqCount),
			Type:         constant.Message("message"),
			Role:         constant.Assistant("assistant"),
			Model:        sdk.ModelClaude3_7SonnetLatest,
			StopReason:   sdk.StopReasonEndTurn,
			StopSequence: "",
			Content: []sdk.ContentBlockUnion{
				{Type: "thinking", Thinking: "Let me think about this...", Signature: "sig_abc123"},
				{Type: "text", Text: "Here's my response"},
			},
			Usage: minimalUsage(),
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)

	client := New(config.AnthropicConfig{APIKey: "k", Model: "claude-sonnet-4-5-latest", BaseURL: srv.URL}, srv.Client())

	// First turn - just user message
	msg1, err := client.Chat(context.Background(), []llm.Message{
		{Role: "user", Content: "First question"},
	}, nil, "")
	if err != nil {
		t.Fatalf("First Chat returned error: %v", err)
	}

	// Verify thinking was captured in ThoughtSignature
	if msg1.ThoughtSignature == "" {
		t.Fatal("expected ThoughtSignature to be set after first response")
	}

	// Parse the thought signature to verify structure
	var thinking []thinkingData
	if err := json.Unmarshal([]byte(msg1.ThoughtSignature), &thinking); err != nil {
		t.Fatalf("failed to parse ThoughtSignature: %v", err)
	}
	if len(thinking) != 1 || thinking[0].Signature != "sig_abc123" {
		t.Fatalf("unexpected thinking data: %+v", thinking)
	}

	// Second turn - include assistant response with thinking
	_, err = client.Chat(context.Background(), []llm.Message{
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "Here's my response", ThoughtSignature: msg1.ThoughtSignature},
		{Role: "user", Content: "Follow up question"},
	}, nil, "")
	if err != nil {
		t.Fatalf("Second Chat returned error: %v", err)
	}

	// Verify the second request included thinking blocks
	if len(reqBodies) < 2 {
		t.Fatal("expected at least 2 requests")
	}

	messages, ok := reqBodies[1]["messages"].([]any)
	if !ok {
		t.Fatalf("expected messages array in request, got %#v", reqBodies[1])
	}

	// Find the assistant message and check for thinking block
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if msg["role"] == "assistant" {
			content, ok := msg["content"].([]any)
			if !ok {
				t.Fatalf("expected content array in assistant message, got %#v", msg["content"])
			}
			// Check first block is thinking
			if len(content) > 0 {
				firstBlock, ok := content[0].(map[string]any)
				if !ok {
					t.Fatalf("expected content block object, got %#v", content[0])
				}
				if firstBlock["type"] != "thinking" {
					t.Fatalf("expected first block to be thinking, got %q", firstBlock["type"])
				}
				if firstBlock["signature"] != "sig_abc123" {
					t.Fatalf("expected signature sig_abc123, got %q", firstBlock["signature"])
				}
			}
			break
		}
	}
}

func TestAdaptMessagesWithThinking(t *testing.T) {
	// Test that adaptMessages correctly includes thinking blocks from ThoughtSignature
	thinking := []thinkingData{
		{Signature: "sig_test", Thinking: "My reasoning"},
	}
	thinkingJSON, _ := json.Marshal(thinking)

	msgs := []llm.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Response", ThoughtSignature: string(thinkingJSON)},
		{Role: "user", Content: "Follow up"},
	}

	_, converted, err := adaptMessages(msgs, config.AnthropicPromptCacheConfig{})
	if err != nil {
		t.Fatalf("adaptMessages error: %v", err)
	}

	if len(converted) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(converted))
	}

	// Check assistant message has thinking block first
	assistantMsg := converted[1]
	if assistantMsg.Role != "assistant" {
		t.Fatalf("expected assistant role, got %s", assistantMsg.Role)
	}

	// The content should have thinking block before text
	if len(assistantMsg.Content) < 2 {
		t.Fatalf("expected at least 2 content blocks, got %d", len(assistantMsg.Content))
	}

	// Marshal to check structure
	contentJSON, _ := json.Marshal(assistantMsg.Content)
	contentStr := string(contentJSON)
	if !strings.Contains(contentStr, "thinking") {
		t.Fatalf("expected thinking block in content, got %s", contentStr)
	}
	if !strings.Contains(contentStr, "sig_test") {
		t.Fatalf("expected signature in content, got %s", contentStr)
	}
}
