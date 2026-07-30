package warpp

import (
	"context"
	"testing"
)

func run(t *testing.T, nodeType string, in NodeInputs) map[string]Value {
	t.Helper()
	r, ok := BuiltinRunners()[nodeType]
	if !ok {
		t.Fatalf("no runner for %s", nodeType)
	}
	out, err := r(context.Background(), RunnerCtx{Path: "n"}, in)
	if err != nil {
		t.Fatalf("%s: %v", nodeType, err)
	}
	return out
}

func vals(m map[string]Value) NodeInputs { return NodeInputs{Values: m} }

func TestExtract(t *testing.T) {
	in := vals(map[string]Value{
		"source": {Type: Type{Kind: KindJSON}, Data: map[string]any{
			"results": []any{map[string]any{"title": "First"}}}},
		"path": NewText("results.0.title"),
		"as":   NewText("text"),
	})
	out := run(t, "data.extract", in)
	if out["value"].Data != "First" || out["value"].Type.Kind != KindText {
		t.Fatalf("got %#v", out["value"])
	}
	in.Values["path"] = NewText("results.9.title")
	r := BuiltinRunners()["data.extract"]
	if _, err := r(context.Background(), RunnerCtx{}, in); err == nil {
		t.Fatal("missing path must error")
	}
	in.Values["path"] = NewText("results.0.title")
	in.Values["as"] = NewText("number")
	if _, err := r(context.Background(), RunnerCtx{}, in); err == nil {
		t.Fatal("as=number over string must error")
	}
}

func TestTemplateAndStringifyAndParse(t *testing.T) {
	out := run(t, "data.template", NodeInputs{
		Values: map[string]Value{"template": NewText("{n} and {j}")},
		Named: map[string]map[string]Value{"vars": {
			"n": {Type: Type{Kind: KindNumber}, Data: float64(4)},
			"j": {Type: Type{Kind: KindJSON}, Data: map[string]any{"k": true}},
		}},
	})
	if out["text"].Data != `4 and {"k":true}` {
		t.Fatalf("template got %q", out["text"].Data)
	}
	out = run(t, "data.stringify", vals(map[string]Value{
		"value": {Type: Type{Kind: KindJSON}, Data: map[string]any{"a": float64(1)}}}))
	if out["text"].Data != "{\n  \"a\": 1\n}" {
		t.Fatalf("stringify got %q", out["text"].Data)
	}
	out = run(t, "data.parse", vals(map[string]Value{"text": NewText(`{"x":1}`)}))
	if m, ok := out["json"].Data.(map[string]any); !ok || m["x"] != float64(1) {
		t.Fatalf("parse got %#v", out["json"].Data)
	}
	if _, err := BuiltinRunners()["data.parse"](context.Background(), RunnerCtx{},
		vals(map[string]Value{"text": NewText("not json")})); err == nil {
		t.Fatal("invalid json must error")
	}
}

func TestMergeConstant(t *testing.T) {
	out := run(t, "data.merge", NodeInputs{List: map[string][]Value{"objects": {
		{Type: Type{Kind: KindJSON}, Data: map[string]any{"a": 1.0, "b": 1.0}},
		{Type: Type{Kind: KindJSON}, Data: map[string]any{"b": 2.0}},
	}}})
	m := out["json"].Data.(map[string]any)
	if m["a"] != 1.0 || m["b"] != 2.0 {
		t.Fatalf("merge got %#v", m)
	}
	out = run(t, "data.constant", vals(map[string]Value{
		"value": {Type: Type{Kind: KindJSON}, Data: float64(7)},
		"as":    NewText("number"),
	}))
	if out["value"].Type.Kind != KindNumber || out["value"].Data != float64(7) {
		t.Fatalf("constant got %#v", out["value"])
	}
}

func TestLogicNodes(t *testing.T) {
	fired := run(t, "logic.if", vals(map[string]Value{
		"condition": {Type: Type{Kind: KindBoolean}, Data: true},
		"value":     NewText("v"),
	}))
	if _, hasElse := fired["else"]; hasElse || fired["then"].Data != "v" {
		t.Fatalf("if(true) got %#v", fired)
	}
	fired = run(t, "logic.if", vals(map[string]Value{
		"condition": {Type: Type{Kind: KindBoolean}, Data: false},
		"value":     NewText("v"),
	}))
	if _, hasThen := fired["then"]; hasThen || fired["else"].Data != "v" {
		t.Fatalf("if(false) got %#v", fired)
	}
	out := run(t, "logic.coalesce", NodeInputs{List: map[string][]Value{
		"values": {NewText("first"), NewText("second")}}})
	if out["value"].Data != "first" {
		t.Fatalf("coalesce got %#v", out["value"])
	}
	out = run(t, "logic.equals", vals(map[string]Value{
		"a": {Type: Type{Kind: KindJSON}, Data: map[string]any{"x": 1.0}},
		"b": {Type: Type{Kind: KindJSON}, Data: map[string]any{"x": 1.0}},
	}))
	if out["result"].Data != true {
		t.Fatal("deep equals failed")
	}
	out = run(t, "logic.contains", vals(map[string]Value{
		"haystack": NewText("hello world"), "needle": NewText("lo w")}))
	if out["result"].Data != true {
		t.Fatal("contains failed")
	}
	out = run(t, "logic.not", vals(map[string]Value{
		"value": {Type: Type{Kind: KindBoolean}, Data: false}}))
	if out["result"].Data != true {
		t.Fatal("not failed")
	}
	out = run(t, "logic.greater_than", vals(map[string]Value{
		"a": {Type: Type{Kind: KindNumber}, Data: 2.0},
		"b": {Type: Type{Kind: KindNumber}, Data: 1.0}}))
	if out["result"].Data != true {
		t.Fatal("greater_than failed")
	}
}

func TestLLMRunner(t *testing.T) {
	var gotInstruction, gotInput, gotModel string
	r := LLMRunner(func(ctx context.Context, instruction, input, model string) (string, error) {
		gotInstruction, gotInput, gotModel = instruction, input, model
		return "answer", nil
	})
	out, err := r(context.Background(), RunnerCtx{}, vals(map[string]Value{
		"instruction": NewText("sys"), "input": NewText("q"), "model": NewText("m1"),
	}))
	if err != nil || out["text"].Data != "answer" {
		t.Fatalf("llm got %#v err=%v", out, err)
	}
	if gotInstruction != "sys" || gotInput != "q" || gotModel != "m1" {
		t.Fatal("chat args not forwarded")
	}
}

func TestSelectPathStructuredOnly(t *testing.T) {
	root := map[string]any{"a": []any{map[string]any{"b": "hit"}}, "s": `{"embedded":"json"}`}
	if v, ok := SelectPath(root, "a.0.b"); !ok || v != "hit" {
		t.Fatalf("got %v %v", v, ok)
	}
	if _, ok := SelectPath(root, "s.embedded"); ok {
		t.Fatal("string values must not be json-parsed during traversal")
	}
}
