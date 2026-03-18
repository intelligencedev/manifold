package tools

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeTool struct {
	name   string
	schema map[string]any
	call   func(context.Context, json.RawMessage) (any, error)
}

func (f *fakeTool) Name() string               { return f.name }
func (f *fakeTool) JSONSchema() map[string]any { return f.schema }
func (f *fakeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	if f.call != nil {
		return f.call(ctx, raw)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return map[string]any{"echo": out}, nil
}

func TestRegistrySchemasAndDispatch(t *testing.T) {
	r := NewRegistryWithLogging(true)
	ft := &fakeTool{name: "echo", schema: map[string]any{"description": "echoes back", "parameters": map[string]any{"type": "object"}}}
	r.Register(ft)

	schemas := r.Schemas()
	if len(schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(schemas))
	}
	if schemas[0].Name != "echo" {
		t.Fatalf("expected schema name echo, got %s", schemas[0].Name)
	}

	// dispatch to unknown tool
	payload, err := r.Dispatch(context.Background(), "nope", nil)
	if err != nil {
		t.Fatalf("expected no error for unknown tool, got %v", err)
	}
	var unknown map[string]any
	_ = json.Unmarshal(payload, &unknown)
	if _, ok := unknown["error"]; !ok {
		t.Fatalf("expected error payload for unknown tool")
	}

	// dispatch to known tool
	args := json.RawMessage([]byte(`{"x":1}`))
	payload2, err := r.Dispatch(context.Background(), "echo", args)
	if err != nil {
		t.Fatalf("unexpected error dispatching: %v", err)
	}
	var resp map[string]any
	_ = json.Unmarshal(payload2, &resp)
	if _, ok := resp["echo"]; !ok {
		t.Fatalf("expected echo field in response")
	}
}

func TestApplyTopLevelPolicy(t *testing.T) {
	t.Parallel()

	base := NewRegistry()
	base.Register(&fakeTool{name: "one", schema: map[string]any{"description": "one", "parameters": map[string]any{"type": "object"}}})
	base.Register(&fakeTool{name: "two", schema: map[string]any{"description": "two", "parameters": map[string]any{"type": "object"}}})

	filtered := ApplyTopLevelPolicy(base, true, []string{"two"})
	if got := SchemaNames(filtered); len(got) != 1 || got[0] != "two" {
		t.Fatalf("expected only tool two after filtering, got %#v", got)
	}

	disabled := ApplyTopLevelPolicy(base, false, nil)
	if got := SchemaNames(disabled); len(got) != 0 {
		t.Fatalf("expected no tools when disabled, got %#v", got)
	}
}
