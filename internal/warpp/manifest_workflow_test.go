package warpp

import "testing"

func TestWorkflowManifest(t *testing.T) {
	doc := mustDoc(t, validDoc)
	m, diags := WorkflowManifest(doc, BuiltinResolver())
	if HasErrors(diags) {
		t.Fatalf("unexpected diags: %s", codes(diags))
	}
	if m.Type != "flow.ok" || m.Category != "flow" {
		t.Fatalf("manifest header wrong: %+v", m)
	}
	if p, ok := m.Input("topic"); !ok || p.Type != "text" {
		t.Fatalf("input topic wrong: %+v", p)
	}
	echo, ok := m.Output("echo")
	if !ok || echo.Type != "text" {
		t.Fatalf("output echo should be text: %+v", echo)
	}
	mapped, ok := m.Output("mapped")
	if !ok || mapped.Type != "list<text>" {
		t.Fatalf("output mapped should be list<text>: %+v", mapped)
	}
}
