package warpp

import (
	"context"
	"encoding/json"
	"testing"

	persist "manifold/internal/persistence"
	"manifold/internal/tools"
	"manifold/internal/tools/utility"
)

// fakeWFStore is a minimal in-memory WarppWorkflowStore for tests.
type fakeWFStore struct{ wfs []persist.WarppWorkflow }

func (f *fakeWFStore) Init(context.Context) error { return nil }
func (f *fakeWFStore) List(context.Context, int64) ([]any, error) {
	out := make([]any, len(f.wfs))
	for i, w := range f.wfs {
		out[i] = w
	}
	return out, nil
}
func (f *fakeWFStore) ListWorkflows(context.Context, int64) ([]persist.WarppWorkflow, error) {
	return f.wfs, nil
}
func (f *fakeWFStore) Get(context.Context, int64, string) (persist.WarppWorkflow, bool, error) {
	return persist.WarppWorkflow{}, false, nil
}
func (f *fakeWFStore) Upsert(context.Context, int64, persist.WarppWorkflow) (persist.WarppWorkflow, error) {
	return persist.WarppWorkflow{}, nil
}
func (f *fakeWFStore) Delete(context.Context, int64, string) error { return nil }
func (f *fakeWFStore) GetByIntent(ctx context.Context, userID int64, intent string) (persist.WarppWorkflow, bool, error) {
	for _, wf := range f.wfs {
		if wf.Intent == intent {
			return wf, true, nil
		}
	}
	return persist.WarppWorkflow{}, false, nil
}

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
	// Configure DB-backed defaults via a fake in-memory store.
	SetDefaultStore(&fakeWFStore{wfs: []persist.WarppWorkflow{
		{Intent: "cli_echo", Description: "d", Keywords: []string{"k"}, Steps: []map[string]any{{"id": "s1", "text": "x"}}},
	}})
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
			{ID: "step3", Text: "transform", Tool: &ToolRef{Name: "llm_transform", Args: map[string]any{"input": "${A.report_md}", "instruction": "fmt"}}},
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

func TestRunnerRunStepUtilityTextbox(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(utility.NewTextboxTool())
	runner := Runner{Tools: reg}
	step := Step{ID: "box1", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "hello", "output_attr": "greeting"}}}
	payload, delta, args, err := runner.runStep(context.Background(), step, Attrs{})
	if err != nil {
		t.Fatalf("runStep error: %v", err)
	}
	if string(payload) == "" {
		t.Fatalf("expected payload from utility_textbox")
	}
	if got := delta["greeting"]; got != "hello" {
		t.Fatalf("expected greeting attribute, got %v", got)
	}
	if args == nil || args["text"] != "hello" {
		t.Fatalf("expected args to contain rendered text, got %v", args)
	}
	step2 := Step{ID: "box2", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "world"}}}
	_, delta2, args2, err := runner.runStep(context.Background(), step2, Attrs{})
	if err != nil {
		t.Fatalf("runStep error: %v", err)
	}
	if got := delta2["box2_text"]; got != "world" {
		t.Fatalf("expected default attr key box2_text, got %v", got)
	}
	if args2 == nil || args2["text"] != "world" {
		t.Fatalf("expected args2 text value, got %v", args2)
	}
}

func TestExecuteWithTraceCapturesRenderedArgs(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(newFunctionalTool("echo_step", func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	}))
	w := Workflow{
		Intent: "trace_test",
		Steps: []Step{
			{ID: "s1", Text: "echo", Tool: &ToolRef{Name: "echo_step", Args: map[string]any{"message": "${A.utter}"}}},
		},
	}
	runner := Runner{Workflows: &Registry{byIntent: map[string]Workflow{"trace_test": w}}, Tools: reg}
	attrs := Attrs{"utter": "hello"}
	traceAllowed := map[string]bool{"echo_step": true}
	_, trace, err := runner.ExecuteWithTrace(context.Background(), w, traceAllowed, attrs, nil)
	if err != nil {
		t.Fatalf("ExecuteWithTrace error: %v", err)
	}
	if len(trace) != 1 {
		t.Fatalf("expected single trace entry, got %d", len(trace))
	}
	if msg, ok := trace[0].RenderedArgs["message"]; !ok || msg != "hello" {
		t.Fatalf("expected rendered args to contain message=hello, got %v", trace[0].RenderedArgs)
	}
	if trace[0].Status != "completed" {
		t.Fatalf("expected status completed, got %s", trace[0].Status)
	}
}

