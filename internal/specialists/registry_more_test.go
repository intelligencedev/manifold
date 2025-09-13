package specialists

import (
	"context"
	"net/http"
	"testing"

	"intelligence.dev/internal/config"
)

func TestNewRegistry_PopulatesAgentFields(t *testing.T) {
	base := config.OpenAIConfig{APIKey: "basekey", Model: "basemodel", BaseURL: ""}
	list := []config.SpecialistConfig{{Name: "s1", APIKey: "specKey", Model: "specModel", System: "mysys", EnableTools: true, ReasoningEffort: " high ", ExtraParams: map[string]any{"k": "v"}}}
	r := NewRegistry(base, list, http.DefaultClient, nil)
	a, ok := r.Get("s1")
	if !ok {
		t.Fatalf("expected s1 present")
	}
	if a.System != "mysys" {
		t.Fatalf("unexpected system: %q", a.System)
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
