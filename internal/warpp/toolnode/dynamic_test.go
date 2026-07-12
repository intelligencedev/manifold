package toolnode

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/llm"
	"manifold/internal/tools"
	"manifold/internal/warpp"
)

type schemaRegistry struct {
	schemas []llm.ToolSchema
	*fakeRegistry
}

func (s *schemaRegistry) Schemas() []llm.ToolSchema { return s.schemas }

func newSchemaRegistry(payload []byte, schemas ...llm.ToolSchema) *schemaRegistry {
	return &schemaRegistry{schemas: schemas, fakeRegistry: &fakeRegistry{payload: payload}}
}

func TestDeriveDynamicToolTypedPorts(t *testing.T) {
	schema := llm.ToolSchema{
		Name:        "brave_search",
		Description: "Search Brave.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":    map[string]any{"type": "string", "description": "Query."},
				"count":    map[string]any{"type": "integer"},
				"safe":     map[string]any{"type": "boolean"},
				"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"filter":   map[string]any{"type": "object"},
				"nullable": map[string]any{"type": []any{"null", "string"}},
			},
			"required": []any{"query"},
		},
	}
	dt := deriveDynamicTool(schema)
	if dt.manifest.Type != "tool.brave_search" || dt.manifest.Category != "tool" {
		t.Fatalf("manifest header: %+v", dt.manifest)
	}
	want := map[string]string{
		"query": "text", "count": "number", "safe": "boolean",
		"tags": "list<text>", "filter": "json", "nullable": "text",
	}
	for name, typ := range want {
		p, ok := dt.manifest.Input(name)
		if !ok || p.Type != typ {
			t.Fatalf("port %s = %+v, want %s", name, p, typ)
		}
	}
	if p, _ := dt.manifest.Input("query"); !p.Required {
		t.Fatal("query must be required")
	}
	if p, _ := dt.manifest.Input("count"); p.Required {
		t.Fatal("count must be optional")
	}
	out, ok := dt.manifest.Output("result")
	if !ok || out.Type != "json" {
		t.Fatalf("result output: %+v", out)
	}
	// every derived port type must parse
	for _, p := range dt.manifest.Inputs {
		if _, err := warpp.ParseType(p.Type); err != nil {
			t.Fatalf("port %s bad type %q: %v", p.Name, p.Type, err)
		}
	}
}

func TestDeriveDynamicToolPassthrough(t *testing.T) {
	dt := deriveDynamicTool(llm.ToolSchema{Name: "freeform", Parameters: map[string]any{"type": "object"}})
	if !dt.passthrough {
		t.Fatal("no-properties tool must be passthrough")
	}
	if p, ok := dt.manifest.Input("args"); !ok || p.Type != "json" {
		t.Fatalf("passthrough must expose args:json, got %+v", p)
	}
}

func TestDynamicResolverExcludesCurated(t *testing.T) {
	reg := newSchemaRegistry(nil,
		llm.ToolSchema{Name: "web_search"}, // curated -> excluded
		llm.ToolSchema{Name: "custom_mcp_tool", Parameters: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}}},
	)
	resolve := DynamicResolver(reg, CuratedToolNames())
	if _, ok := resolve("tool.web_search"); ok {
		t.Fatal("curated tool must be excluded from dynamic resolver")
	}
	if _, ok := resolve("tool.custom_mcp_tool"); !ok {
		t.Fatal("mcp tool must resolve dynamically")
	}
}

func TestDynamicRunnerDispatches(t *testing.T) {
	reg := newSchemaRegistry([]byte(`{"ok":true,"answer":42}`),
		llm.ToolSchema{Name: "mcp_tool", Parameters: map[string]any{
			"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}},
		}})
	runner := DynamicRunners(reg, reg, nil)["tool.mcp_tool"]
	out, err := runner(context.Background(), warpp.RunnerCtx{}, warpp.NodeInputs{
		Values: map[string]warpp.Value{"q": warpp.NewText("hi")}})
	if err != nil {
		t.Fatal(err)
	}
	if reg.lastName != "mcp_tool" || reg.lastArgs["q"] != "hi" {
		t.Fatalf("dispatch %s %v", reg.lastName, reg.lastArgs)
	}
	m, _ := out["result"].Data.(map[string]any)
	if m["answer"] != float64(42) {
		t.Fatalf("result=%v", out["result"].Data)
	}
}

func TestDynamicPassthroughRunnerSpreadsArgs(t *testing.T) {
	reg := newSchemaRegistry([]byte(`{"ok":true}`),
		llm.ToolSchema{Name: "freeform", Parameters: map[string]any{"type": "object"}})
	runner := DynamicRunners(reg, reg, nil)["tool.freeform"]
	_, err := runner(context.Background(), warpp.RunnerCtx{}, warpp.NodeInputs{
		Values: map[string]warpp.Value{"args": {Type: warpp.Type{Kind: warpp.KindJSON}, Data: map[string]any{"a": float64(1)}}}})
	if err != nil {
		t.Fatal(err)
	}
	if reg.lastArgs["a"] != float64(1) {
		t.Fatalf("passthrough args not spread: %v", reg.lastArgs)
	}
}

var _ = json.Marshal
var _ tools.Registry = (*schemaRegistry)(nil)