func TestMultiInputSubstitutionAcrossPredecessors(t *testing.T) {
	// Tools: textbox and a combine step that lets us inspect rendered args
	reg := tools.NewRegistry()
	reg.Register(utility.NewTextboxTool())
	reg.Register(newFunctionalTool("combine", func(ctx context.Context, raw json.RawMessage) (any, error) {
		// just return ok; we'll assert via RenderedArgs in trace
		return map[string]any{"ok": true}, nil
	}))

	// Workflow: two textboxes -> combine step using both values via ${A.<step>.json.text}
	w := Workflow{
		Intent: "multi_input",
		Steps: []Step{
			{ID: "boxA", Text: "A", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "alpha"}}},
			{ID: "boxB", Text: "B", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "beta"}}},
			{ID: "combine", Text: "C", Tool: &ToolRef{Name: "combine", Args: map[string]any{
				"input": "A=${A.boxA.json.text} B=${A.boxB.json.text}",
			}}},
		},
	}
	runner := Runner{Workflows: &Registry{byIntent: map[string]Workflow{"multi_input": w}}, Tools: reg}

	// Allow all tools
	allowed := map[string]bool{"utility_textbox": true, "combine": true}

	// Execute with trace to capture rendered args at combine
	_, trace, err := runner.ExecuteWithTrace(context.Background(), w, allowed, Attrs{}, nil)
	if err != nil {
		t.Fatalf("ExecuteWithTrace error: %v", err)
	}
	if len(trace) != 3 {
		t.Fatalf("expected 3 trace entries, got %d", len(trace))
	}
	// last step should be combine with both substitutions present
	last := trace[2]
	in, ok := last.RenderedArgs["input"].(string)
	if !ok {
		t.Fatalf("missing rendered input in combine args: %v", last.RenderedArgs)
	}
	if in != "A=alpha B=beta" {
		t.Fatalf("unexpected combined input: %q", in)
	}
}

func TestMultiInputSubstitutionIntoLLMTransform(t *testing.T) {
	// Tools: textbox and a fake llm_transform that captures args
	reg := tools.NewRegistry()
	reg.Register(utility.NewTextboxTool())
	var capturedInput string
	reg.Register(newFunctionalTool("llm_transform", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var args map[string]any
		_ = json.Unmarshal(raw, &args)
		if s, ok := args["input"].(string); ok {
			capturedInput = s
		}
		return map[string]any{"ok": true, "output": "ok"}, nil
	}))

	w := Workflow{
		Intent: "multi_input_llm",
		Steps: []Step{
			{ID: "box1", Text: "A", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "left"}}},
			{ID: "box2", Text: "B", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "right"}}},
			{ID: "t", Text: "T", Tool: &ToolRef{Name: "llm_transform", Args: map[string]any{
				"instruction": "combine",
				"input":       "L=${A.box1.json.text} R=${A.box2.json.text}",
			}}},
		},
	}
	runner := Runner{Workflows: &Registry{byIntent: map[string]Workflow{"multi_input_llm": w}}, Tools: reg}
	allowed := map[string]bool{"utility_textbox": true, "llm_transform": true}
	if _, err := runner.Execute(context.Background(), w, allowed, Attrs{}, nil); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if capturedInput != "L=left R=right" {
		t.Fatalf("unexpected llm input: %q", capturedInput)
	}
}

func TestMultiInputWithHyphenatedIDsAndJsonPath(t *testing.T) {
	// Ensure paths like ${A.utility-textbox-XXXX.json.text} resolve for multiple predecessors
	reg := tools.NewRegistry()
	reg.Register(utility.NewTextboxTool())
	reg.Register(newFunctionalTool("llm_transform", func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{"ok": true, "output": "done"}, nil
	}))

	w := Workflow{
		Intent: "hyphen_ids",
		Steps: []Step{
			{ID: "utility-textbox-8103c942", Text: "A", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "AAA"}}},
			{ID: "utility-textbox-e2d8e9ee", Text: "B", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "BBB"}}},
			{ID: "utility-textbox-5d9535c5", Text: "C", Tool: &ToolRef{Name: "utility_textbox", Args: map[string]any{"text": "CCC"}}},
			{ID: "llm-transform-212d3402", Text: "Combine", Tool: &ToolRef{Name: "llm_transform", Args: map[string]any{
				"instruction": "noop",
				"input":       "X=${A.utility-textbox-8103c942.json.text} Y=${A.utility-textbox-e2d8e9ee.json.text} Z=${A.utility-textbox-5d9535c5.json.text}",
			}}, DependsOn: []string{"utility-textbox-8103c942", "utility-textbox-e2d8e9ee", "utility-textbox-5d9535c5"}},
		},
	}
	runner := Runner{Workflows: &Registry{byIntent: map[string]Workflow{"hyphen_ids": w}}, Tools: reg}
	allowed := map[string]bool{"utility_textbox": true, "llm_transform": true}
	_, trace, err := runner.ExecuteWithTrace(context.Background(), w, allowed, Attrs{}, nil)
	if err != nil {
		t.Fatalf("ExecuteWithTrace error: %v", err)
	}
	if len(trace) != 4 {
		t.Fatalf("expected 4 trace entries, got %d", len(trace))
	}
	last := trace[3]
	inp, _ := last.RenderedArgs["input"].(string)
	if inp != "X=AAA Y=BBB Z=CCC" {
		t.Fatalf("unexpected input: %q", inp)
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
