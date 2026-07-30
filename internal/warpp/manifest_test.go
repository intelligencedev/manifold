package warpp

import "testing"

func TestBuiltinManifestsComplete(t *testing.T) {
	want := []string{
		"data.extract", "data.template", "data.merge", "data.stringify",
		"data.parse", "data.constant",
		"logic.if", "logic.coalesce", "logic.equals", "logic.contains",
		"logic.not", "logic.greater_than",
		"control.map", "llm.generate",
	}
	resolve := BuiltinResolver()
	for _, nodeType := range want {
		m, ok := resolve(nodeType)
		if !ok {
			t.Fatalf("missing builtin manifest %q", nodeType)
		}
		if m.Type != nodeType || m.Title == "" || m.Category == "" {
			t.Fatalf("manifest %q incomplete: %+v", nodeType, m)
		}
		for _, p := range append(append([]PortSpec{}, m.Inputs...), m.Outputs...) {
			if p.Type == DynamicBody || (len(p.Type) > len(DynamicPrefix) && p.Type[:len(DynamicPrefix)] == DynamicPrefix) {
				continue
			}
			if _, err := ParseType(p.Type); err != nil {
				t.Fatalf("manifest %q port %q bad type %q: %v", nodeType, p.Name, p.Type, err)
			}
		}
	}
	if _, ok := resolve("nope.nothing"); ok {
		t.Fatal("unknown type must not resolve")
	}
}

func TestManifestShapes(t *testing.T) {
	resolve := BuiltinResolver()
	ifm, _ := resolve("logic.if")
	if p, ok := ifm.Input("condition"); !ok || p.Type != "boolean" || !p.Required {
		t.Fatalf("logic.if condition port wrong: %+v", p)
	}
	if len(ifm.Outputs) != 2 || ifm.Outputs[0].Name != "then" || ifm.Outputs[1].Name != "else" {
		t.Fatalf("logic.if outputs wrong: %+v", ifm.Outputs)
	}
	co, _ := resolve("logic.coalesce")
	if p, ok := co.Input("values"); !ok || p.Variadic != "list" || p.Type != "T" {
		t.Fatalf("coalesce values port wrong: %+v", p)
	}
	tpl, _ := resolve("data.template")
	if p, ok := tpl.Input("vars"); !ok || p.Variadic != "named" {
		t.Fatalf("template vars port wrong: %+v", p)
	}
	ex, _ := resolve("data.extract")
	if p, ok := ex.Output("value"); !ok || p.Type != "dynamic:as" {
		t.Fatalf("extract output wrong: %+v", p)
	}
	mp, _ := resolve("control.map")
	if p, ok := mp.Output("results"); !ok || p.Type != DynamicBody {
		t.Fatalf("map output wrong: %+v", p)
	}
	if p, ok := mp.Input("on_item_error"); !ok || p.Default != "fail" {
		t.Fatalf("map on_item_error default wrong: %+v", p)
	}
}

func TestChainResolvers(t *testing.T) {
	a := func(nt string) (Manifest, bool) {
		if nt == "x" {
			return Manifest{Type: "x", Title: "A"}, true
		}
		return Manifest{}, false
	}
	b := func(nt string) (Manifest, bool) {
		return Manifest{Type: nt, Title: "B"}, true
	}
	chained := ChainResolvers(a, b)
	if m, _ := chained("x"); m.Title != "A" {
		t.Fatal("first resolver must win")
	}
	if m, _ := chained("y"); m.Title != "B" {
		t.Fatal("fallthrough must reach later resolvers")
	}
}
