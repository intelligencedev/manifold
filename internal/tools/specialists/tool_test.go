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
	base := config.OpenAIConfig{}
	list := []config.SpecialistConfig{{Name: "x"}, {Name: "y"}}
	r := specialists.NewRegistry(base, list, &http.Client{}, tools.NewRegistry())
	tool := New(r)
	schema := tool.JSONSchema()
	params, ok := schema["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("missing params")
	}
	props := params["properties"].(map[string]any)
	sp := props["specialist"].(map[string]any)
	// enum may be []string or []any depending on how the map was built.
	switch v := sp["enum"].(type) {
	case []string:
		if len(v) != 2 {
			t.Fatalf("expected 2 enums, got %d", len(v))
		}
	case []any:
		if len(v) != 2 {
			t.Fatalf("expected 2 enums, got %d", len(v))
		}
	default:
		t.Fatalf("unexpected enum type: %T", v)
	}
}

func TestCallUnknownSpecialist(t *testing.T) {
	base := config.OpenAIConfig{}
	list := []config.SpecialistConfig{{Name: "a"}}
	r := specialists.NewRegistry(base, list, &http.Client{}, tools.NewRegistry())
	tool := New(r)
	in, _ := json.Marshal(map[string]any{"specialist": "missing", "prompt": "hi"})
	out, err := tool.Call(context.Background(), in)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}
	m := out.(map[string]any)
	if ok, _ := m["ok"].(bool); ok {
		t.Fatalf("expected ok=false for unknown specialist")
	}
}
