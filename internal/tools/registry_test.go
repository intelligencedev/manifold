package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewRegistryBuilds(t *testing.T) {
	_ = NewRegistry()
}

type aliasTestTool struct{}

func (aliasTestTool) Name() string { return "multi_tool_use_parallel" }
func (aliasTestTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "parallel alias test",
		"parameters":  map[string]any{"type": "object"},
	}
}
func (aliasTestTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return map[string]any{"ok": true}, nil
}

func TestRegistryDispatchesDottedParallelAlias(t *testing.T) {
	reg := NewRegistry()
	reg.Register(aliasTestTool{})

	payload, err := reg.Dispatch(context.Background(), "multi_tool_use.parallel", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("dispatch alias: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if parsed["ok"] != true {
		t.Fatalf("expected ok payload, got %s", payload)
	}
}

func TestFilteredRegistryAcceptsDottedParallelAliasInAllowList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(aliasTestTool{})
	filtered := NewFilteredRegistry(reg, []string{"multi_tool_use.parallel"})

	if names := SchemaNames(filtered); len(names) != 1 || names[0] != "multi_tool_use_parallel" {
		t.Fatalf("expected aliased schema to be exposed, got %#v", names)
	}
}
