package warpptool

import (
    "context"
    "encoding/json"
    "testing"

    "manifold/internal/tools"
    "manifold/internal/warpp"
)

// dummyTool is a simple tool used by tests to verify WARPP execution.
type dummyTool struct{ calls int }

func (d *dummyTool) Name() string { return "dummy_tool" }
func (d *dummyTool) JSONSchema() map[string]any {
    return map[string]any{
        "name":        d.Name(),
        "description": "dummy",
        "parameters": map[string]any{
            "type":       "object",
            "properties": map[string]any{},
        },
    }
}
func (d *dummyTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    d.calls++
    return map[string]any{"ok": true, "text": "done"}, nil
}

func TestRegisterAllAndSchemaDescription(t *testing.T) {
    reg := tools.NewRegistry()
    runner := &warpp.Runner{Workflows: &warpp.Registry{}, Tools: reg}

    wf := warpp.Workflow{Intent: "web_research", Description: "Deep web research", Steps: []warpp.Step{}}
    runner.Workflows.Upsert(wf, "")

    RegisterAll(reg, runner)

    // Find schema for warpp_web_research
    found := false
    for _, s := range reg.Schemas() {
        if s.Name == ToolPrefix+"web_research" {
            found = true
            if s.Description != wf.Description {
                t.Fatalf("expected description %q, got %q", wf.Description, s.Description)
            }
            // Ensure 'query' is in parameters and required
            params := s.Parameters
            props, _ := params["properties"].(map[string]any)
            if props == nil || props["query"] == nil {
                t.Fatalf("expected 'query' property in parameters")
            }
        }
    }
    if !found {
        t.Fatalf("warpp tool schema not found")
    }
}

func TestWarppToolCallExecutesWorkflow(t *testing.T) {
    ctx := context.Background()
    reg := tools.NewRegistry()
    // Register a dummy tool that the workflow will call
    d := &dummyTool{}
    reg.Register(d)

    // Build workflow that calls dummy_tool
    wf := warpp.Workflow{
        Intent:       "unit_flow",
        Description:  "unit test flow",
        Steps:        []warpp.Step{{ID: "s1", Text: "do", Tool: &warpp.ToolRef{Name: d.Name()}}},
        MaxConcurrency: 1,
        FailFast:       true,
    }
    wreg := &warpp.Registry{}
    wreg.Upsert(wf, "")

    runner := &warpp.Runner{Workflows: wreg, Tools: reg}

    // Expose the workflow as a tool and invoke it
    RegisterAll(reg, runner)

    args := map[string]any{"query": "hello world"}
    raw, _ := json.Marshal(args)
    payload, err := reg.Dispatch(ctx, ToolPrefix+"unit_flow", raw)
    if err != nil {
        t.Fatalf("dispatch error: %v", err)
    }
    var resp struct{
        OK bool `json:"ok"`
        Intent string `json:"intent"`
    }
    _ = json.Unmarshal(payload, &resp)
    if !resp.OK {
        t.Fatalf("expected ok=true, got payload: %s", string(payload))
    }
    if resp.Intent != "unit_flow" {
        t.Fatalf("expected intent 'unit_flow', got %q", resp.Intent)
    }
    if d.calls == 0 {
        t.Fatalf("expected dummy tool to be called at least once")
    }
}

