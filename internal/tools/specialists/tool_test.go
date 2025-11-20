package specialists_tool

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"manifold/internal/config"
	"manifold/internal/specialists"
	"manifold/internal/tools"
)

func TestJSONSchemaContainsEnums(t *testing.T) {
	llmConfig := config.LLMClientConfig{
		Provider: "openai",
		OpenAI:   config.OpenAIConfig{},
	}
	list := []config.SpecialistConfig{{Name: "x"}, {Name: "y"}}
	r := specialists.NewRegistry(llmConfig, list, &http.Client{}, tools.NewRegistry())
	tool := New(r)
	schema := tool.JSONSchema()
	params := schema["parameters"].(map[string]any)
	props := params["properties"].(map[string]any)
	sp := props["specialist"].(map[string]any)
	if v, ok := sp["enum"]; ok {
		switch vv := v.(type) {
		case []string:
			if len(vv) != 2 {
				t.Fatalf("expected 2 enums, got %d", len(vv))
			}
		case []any:
			if len(vv) != 2 {
				t.Fatalf("expected 2 enums, got %d", len(vv))
			}
		default:
			t.Fatalf("unexpected enum type: %T", v)
		}
	}
}

func TestCallUnknownSpecialist(t *testing.T) {
	llmConfig := config.LLMClientConfig{
		Provider: "openai",
		OpenAI:   config.OpenAIConfig{},
	}
	list := []config.SpecialistConfig{{Name: "a"}}
	r := specialists.NewRegistry(llmConfig, list, &http.Client{}, tools.NewRegistry())
	tool := New(r)

	in, err := json.Marshal(map[string]any{"specialist": "missing", "prompt": "hi"})
	if err != nil {
		t.Fatalf("marshal err: %v", err)
	}

	out, err := tool.Call(context.Background(), in)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}

	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", out)
	}

	if okVal, _ := m["ok"].(bool); okVal {
		t.Fatalf("expected ok=false for unknown specialist")
	}
}
