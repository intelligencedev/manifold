package specialists

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http/httptest"

	"manifold/internal/config"
	"manifold/internal/llm"
)

func TestNewRegistry_PopulatesAgentFields(t *testing.T) {
	base := config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{APIKey: "basekey", Model: "basemodel", BaseURL: ""}}
	list := []config.SpecialistConfig{{Name: "s1", Description: "desc", APIKey: "specKey", Model: "specModel", System: "mysys", EnableTools: true, ReasoningEffort: " high ", ExtraParams: map[string]any{"k": "v"}}}
	r := NewRegistry(base, list, http.DefaultClient, nil)
	a, ok := r.Get("s1")
	if !ok {
		t.Fatalf("expected s1 present")
	}
	if !strings.Contains(a.System, "mysys") {
		t.Fatalf("system prompt missing base content: %q", a.System)
	}
	if !strings.Contains(a.UserPromptContext, "Available specialists you can invoke:") {
		t.Fatalf("user prompt context missing specialist addendum: %q", a.UserPromptContext)
	}
	if !strings.Contains(a.UserPromptContext, "s1: desc") {
		t.Fatalf("user prompt context missing specialist details: %q", a.UserPromptContext)
	}
	if strings.Contains(a.System, "Available specialists you can invoke:") {
		t.Fatalf("did not expect specialist addendum in system prompt: %q", a.System)
	}
	if a.Model != "specModel" {
		t.Fatalf("unexpected model: %q", a.Model)
	}
	if a.ReasoningEffort != "high" {
		t.Fatalf("reasoning not trimmed, got %q", a.ReasoningEffort)
	}
	if !a.EnableTools {
		t.Fatalf("expected tools enabled")
	}
	if v, ok := a.ExtraParams["k"]; !ok || v != "v" {
		t.Fatalf("expected extra param present, got %#v", a.ExtraParams)
	}
}

func TestAgent_Inference_NoProvider(t *testing.T) {
	a := &Agent{}
	if _, err := a.Inference(context.TODO(), "u", nil); err == nil {
		t.Fatalf("expected error when provider nil")
	}
}

func TestAgentInferenceImageGenerationSendsOnlyPrompt(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/images/generations" {
			http.Error(w, "unexpected chat request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"cG5nYnl0ZXM="}]}`))
	}))
	defer server.Close()

	base := config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{APIKey: "test", Model: "fallback", BaseURL: server.URL}}
	registry := NewRegistry(base, []config.SpecialistConfig{{
		Name:            "image-maker",
		Provider:        "openai",
		BaseURL:         server.URL,
		APIKey:          "test",
		Model:           "gpt-image-2",
		System:          "Never send this system prompt to image generation.",
		EnableTools:     true,
		ImageGeneration: true,
		ExtraParams:     map[string]any{"size": "2048x2048"},
	}}, server.Client(), nil)
	agent, ok := registry.Get("image-maker")
	if !ok {
		t.Fatal("expected image-maker specialist")
	}
	out, err := agent.Inference(context.Background(), "draw a skyline", []llm.Message{
		{Role: "user", Content: "previous prompt that must be ignored"},
		{Role: "assistant", Content: "previous answer that must be ignored"},
	})
	if err != nil {
		t.Fatalf("Inference() error = %v", err)
	}
	if out != "Generated image" {
		t.Fatalf("expected generated image result, got %q", out)
	}
	if gotPath != "/images/generations" {
		t.Fatalf("expected image generation endpoint, got %q", gotPath)
	}
	if prompt, _ := gotBody["prompt"].(string); prompt != "draw a skyline" {
		t.Fatalf("expected current prompt only, got %#v", gotBody["prompt"])
	}
	if size, _ := gotBody["size"].(string); size != "2048x2048" {
		t.Fatalf("expected configured image size, got %#v", gotBody["size"])
	}
}

func TestImageGenerationSpecialistDoesNotInheritBaseOpenAIExtraParams(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/images/generations" {
			http.Error(w, "unexpected chat request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"cG5nYnl0ZXM="}]}`))
	}))
	defer server.Close()

	base := config.LLMClientConfig{
		Provider: "openai",
		OpenAI: config.OpenAIConfig{
			APIKey:  "test",
			Model:   "fallback",
			BaseURL: server.URL,
			ExtraParams: map[string]any{
				"reasoning_effort": "medium",
				"temperature":      0.2,
			},
		},
	}
	registry := NewRegistry(base, []config.SpecialistConfig{{
		Name:            "image-maker",
		Provider:        "openai",
		BaseURL:         server.URL,
		APIKey:          "test",
		Model:           "gpt-image-2",
		ImageGeneration: true,
	}}, server.Client(), nil)
	agent, ok := registry.Get("image-maker")
	if !ok {
		t.Fatal("expected image-maker specialist")
	}
	if _, err := agent.Inference(context.Background(), "draw a lake", nil); err != nil {
		t.Fatalf("Inference() error = %v", err)
	}
	if _, ok := gotBody["reasoning_effort"]; ok {
		t.Fatalf("did not expect inherited reasoning_effort on image request: %#v", gotBody)
	}
	if _, ok := gotBody["temperature"]; ok {
		t.Fatalf("did not expect inherited temperature on image request: %#v", gotBody)
	}
	if prompt, _ := gotBody["prompt"].(string); prompt != "draw a lake" {
		t.Fatalf("expected prompt only, got %#v", gotBody["prompt"])
	}
	if size, _ := gotBody["size"].(string); size != "1024x1024" {
		t.Fatalf("expected default image size, got %#v", gotBody["size"])
	}
}

