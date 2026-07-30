package toolnode

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"manifold/internal/llm"
	"manifold/internal/tools"
	"manifold/internal/warpp"
)

type fakeRegistry struct {
	lastName string
	lastArgs map[string]any
	payload  []byte
	err      error
}

func (f *fakeRegistry) Schemas() []llm.ToolSchema { return nil }
func (f *fakeRegistry) Register(t tools.Tool)     {}
func (f *fakeRegistry) Unregister(name string)    {}
func (f *fakeRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
	f.lastName = name
	f.lastArgs = map[string]any{}
	_ = json.Unmarshal(raw, &f.lastArgs)
	return f.payload, f.err
}

func adapterByType(t *testing.T, nodeType string) Adapter {
	t.Helper()
	for _, a := range Builtin() {
		if a.NodeType == nodeType {
			return a
		}
	}
	t.Fatalf("no adapter %s", nodeType)
	return Adapter{}
}

func TestWebSearchAdapter(t *testing.T) {
	reg := &fakeRegistry{payload: []byte(`{"ok":true,"results":[{"title":"A","url":"http://a"},{"title":"B","url":"http://b"}]}`)}
	a := adapterByType(t, "tool.web_search")
	runner := Runners(reg, []Adapter{a})[a.NodeType]
	out, err := runner(context.Background(), warpp.RunnerCtx{}, warpp.NodeInputs{
		Values: map[string]warpp.Value{
			"query":       warpp.NewText("go"),
			"max_results": {Type: warpp.Type{Kind: warpp.KindNumber}, Data: float64(2)},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if reg.lastName != "web_search" || reg.lastArgs["query"] != "go" || reg.lastArgs["max_results"] != float64(2) {
		t.Fatalf("dispatched %s %v", reg.lastName, reg.lastArgs)
	}
	if rs, _ := out["results"].Data.([]any); len(rs) != 2 {
		t.Fatalf("results=%v", out["results"].Data)
	}
	txt, _ := out["results_text"].Data.(string)
	if !strings.Contains(txt, "1. A — http://a") || !strings.Contains(txt, "2. B — http://b") {
		t.Fatalf("results_text=%q", txt)
	}
	if out["raw"].Type.Kind != warpp.KindJSON {
		t.Fatal("raw output missing")
	}
}

func TestAdapterOmitsAbsentOptionalArgs(t *testing.T) {
	reg := &fakeRegistry{payload: []byte(`{"ok":true,"results":[]}`)}
	a := adapterByType(t, "tool.web_search")
	runner := Runners(reg, []Adapter{a})[a.NodeType]
	if _, err := runner(context.Background(), warpp.RunnerCtx{}, warpp.NodeInputs{
		Values: map[string]warpp.Value{"query": warpp.NewText("go")}}); err != nil {
		t.Fatal(err)
	}
	if _, present := reg.lastArgs["max_results"]; present {
		t.Fatal("absent optional port must not become an arg")
	}
}

func TestAdapterToolErrorPropagates(t *testing.T) {
	reg := &fakeRegistry{payload: []byte(`{"ok":false,"error":"rate limited"}`)}
	a := adapterByType(t, "tool.web_search")
	runner := Runners(reg, []Adapter{a})[a.NodeType]
	_, err := runner(context.Background(), warpp.RunnerCtx{}, warpp.NodeInputs{
		Values: map[string]warpp.Value{"query": warpp.NewText("go")}})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err=%v", err)
	}
}

func TestAdapterContractViolation(t *testing.T) {
	reg := &fakeRegistry{payload: []byte(`{"ok":true,"unexpected":true}`)}
	a := adapterByType(t, "tool.web_search")
	runner := Runners(reg, []Adapter{a})[a.NodeType]
	_, err := runner(context.Background(), warpp.RunnerCtx{}, warpp.NodeInputs{
		Values: map[string]warpp.Value{"query": warpp.NewText("go")}})
	if err == nil || !strings.Contains(err.Error(), "contract violation") {
		t.Fatalf("err=%v", err)
	}
}

func TestGenericTool(t *testing.T) {
	reg := &fakeRegistry{payload: []byte(`{"ok":true,"anything":42}`)}
	runner := Runners(reg, nil)["tool.generic"]
	out, err := runner(context.Background(), warpp.RunnerCtx{}, warpp.NodeInputs{
		Values: map[string]warpp.Value{
			"tool": warpp.NewText("magma_lifecycle"),
			"args": {Type: warpp.Type{Kind: warpp.KindJSON}, Data: map[string]any{"x": float64(1)}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if reg.lastName != "magma_lifecycle" || reg.lastArgs["x"] != float64(1) {
		t.Fatalf("dispatch %s %v", reg.lastName, reg.lastArgs)
	}
	m, _ := out["result"].Data.(map[string]any)
	if m["anything"] != float64(42) {
		t.Fatalf("result=%v", out["result"].Data)
	}
}

func TestManifestsAndResolverCoverAllAdapters(t *testing.T) {
	adapters := Builtin()
	if len(adapters) != 9 {
		t.Fatalf("expected 9 curated adapters, got %d", len(adapters))
	}
	resolve := Resolver(adapters)
	for _, a := range adapters {
		m, ok := resolve(a.NodeType)
		if !ok || m.Type != a.NodeType || m.Category != "tool" {
			t.Fatalf("resolver missing %s", a.NodeType)
		}
		if _, ok := m.Output("raw"); !ok {
			t.Fatalf("%s must expose raw output", a.NodeType)
		}
	}
	if _, ok := resolve("tool.generic"); !ok {
		t.Fatal("resolver must include tool.generic")
	}
	for _, m := range Manifests(adapters) {
		for _, p := range append(append([]warpp.PortSpec{}, m.Inputs...), m.Outputs...) {
			if _, err := warpp.ParseType(p.Type); err != nil {
				t.Fatalf("%s port %s: %v", m.Type, p.Name, err)
			}
		}
	}
}
