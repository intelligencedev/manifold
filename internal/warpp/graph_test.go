package warpp

import (
	"reflect"
	"testing"
)

func wired(from string) Input { return Input{One: &Binding{From: from}} }

func TestParseRef(t *testing.T) {
	ref, err := ParseRef("search.results_text")
	if err != nil || ref.Node != "search" || ref.Port != "results_text" {
		t.Fatalf("got %+v err=%v", ref, err)
	}
	ref, err = ParseRef("in.topic")
	if err != nil || ref.Node != "in" {
		t.Fatalf("got %+v err=%v", ref, err)
	}
	ref, err = ParseRef("node.output.nested")
	if err != nil || ref.Port != "output.nested" {
		t.Fatalf("first-dot split failed: %+v err=%v", ref, err)
	}
	for _, bad := range []string{"", "nodeonly", ".port", "node.", "no de.port"} {
		if _, err := ParseRef(bad); err == nil {
			t.Fatalf("ParseRef(%q) should fail", bad)
		}
	}
}

func TestNodeDepsLiftsMapBodyOuterRefs(t *testing.T) {
	m := Node{
		ID: "m", Type: "control.map",
		Inputs: map[string]Input{"items": wired("src.list")},
		Body: &Body{
			Nodes: []Node{
				{ID: "inner", Type: "data.stringify",
					Inputs: map[string]Input{"value": wired("item.value")}},
				{ID: "inner2", Type: "data.template",
					Inputs: map[string]Input{
						"template": wired("outer.text"), // outer ref -> lifts
						"vars":     {Named: map[string]Binding{"a": {From: "inner.text"}}},
					}},
			},
			Outputs: map[string]Binding{"result": {From: "inner2.text"}},
		},
	}
	local := map[string]bool{"src": true, "outer": true, "m": true}
	got := nodeDeps(&m, func(id string) bool { return local[id] })
	want := []string{"outer", "src"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deps=%v want %v", got, want)
	}
}

func TestTopoOrderDeterministicAndCycles(t *testing.T) {
	nodes := []Node{
		{ID: "c", Inputs: map[string]Input{"x": wired("a.out")}},
		{ID: "a"},
		{ID: "b", Inputs: map[string]Input{"x": wired("a.out")}},
		{ID: "d", Inputs: map[string]Input{"x": wired("c.out"), "y": wired("b.out")}},
	}
	isLocal := func(id string) bool {
		for _, n := range nodes {
			if n.ID == id {
				return true
			}
		}
		return false
	}
	order, ok := topoOrder(nodes, func(n *Node) []string { return nodeDeps(n, isLocal) })
	if !ok {
		t.Fatal("unexpected cycle")
	}
	// a first; then c before b (declaration order tie-break); then d.
	want := []string{"a", "c", "b", "d"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order=%v want %v", order, want)
	}

	cyc := []Node{
		{ID: "x", Inputs: map[string]Input{"v": wired("y.out")}},
		{ID: "y", Inputs: map[string]Input{"v": wired("x.out")}},
	}
	if _, ok := topoOrder(cyc, func(n *Node) []string {
		return nodeDeps(n, func(string) bool { return true })
	}); ok {
		t.Fatal("cycle must be reported")
	}
}
