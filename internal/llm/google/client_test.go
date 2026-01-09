package google

import (
	"context"
	"encoding/json"
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
	images []llm.GeneratedImage
}

func (s *streamRecorder) OnDelta(content string) { s.deltas = append(s.deltas, content) }
func (s *streamRecorder) OnToolCall(tc llm.ToolCall) {
	s.calls = append(s.calls, tc)
}
func (s *streamRecorder) OnImage(img llm.GeneratedImage) {
	s.images = append(s.images, img)
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

func TestChatFunctionCalling(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"doThing","id":"call-1","args":{"x":1}}}]}}]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	msg, err := client.Chat(context.Background(), []llm.Message{
		{Role: "user", Content: "hi"},
	}, []llm.ToolSchema{
		{Name: "doThing", Description: "test fn", Parameters: map[string]any{"type": "object"}},
	}, "")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "doThing" {
		t.Fatalf("expected tool call, got %+v", msg.ToolCalls)
	}
	if len(body) == 0 {
		t.Fatalf("expected request body to be captured")
	}
	cfg, ok := body["toolConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected toolConfig in request")
	}
	fcfg, ok := cfg["functionCallingConfig"].(map[string]any)
	if !ok || fcfg["mode"] != "AUTO" {
		t.Fatalf("expected functionCallingConfig with mode AUTO, got %#v", cfg)
	}
	// In AUTO mode, allowedFunctionNames should not be set per Google API requirements
	if names, ok := fcfg["allowedFunctionNames"]; ok && names != nil {
		t.Fatalf("expected allowedFunctionNames to be unset in AUTO mode, got %#v", names)
	}
}

func TestToolResponseIsForwarded(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.Chat(context.Background(), []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "lookup", ID: "c1", Args: json.RawMessage(`{"foo":"bar"}`), ThoughtSignature: "c2ln"}}},
		{Role: "tool", ToolID: "c1", Content: `{"result":"ok"}`},
	}, []llm.ToolSchema{{Name: "lookup", Parameters: map[string]any{"type": "object"}}}, "")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	contents, ok := body["contents"].([]any)
	if !ok || len(contents) < 3 {
		t.Fatalf("expected at least 3 contents, got %#v", body["contents"])
	}
	toolMsg, ok := contents[2].(map[string]any)
	if !ok {
		t.Fatalf("expected map for tool content, got %#v", contents[2])
	}
	parts, ok := toolMsg["parts"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("expected parts for tool content, got %#v", toolMsg["parts"])
	}
	fr, ok := parts[0].(map[string]any)["functionResponse"].(map[string]any)
	if !ok {
		t.Fatalf("expected functionResponse part, got %#v", parts[0])
	}
	if fr["name"] != "lookup" {
		t.Fatalf("expected functionResponse name lookup, got %v", fr["name"])
	}
	if fr["id"] != "c1" {
		t.Fatalf("expected functionResponse id c1, got %v", fr["id"])
	}
	// ensure thought signature round-tripped on assistant functionCall part
	assistant, ok := contents[1].(map[string]any)
	if !ok {
		t.Fatalf("expected assistant content map, got %#v", contents[1])
	}
	parts2, ok := assistant["parts"].([]any)
	if !ok || len(parts2) == 0 {
		t.Fatalf("expected parts slice in assistant content")
	}
	thoughtSig := parts2[0].(map[string]any)["thoughtSignature"]
	if thoughtSig == nil {
		t.Fatalf("expected thoughtSignature on functionCall part")
	}
	// tool response should carry the same signature on its part for safety
	if sigBytes, ok := parts[0].(map[string]any)["thoughtSignature"]; !ok || sigBytes == nil {
		t.Fatalf("expected thoughtSignature on tool response part")
	}
}

func TestStreamEmitsToolCalls(t *testing.T) {
	// ChatStream now uses streaming even when tools are provided.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","id":"c1","args":{"x":2}},"thoughtSignature":"YWJj"}]}}]}` + "\n\n"))
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	rec := &streamRecorder{}
	err = client.ChatStream(context.Background(), []llm.Message{{Role: "user", Content: "start"}}, []llm.ToolSchema{
		{Name: "lookup", Parameters: map[string]any{"type": "object"}},
	}, "", rec)
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}
	if len(rec.calls) != 1 || rec.calls[0].Name != "lookup" {
		t.Fatalf("expected tool call lookup, got %+v", rec.calls)
	}
	if rec.calls[0].ThoughtSignature != "YWJj" {
		t.Fatalf("expected thought signature propagated, got %q", rec.calls[0].ThoughtSignature)
	}
	if rec.calls[0].ID == "" {
		t.Fatalf("expected generated id on tool call")
	}
}

