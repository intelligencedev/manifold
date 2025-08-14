package mcpclient

import (
	"encoding/json"
	"testing"

	mcppkg "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSanitizeSchema_ObjectAddsProperties(t *testing.T) {
	s := map[string]any{"type": "object"}
	sanitizeSchema(s, "")
	if props, ok := s["properties"].(map[string]any); !ok || props == nil {
		t.Fatalf("expected properties map, got %#v", s["properties"])
	}
}

func TestSanitizeSchema_ArrayAddsItems(t *testing.T) {
	s := map[string]any{"type": "array"}
	sanitizeSchema(s, "")
	v, ok := s["items"].(map[string]any)
	if !ok || v == nil {
		t.Fatalf("expected items map, got %#v", s["items"])
	}
	if v["type"] != "string" {
		t.Fatalf("expected default items.type string, got %v", v["type"])
	}
}

func TestSanitizeSchema_CompositionAndRequiredNormalization(t *testing.T) {
	// Build a schema with oneOf and required as []any
	top := map[string]any{
		"oneOf": []any{
			map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{}}, "required": []any{"a"}},
		},
		"required": []any{"root"},
	}
	sanitizeSchema(top, "")
	// Ensure nested required normalized to []string
	one := top["oneOf"].([]any)[0].(map[string]any)
	if _, ok := one["required"].([]string); !ok {
		t.Fatalf("expected nested required to be []string, got %#v", one["required"])
	}
	if _, ok := top["required"].([]string); !ok {
		t.Fatalf("expected top required to be []string, got %#v", top["required"])
	}
}

func TestMCPTool_JSONSchema_DefaultsAndDescription(t *testing.T) {
	// Create a tool with nil InputSchema to exercise defaults
	tool := &mcpTool{server: "s", session: nil, tool: &mcppkg.Tool{Name: "t", Description: "d", InputSchema: nil}}
	out := tool.JSONSchema()
	// Should include parameters with type object and properties map
	params, ok := out["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected parameters map, got %#v", out["parameters"])
	}
	if params["type"] != "object" {
		t.Fatalf("expected object type, got %v", params["type"])
	}
	if _, ok := params["properties"].(map[string]any); !ok {
		t.Fatalf("expected properties map, got %#v", params["properties"])
	}
	if out["description"] != "d" {
		t.Fatalf("expected description d, got %v", out["description"])
	}
	// Ensure we can marshal to JSON
	if _, err := json.Marshal(out); err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
}
