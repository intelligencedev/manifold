package agentd

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/config"
	llmpkg "manifold/internal/llm"
	"manifold/internal/tools"
	"manifold/internal/warpp"
)

type fakeToolRegistry struct {
	byName map[string]tools.Tool
}

func newFakeToolRegistry() *fakeToolRegistry {
	return &fakeToolRegistry{byName: map[string]tools.Tool{}}
}
func (f *fakeToolRegistry) Schemas() []llmpkg.ToolSchema { return nil }
func (f *fakeToolRegistry) Register(t tools.Tool)        { f.byName[t.Name()] = t }
func (f *fakeToolRegistry) Unregister(name string)       { delete(f.byName, name) }
func (f *fakeToolRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
	return nil, nil
}

func newWarppToolTestApp() *app {
	reg := newFakeToolRegistry()
	return &app{
		cfg:              &config.Config{},
		warpp:            newWarppRuntime(newFakeWarppStore()),
		baseToolRegistry: reg,
	}
}

func saveDoc(t *testing.T, a *app, doc warpp.Document) {
	t.Helper()
	if _, _, err := a.warppState().upsertWorkflow(context.Background(), systemUserID, doc, warpp.Canvas{}); err != nil {
		t.Fatalf("save %s: %v", doc.ID, err)
	}
}

func linDoc(id string) warpp.Document {
	return warpp.Document{
		ID: id, Name: id, Publish: warpp.Publish{Tool: true},
		Inputs: []warpp.PortSpec{{Name: "topic", Type: "text", Required: true}},
		Nodes: []warpp.Node{{ID: "tpl", Type: "data.template",
			Inputs: map[string]warpp.Input{
				"template": {One: &warpp.Binding{Value: "about {t}", HasValue: true}},
				"vars":     {Named: map[string]warpp.Binding{"t": {From: "in.topic"}}},
			}}},
		Outputs: map[string]warpp.Binding{"out": {From: "tpl.text"}},
	}
}

func TestPublishedToolRegistrationAndCall(t *testing.T) {
	a := newWarppToolTestApp()
	reg := a.baseToolRegistry.(*fakeToolRegistry)
	saveDoc(t, a, linDoc("brief"))
	a.syncPublishedWorkflowTools(context.Background())
	tool, ok := reg.byName["warpp_brief"]
	if !ok {
		t.Fatalf("expected warpp_brief registered, have %v", reg.byName)
	}
	raw, _ := json.Marshal(map[string]any{"topic": "go"})
	res, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["ok"] != true {
		t.Fatalf("call not ok: %v", m)
	}
	outputs, _ := m["outputs"].(map[string]any)
	if outputs["out"] != "about go" {
		t.Fatalf("outputs=%v", outputs)
	}
}

func TestWorkflowSaveToolReturnsDiagnostics(t *testing.T) {
	a := newWarppToolTestApp()
	save := &warppSaveTool{app: a}
	raw, _ := json.Marshal(map[string]any{
		"document": map[string]any{"id": "bad", "name": "Bad",
			"nodes": []any{map[string]any{"id": "a", "type": "nope.x"}}},
	})
	res, err := save.Call(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["ok"] != false {
		t.Fatalf("expected ok:false, got %v", m)
	}
	diags, _ := m["diagnostics"].([]warpp.Diagnostic)
	if !hasDiagCode(diags, "node.type.unknown") {
		t.Fatalf("want node.type.unknown, got %+v", diags)
	}
}

func TestSubflowRunnerExecutesChildWorkflow(t *testing.T) {
	a := newWarppToolTestApp()
	saveDoc(t, a, linDoc("child"))
	parent := warpp.Document{
		ID: "parent", Name: "parent",
		Inputs: []warpp.PortSpec{{Name: "topic", Type: "text", Required: true}},
		Nodes: []warpp.Node{{ID: "sub", Type: "flow.child",
			Inputs: map[string]warpp.Input{"topic": {One: &warpp.Binding{From: "in.topic"}}}}},
		Outputs: map[string]warpp.Binding{"result": {From: "sub.out"}},
	}
	saveDoc(t, a, parent)
	res, err := a.ExecuteWarppSync(context.Background(), systemUserID, "parent", map[string]any{"topic": "go"})
	if err != nil {
		t.Fatal(err)
	}
	outputs, _ := res["outputs"].(map[string]any)
	if outputs["result"] != "about go" {
		t.Fatalf("subflow output=%v", outputs)
	}
}

func TestSubflowCycleRejected(t *testing.T) {
	a := newWarppToolTestApp()
	// A includes B
	docA := warpp.Document{ID: "A", Name: "A",
		Nodes:   []warpp.Node{{ID: "b", Type: "flow.B"}},
		Outputs: map[string]warpp.Binding{},
	}
	docB := warpp.Document{ID: "B", Name: "B",
		Nodes:   []warpp.Node{{ID: "a", Type: "flow.A"}},
		Outputs: map[string]warpp.Binding{},
	}
	saveDoc(t, a, docA)
	diags := a.checkSubflowCycles(context.Background(), systemUserID, docB)
	if !hasDiagCode(diags, "workflow.subflow.cycle") {
		t.Fatalf("want cycle diagnostic, got %+v", diags)
	}
}

var _ = llmpkg.Message{}