func TestRegistry_AppendsSpecialistsToSystemPrompt(t *testing.T) {
	base := config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{APIKey: "basekey", Model: "basemodel"}}
	list := []config.SpecialistConfig{
		{Name: "beta", Description: "second", Model: "m1"},
		{Name: "alpha", Description: "first", Model: "m2"},
	}
	r := NewRegistry(base, list, http.DefaultClient, nil)
	combined := r.AppendToSystemPrompt("base sys")
	if !strings.Contains(combined, "base sys") {
		t.Fatalf("combined prompt missing base: %q", combined)
	}
	if !strings.Contains(combined, "alpha: first") || !strings.Contains(combined, "beta: second") {
		t.Fatalf("combined prompt missing specialists: %q", combined)
	}
	if strings.Index(combined, "alpha") > strings.Index(combined, "beta") {
		t.Fatalf("expected alphabetical ordering, got %q", combined)
	}
	a, _ := r.Get("alpha")
	if !strings.Contains(a.UserPromptContext, "Available specialists you can invoke:") {
		t.Fatalf("agent user prompt context missing specialist addendum: %q", a.UserPromptContext)
	}
}

func TestSetWorkdirRebuildsSpecialistSystemPrompt(t *testing.T) {
	t.Parallel()

	base := config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{APIKey: "basekey", Model: "basemodel"}}
	r := NewRegistry(base, []config.SpecialistConfig{{Name: "alpha", Description: "first", Model: "m", System: "mysys"}}, http.DefaultClient, nil)
	r.SetWorkdir(filepath.Join("tmp", "workspace"))

	a, ok := r.Get("alpha")
	if !ok {
		t.Fatalf("expected alpha present")
	}
	if !strings.Contains(a.System, "tmp/workspace") {
		t.Fatalf("expected rebuilt system prompt to include workdir, got %q", a.System)
	}
	if !strings.Contains(a.System, "mysys") {
		t.Fatalf("expected rebuilt system prompt to preserve specialist system, got %q", a.System)
	}
	if !strings.Contains(a.UserPromptContext, "Available specialists you can invoke:") {
		t.Fatalf("expected rebuilt user prompt context to preserve specialist catalog, got %q", a.UserPromptContext)
	}
	if !strings.Contains(a.UserPromptContext, "alpha: first") {
		t.Fatalf("expected rebuilt user prompt context to include specialist metadata, got %q", a.UserPromptContext)
	}
	if strings.Contains(a.System, "alpha: first") {
		t.Fatalf("did not expect rebuilt system prompt to include specialist metadata, got %q", a.System)
	}
	if !strings.Contains(a.System, "locked working directory") {
		t.Fatalf("expected rebuilt prompt to include default workdir guidance, got %q", a.System)
	}
}

func TestLocalSpecialistIgnoresResponsesAPIOverride(t *testing.T) {
	t.Parallel()

	var paths []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"chat-ok","tool_calls":[]}}]}`))
		case "/responses":
			resp := map[string]any{
				"output": []map[string]any{{"type": "message", "content": []map[string]any{{"type": "output_text", "text": "responses-ok"}}}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	base := config.LLMClientConfig{
		Provider: "openai",
		OpenAI: config.OpenAIConfig{
			APIKey:  "test",
			Model:   "m",
			BaseURL: srv.URL,
			API:     "responses",
		},
	}
	list := []config.SpecialistConfig{{
		Name:     "local-s",
		Provider: "local",
		API:      "responses",
	}}

	r := NewRegistry(base, list, srv.Client(), nil)
	a, ok := r.Get("local-s")
	if !ok {
		t.Fatalf("expected local specialist present")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := a.Inference(ctx, "hello", nil)
	if err != nil {
		t.Fatalf("unexpected inference error: %v", err)
	}
	if out != "chat-ok" {
		t.Fatalf("expected chat completion output, got %q", out)
	}
	if len(paths) == 0 || paths[0] != "/chat/completions" {
		t.Fatalf("expected first request to /chat/completions, got %#v", paths)
	}
	for _, p := range paths {
		if p == "/responses" {
			t.Fatalf("unexpected call to /responses for local specialist: %#v", paths)
		}
	}
}
