package imagetool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"manifold/internal/config"
	"manifold/internal/llm/openai"
)

func TestDescribeToolOpenAIAttachmentAvoidsInlineDataURL(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tokenize":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count":1}`))
			return
		case "/chat/completions":
			// handled below
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	workdir := t.TempDir()
	imagePath := filepath.Join(workdir, "sample.png")
	writeTestPNG(t, imagePath)

	cli := openai.New(config.OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "vision-model",
		API:     "completions",
	}, server.Client())

	tool := NewDescribeTool(cli, workdir, "", "", nil)
	args := json.RawMessage(`{"path":"sample.png","prompt":"What is in this image?"}`)
	result, err := tool.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	resMap, ok := result.(map[string]any)
	if !ok || resMap["ok"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}

	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected messages payload: %#v", requestBody["messages"])
	}
	userMsg, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("unexpected user message: %#v", messages[1])
	}
	contentParts, ok := userMsg["content"].([]any)
	if !ok || len(contentParts) != 2 {
		t.Fatalf("unexpected content parts: %#v", userMsg["content"])
	}

	textPart, ok := contentParts[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected text part: %#v", contentParts[0])
	}
	if strings.Contains(mustString(t, textPart["text"]), "data:image/") {
		t.Fatalf("text part unexpectedly contains inline data URL: %#v", textPart)
	}

	imagePart, ok := contentParts[1].(map[string]any)
	if !ok {
		t.Fatalf("unexpected image part: %#v", contentParts[1])
	}
	imageURL, ok := imagePart["image_url"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected image_url payload: %#v", imagePart["image_url"])
	}
	urlValue := mustString(t, imageURL["url"])
	if !strings.HasPrefix(urlValue, "data:image/png;base64,") {
		t.Fatalf("unexpected image data url prefix: %q", urlValue)
	}
	encoded := strings.TrimPrefix(urlValue, "data:image/png;base64,")
	if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
		t.Fatalf("image attachment is not valid base64: %v", err)
	}
	if strings.Contains(urlValue, "What is in this image?") {
		t.Fatalf("image attachment unexpectedly contains prompt text")
	}
}

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test png: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
}

func mustString(t *testing.T, v any) string {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected string, got %#v", v)
	}
	return s
}
