package warpp

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/tools"
)

// functional tool for tests
type fnTool struct {
	name string
	f    func(context.Context, json.RawMessage) (any, error)
}

func (t fnTool) Name() string                                               { return t.name }
func (t fnTool) JSONSchema() map[string]any                                 { return map[string]any{"description": "fn"} }
func (t fnTool) Call(ctx context.Context, raw json.RawMessage) (any, error) { return t.f(ctx, raw) }

func TestCrossStepPathResolution_JSONAndDelta(t *testing.T) {
	reg := tools.NewRegistry()
	// Step 1: pretend web_search to populate delta (first_url) and return JSON results array
	reg.Register(fnTool{name: "web_search", f: func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{
			"ok":      true,
			"results": []map[string]any{{"url": "http://a", "title": "A"}, {"url": "http://b", "title": "B"}},
		}, nil
	}})
	// Step 2: echo tool to capture rendered args
	reg.Register(fnTool{name: "echo_step", f: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var a map[string]any
		_ = json.Unmarshal(raw, &a)
		return map[string]any{"ok": true, "args": a}, nil
	}})

	r := Runner{Tools: reg}
	w := Workflow{Intent: "x", Steps: []Step{
		{ID: "s1", Text: "search", Tool: &ToolRef{Name: "web_search", Args: map[string]any{"query": "q"}}},
		{ID: "s2", Text: "use prior", Tool: &ToolRef{Name: "echo_step", Args: map[string]any{
			"picked": "${A.s1.json.results.1.title}",
			"first":  "${A.s1.first_url}",
		}}},
	}}
	allowed := map[string]bool{"web_search": true, "echo_step": true}
	attrs := Attrs{"utter": "u"}
	_, trace, err := r.ExecuteWithTrace(context.Background(), w, allowed, attrs, nil)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if len(trace) != 2 {
		t.Fatalf("expected 2 trace entries, got %d", len(trace))
	}
	args := trace[1].RenderedArgs
	if args == nil {
		t.Fatalf("expected rendered args in trace[1]")
	}
	if args["picked"] != "B" {
		t.Fatalf("expected picked=B from JSON path, got %v", args["picked"])
	}
	if args["first"] != "http://a" {
		t.Fatalf("expected first from delta alias, got %v", args["first"])
	}
}

func TestCrossStep_JSONStringNestedAccess(t *testing.T) {
	reg := tools.NewRegistry()
	// Step 1: mimic specialists_infer returning JSON with output as JSON string
	reg.Register(fnTool{name: "specialists_infer", f: func(ctx context.Context, raw json.RawMessage) (any, error) {
		// output is a JSON string
		return map[string]any{
			"ok":     true,
			"output": `{"queries":["q1","q2","q3"]}`,
		}, nil
	}})
	// Step 2: echo rendered selection from nested JSON string
	reg.Register(fnTool{name: "echo", f: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var a map[string]any
		_ = json.Unmarshal(raw, &a)
		return map[string]any{"ok": true, "picked": a["picked"]}, nil
	}})

	r := Runner{Tools: reg}
	w := Workflow{Intent: "x2", Steps: []Step{
		{ID: "s1", Text: "plan", Tool: &ToolRef{Name: "specialists_infer", Args: map[string]any{"prompt": "x", "specialist": "planner"}}},
		{ID: "s2", Text: "use nested", Tool: &ToolRef{Name: "echo", Args: map[string]any{"picked": "${A.s1.json.output.queries.1}"}}},
	}}
	allowed := map[string]bool{"specialists_infer": true, "echo": true}
	attrs := Attrs{"utter": "u"}
	_, trace, err := r.ExecuteWithTrace(context.Background(), w, allowed, attrs, nil)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if len(trace) != 2 {
		t.Fatalf("expected 2 trace entries, got %d", len(trace))
	}
	args := trace[1].RenderedArgs
	if args["picked"] != "q2" {
		t.Fatalf("expected picked=q2 via JSON-in-string traversal, got %v", args["picked"])
	}
}
