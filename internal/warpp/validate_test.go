package warpp

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustDoc(t *testing.T, src string) Document {
	t.Helper()
	var doc Document
	if err := json.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("test doc invalid: %v", err)
	}
	return doc
}

func codes(diags []Diagnostic) string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return strings.Join(out, ",")
}

func hasCode(diags []Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

const validDoc = `{
  "id": "ok", "name": "OK",
  "inputs": [{"name": "topic", "type": "text", "required": true}],
  "nodes": [
    {"id": "c", "type": "data.constant",
     "inputs": {"value": {"value": ["a","b"]}, "as": {"value": "list<json>"}}},
    {"id": "m", "type": "control.map",
     "inputs": {"items": {"from": "c.value"}},
     "body": {"nodes": [
        {"id": "s", "type": "data.stringify", "inputs": {"value": {"from": "item.value"}}},
        {"id": "t", "type": "data.template",
         "inputs": {"template": {"value": "{x} about {topic}"},
                    "vars": {"x": {"from": "s.text"}, "topic": {"from": "in.topic"}}}}],
      "outputs": {"result": {"from": "t.text"}}}},
    {"id": "eq", "type": "logic.equals",
     "inputs": {"a": {"from": "in.topic"}, "b": {"value": "x"}}},
    {"id": "gate", "type": "logic.if",
     "inputs": {"condition": {"from": "eq.result"}, "value": {"from": "in.topic"}}},
    {"id": "co", "type": "logic.coalesce",
     "inputs": {"values": [{"from": "gate.then"}, {"from": "gate.else"}]}}
  ],
  "outputs": {"echo": {"from": "co.value"}, "mapped": {"from": "m.results"}}
}`

func TestValidateAcceptsValidDocument(t *testing.T) {
	diags := Validate(mustDoc(t, validDoc), BuiltinResolver())
	if HasErrors(diags) {
		t.Fatalf("expected valid, got: %s", codes(diags))
	}
}

func TestResolveOutputTypes(t *testing.T) {
	types, diags := ResolveOutputTypes(mustDoc(t, validDoc), BuiltinResolver())
	if HasErrors(diags) {
		t.Fatalf("unexpected: %s", codes(diags))
	}
	if got := types["m"]["results"]; got.Kind != KindList {
		t.Fatalf("map results should be a list, got %v", got)
	}
	if got := types["co"]["value"]; got.Kind != KindText {
		t.Fatalf("coalesce T should unify to text, got %v", got)
	}
	if got := types["in"]["topic"]; got.Kind != KindText {
		t.Fatalf("in ports missing: %v", types["in"])
	}
}

func TestValidateCatchesErrors(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		code string
	}{
		{"unknown type", `{"id":"x","name":"x","nodes":[{"id":"a","type":"nope.x"}]}`, "node.type.unknown"},
		{"dup id", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.parse","inputs":{"text":{"value":"1"}}},{"id":"a","type":"data.parse","inputs":{"text":{"value":"1"}}}]}`, "node.id.duplicate"},
		{"reserved id", `{"id":"x","name":"x","nodes":[{"id":"in","type":"data.parse","inputs":{"text":{"value":"1"}}}]}`, "node.id.reserved"},
		{"missing required", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.parse"}]}`, "node.input.required"},
		{"unknown input", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.parse","inputs":{"text":{"value":"1"},"bogus":{"value":1}}}]}`, "node.input.unknown"},
		{"bad ref", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.parse","inputs":{"text":{"from":"ghost.out"}}}]}`, "node.input.ref"},
		{"bad port", `{"id":"x","name":"x","nodes":[{"id":"c","type":"data.constant","inputs":{"value":{"value":1},"as":{"value":"number"}}},{"id":"a","type":"data.parse","inputs":{"text":{"from":"c.nope"}}}]}`, "node.input.ref"},
		{"type mismatch", `{"id":"x","name":"x","nodes":[{"id":"c","type":"data.constant","inputs":{"value":{"value":{"k":1}},"as":{"value":"json"}}},{"id":"a","type":"data.parse","inputs":{"text":{"from":"c.value"}}}]}`, "node.input.type_mismatch"},
		{"literal mismatch", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.parse","inputs":{"text":{"value":5}}}]}`, "node.input.literal"},
		{"tvar conflict", `{"id":"x","name":"x","inputs":[{"name":"t","type":"text"},{"name":"n","type":"number"}],"nodes":[{"id":"e","type":"logic.equals","inputs":{"a":{"from":"in.t"},"b":{"from":"in.n"}}}]}`, "node.type_var.conflict"},
		{"form mismatch", `{"id":"x","name":"x","nodes":[{"id":"co","type":"logic.coalesce","inputs":{"values":{"value":"solo"}}}]}`, "node.input.form"},
		{"dynamic wired", `{"id":"x","name":"x","inputs":[{"name":"t","type":"text"}],"nodes":[{"id":"c","type":"data.constant","inputs":{"value":{"value":1},"as":{"from":"in.t"}}}]}`, "node.dynamic.literal_required"},
		{"dynamic enum", `{"id":"x","name":"x","nodes":[{"id":"c","type":"data.constant","inputs":{"value":{"value":1},"as":{"value":"blob"}}}]}`, "node.dynamic.enum"},
		{"cycle", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.stringify","inputs":{"value":{"from":"b.text"}}},{"id":"b","type":"data.stringify","inputs":{"value":{"from":"a.text"}}}]}`, "workflow.graph.cycle"},
		{"map no body", `{"id":"x","name":"x","inputs":[{"name":"l","type":"list<text>"}],"nodes":[{"id":"m","type":"control.map","inputs":{"items":{"from":"in.l"}}}]}`, "node.body.required"},
		{"body on non-map", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.parse","inputs":{"text":{"value":"1"}},"body":{"nodes":[],"outputs":{}}}]}`, "node.body.forbidden"},
		{"map items not list", `{"id":"x","name":"x","inputs":[{"name":"t","type":"text"}],"nodes":[{"id":"m","type":"control.map","inputs":{"items":{"from":"in.t"}},"body":{"nodes":[{"id":"s","type":"data.stringify","inputs":{"value":{"from":"item.value"}}}],"outputs":{"result":{"from":"s.text"}}}}]}`, "map.items.type"},
		{"map bad outputs", `{"id":"x","name":"x","inputs":[{"name":"l","type":"list<text>"}],"nodes":[{"id":"m","type":"control.map","inputs":{"items":{"from":"in.l"}},"body":{"nodes":[{"id":"s","type":"data.stringify","inputs":{"value":{"from":"item.value"}}}],"outputs":{"wrong":{"from":"s.text"}}}}]}`, "map.body.outputs.single"},
		{"workflow output ref", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.parse","inputs":{"text":{"value":"1"}}}],"outputs":{"o":{"from":"a.nothing"}}}`, "workflow.output.ref"},
		{"bad policy", `{"id":"x","name":"x","nodes":[{"id":"a","type":"data.parse","inputs":{"text":{"value":"1"}},"policy":{"timeout":"soon","on_error":"explode"}}]}`, "node.policy.timeout"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := Validate(mustDoc(t, c.doc), BuiltinResolver())
			if !hasCode(diags, c.code) {
				t.Fatalf("want code %s, got: %s", c.code, codes(diags))
			}
		})
	}
}

func TestValidatePublishWarning(t *testing.T) {
	doc := mustDoc(t, `{"id":"x","name":"x","publish":{"tool":true},
	  "nodes":[{"id":"a","type":"data.parse","inputs":{"text":{"value":"1"}}}]}`)
	diags := Validate(doc, BuiltinResolver())
	if HasErrors(diags) {
		t.Fatalf("warnings only expected, got %s", codes(diags))
	}
	if !hasCode(diags, "workflow.outputs.empty") {
		t.Fatalf("want workflow.outputs.empty warning, got %s", codes(diags))
	}
}
