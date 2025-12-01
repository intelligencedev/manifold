package specialists

import (
	"context"
	"net/http"
	"strings"
	"testing"

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
