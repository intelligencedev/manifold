package warpp

import (
	"encoding/json"
	"reflect"
	"testing"
)

const docJSON = `{
  "id": "research-brief",
  "name": "Research brief",
  "inputs": [{"name": "topic", "type": "text", "required": true}],
  "nodes": [
    {"id": "search", "type": "tool.web_search",
     "inputs": {"query": {"from": "in.topic"}, "max_results": {"value": 5}}},
    {"id": "join", "type": "logic.coalesce",
     "inputs": {"values": [{"from": "search.results_text"}, {"value": "none"}]}},
    {"id": "tpl", "type": "data.template",
     "inputs": {"template": {"value": "{a}"}, "vars": {"a": {"from": "join.value"}}}}
  ],
  "outputs": {"brief": {"from": "tpl.text"}},
  "settings": {"max_concurrency": 2}
}`

func TestDocumentRoundTrip(t *testing.T) {
	var doc Document
	if err := json.Unmarshal([]byte(docJSON), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.ID != "research-brief" || len(doc.Nodes) != 3 {
		t.Fatalf("basic fields wrong: %+v", doc)
	}
	q := doc.Nodes[0].Inputs["query"]
	if q.One == nil || q.One.From != "in.topic" || q.One.HasValue {
		t.Fatalf("query binding wrong: %+v", q)
	}
	mr := doc.Nodes[0].Inputs["max_results"]
	if mr.One == nil || !mr.One.HasValue || mr.One.Value != float64(5) {
		t.Fatalf("literal binding wrong: %+v", mr)
	}
	vals := doc.Nodes[1].Inputs["values"]
	if len(vals.List) != 2 || vals.List[0].From != "search.results_text" || !vals.List[1].HasValue {
		t.Fatalf("list variadic wrong: %+v", vals)
	}
	vars := doc.Nodes[2].Inputs["vars"]
	if len(vars.Named) != 1 || vars.Named["a"].From != "join.value" {
		t.Fatalf("named variadic wrong: %+v", vars)
	}
	if doc.Outputs["brief"].From != "tpl.text" {
		t.Fatalf("outputs wrong: %+v", doc.Outputs)
	}

	round, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc2 Document
	if err := json.Unmarshal(round, &doc2); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if !reflect.DeepEqual(doc, doc2) {
		t.Fatalf("round trip mismatch:\n%+v\n%+v", doc, doc2)
	}
}

func TestBindingExclusivity(t *testing.T) {
	var b Binding
	if err := json.Unmarshal([]byte(`{"from": "a.b", "value": 3}`), &b); err == nil {
		t.Fatal("binding with both from and value must fail to parse")
	}
	if err := json.Unmarshal([]byte(`{}`), &b); err == nil {
		t.Fatal("binding with neither from nor value must fail to parse")
	}
	if err := json.Unmarshal([]byte(`{"value": null}`), &b); err != nil || !b.HasValue {
		t.Fatalf("explicit null literal must parse with HasValue, err=%v b=%+v", err, b)
	}
	if err := json.Unmarshal([]byte(`{"value": false}`), &b); err != nil || !b.HasValue || b.Value != false {
		t.Fatalf("false literal must survive, err=%v b=%+v", err, b)
	}
}

func TestInputFormDetection(t *testing.T) {
	var in Input
	if err := json.Unmarshal([]byte(`{"from": "a.b"}`), &in); err != nil || in.One == nil {
		t.Fatalf("single form: err=%v in=%+v", err, in)
	}
	if err := json.Unmarshal([]byte(`[{"value": 1}, {"from": "a.b"}]`), &in); err != nil || len(in.List) != 2 {
		t.Fatalf("list form: err=%v in=%+v", err, in)
	}
	if err := json.Unmarshal([]byte(`{"x": {"value": 1}, "y": {"from": "a.b"}}`), &in); err != nil || len(in.Named) != 2 {
		t.Fatalf("named form: err=%v in=%+v", err, in)
	}
	if err := json.Unmarshal([]byte(`"bare string"`), &in); err == nil {
		t.Fatal("bare scalar must fail: bindings are objects or arrays")
	}
}

func TestMapBodyParses(t *testing.T) {
	src := `{"id": "m", "type": "control.map",
	  "inputs": {"items": {"from": "in.list"}},
	  "body": {"nodes": [{"id": "n", "type": "data.stringify",
	            "inputs": {"value": {"from": "item.value"}}}],
	           "outputs": {"result": {"from": "n.text"}}}}`
	var n Node
	if err := json.Unmarshal([]byte(src), &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if n.Body == nil || len(n.Body.Nodes) != 1 || n.Body.Outputs["result"].From != "n.text" {
		t.Fatalf("body wrong: %+v", n.Body)
	}
}
