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
	if !strings.Contains(a.System, "Available specialists you can invoke:") {
		t.Fatalf("system prompt missing specialist addendum: %q", a.System)
	}
	if !strings.Contains(a.System, "s1: desc") {
		t.Fatalf("system prompt missing specialist details: %q", a.System)
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
	if !strings.Contains(a.System, "Available specialists you can invoke:") {
		t.Fatalf("agent system prompt missing specialist addendum: %q", a.System)
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
	if !strings.Contains(a.System, "Available specialists you can invoke:") {
		t.Fatalf("expected rebuilt system prompt to preserve specialist catalog, got %q", a.System)
	}
	if !strings.Contains(a.System, "alpha: first") {
		t.Fatalf("expected rebuilt system prompt to include specialist metadata, got %q", a.System)
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
