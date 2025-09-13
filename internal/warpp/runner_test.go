package warpp

import (
	"context"
	"encoding/json"
	"testing"

	"intelligence.dev/internal/tools"
)

// simple tool implementing tools.Tool
type echoTool struct{ name string }

func (e echoTool) Name() string               { return e.name }
func (e echoTool) JSONSchema() map[string]any { return map[string]any{"description": "echo"} }
func (e echoTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var a any
	_ = json.Unmarshal(raw, &a)
	return map[string]any{"ok": true, "echo": a}, nil
}

func TestRenderAndSubstitute(t *testing.T) {
	A := Attrs{"utter": "Hello", "name": "z"}
	in := map[string]any{
		"s":   "greeting: ${A.utter}",
		"arr": []any{"x", "y ${A.name}"},
		"m":   map[string]any{"k": "val ${A.name}"},
	}
	out := renderArgs(in, A)
	if out["s"] != "greeting: Hello" {
		t.Fatalf("unexpected s: %v", out["s"])
	}
	arr := out["arr"].([]any)
	if arr[1] != "y z" {
		t.Fatalf("unexpected arr[1]: %v", arr[1])
	}
	m := out["m"].(map[string]any)
	if m["k"] != "val z" {
		t.Fatalf("unexpected m.k: %v", m["k"])
	}
}

func TestRegistryDefaultsAndGet(t *testing.T) {
	r, err := LoadFromDir("")
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	all := r.All()
	if len(all) == 0 {
		t.Fatalf("expected default workflows, got none")
	}
	// Get a known intent from defaults
	_, err = r.Get("cli_echo")
	if err != nil {
		t.Fatalf("Get(cli_echo) failed: %v", err)
	}
	// Missing intent
	if _, err := r.Get("no_such_intent_xyz"); err == nil {
		t.Fatalf("expected error for missing workflow")
	}
}

func TestRunnerDetectPersonalizeExecute(t *testing.T) {
	// Setup base registry
	base := tools.NewRegistry()
	base.Register(echoTool{name: "web_search"})
	base.Register(echoTool{name: "web_fetch"})
	base.Register(echoTool{name: "llm_transform"})
	base.Register(echoTool{name: "write_file"})

	// Build a custom workflow
	w := Workflow{
		Intent:   "test_intent",
		Keywords: []string{"report", "research"},
		Steps: []Step{
			{ID: "s1", Text: "search", Tool: &ToolRef{Name: "web_search", Args: map[string]any{"query": "${A.query}"}}},
			{ID: "s2", Text: "fetch", Guard: "A.first_url", Tool: &ToolRef{Name: "web_fetch", Args: map[string]any{"url": "${A.first_url}", "prefer_readable": true}}},
			{ID: "s3", Text: "transform", Tool: &ToolRef{Name: "llm_transform", Args: map[string]any{"input": "${A.report_md}", "instruction": "fmt"}}},
			{ID: "s4", Text: "write", Tool: &ToolRef{Name: "write_file", Args: map[string]any{"path": "report.md", "content": "${A.report_md}"}}},
		},
	}
	reg := tools.NewRecordingRegistry(base, nil)
	runner := Runner{Workflows: &Registry{byIntent: map[string]Workflow{"test_intent": w}}, Tools: reg}

	ctx := context.Background()
	intent := runner.DetectIntent(ctx, "Please write a research report")
	if intent != "test_intent" {
		t.Fatalf("expected test_intent, got %s", intent)
	}

	// Personalize
	pw, _, A, err := runner.Personalize(ctx, w, Attrs{"utter": "Please write a research report"})
	if err != nil {
		t.Fatalf("Personalize error: %v", err)
	}
	if A["os"] == nil {
		t.Fatalf("expected os to be set")
	}
	// Allowed tools: permit all
	allowed := map[string]bool{"web_search": true, "web_fetch": true, "llm_transform": true, "write_file": true}

	// To simulate specific payloads, register functional tools that return controlled responses
	reg.Register(newFunctionalTool("web_search", func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{"ok": true, "results": []map[string]any{{"url": "http://a", "title": "A"}, {"url": "http://b", "title": "B"}}}, nil
	}))
	reg.Register(newFunctionalTool("web_fetch", func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{"ok": true, "title": "T", "markdown": "MD", "final_url": "http://final", "input_url": "http://in", "used_readable": true}, nil
	}))
	reg.Register(newFunctionalTool("llm_transform", func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{"ok": true, "output": "# Report\nContent"}, nil
	}))
	reg.Register(newFunctionalTool("write_file", func(ctx context.Context, raw json.RawMessage) (any, error) {
		// verify content contains the report path
		var args map[string]any
		_ = json.Unmarshal(raw, &args)
		if args["path"] != "report.md" {
			t.Fatalf("unexpected path arg: %v", args["path"])
		}
		return map[string]any{"ok": true}, nil
	}))

	summary, err := runner.Execute(ctx, pw, allowed, A, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

// helpers: functional tool
type functionalTool struct {
	name string
	f    func(context.Context, json.RawMessage) (any, error)
}

func (t functionalTool) Name() string               { return t.name }
func (t functionalTool) JSONSchema() map[string]any { return map[string]any{"description": "f"} }
func (t functionalTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return t.f(ctx, raw)
}
func newFunctionalTool(name string, f func(context.Context, json.RawMessage) (any, error)) tools.Tool {
	return functionalTool{name: name, f: f}
}