func TestFunctionCallIDGeneratedWhenMissing(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"x":2}},"thoughtSignature":"c2ln"}]}}]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	msg, err := client.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, []llm.ToolSchema{{Name: "lookup", Parameters: map[string]any{"type": "object"}}}, "")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID == "" {
		t.Fatalf("expected generated tool call id, got %+v", msg.ToolCalls)
	}

	// ensure tool response uses the same generated id so name mapping works
	id := msg.ToolCalls[0].ID
	_, err = client.Chat(context.Background(), []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{Name: "lookup", ID: id, Args: json.RawMessage(`{"x":2}`), ThoughtSignature: "c2ln"}}},
		{Role: "tool", ToolID: id, Content: `{"result":"ok"}`},
	}, []llm.ToolSchema{{Name: "lookup", Parameters: map[string]any{"type": "object"}}}, "")
	if err != nil {
		t.Fatalf("Chat with tool response returned error: %v", err)
	}
	contents, ok := body["contents"].([]any)
	if !ok || len(contents) < 3 {
		t.Fatalf("expected contents in request, got %#v", body["contents"])
	}
	frPart := contents[2].(map[string]any)["parts"].([]any)[0].(map[string]any)
	if frName := frPart["functionResponse"].(map[string]any)["name"]; frName != "lookup" {
		t.Fatalf("expected functionResponse name lookup, got %v", frName)
	}
}

func TestChatImageInlineData(t *testing.T) {
	t.Parallel()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}},{"text":"done"}]}}]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	ctx := llm.WithImagePrompt(context.Background(), llm.ImagePromptOptions{Size: "1K"})
	msg, err := client.Chat(ctx, []llm.Message{{Role: "user", Content: "make an image"}}, nil, "")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if msg.Content != "done" {
		t.Fatalf("unexpected content %q", msg.Content)
	}
	if len(msg.Images) != 1 {
		t.Fatalf("expected one image, got %d", len(msg.Images))
	}
	if msg.Images[0].MIMEType != "image/png" {
		t.Fatalf("unexpected mime %q", msg.Images[0].MIMEType)
	}
	if string(msg.Images[0].Data) != "hello" {
		t.Fatalf("unexpected image data %q", string(msg.Images[0].Data))
	}
	// ensure responseModalities/imageConfig were sent when image prompt is requested
	if modes, ok := body["generationConfig"].(map[string]any); !ok || len(modes) == 0 {
		t.Fatalf("expected generationConfig in request, got %#v", body)
	}
	generationConfig := body["generationConfig"].(map[string]any)
	if modes, ok := generationConfig["responseModalities"].([]any); !ok || len(modes) != 2 {
		t.Fatalf("expected responseModalities with two entries, got %#v", generationConfig["responseModalities"])
	}
	imgCfg, ok := generationConfig["imageConfig"].(map[string]any)
	if !ok || imgCfg["imageSize"] != "1K" {
		t.Fatalf("expected imageConfig with size 1K, got %#v", generationConfig["imageConfig"])
	}
}

func TestChatStreamEmitsImages(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunk := `{"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":"image/jpeg","data":"Yg=="}}]}}]}`
		_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	rec := &streamRecorder{}
	ctx := llm.WithImagePrompt(context.Background(), llm.ImagePromptOptions{})
	if err := client.ChatStream(ctx, []llm.Message{{Role: "user", Content: "image please"}}, nil, "", rec); err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}
	if len(rec.images) != 1 {
		t.Fatalf("expected one image, got %d", len(rec.images))
	}
	if string(rec.images[0].Data) != "b" {
		t.Fatalf("unexpected image data %q", string(rec.images[0].Data))
	}
}

func TestChatHandlesSafetyBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Response blocked by safety filter
		_, _ = w.Write([]byte(`{"candidates":[{"finishReason":"SAFETY","content":null}]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, "")
	if err == nil {
		t.Fatal("expected error for safety blocked response")
	}
	if !strings.Contains(err.Error(), "safety") {
		t.Fatalf("expected safety error, got: %v", err)
	}
}

func TestChatHandlesPromptBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Prompt blocked due to safety
		_, _ = w.Write([]byte(`{"promptFeedback":{"blockReason":"SAFETY"},"candidates":[]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, "")
	if err == nil {
		t.Fatal("expected error for prompt blocked response")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked error, got: %v", err)
	}
}

func TestChatHandlesEmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Response with nil content (can happen in streaming intermediate chunks)
		_, _ = w.Write([]byte(`{"candidates":[{"content":null,"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	msg, err := client.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, "")
	if err != nil {
		t.Fatalf("Chat returned unexpected error: %v", err)
	}
	// Empty content should not be an error - just return empty message
	if msg.Role != "assistant" {
		t.Fatalf("expected assistant role, got: %s", msg.Role)
	}
}

// TestStreamHandlesIntermediateEmptyChunks verifies that ChatStream tolerates
// streaming responses that include intermediate chunks with empty candidates
// or nil content, which is normal behavior when Gemini responds after tool calls.
func TestStreamHandlesIntermediateEmptyChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			// Intermediate chunk with empty candidates (happens between tool response and final answer)
			`{"candidates":[]}`,
			// Intermediate chunk with nil content
			`{"candidates":[{"content":null}]}`,
			// Intermediate chunk with empty parts
			`{"candidates":[{"content":{"role":"model","parts":[]}}]}`,
			// Final response with actual content
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"Here is your answer!"}]},"finishReason":"STOP"}]}`,
		}
		for _, c := range chunks {
			_, _ = w.Write([]byte("data: " + c + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(config.GoogleConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	rec := &streamRecorder{}
	err = client.ChatStream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, nil, "", rec)
	if err != nil {
		t.Fatalf("ChatStream returned unexpected error: %v", err)
	}

	got := strings.Join(rec.deltas, "")
	if got != "Here is your answer!" {
		t.Fatalf("expected final content, got %q", got)
	}
}
