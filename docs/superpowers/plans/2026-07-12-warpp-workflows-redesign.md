# WARPP Workflows Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Flow v2 / legacy WARPP workflow system with a typed-port dataflow engine (ComfyUI model: data moves only through typed wires), per the approved spec at `docs/superpowers/specs/2026-07-12-warpp-workflows-redesign-design.md`.

**Architecture:** New pure-Go engine package `internal/warpp` (document, type system, manifests, validator, executor) + tool adapters in `internal/warpp/toolnode` + service wiring in `internal/agentd` (runtime, HTTP handlers under `/api/warpp/*`, agent tools, durable task) + a rebuilt Vue Flow editor in `web/agentd-ui`. All Flow v2 / legacy WARPP code is deleted (clean break, spec §12).

**Tech Stack:** Go 1.22+ (stdlib only for the engine), existing `manifold/internal/tools` registry, `manifold/internal/durable`, `manifold/internal/llm`, Postgres/SQLite persistence, Vue 3 + Pinia + `@vue-flow/core`, Vitest.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-12-warpp-workflows-redesign-design.md`. On conflict, the spec wins.
- Port types: `text`, `number`, `boolean`, `json`, `file`, `list<T>` with `T ∈ {text, number, boolean, json, file}` (spec §5).
- Coercion table (complete): `number → text`, `boolean → text`. Nothing else (spec §5).
- Input binding is exactly one of `{"from": "nodeID.port"}` or `{"value": <literal>}` (spec §4). No expression strings anywhere.
- Reserved node IDs: `in` (workflow inputs), `item` (Map body). Node IDs match `^[a-zA-Z0-9_-]+$`.
- Skip rule (spec §7): required input's source skipped/never-fired → node skips, cascades. Optional input's source skipped → node runs with the port default.
- Node policy: `timeout` (Go duration string), `retries {max, backoff: fixed|exponential}`, `on_error: fail|skip`. Retry base delay 200ms, exponential doubles per attempt (matches current behavior).
- Run statuses: `running`, `completed`, `completed_with_skips`, `failed`, `cancelled`.
- Event types: `run_started|run_completed|run_failed|run_cancelled`, `node_started|node_completed|node_failed|node_skipped|node_retrying`. Node events carry `node_path` (Map iterations: `mapID[3].nodeID`).
- API namespace: `/api/warpp/*` (spec §10). Old `/api/flows/v2/*` routes are removed, not aliased.
- Agent-facing published workflow tools keep the `warpp_` name prefix (existing `warpptool.ToolPrefix` value).
- Go tests: `go test ./internal/warpp/... ./internal/agentd/...`. Frontend: `cd web/agentd-ui && npx vitest run`. Build gates: `go build ./...` and `npm run build`.
- Commit after every task (each task's final step). Never commit with failing tests.
- TDD: write the failing test first for every behavior-bearing change.
- Execution environment: create an isolated worktree via superpowers:using-git-worktrees before Task 1, branch name `warpp-redesign`, based off `idev`.

---

# Phase A — Engine (`internal/warpp`), no service dependencies

File map for the phase:

| File | Responsibility |
|---|---|
| `internal/warpp/value.go` | Port types, parsing, coercion table, value conformance, literal inference |
| `internal/warpp/manifest.go` | `PortSpec`/`Manifest`/`Resolver`, builtin data/logic/control/llm manifests |
| `internal/warpp/document.go` | Workflow document model + strict JSON (`Binding`/`Input` forms), canvas sidecar types |
| `internal/warpp/graph.go` | Ref parsing, dependency extraction (incl. Map body lifting), deterministic topo order |
| `internal/warpp/validate.go` | Static validation → diagnostics (structure, refs, types, unification, dynamic ports) |
| `internal/warpp/runner.go` | `NodeRunner` contract, `NodeInputs`, builtin data/logic runners, `LLMRunner` |
| `internal/warpp/executor.go` | Wavefront scheduler, skip cascade, policies, events, statuses, durable hook |
| `internal/warpp/executor_map.go` | `control.map` execution (fan-out, lexical scope, gather) |
| `internal/warpp/toolnode/toolnode.go` | Tool adapter framework + curated adapters + `tool.generic` |

### Task 1: Port types, coercion, and values (`value.go`)

**Files:**
- Create: `internal/warpp/value.go`
- Test: `internal/warpp/value_test.go`

**Interfaces:**
- Consumes: nothing (stdlib only).
- Produces (used by every later task):
  - `type Kind string` with consts `KindText, KindNumber, KindBoolean, KindJSON, KindFile, KindList, KindVar` (`KindVar = "T"`)
  - `type Type struct { Kind Kind; Elem Kind }`
  - `func ParseType(s string) (Type, error)` — accepts `"text"`, `"list<json>"`, `"T"`, `"list<T>"`
  - `func (t Type) String() string`, `func (t Type) HasVar() bool`
  - `func Assignable(from, to Type) bool` — equality or the coercion table
  - `func Conforms(data any, t Type) error` — structural check of a raw JSON value
  - `func Coerce(v Value, to Type) (Value, error)` — applies `number/boolean → text` stringification
  - `func InferLiteral(data any) Type`
  - `type Value struct { Type Type "json:\"type\""; Data any "json:\"data\"" }`
  - `func NewText(s string) Value`, `func NewValue(t Type, data any) Value` (convenience)

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/value_test.go
package warpp

import "testing"

func TestParseType(t *testing.T) {
	cases := []struct {
		in      string
		want    Type
		wantErr bool
	}{
		{"text", Type{Kind: KindText}, false},
		{"number", Type{Kind: KindNumber}, false},
		{"boolean", Type{Kind: KindBoolean}, false},
		{"json", Type{Kind: KindJSON}, false},
		{"file", Type{Kind: KindFile}, false},
		{"list<text>", Type{Kind: KindList, Elem: KindText}, false},
		{"list<json>", Type{Kind: KindList, Elem: KindJSON}, false},
		{"T", Type{Kind: KindVar}, false},
		{"list<T>", Type{Kind: KindList, Elem: KindVar}, false},
		{"list<list<text>>", Type{}, true},
		{"blob", Type{}, true},
		{"list<>", Type{}, true},
		{"", Type{}, true},
	}
	for _, c := range cases {
		got, err := ParseType(c.in)
		if c.wantErr != (err != nil) {
			t.Fatalf("ParseType(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
		if err == nil && got != c.want {
			t.Fatalf("ParseType(%q)=%v want %v", c.in, got, c.want)
		}
		if err == nil && got.String() != c.in {
			t.Fatalf("String() round trip %q != %q", got.String(), c.in)
		}
	}
}

func TestAssignable(t *testing.T) {
	txt := Type{Kind: KindText}
	num := Type{Kind: KindNumber}
	boo := Type{Kind: KindBoolean}
	jsn := Type{Kind: KindJSON}
	fil := Type{Kind: KindFile}
	lstTxt := Type{Kind: KindList, Elem: KindText}
	lstNum := Type{Kind: KindList, Elem: KindNumber}

	if !Assignable(txt, txt) || !Assignable(lstTxt, lstTxt) {
		t.Fatal("identity must be assignable")
	}
	if !Assignable(num, txt) || !Assignable(boo, txt) {
		t.Fatal("number/boolean must coerce to text")
	}
	for _, bad := range [][2]Type{
		{txt, num}, {jsn, txt}, {txt, jsn}, {fil, txt}, {txt, fil},
		{lstNum, lstTxt}, {num, lstNum}, {jsn, boo},
	} {
		if Assignable(bad[0], bad[1]) {
			t.Fatalf("%v -> %v must NOT be assignable", bad[0], bad[1])
		}
	}
}

func TestConforms(t *testing.T) {
	ok := []struct {
		data any
		t    Type
	}{
		{"hi", Type{Kind: KindText}},
		{"path/to.txt", Type{Kind: KindFile}},
		{float64(3), Type{Kind: KindNumber}},
		{int(3), Type{Kind: KindNumber}},
		{true, Type{Kind: KindBoolean}},
		{map[string]any{"a": 1}, Type{Kind: KindJSON}},
		{[]any{"x"}, Type{Kind: KindJSON}},
		{"scalar is valid json value", Type{Kind: KindJSON}},
		{nil, Type{Kind: KindJSON}},
		{[]any{"a", "b"}, Type{Kind: KindList, Elem: KindText}},
		{[]any{}, Type{Kind: KindList, Elem: KindNumber}},
		{[]any{map[string]any{}}, Type{Kind: KindList, Elem: KindJSON}},
	}
	for _, c := range ok {
		if err := Conforms(c.data, c.t); err != nil {
			t.Fatalf("Conforms(%v, %v) unexpected err: %v", c.data, c.t, err)
		}
	}
	bad := []struct {
		data any
		t    Type
	}{
		{3, Type{Kind: KindText}},
		{"x", Type{Kind: KindNumber}},
		{nil, Type{Kind: KindText}},
		{[]any{"a", 1}, Type{Kind: KindList, Elem: KindText}},
		{"notalist", Type{Kind: KindList, Elem: KindText}},
	}
	for _, c := range bad {
		if err := Conforms(c.data, c.t); err == nil {
			t.Fatalf("Conforms(%v, %v) expected error", c.data, c.t)
		}
	}
}

func TestCoerce(t *testing.T) {
	out, err := Coerce(Value{Type: Type{Kind: KindNumber}, Data: float64(4.5)}, Type{Kind: KindText})
	if err != nil || out.Data != "4.5" || out.Type.Kind != KindText {
		t.Fatalf("number->text got %#v err=%v", out, err)
	}
	out, err = Coerce(Value{Type: Type{Kind: KindNumber}, Data: float64(4)}, Type{Kind: KindText})
	if err != nil || out.Data != "4" {
		t.Fatalf("integral float should render without decimals, got %#v", out.Data)
	}
	out, err = Coerce(Value{Type: Type{Kind: KindBoolean}, Data: true}, Type{Kind: KindText})
	if err != nil || out.Data != "true" {
		t.Fatalf("boolean->text got %#v err=%v", out, err)
	}
	if _, err = Coerce(Value{Type: Type{Kind: KindJSON}, Data: map[string]any{}}, Type{Kind: KindText}); err == nil {
		t.Fatal("json->text must not coerce implicitly")
	}
	same, err := Coerce(Value{Type: Type{Kind: KindText}, Data: "x"}, Type{Kind: KindText})
	if err != nil || same.Data != "x" {
		t.Fatal("identity coerce must pass through")
	}
}

func TestInferLiteral(t *testing.T) {
	cases := []struct {
		data any
		want Type
	}{
		{"s", Type{Kind: KindText}},
		{float64(1), Type{Kind: KindNumber}},
		{int(1), Type{Kind: KindNumber}},
		{false, Type{Kind: KindBoolean}},
		{map[string]any{}, Type{Kind: KindJSON}},
		{[]any{"a", "b"}, Type{Kind: KindList, Elem: KindText}},
		{[]any{1.0, 2.0}, Type{Kind: KindList, Elem: KindNumber}},
		{[]any{"a", 1.0}, Type{Kind: KindList, Elem: KindJSON}},
		{[]any{}, Type{Kind: KindList, Elem: KindJSON}},
		{nil, Type{Kind: KindJSON}},
	}
	for _, c := range cases {
		if got := InferLiteral(c.data); got != c.want {
			t.Fatalf("InferLiteral(%v)=%v want %v", c.data, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ -run 'TestParseType|TestAssignable|TestConforms|TestCoerce|TestInferLiteral' -v`
Expected: FAIL — package does not exist / undefined identifiers.

- [ ] **Step 3: Implement `value.go`**

```go
// internal/warpp/value.go
// Package warpp implements the typed-port dataflow workflow engine.
package warpp

import (
	"fmt"
	"strconv"
	"strings"
)

type Kind string

const (
	KindText    Kind = "text"
	KindNumber  Kind = "number"
	KindBoolean Kind = "boolean"
	KindJSON    Kind = "json"
	KindFile    Kind = "file"
	KindList    Kind = "list"
	// KindVar is the single type variable allowed in builtin manifests.
	KindVar Kind = "T"
)

// Type is a port type. Elem is set only when Kind == KindList.
type Type struct {
	Kind Kind `json:"kind"`
	Elem Kind `json:"elem,omitempty"`
}

func scalarKind(s string) (Kind, bool) {
	switch Kind(s) {
	case KindText, KindNumber, KindBoolean, KindJSON, KindFile, KindVar:
		return Kind(s), true
	}
	return "", false
}

// ParseType parses "text", "list<json>", "T", "list<T>".
func ParseType(s string) (Type, error) {
	s = strings.TrimSpace(s)
	if inner, ok := strings.CutPrefix(s, "list<"); ok {
		inner, ok = strings.CutSuffix(inner, ">")
		if !ok {
			return Type{}, fmt.Errorf("invalid type %q", s)
		}
		k, ok := scalarKind(inner)
		if !ok {
			return Type{}, fmt.Errorf("invalid list element type %q", inner)
		}
		return Type{Kind: KindList, Elem: k}, nil
	}
	k, ok := scalarKind(s)
	if !ok {
		return Type{}, fmt.Errorf("invalid type %q", s)
	}
	return Type{Kind: k}, nil
}

func (t Type) String() string {
	if t.Kind == KindList {
		return "list<" + string(t.Elem) + ">"
	}
	return string(t.Kind)
}

func (t Type) HasVar() bool {
	return t.Kind == KindVar || (t.Kind == KindList && t.Elem == KindVar)
}

// Assignable reports whether a value of type `from` may be wired into a port
// of type `to`. The implicit coercion table is exactly: number→text,
// boolean→text (spec §5).
func Assignable(from, to Type) bool {
	if from == to {
		return true
	}
	if to.Kind == KindText && (from.Kind == KindNumber || from.Kind == KindBoolean) {
		return true
	}
	return false
}

// Value is a typed runtime value flowing on a wire.
type Value struct {
	Type Type `json:"type"`
	Data any  `json:"data"`
}

func NewValue(t Type, data any) Value { return Value{Type: t, Data: data} }
func NewText(s string) Value          { return Value{Type: Type{Kind: KindText}, Data: s} }

func asNumber(data any) (float64, bool) {
	switch n := data.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// Conforms checks that a raw JSON value structurally matches a concrete type.
func Conforms(data any, t Type) error {
	switch t.Kind {
	case KindText, KindFile:
		if _, ok := data.(string); !ok {
			return fmt.Errorf("expected %s (string), got %T", t.Kind, data)
		}
	case KindNumber:
		if _, ok := asNumber(data); !ok {
			return fmt.Errorf("expected number, got %T", data)
		}
	case KindBoolean:
		if _, ok := data.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", data)
		}
	case KindJSON:
		return nil // any JSON value, including null
	case KindList:
		items, ok := data.([]any)
		if !ok {
			return fmt.Errorf("expected list, got %T", data)
		}
		for i, item := range items {
			if err := Conforms(item, Type{Kind: t.Elem}); err != nil {
				return fmt.Errorf("item %d: %w", i, err)
			}
		}
	default:
		return fmt.Errorf("cannot conform to type %q", t)
	}
	return nil
}

func stringify(v Value) (string, error) {
	switch v.Type.Kind {
	case KindText, KindFile:
		s, _ := v.Data.(string)
		return s, nil
	case KindNumber:
		n, ok := asNumber(v.Data)
		if !ok {
			return "", fmt.Errorf("number value is %T", v.Data)
		}
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	case KindBoolean:
		b, _ := v.Data.(bool)
		return strconv.FormatBool(b), nil
	}
	return "", fmt.Errorf("cannot stringify %s", v.Type)
}

// Coerce converts v to the target type using only the implicit coercion
// table. Identity passes through after a Conforms check.
func Coerce(v Value, to Type) (Value, error) {
	if v.Type == to {
		if err := Conforms(v.Data, to); err != nil {
			return Value{}, err
		}
		return v, nil
	}
	if !Assignable(v.Type, to) {
		return Value{}, fmt.Errorf("cannot assign %s to %s", v.Type, to)
	}
	s, err := stringify(v)
	if err != nil {
		return Value{}, err
	}
	return Value{Type: to, Data: s}, nil
}

// InferLiteral infers the type of a literal JSON value. Homogeneous arrays
// infer a typed list; mixed or empty arrays infer list<json>; null infers json.
func InferLiteral(data any) Type {
	switch d := data.(type) {
	case string:
		return Type{Kind: KindText}
	case bool:
		return Type{Kind: KindBoolean}
	case float64, int, int64:
		return Type{Kind: KindNumber}
	case []any:
		elem := Kind("")
		for _, item := range d {
			k := InferLiteral(item).Kind
			if k == KindList {
				k = KindJSON
			}
			if elem == "" {
				elem = k
			} else if elem != k {
				elem = KindJSON
			}
		}
		if elem == "" {
			elem = KindJSON
		}
		return Type{Kind: KindList, Elem: elem}
	default:
		return Type{Kind: KindJSON}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/ -v`
Expected: PASS (all five tests).

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/value.go internal/warpp/value_test.go
git commit -m "feat(warpp): add port type system, coercion table, and typed values"
```

### Task 2: Document model with strict binding JSON (`document.go`)

**Files:**
- Create: `internal/warpp/document.go`
- Test: `internal/warpp/document_test.go`

**Interfaces:**
- Consumes: Task 1 types (`PortSpec` uses type strings, parsed later).
- Produces:
  - `type Document struct { ID, Name, Description string; Inputs []PortSpec; Nodes []Node; Outputs map[string]Binding; Settings Settings; Publish Publish }` (JSON tags: `id, name, description, inputs, nodes, outputs, settings, publish`)
  - `type Node struct { ID, Type string; Inputs map[string]Input; Policy *Policy; Body *Body }` (JSON: `id, type, inputs, policy, body`)
  - `type Body struct { Nodes []Node; Outputs map[string]Binding }`
  - `type Binding struct { From string; Value any; HasValue bool }` — custom JSON; `HasValue` distinguishes an explicit `"value": null/false/0`
  - `type Input struct { One *Binding; List []Binding; Named map[string]Binding }` — custom JSON: object with `from`/`value` key → `One`; array → `List`; other object → `Named`
  - `type Policy struct { Timeout string; Retries Retries; OnError string }`, `type Retries struct { Max int; Backoff string }`
  - `type Settings struct { MaxConcurrency int; DefaultPolicy Policy }` (JSON: `max_concurrency, default_policy`)
  - `type Publish struct { Tool bool }`
  - Canvas sidecar: `type Canvas struct { Nodes map[string]CanvasNode; Groups []CanvasGroup; Notes []CanvasNote }` with `CanvasNode{X, Y float64; Width, Height *float64; Label string}`, `CanvasGroup{ID, Label, Color string; Collapsed bool}`, `CanvasNote{ID, Label, Note, Color string}` (copied shape from the old `flow.WorkflowCanvas`, minus `Parents`/`EdgeStyle` which the new editor does not use)
  - `PortSpec` gains nothing here — it is defined in Task 3 (`manifest.go`); to keep this task compilable, define `PortSpec` in THIS task inside `document.go` exactly as specified in Task 3's interface block, and Task 3 will NOT redefine it.

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/document_test.go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ -run 'TestDocument|TestBinding|TestInputForm|TestMapBody' -v`
Expected: FAIL — undefined: Document, Binding, Input.

- [ ] **Step 3: Implement `document.go`**

```go
// internal/warpp/document.go
package warpp

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// PortSpec declares one port on a manifest or one workflow-level input.
type PortSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "text", "list<json>", "T", "list<T>", "dynamic:<port>"
	Required    bool   `json:"required,omitempty"`
	Default     any    `json:"default,omitempty"`
	Variadic    string `json:"variadic,omitempty"` // "", "list", "named"
	Description string `json:"description,omitempty"`
}

// Document is the canonical workflow definition (spec §4).
type Document struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Inputs      []PortSpec         `json:"inputs,omitempty"`
	Nodes       []Node             `json:"nodes"`
	Outputs     map[string]Binding `json:"outputs,omitempty"`
	Settings    Settings           `json:"settings,omitempty"`
	Publish     Publish            `json:"publish,omitempty"`
}

type Node struct {
	ID     string           `json:"id"`
	Type   string           `json:"type"`
	Inputs map[string]Input `json:"inputs,omitempty"`
	Policy *Policy          `json:"policy,omitempty"`
	Body   *Body            `json:"body,omitempty"`
}

// Body is the nested subgraph of a control.map node.
type Body struct {
	Nodes   []Node             `json:"nodes"`
	Outputs map[string]Binding `json:"outputs"`
}

type Policy struct {
	Timeout string  `json:"timeout,omitempty"`
	Retries Retries `json:"retries,omitempty"`
	OnError string  `json:"on_error,omitempty"` // "", "fail", "skip"
}

type Retries struct {
	Max     int    `json:"max,omitempty"`
	Backoff string `json:"backoff,omitempty"` // "", "fixed", "exponential"
}

type Settings struct {
	MaxConcurrency int    `json:"max_concurrency,omitempty"`
	DefaultPolicy  Policy `json:"default_policy,omitempty"`
}

type Publish struct {
	Tool bool `json:"tool,omitempty"`
}

// Binding wires an input from an upstream port or fixes it to a literal.
// Exactly one of From / Value is set; HasValue records an explicit literal
// so that null/false/0/"" literals are representable.
type Binding struct {
	From     string
	Value    any
	HasValue bool
}

func (b Binding) MarshalJSON() ([]byte, error) {
	if b.HasValue {
		return json.Marshal(map[string]any{"value": b.Value})
	}
	return json.Marshal(map[string]string{"from": b.From})
}

func (b *Binding) UnmarshalJSON(data []byte) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("binding must be an object: %w", err)
	}
	_, hasFrom := probe["from"]
	rawValue, hasValue := probe["value"]
	if hasFrom == hasValue {
		return fmt.Errorf("binding must set exactly one of \"from\" or \"value\"")
	}
	for key := range probe {
		if key != "from" && key != "value" {
			return fmt.Errorf("binding has unknown key %q", key)
		}
	}
	if hasFrom {
		if err := json.Unmarshal(probe["from"], &b.From); err != nil {
			return fmt.Errorf("binding \"from\" must be a string: %w", err)
		}
		b.HasValue = false
		b.Value = nil
		return nil
	}
	b.From = ""
	b.HasValue = true
	return json.Unmarshal(rawValue, &b.Value)
}

// Input is the value of one entry in Node.Inputs. Scalar ports use One;
// list-variadic ports use List; named-variadic ports use Named.
type Input struct {
	One   *Binding
	List  []Binding
	Named map[string]Binding
}

func (in Input) MarshalJSON() ([]byte, error) {
	switch {
	case in.One != nil:
		return json.Marshal(in.One)
	case in.List != nil:
		return json.Marshal(in.List)
	case in.Named != nil:
		return json.Marshal(in.Named)
	}
	return nil, fmt.Errorf("empty input binding")
}

func (in *Input) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("empty input binding")
	}
	*in = Input{}
	if trimmed[0] == '[' {
		return json.Unmarshal(trimmed, &in.List)
	}
	if trimmed[0] != '{' {
		return fmt.Errorf("input binding must be an object or array")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return err
	}
	_, hasFrom := probe["from"]
	_, hasValue := probe["value"]
	if hasFrom || hasValue {
		in.One = &Binding{}
		return json.Unmarshal(trimmed, in.One)
	}
	return json.Unmarshal(trimmed, &in.Named)
}

// Canvas is editor-only layout metadata, persisted as a sidecar. It never
// affects execution.
type Canvas struct {
	Nodes  map[string]CanvasNode `json:"nodes,omitempty"`
	Groups []CanvasGroup         `json:"groups,omitempty"`
	Notes  []CanvasNote          `json:"notes,omitempty"`
}

type CanvasNode struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Width  *float64 `json:"width,omitempty"`
	Height *float64 `json:"height,omitempty"`
	Label  string   `json:"label,omitempty"`
}

type CanvasGroup struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Color     string `json:"color,omitempty"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

type CanvasNote struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Note  string `json:"note,omitempty"`
	Color string `json:"color,omitempty"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/document.go internal/warpp/document_test.go
git commit -m "feat(warpp): add workflow document model with strict binding JSON"
```

### Task 3: Manifests and builtin catalog (`manifest.go`)

**Files:**
- Create: `internal/warpp/manifest.go`
- Test: `internal/warpp/manifest_test.go`

**Interfaces:**
- Consumes: `PortSpec` (Task 2), `ParseType` (Task 1).
- Produces:
  - `type Manifest struct { Type, Title, Category, Description string; Inputs, Outputs []PortSpec }` (JSON: `type, title, category, description, inputs, outputs`)
  - `type Resolver func(nodeType string) (Manifest, bool)`
  - `func BuiltinManifests() []Manifest` — all `data.*`, `logic.*`, `control.map`, `llm.generate` manifests exactly as specified below
  - `func BuiltinResolver() Resolver`
  - `func ChainResolvers(rs ...Resolver) Resolver` — first match wins
  - `func (m Manifest) Input(name string) (PortSpec, bool)`, `func (m Manifest) Output(name string) (PortSpec, bool)`
  - Const `DynamicPrefix = "dynamic:"` and `const DynamicBody = "dynamic:body"`

The exact builtin manifests (verbatim contract — the validator, runners, editor, and LLM authoring all rely on these names):

| Node type | Inputs | Outputs |
|---|---|---|
| `data.extract` | `source: json` req; `path: text` req; `as: text` default `"json"` | `value: dynamic:as` |
| `data.template` | `template: text` req; `vars: T` named-variadic | `text: text` |
| `data.merge` | `objects: json` list-variadic req | `json: json` |
| `data.stringify` | `value: T` req | `text: text` |
| `data.parse` | `text: text` req | `json: json` |
| `data.constant` | `value: json` req; `as: text` default `"json"` | `value: dynamic:as` |
| `logic.if` | `condition: boolean` req; `value: T` req | `then: T`; `else: T` |
| `logic.coalesce` | `values: T` list-variadic req | `value: T` |
| `logic.equals` | `a: T` req; `b: T` req | `result: boolean` |
| `logic.contains` | `haystack: text` req; `needle: text` req | `result: boolean` |
| `logic.not` | `value: boolean` req | `result: boolean` |
| `logic.greater_than` | `a: number` req; `b: number` req | `result: boolean` |
| `control.map` | `items: list<T>` req; `concurrency: number` default `4`; `on_item_error: text` default `"fail"` | `results: dynamic:body` |
| `llm.generate` | `instruction: text` default `""`; `input: text` req; `model: text` default `""` | `text: text` |

`as` accepts exactly: `"text"`, `"number"`, `"boolean"`, `"json"`, `"list<json>"`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/manifest_test.go
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
			if p.Type == DynamicBody || len(p.Type) > len(DynamicPrefix) && p.Type[:len(DynamicPrefix)] == DynamicPrefix {
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ -run 'TestBuiltin|TestManifest|TestChain' -v`
Expected: FAIL — undefined: Manifest, BuiltinResolver.

- [ ] **Step 3: Implement `manifest.go`**

```go
// internal/warpp/manifest.go
package warpp

const (
	// DynamicPrefix marks an output port whose concrete type is named by a
	// literal config input, e.g. "dynamic:as".
	DynamicPrefix = "dynamic:"
	// DynamicBody marks control.map's results port; its element type is the
	// body's declared output type.
	DynamicBody = "dynamic:body"
)

// Manifest declares a node type's interface (spec §6).
type Manifest struct {
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Category    string     `json:"category"`
	Description string     `json:"description,omitempty"`
	Inputs      []PortSpec `json:"inputs"`
	Outputs     []PortSpec `json:"outputs"`
}

func (m Manifest) Input(name string) (PortSpec, bool) {
	for _, p := range m.Inputs {
		if p.Name == name {
			return p, true
		}
	}
	return PortSpec{}, false
}

func (m Manifest) Output(name string) (PortSpec, bool) {
	for _, p := range m.Outputs {
		if p.Name == name {
			return p, true
		}
	}
	return PortSpec{}, false
}

// Resolver maps a node type to its manifest.
type Resolver func(nodeType string) (Manifest, bool)

// ChainResolvers combines resolvers; the first match wins.
func ChainResolvers(rs ...Resolver) Resolver {
	return func(nodeType string) (Manifest, bool) {
		for _, r := range rs {
			if r == nil {
				continue
			}
			if m, ok := r(nodeType); ok {
				return m, true
			}
		}
		return Manifest{}, false
	}
}

func req(name, typ, desc string) PortSpec {
	return PortSpec{Name: name, Type: typ, Required: true, Description: desc}
}

func opt(name, typ string, def any, desc string) PortSpec {
	return PortSpec{Name: name, Type: typ, Default: def, Description: desc}
}

// BuiltinManifests returns manifests for all built-in node types.
func BuiltinManifests() []Manifest {
	return []Manifest{
		{
			Type: "data.extract", Title: "Extract", Category: "data",
			Description: "Pluck a value out of a JSON structure by dot/index path.",
			Inputs: []PortSpec{
				req("source", "json", "Structure to extract from."),
				req("path", "text", "Dot/index path, e.g. results.0.title."),
				opt("as", "text", "json", "Output type: text, number, boolean, json, list<json>."),
			},
			Outputs: []PortSpec{{Name: "value", Type: "dynamic:as"}},
		},
		{
			Type: "data.template", Title: "Template", Category: "data",
			Description: "Build a string from named inputs using {name} slots.",
			Inputs: []PortSpec{
				req("template", "text", "Template with {name} placeholders."),
				{Name: "vars", Type: "T", Variadic: "named", Description: "Values for placeholders."},
			},
			Outputs: []PortSpec{{Name: "text", Type: "text"}},
		},
		{
			Type: "data.merge", Title: "Merge", Category: "data",
			Description: "Shallow-merge JSON objects; later inputs win.",
			Inputs: []PortSpec{
				{Name: "objects", Type: "json", Required: true, Variadic: "list", Description: "Objects to merge in order."},
			},
			Outputs: []PortSpec{{Name: "json", Type: "json"}},
		},
		{
			Type: "data.stringify", Title: "Stringify", Category: "data",
			Description: "Render any value as text (JSON values pretty-printed).",
			Inputs:      []PortSpec{req("value", "T", "Value to render.")},
			Outputs:     []PortSpec{{Name: "text", Type: "text"}},
		},
		{
			Type: "data.parse", Title: "Parse JSON", Category: "data",
			Description: "Parse text as JSON; fails on invalid input.",
			Inputs:      []PortSpec{req("text", "text", "JSON text.")},
			Outputs:     []PortSpec{{Name: "json", Type: "json"}},
		},
		{
			Type: "data.constant", Title: "Constant", Category: "data",
			Description: "A fixed literal value shared by multiple consumers.",
			Inputs: []PortSpec{
				req("value", "json", "The literal value."),
				opt("as", "text", "json", "Output type: text, number, boolean, json, list<json>."),
			},
			Outputs: []PortSpec{{Name: "value", Type: "dynamic:as"}},
		},
		{
			Type: "logic.if", Title: "If", Category: "logic",
			Description: "Route a value to exactly one branch based on a condition.",
			Inputs: []PortSpec{
				req("condition", "boolean", "Branch selector."),
				req("value", "T", "Value to route."),
			},
			Outputs: []PortSpec{{Name: "then", Type: "T"}, {Name: "else", Type: "T"}},
		},
		{
			Type: "logic.coalesce", Title: "Coalesce", Category: "logic",
			Description: "Emit the first input that fired; rejoins branches.",
			Inputs: []PortSpec{
				{Name: "values", Type: "T", Required: true, Variadic: "list", Description: "Candidates in priority order."},
			},
			Outputs: []PortSpec{{Name: "value", Type: "T"}},
		},
		{
			Type: "logic.equals", Title: "Equals", Category: "logic",
			Description: "Deep equality comparison.",
			Inputs:      []PortSpec{req("a", "T", "Left."), req("b", "T", "Right.")},
			Outputs:     []PortSpec{{Name: "result", Type: "boolean"}},
		},
		{
			Type: "logic.contains", Title: "Contains", Category: "logic",
			Description: "Substring test.",
			Inputs:      []PortSpec{req("haystack", "text", "Text to search."), req("needle", "text", "Text to find.")},
			Outputs:     []PortSpec{{Name: "result", Type: "boolean"}},
		},
		{
			Type: "logic.not", Title: "Not", Category: "logic",
			Description: "Boolean negation.",
			Inputs:      []PortSpec{req("value", "boolean", "Value to negate.")},
			Outputs:     []PortSpec{{Name: "result", Type: "boolean"}},
		},
		{
			Type: "logic.greater_than", Title: "Greater Than", Category: "logic",
			Description: "Numeric comparison a > b.",
			Inputs:      []PortSpec{req("a", "number", "Left."), req("b", "number", "Right.")},
			Outputs:     []PortSpec{{Name: "result", Type: "boolean"}},
		},
		{
			Type: "control.map", Title: "Map", Category: "control",
			Description: "Run the body subgraph once per item, gathering results.",
			Inputs: []PortSpec{
				req("items", "list<T>", "Items to iterate."),
				opt("concurrency", "number", float64(4), "Max parallel iterations."),
				opt("on_item_error", "text", "fail", "fail | skip."),
			},
			Outputs: []PortSpec{{Name: "results", Type: DynamicBody}},
		},
		{
			Type: "llm.generate", Title: "LLM", Category: "llm",
			Description: "Single LLM completion over the configured provider.",
			Inputs: []PortSpec{
				opt("instruction", "text", "", "System instruction."),
				req("input", "text", "User content."),
				opt("model", "text", "", "Model override; empty uses the default."),
			},
			Outputs: []PortSpec{{Name: "text", Type: "text"}},
		},
	}
}

// BuiltinResolver resolves the builtin node types.
func BuiltinResolver() Resolver {
	byType := map[string]Manifest{}
	for _, m := range BuiltinManifests() {
		byType[m.Type] = m
	}
	return func(nodeType string) (Manifest, bool) {
		m, ok := byType[nodeType]
		return m, ok
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/manifest.go internal/warpp/manifest_test.go
git commit -m "feat(warpp): add node manifests and builtin catalog"
```

### Task 4: Refs, dependency graph, topo order (`graph.go`)

**Files:**
- Create: `internal/warpp/graph.go`
- Test: `internal/warpp/graph_test.go`

**Interfaces:**
- Consumes: `Node`, `Input`, `Binding` (Task 2).
- Produces (used by validator and executor):
  - `type PortRef struct { Node, Port string }`
  - `func ParseRef(s string) (PortRef, error)` — splits on the FIRST dot; both halves non-empty; node half matches `^[a-zA-Z0-9_-]+$`
  - `func nodeBindings(n *Node) []Binding` — flattens every binding in `n.Inputs` (One + List + Named, deterministic order: sorted input names, then list order, then sorted named keys)
  - `func nodeDeps(n *Node, isLocal func(id string) bool) []string` — unique local node IDs referenced by `n` (including, for `control.map`, refs made anywhere inside `n.Body` that point OUTSIDE the body — the "lifting" rule, spec §6)
  - `func topoOrder(nodes []Node, deps func(n *Node) []string) ([]string, bool)` — Kahn's algorithm, ties broken by declaration order; second return false when a cycle exists
  - `const ReservedInputNode = "in"`, `const ReservedItemNode = "item"`

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/graph_test.go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ -run 'TestParseRef|TestNodeDeps|TestTopoOrder' -v`
Expected: FAIL — undefined: ParseRef, nodeDeps, topoOrder.

- [ ] **Step 3: Implement `graph.go`**

```go
// internal/warpp/graph.go
package warpp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	// ReservedInputNode exposes workflow inputs as ports ("in.<name>").
	ReservedInputNode = "in"
	// ReservedItemNode exposes the current Map iteration ("item.value",
	// "item.index") inside a body.
	ReservedItemNode = "item"
)

var nodeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// PortRef addresses one output port: "<node>.<port>".
type PortRef struct {
	Node string
	Port string
}

// ParseRef splits a ref on the first dot.
func ParseRef(s string) (PortRef, error) {
	node, port, found := strings.Cut(s, ".")
	if !found || node == "" || port == "" {
		return PortRef{}, fmt.Errorf("invalid port ref %q (want node.port)", s)
	}
	if !nodeIDPattern.MatchString(node) {
		return PortRef{}, fmt.Errorf("invalid node id in ref %q", s)
	}
	return PortRef{Node: node, Port: port}, nil
}

// nodeBindings flattens all bindings of a node in deterministic order.
func nodeBindings(n *Node) []Binding {
	names := make([]string, 0, len(n.Inputs))
	for name := range n.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Binding, 0, len(names))
	for _, name := range names {
		in := n.Inputs[name]
		if in.One != nil {
			out = append(out, *in.One)
		}
		out = append(out, in.List...)
		keys := make([]string, 0, len(in.Named))
		for k := range in.Named {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, in.Named[k])
		}
	}
	return out
}

// nodeDeps returns the unique local node ids this node depends on. For
// control.map, refs inside the body that do not resolve to body-local nodes
// (or "item") lift to become dependencies of the map node itself (spec §6).
func nodeDeps(n *Node, isLocal func(id string) bool) []string {
	seen := map[string]bool{}
	var deps []string
	add := func(id string) {
		if id != "" && !seen[id] && isLocal(id) && id != n.ID {
			seen[id] = true
			deps = append(deps, id)
		}
	}
	collect := func(bindings []Binding, bodyLocal map[string]bool) {
		for _, b := range bindings {
			if b.HasValue || b.From == "" {
				continue
			}
			ref, err := ParseRef(b.From)
			if err != nil {
				continue // validator reports malformed refs
			}
			if bodyLocal != nil && (bodyLocal[ref.Node] || ref.Node == ReservedItemNode) {
				continue
			}
			add(ref.Node)
		}
	}
	collect(nodeBindings(n), nil)
	if n.Body != nil {
		bodyLocal := map[string]bool{}
		for _, bn := range n.Body.Nodes {
			bodyLocal[bn.ID] = true
		}
		for i := range n.Body.Nodes {
			collect(nodeBindings(&n.Body.Nodes[i]), bodyLocal)
			// Nested maps lift transitively through their own bodies.
			if inner := n.Body.Nodes[i].Body; inner != nil {
				innerNode := n.Body.Nodes[i]
				for _, dep := range nodeDeps(&innerNode, func(id string) bool {
					return !bodyLocal[id] && id != ReservedItemNode
				}) {
					if !bodyLocal[dep] {
						add(dep)
					}
				}
			}
		}
		keys := make([]string, 0, len(n.Body.Outputs))
		for k := range n.Body.Outputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		bodyOut := make([]Binding, 0, len(keys))
		for _, k := range keys {
			bodyOut = append(bodyOut, n.Body.Outputs[k])
		}
		collect(bodyOut, bodyLocal)
	}
	sort.Strings(deps)
	return deps
}

// topoOrder returns node ids in dependency order (Kahn), declaration order
// as tie-break. ok=false when the graph has a cycle.
func topoOrder(nodes []Node, deps func(n *Node) []string) ([]string, bool) {
	index := make(map[string]int, len(nodes))
	for i, n := range nodes {
		index[n.ID] = i
	}
	indegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		ds := deps(n)
		indegree[n.ID] = len(ds)
		for _, d := range ds {
			dependents[d] = append(dependents[d], n.ID)
		}
	}
	var queue []string
	for _, n := range nodes {
		if indegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	order := make([]string, 0, len(nodes))
	for len(queue) > 0 {
		sort.Slice(queue, func(a, b int) bool { return index[queue[a]] < index[queue[b]] })
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, dep := range dependents[id] {
			indegree[dep]--
			if indegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	return order, len(order) == len(nodes)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/graph.go internal/warpp/graph_test.go
git commit -m "feat(warpp): add port refs, dependency lifting, and topo ordering"
```

### Task 5: Static validator (`validate.go`)

**Files:**
- Create: `internal/warpp/validate.go`
- Test: `internal/warpp/validate_test.go`

**Interfaces:**
- Consumes: Tasks 1–4 (`ParseType`, `Assignable`, `Conforms`, `InferLiteral`, `Document`, `Manifest`, `Resolver`, `ParseRef`, `nodeDeps`, `topoOrder`, `nodeIDPattern`).
- Produces:
  - `type Severity string` (`SeverityError = "error"`, `SeverityWarning = "warning"`)
  - `type Diagnostic struct { Severity Severity; Code, Message, Path string }` (JSON: `severity, code, message, path`)
  - `func HasErrors(diags []Diagnostic) bool`
  - `func Validate(doc Document, resolve Resolver) []Diagnostic`
  - `func ResolveOutputTypes(doc Document, resolve Resolver) (map[string]map[string]Type, []Diagnostic)` — resolved output-port types per root-scope node ID (includes `"in"`); used by the catalog handler and the agent `workflow_save` tool for richer diagnostics; `Validate` is implemented on top of it.

**Validation rules (complete list — each has a stable code):**

| Code | Condition |
|---|---|
| `workflow.id.required` / `workflow.id.invalid` | empty / fails `^[a-zA-Z0-9_-]+$` |
| `workflow.name.required` | empty name |
| `workflow.input.duplicate` / `workflow.input.type` / `workflow.input.default` | dup input name / bad or var/dynamic type / default fails `Conforms` |
| `node.id.required` / `node.id.invalid` / `node.id.duplicate` / `node.id.reserved` | per scope; reserved = `in`, `item` |
| `node.type.unknown` | resolver miss |
| `node.body.required` / `node.body.forbidden` | `control.map` without body / body on non-map |
| `node.policy.timeout` / `node.policy.on_error` / `node.policy.retries` / `node.policy.backoff` | timeout not a Go duration / on_error not in {"",fail,skip} / max < 0 / backoff not in {"",fixed,exponential} |
| `node.input.unknown` | binding for a port not in the manifest |
| `node.input.required` | required port with no binding and no default |
| `node.input.form` | binding form doesn't match Variadic ("", "list", "named") |
| `node.input.ref` | `from` fails `ParseRef` or doesn't resolve to a known node+port in the scope chain |
| `node.input.type_mismatch` | source type not `Assignable` to port type (concrete ports) |
| `node.input.literal` | literal fails `Conforms` and is not coercible via `Assignable(InferLiteral(v), port)` |
| `node.type_var.conflict` / `node.type_var.unresolved` | `T` unifies to two types / `T` never bound but outputs use it |
| `node.dynamic.literal_required` / `node.dynamic.enum` | `as` port wired instead of literal / literal not in {text,number,boolean,json,list<json>} |
| `map.items.type` | `items` source is not a `list<...>` |
| `map.body.outputs.single` | body outputs != exactly one key named `result` |
| `map.on_item_error.enum` | literal `on_item_error` not `fail`/`skip` |
| `workflow.graph.cycle` | topo fails in any scope (body refs lifted) |
| `workflow.output.ref` | workflow/body output `from` unresolvable |
| `workflow.outputs.empty` (warning) | `publish.tool` true and no outputs |

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/validate_test.go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ -run TestValidate -v`
Expected: FAIL — undefined: Validate, Diagnostic.

- [ ] **Step 3: Implement `validate.go`**

Implementation outline with the exact structure (the file is ~380 lines; write it exactly to these semantics — every rule in the table above, every code):

```go
// internal/warpp/validate.go
package warpp

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Path     string   `json:"path,omitempty"`
}

func HasErrors(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

type collector struct{ diags []Diagnostic }

func (c *collector) errf(code, path, format string, args ...any) {
	c.diags = append(c.diags, Diagnostic{SeverityError, code, fmt.Sprintf(format, args...), path})
}
func (c *collector) warnf(code, path, format string, args ...any) {
	c.diags = append(c.diags, Diagnostic{SeverityWarning, code, fmt.Sprintf(format, args...), path})
}

// typeScope resolves "node.port" -> concrete output type through the scope chain.
type typeScope struct {
	parent *typeScope
	types  map[string]map[string]Type // nodeID -> port -> type
}

func (s *typeScope) lookup(ref PortRef) (Type, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if ports, ok := cur.types[ref.Node]; ok {
			t, ok := ports[ref.Port]
			return t, ok
		}
	}
	return Type{}, false
}

func (s *typeScope) hasNode(id string) bool {
	for cur := s; cur != nil; cur = cur.parent {
		if _, ok := cur.types[id]; ok {
			return true
		}
	}
	return false
}
```

Then `ResolveOutputTypes(doc, resolve)`:
1. Validate metadata + workflow inputs (`workflow.*` codes). Build root scope: `types["in"] = {inputName: parsedType}`.
2. `validateScope(c, scopePath, nodes, outputs, scope, resolve)`:
   - Pass 1 over nodes: ID rules (`node.id.*`), manifest lookup (`node.type.unknown`), body presence (`node.body.required` / `node.body.forbidden`), policy checks (`node.policy.*` — `time.ParseDuration` for timeout when non-empty; enum checks).
   - Cycle check: `topoOrder(nodes, deps)` where `deps` uses `nodeDeps` with `isLocal` = scope-local IDs; on failure emit `workflow.graph.cycle` on `scopePath+".nodes"` and STOP type checking for this scope (structural diags already emitted).
   - Pass 2 in topo order: for each node call `checkNode` which (a) verifies each provided input name exists in the manifest (`node.input.unknown`), form matches Variadic (`node.input.form`); (b) resolves every binding: `from` → `ParseRef` + `scope.lookup` (`node.input.ref`), literal → `InferLiteral`; (c) unifies `T` across all var ports (`node.type_var.conflict`); (d) concrete ports: wired sources must be `Assignable` (`node.input.type_mismatch`); literals must `Conforms` OR coerce (`node.input.literal`); (e) required ports must have a binding or a manifest default (`node.input.required`); (f) `dynamic:` config ports (`as`): must be bound as a literal string (`node.dynamic.literal_required`) in the enum {text,number,boolean,json,list<json>} (`node.dynamic.enum`); (g) `control.map`: `items` source must be `list<E>` (`map.items.type`, unify `T`=E); body outputs must be exactly `{"result": ...}` (`map.body.outputs.single`); literal `on_item_error` must be fail|skip (`map.on_item_error.enum`); recurse into the body with a child scope whose `types` = body-node types plus `item` = `{value: E, index: number}`; the map node's `results` type = `list<K>` where K is the body's resolved `result` type's Kind (list results collapse to `json` since nested lists are not representable).
   - Record the node's resolved output types into the scope (substituting `T`, `dynamic:as`, `dynamic:body`). If an output uses `T` and `T` never bound → `node.type_var.unresolved`.
3. Scope `outputs` (workflow root and map bodies): every `from` must resolve (`workflow.output.ref`).
4. Root only: `publish.tool && len(doc.Outputs)==0` → `workflow.outputs.empty` warning.

`Validate` calls `ResolveOutputTypes` and returns the diagnostics.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/ -v`
Expected: PASS (all validator cases green).

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/validate.go internal/warpp/validate_test.go
git commit -m "feat(warpp): add static validator with typed-port checking and unification"
```

### Task 6: Builtin node runners + LLM runner (`runner.go`)

**Files:**
- Create: `internal/warpp/runner.go`
- Test: `internal/warpp/runner_test.go`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces:
  - `type NodeInputs struct { Values map[string]Value; List map[string][]Value; Named map[string]map[string]Value }`
  - `type RunnerCtx struct { Path string; Node *Node; Manifest Manifest }`
  - `type NodeRunner func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error)`
  - `func BuiltinRunners() map[string]NodeRunner` — every `data.*` and `logic.*` type (control.map and llm.generate are NOT in this map; the executor owns map, the service injects LLM)
  - `type ChatFunc func(ctx context.Context, instruction, input, model string) (string, error)`
  - `func LLMRunner(chat ChatFunc) NodeRunner` — registered under `llm.generate` by the service
  - `func SelectPath(root any, path string) (any, bool)` — dot/index traversal over structured data ONLY (no string re-parsing — spec §1 kills that behavior)
  - `func CoerceRaw(data any, to Type) (Value, error)` — `Conforms` → wrap as-is; else `Coerce(Value{InferLiteral(data), data}, to)`
  - `func renderScalar(v Value) string` — text/file verbatim, number/boolean via stringify, json compact-marshaled (helper used by template/stringify)
- Key semantics: `logic.if` returns ONLY the fired port in its output map (absent port = never fired — the executor's skip machinery keys off this).

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/runner_test.go
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
	// missing path must error
	in.Values["path"] = NewText("results.9.title")
	r := BuiltinRunners()["data.extract"]
	if _, err := r(context.Background(), RunnerCtx{}, in); err == nil {
		t.Fatal("missing path must error")
	}
	// as mismatch must error
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
	// Strings are NOT re-parsed mid-path (the old system's sin).
	if _, ok := SelectPath(root, "s.embedded"); ok {
		t.Fatal("string values must not be json-parsed during traversal")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ -run 'TestExtract|TestTemplate|TestMerge|TestLogic|TestLLM|TestSelectPath' -v`
Expected: FAIL — undefined: BuiltinRunners, LLMRunner, SelectPath.

- [ ] **Step 3: Implement `runner.go`**

Complete semantics (write exactly):

```go
// internal/warpp/runner.go
package warpp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type NodeInputs struct {
	Values map[string]Value
	List   map[string][]Value
	Named  map[string]map[string]Value
}

type RunnerCtx struct {
	Path     string
	Node     *Node
	Manifest Manifest
}

type NodeRunner func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error)

// SelectPath traverses structured data by dot/index path. It never parses
// strings mid-path.
func SelectPath(root any, path string) (any, bool) {
	cur := root
	if strings.TrimSpace(path) == "" {
		return cur, true
	}
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[part]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// CoerceRaw types a raw JSON value against a concrete port type, applying the
// implicit coercion table when needed.
func CoerceRaw(data any, to Type) (Value, error) {
	if err := Conforms(data, to); err == nil {
		return Value{Type: to, Data: data}, nil
	}
	return Coerce(Value{Type: InferLiteral(data), Data: data}, to)
}

func renderScalar(v Value) string { /* text/file: as-is; number/boolean via
	stringify(); json: compact json.Marshal; list: compact json.Marshal */ }

func parseAs(in NodeInputs) (Type, error) { /* read in.Values["as"] string,
	allow exactly text|number|boolean|json|list<json>, ParseType */ }
```

Then `BuiltinRunners()` returning the map with all runners implemented to the tested semantics:
- `data.extract`: `SelectPath(source.Data, path)` → missing → `fmt.Errorf("path not found: %s", path)`; then `CoerceRaw(found, asType)`.
- `data.template`: for each `{name}` present in vars replace with `renderScalar`; unresolved `{name}` slots (no var supplied) → error `fmt.Errorf("template var %q not provided", name)` (find via regexp `\{([a-zA-Z0-9_]+)\}`).
- `data.merge`: every input must be `map[string]any` (else error); shallow merge in order.
- `data.stringify`: json → `json.MarshalIndent(v.Data, "", "  ")`; scalars via `stringify`.
- `data.parse`: `json.Unmarshal([]byte(text))` strict; output json.
- `data.constant`: `CoerceRaw(value.Data, asType)`.
- `logic.if`: return map containing ONLY `then` or ONLY `else` (the input `value` unchanged).
- `logic.coalesce`: `in.List["values"][0]` (executor guarantees only fired values are present; empty → executor skips the node before calling).
- `logic.equals`: `reflect.DeepEqual(a.Data, b.Data)`.
- `logic.contains`/`logic.not`/`logic.greater_than`: direct.
- `LLMRunner(chat)`: pull instruction/input/model strings, call chat, output `{"text": NewText(result)}`; chat error propagates.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/runner.go internal/warpp/runner_test.go
git commit -m "feat(warpp): add builtin data/logic runners and LLM runner"
```

### Task 7: Executor — wavefront, skip cascade, policies, events (`executor.go`)

**Files:**
- Create: `internal/warpp/executor.go`
- Test: `internal/warpp/executor_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces:
  - `type EventType string` with consts `EventRunStarted "run_started"`, `EventRunCompleted "run_completed"`, `EventRunFailed "run_failed"`, `EventRunCancelled "run_cancelled"`, `EventNodeStarted "node_started"`, `EventNodeCompleted "node_completed"`, `EventNodeFailed "node_failed"`, `EventNodeSkipped "node_skipped"`, `EventNodeRetrying "node_retrying"`
  - `type Event struct { RunID string; Sequence int64; Type EventType; NodePath string; Status string; Message string; Outputs map[string]any; Error string; OccurredAt time.Time }` (JSON: `run_id, sequence, type, node_path, status, message, outputs, error, occurred_at`)
  - `type StepFunc func(ctx context.Context, key string, fn func(context.Context) (map[string]Value, error)) (map[string]Value, error)`
  - `type Engine struct { Resolve Resolver; Runners map[string]NodeRunner; Emit func(Event); Step StepFunc; MaxConcurrency int }`
  - `type Result struct { Status string; Outputs map[string]any; Err error }`
  - `func (e *Engine) Execute(ctx context.Context, doc Document, input map[string]any) Result`
  - Statuses: `StatusRunning/StatusCompleted/StatusCompletedWithSkips/StatusFailed/StatusCancelled` string consts (`"running"`, `"completed"`, `"completed_with_skips"`, `"failed"`, `"cancelled"`)
  - Internal (consumed by Task 8): `type execScope struct { parent *execScope; outputs map[string]map[string]Value; terminal, skipped map[string]bool }` with `lookup(ref PortRef) (Value, bool /*fired*/, bool /*known*/)`; `func (e *Engine) runScope(ctx context.Context, nodes []Node, scope *execScope, pathPrefix string, defaults Policy, maxConc int) (anySkipped bool, err error)`; `func (e *Engine) resolveInputs(node *Node, m Manifest, scope *execScope) (NodeInputs, bool /*skip*/, error)`
- Durable rule: `e.Step` (when non-nil) wraps each NON-map node execution with key `"node:" + path` around the full retry loop; `control.map` nodes are never step-wrapped (their body nodes are, via prefixed paths — Task 8).

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/executor_test.go
package warpp

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type capture struct{ events []Event }

func (c *capture) emit(ev Event) { c.events = append(c.events, ev) }
func (c *capture) byType(t EventType) []Event {
	var out []Event
	for _, ev := range c.events {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}
func (c *capture) nodeEvent(t EventType, path string) *Event {
	for i := range c.events {
		if c.events[i].Type == t && c.events[i].NodePath == path {
			return &c.events[i]
		}
	}
	return nil
}

func testEngine(cap *capture, extra map[string]NodeRunner) *Engine {
	runners := BuiltinRunners()
	for k, v := range extra {
		runners[k] = v
	}
	return &Engine{
		Resolve:        BuiltinResolver(),
		Runners:        runners,
		Emit:           cap.emit,
		MaxConcurrency: 4,
	}
}

func exec(t *testing.T, e *Engine, src string, input map[string]any) Result {
	t.Helper()
	var doc Document
	if err := json.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("doc: %v", err)
	}
	if diags := Validate(doc, e.Resolve); HasErrors(diags) {
		t.Fatalf("doc invalid: %+v", diags)
	}
	return e.Execute(context.Background(), doc, input)
}

func TestExecuteLinearPipeline(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"lin","name":"lin",
	  "inputs":[{"name":"topic","type":"text","required":true}],
	  "nodes":[
	    {"id":"tpl","type":"data.template",
	     "inputs":{"template":{"value":"about {t}"},
	               "vars":{"t":{"from":"in.topic"}}}}],
	  "outputs":{"out":{"from":"tpl.text"}}}`,
		map[string]any{"topic": "go"})
	if res.Err != nil || res.Status != StatusCompleted {
		t.Fatalf("res=%+v", res)
	}
	if res.Outputs["out"] != "about go" {
		t.Fatalf("outputs=%v", res.Outputs)
	}
	done := cap.nodeEvent(EventNodeCompleted, "tpl")
	if done == nil || done.Outputs["text"] != "about go" {
		t.Fatalf("node_completed must carry port outputs: %+v", done)
	}
	if len(cap.byType(EventRunCompleted)) != 1 {
		t.Fatal("run_completed missing")
	}
}

func TestExecuteMissingRequiredInputFails(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"x","name":"x",
	  "inputs":[{"name":"topic","type":"text","required":true}],
	  "nodes":[{"id":"s","type":"data.stringify","inputs":{"value":{"from":"in.topic"}}}]}`,
		map[string]any{})
	if res.Status != StatusFailed || res.Err == nil {
		t.Fatalf("expected failed, got %+v", res)
	}
}

func TestIfSkipCascadeAndCoalesce(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"br","name":"br",
	  "inputs":[{"name":"flag","type":"boolean","required":true}],
	  "nodes":[
	    {"id":"gate","type":"logic.if",
	     "inputs":{"condition":{"from":"in.flag"},"value":{"value":"yes"}}},
	    {"id":"up","type":"data.stringify","inputs":{"value":{"from":"gate.then"}}},
	    {"id":"down","type":"data.stringify","inputs":{"value":{"from":"gate.else"}}},
	    {"id":"after_down","type":"data.stringify","inputs":{"value":{"from":"down.text"}}},
	    {"id":"join","type":"logic.coalesce",
	     "inputs":{"values":[{"from":"up.text"},{"from":"after_down.text"}]}}],
	  "outputs":{"result":{"from":"join.value"}}}`,
		map[string]any{"flag": true})
	if res.Status != StatusCompletedWithSkips {
		t.Fatalf("status=%s err=%v", res.Status, res.Err)
	}
	if res.Outputs["result"] != "yes" {
		t.Fatalf("outputs=%v", res.Outputs)
	}
	if cap.nodeEvent(EventNodeSkipped, "down") == nil ||
		cap.nodeEvent(EventNodeSkipped, "after_down") == nil {
		t.Fatal("else branch must cascade-skip")
	}
	if cap.nodeEvent(EventNodeCompleted, "join") == nil {
		t.Fatal("coalesce must still complete")
	}
}

func TestOptionalInputSourceSkippedUsesDefault(t *testing.T) {
	// Custom node type with an optional input that has a default.
	man := Manifest{Type: "test.opt", Title: "Opt", Category: "test",
		Inputs: []PortSpec{
			{Name: "maybe", Type: "text", Default: "fallback"},
		},
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	resolve := ChainResolvers(func(nt string) (Manifest, bool) {
		if nt == "test.opt" {
			return man, true
		}
		return Manifest{}, false
	}, BuiltinResolver())
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.opt": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			return map[string]Value{"out": in.Values["maybe"]}, nil
		}})
	e.Resolve = resolve
	res := exec(t, e, `{
	  "id":"opt","name":"opt",
	  "inputs":[{"name":"flag","type":"boolean","required":true}],
	  "nodes":[
	    {"id":"gate","type":"logic.if",
	     "inputs":{"condition":{"from":"in.flag"},"value":{"value":"wired"}}},
	    {"id":"n","type":"test.opt","inputs":{"maybe":{"from":"gate.then"}}}],
	  "outputs":{"o":{"from":"n.out"}}}`,
		map[string]any{"flag": false})
	if res.Status != StatusCompletedWithSkips || res.Outputs["o"] != "fallback" {
		t.Fatalf("res=%+v", res)
	}
}

func TestRetriesThenSuccess(t *testing.T) {
	var calls atomic.Int32
	man := Manifest{Type: "test.flaky", Title: "F", Category: "test",
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.flaky": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			if calls.Add(1) < 3 {
				return nil, errors.New("boom")
			}
			return map[string]Value{"out": NewText("ok")}, nil
		}})
	e.Resolve = ChainResolvers(func(nt string) (Manifest, bool) {
		if nt == "test.flaky" {
			return man, true
		}
		return Manifest{}, false
	}, BuiltinResolver())
	res := exec(t, e, `{"id":"r","name":"r","nodes":[
	  {"id":"f","type":"test.flaky","policy":{"retries":{"max":2,"backoff":"fixed"}}}],
	  "outputs":{"o":{"from":"f.out"}}}`, nil)
	if res.Status != StatusCompleted || calls.Load() != 3 {
		t.Fatalf("res=%+v calls=%d", res, calls.Load())
	}
	if len(cap.byType(EventNodeRetrying)) != 2 {
		t.Fatalf("expected 2 retry events, got %d", len(cap.byType(EventNodeRetrying)))
	}
}

func TestOnErrorSkipContinuesRun(t *testing.T) {
	man := Manifest{Type: "test.fail", Title: "F", Category: "test",
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.fail": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			return nil, errors.New("always")
		}})
	e.Resolve = ChainResolvers(func(nt string) (Manifest, bool) {
		if nt == "test.fail" {
			return man, true
		}
		return Manifest{}, false
	}, BuiltinResolver())
	res := exec(t, e, `{"id":"s","name":"s","nodes":[
	  {"id":"bad","type":"test.fail","policy":{"on_error":"skip"}},
	  {"id":"dep","type":"data.stringify","inputs":{"value":{"from":"bad.out"}}},
	  {"id":"solo","type":"data.constant","inputs":{"value":{"value":"alive"},"as":{"value":"text"}}}],
	  "outputs":{"o":{"from":"solo.value"}}}`, nil)
	if res.Status != StatusCompletedWithSkips {
		t.Fatalf("res=%+v", res)
	}
	if res.Outputs["o"] != "alive" {
		t.Fatalf("outputs=%v", res.Outputs)
	}
	if cap.nodeEvent(EventNodeFailed, "bad") == nil || cap.nodeEvent(EventNodeSkipped, "dep") == nil {
		t.Fatal("failed node must emit node_failed and cascade-skip dependents")
	}
}

func TestFatalErrorFailsRun(t *testing.T) {
	man := Manifest{Type: "test.fail", Title: "F", Category: "test",
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.fail": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			return nil, errors.New("fatal")
		}})
	e.Resolve = ChainResolvers(func(nt string) (Manifest, bool) {
		if nt == "test.fail" {
			return man, true
		}
		return Manifest{}, false
	}, BuiltinResolver())
	res := exec(t, e, `{"id":"f","name":"f","nodes":[
	  {"id":"bad","type":"test.fail"}]}`, nil)
	if res.Status != StatusFailed || res.Err == nil {
		t.Fatalf("res=%+v", res)
	}
	if len(cap.byType(EventRunFailed)) != 1 {
		t.Fatal("run_failed missing")
	}
}

func TestTimeoutPolicy(t *testing.T) {
	man := Manifest{Type: "test.slow", Title: "S", Category: "test",
		Outputs: []PortSpec{{Name: "out", Type: "text"}}}
	cap := &capture{}
	e := testEngine(cap, map[string]NodeRunner{
		"test.slow": func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				return map[string]Value{"out": NewText("late")}, nil
			}
		}})
	e.Resolve = ChainResolvers(func(nt string) (Manifest, bool) {
		if nt == "test.slow" {
			return man, true
		}
		return Manifest{}, false
	}, BuiltinResolver())
	start := time.Now()
	res := exec(t, e, `{"id":"t","name":"t","nodes":[
	  {"id":"slow","type":"test.slow","policy":{"timeout":"50ms"}}]}`, nil)
	if res.Status != StatusFailed {
		t.Fatalf("res=%+v", res)
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout did not apply")
	}
}

func TestStepFuncWrapsNodesWithPathKeys(t *testing.T) {
	var keys []string
	cap := &capture{}
	e := testEngine(cap, nil)
	e.Step = func(ctx context.Context, key string, fn func(context.Context) (map[string]Value, error)) (map[string]Value, error) {
		keys = append(keys, key)
		return fn(ctx)
	}
	res := exec(t, e, `{"id":"d","name":"d","nodes":[
	  {"id":"c","type":"data.constant","inputs":{"value":{"value":"x"},"as":{"value":"text"}}},
	  {"id":"s","type":"data.stringify","inputs":{"value":{"from":"c.value"}}}],
	  "outputs":{"o":{"from":"s.text"}}}`, nil)
	if res.Status != StatusCompleted {
		t.Fatalf("res=%+v", res)
	}
	want := map[string]bool{"node:c": true, "node:s": true}
	for _, k := range keys {
		delete(want, k)
	}
	if len(want) != 0 {
		t.Fatalf("missing step keys %v (got %v)", want, keys)
	}
}

func TestDiamondJoinBothBranchesFeedOneNode(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"dia","name":"dia",
	  "inputs":[{"name":"x","type":"text","required":true}],
	  "nodes":[
	    {"id":"l","type":"data.template","inputs":{"template":{"value":"L{v}"},"vars":{"v":{"from":"in.x"}}}},
	    {"id":"r","type":"data.template","inputs":{"template":{"value":"R{v}"},"vars":{"v":{"from":"in.x"}}}},
	    {"id":"joinT","type":"data.template",
	     "inputs":{"template":{"value":"{a}|{b}"},
	               "vars":{"a":{"from":"l.text"},"b":{"from":"r.text"}}}}],
	  "outputs":{"o":{"from":"joinT.text"}}}`,
		map[string]any{"x": "1"})
	if res.Status != StatusCompleted || res.Outputs["o"] != "L1|R1" {
		t.Fatalf("res=%+v", res)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ -run 'TestExecute|TestIf|TestOptional|TestRetries|TestOnError|TestFatal|TestTimeout|TestStepFunc|TestDiamond' -v`
Expected: FAIL — undefined: Engine, Event, Result.

- [ ] **Step 3: Implement `executor.go`**

Write to exactly these semantics (~340 lines). Core structure:

```go
// internal/warpp/executor.go
package warpp

import (
	"context"
	"fmt"
	"sort"
	"time"
)

type EventType string

const (
	EventRunStarted    EventType = "run_started"
	EventRunCompleted  EventType = "run_completed"
	EventRunFailed     EventType = "run_failed"
	EventRunCancelled  EventType = "run_cancelled"
	EventNodeStarted   EventType = "node_started"
	EventNodeCompleted EventType = "node_completed"
	EventNodeFailed    EventType = "node_failed"
	EventNodeSkipped   EventType = "node_skipped"
	EventNodeRetrying  EventType = "node_retrying"
)

const (
	StatusRunning            = "running"
	StatusCompleted          = "completed"
	StatusCompletedWithSkips = "completed_with_skips"
	StatusFailed             = "failed"
	StatusCancelled          = "cancelled"
)

type Event struct {
	RunID      string         `json:"run_id"`
	Sequence   int64          `json:"sequence"`
	Type       EventType      `json:"type"`
	NodePath   string         `json:"node_path,omitempty"`
	Status     string         `json:"status,omitempty"`
	Message    string         `json:"message,omitempty"`
	Outputs    map[string]any `json:"outputs,omitempty"`
	Error      string         `json:"error,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type StepFunc func(ctx context.Context, key string, fn func(context.Context) (map[string]Value, error)) (map[string]Value, error)

type Engine struct {
	Resolve        Resolver
	Runners        map[string]NodeRunner
	Emit           func(Event)
	Step           StepFunc
	MaxConcurrency int
}

type Result struct {
	Status  string
	Outputs map[string]any
	Err     error
}
```

Key implementation rules:
1. `emit` helper stamps `OccurredAt: time.Now().UTC()` and nil-checks `e.Emit`. Sequence stays 0 — the service runtime assigns it.
2. `Execute`: emit `run_started`; build the root `execScope` with `outputs["in"]` from `doc.Inputs` + `input` map — for each declared input: present → `CoerceRaw(value, parsedType)` (error → run fails), absent with Default → default, absent required → run fails with `fmt.Errorf("input %q required", name)`. Then `runScope(ctx, doc.Nodes, root, "", doc.Settings.DefaultPolicy, maxConc)`. Afterwards: collect `doc.Outputs` (literal → as-is; `from` → include only when fired). Status decision: fatal error → `StatusFailed` + `run_failed` event; `ctx.Err() != nil` → `StatusCancelled` + `run_cancelled`; anySkipped → `StatusCompletedWithSkips`; else `StatusCompleted`. The final run event carries `Status` and, on success, `Outputs`.
3. `runScope` scheduling: compute `deps` via `nodeDeps` with `isLocal` = scope node IDs; `remaining` counts; ready queue sorted by declaration index; launch up to `maxConc` goroutines; each sends `(nodeID, outputs, skip, err)` on a result channel; on result: mark state, emit event, decrement dependents, enqueue newly-ready. Fatal error (on_error=fail after retries, or input resolution error) cancels the scope context and drains in-flight nodes before returning.
4. `resolveInputs(node, manifest, scope)` — implements the spec §7 skip rule exactly:
   - scalar wired: `lookup` → fired: `CoerceRaw`-style assignment (concrete port: coerce; var port: pass through); not fired: required → `skip=true`; optional → Default if non-nil else omit.
   - scalar literal: concrete port → `CoerceRaw` (validator guarantees, but re-check); var port → `Value{InferLiteral, data}`.
   - unbound optional with Default → include default; unbound required → error (validator prevents; belt-and-braces).
   - list variadic: collect FIRED entries in binding order (literals always fire); `Required && len==0` → skip.
   - named variadic: every entry must fire; any unfired → skip.
5. `executeNode`: `resolveInputs` → skip → emit `node_skipped` (no `node_started`), mark skipped. Else emit `node_started`; effective policy = node.Policy over defaults (empty fields inherit; OnError default `"fail"`); `control.map` → `e.runMapNode(...)` (Task 8, NOT step-wrapped); other types → look up runner (missing → error `no runner for type %s`); wrap the retry loop in `e.Step(ctx, "node:"+path, ...)` when `e.Step != nil`; retry loop: attempts = 1+max, `node_retrying` between attempts, backoff base 200ms (`fixed` constant, `exponential` doubles), timeout via `context.WithTimeout` per attempt when policy.Timeout parses.
6. On node error after retries: `on_error == "skip"` → emit `node_failed` (message "node failed; downstream will skip"), mark as skipped-for-cascade (`failedContinue` semantics — terminal + not fired). Else → fatal.
7. Skip cascade is NOT precomputed: it emerges from the readiness rule — every dependent still runs `resolveInputs`, sees its required source unfired, and skips. (This handles `logic.if`'s partially-fired outputs uniformly.)
8. `execScope.lookup` walks the parent chain: fired = node terminal AND port present in outputs map.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/ -v`
Expected: PASS. Also run with `-race`: `go test ./internal/warpp/ -race`
Expected: PASS, no data races.

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/executor.go internal/warpp/executor_test.go
git commit -m "feat(warpp): add wavefront executor with skip cascade, policies, and events"
```

### Task 8: Map fan-out (`executor_map.go`)

**Files:**
- Create: `internal/warpp/executor_map.go`
- Test: `internal/warpp/executor_map_test.go`

**Interfaces:**
- Consumes: Task 7 internals (`execScope`, `runScope`, `resolveInputs`, emit helpers).
- Produces: `func (e *Engine) runMapNode(ctx context.Context, node *Node, in NodeInputs, scope *execScope, path string, defaults Policy) (map[string]Value, error)` — called by `executeNode` for `control.map`.
- Semantics (spec §6): iterations run concurrently up to `concurrency`; each iteration `i` executes `node.Body.Nodes` in a child scope whose extra node `item` has outputs `{value: items[i] (typed list-elem), index: number(i)}` and whose parent chain is the map's enclosing scope (lexical refs). Body node event paths and step keys are prefixed `"<mapPath>[<i>]."`. Body output binding `result`: fired → kept; unfired → item skipped. Item failure: `on_item_error == "skip"` → item omitted, run continues (map reports anySkipped); else map node fails with `fmt.Errorf("item %d: %w", i, err)`. Results keep item order, skipped items omitted. Output: `{"results": Value{list<inferred>, gathered}}`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/executor_map_test.go
package warpp

import (
	"testing"
)

func TestMapFanOutGathersInOrder(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"m","name":"m",
	  "inputs":[{"name":"names","type":"list<text>","required":true},
	            {"name":"suffix","type":"text","required":true}],
	  "nodes":[
	    {"id":"per","type":"control.map",
	     "inputs":{"items":{"from":"in.names"},"concurrency":{"value":3}},
	     "body":{"nodes":[
	        {"id":"t","type":"data.template",
	         "inputs":{"template":{"value":"{n}-{s}"},
	                   "vars":{"n":{"from":"item.value"},"s":{"from":"in.suffix"}}}}],
	      "outputs":{"result":{"from":"t.text"}}}}],
	  "outputs":{"all":{"from":"per.results"}}}`,
		map[string]any{"names": []any{"a", "b", "c"}, "suffix": "z"})
	if res.Err != nil || res.Status != StatusCompleted {
		t.Fatalf("res=%+v", res)
	}
	got, _ := res.Outputs["all"].([]any)
	if len(got) != 3 || got[0] != "a-z" || got[1] != "b-z" || got[2] != "c-z" {
		t.Fatalf("results=%v", got)
	}
	// child events carry iteration paths
	if cap.nodeEvent(EventNodeCompleted, "per[1].t") == nil {
		t.Fatal("body events must be path-prefixed per iteration")
	}
}

func TestMapOnItemErrorSkipOmitsItem(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"m2","name":"m2",
	  "inputs":[{"name":"docs","type":"list<text>","required":true}],
	  "nodes":[
	    {"id":"per","type":"control.map",
	     "inputs":{"items":{"from":"in.docs"},"on_item_error":{"value":"skip"}},
	     "body":{"nodes":[
	        {"id":"p","type":"data.parse","inputs":{"text":{"from":"item.value"}}}],
	      "outputs":{"result":{"from":"p.json"}}}}],
	  "outputs":{"parsed":{"from":"per.results"}}}`,
		map[string]any{"docs": []any{`{"n":1}`, `broken`, `{"n":3}`}})
	if res.Status != StatusCompletedWithSkips {
		t.Fatalf("res=%+v", res)
	}
	got, _ := res.Outputs["parsed"].([]any)
	if len(got) != 2 {
		t.Fatalf("skipped item must be omitted, got %v", got)
	}
}

func TestMapOnItemErrorFailFailsRun(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"m3","name":"m3",
	  "inputs":[{"name":"docs","type":"list<text>","required":true}],
	  "nodes":[
	    {"id":"per","type":"control.map",
	     "inputs":{"items":{"from":"in.docs"}},
	     "body":{"nodes":[
	        {"id":"p","type":"data.parse","inputs":{"text":{"from":"item.value"}}}],
	      "outputs":{"result":{"from":"p.json"}}}}]}`,
		map[string]any{"docs": []any{`broken`}})
	if res.Status != StatusFailed {
		t.Fatalf("res=%+v", res)
	}
}

func TestNestedMap(t *testing.T) {
	cap := &capture{}
	res := exec(t, testEngine(cap, nil), `{
	  "id":"nest","name":"nest",
	  "inputs":[{"name":"grid","type":"list<json>","required":true}],
	  "nodes":[
	    {"id":"rows","type":"control.map",
	     "inputs":{"items":{"from":"in.grid"}},
	     "body":{"nodes":[
	        {"id":"cells","type":"control.map",
	         "inputs":{"items":{"from":"item.value"}},
	         "body":{"nodes":[
	            {"id":"s","type":"data.stringify","inputs":{"value":{"from":"item.value"}}}],
	          "outputs":{"result":{"from":"s.text"}}}}],
	      "outputs":{"result":{"from":"cells.results"}}}}],
	  "outputs":{"o":{"from":"rows.results"}}}`,
		map[string]any{"grid": []any{[]any{1.0, 2.0}, []any{3.0}}})
	if res.Err != nil {
		t.Fatalf("res=%+v", res)
	}
	rows, _ := res.Outputs["o"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows=%v", rows)
	}
	first, _ := rows[0].([]any)
	if len(first) != 2 || first[0] != "1" {
		t.Fatalf("inner=%v", first)
	}
	if cap.nodeEvent(EventNodeCompleted, "rows[0].cells[1].s") == nil {
		t.Fatal("nested iteration paths must compose")
	}
}

// TestMapStepKeysIncludeIteration is below — it asserts durable step keys
// carry iteration indexes and that the map node itself is never wrapped.
```

```go
func TestMapStepKeysIncludeIteration(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	cap := &capture{}
	e := testEngine(cap, nil)
	e.Step = func(ctx context.Context, key string, fn func(context.Context) (map[string]Value, error)) (map[string]Value, error) {
		mu.Lock()
		keys = append(keys, key)
		mu.Unlock()
		return fn(ctx)
	}
	res := exec(t, e, `{
	  "id":"m","name":"m",
	  "inputs":[{"name":"names","type":"list<text>","required":true}],
	  "nodes":[
	    {"id":"per","type":"control.map",
	     "inputs":{"items":{"from":"in.names"}},
	     "body":{"nodes":[
	        {"id":"t","type":"data.stringify","inputs":{"value":{"from":"item.value"}}}],
	      "outputs":{"result":{"from":"t.text"}}}}],
	  "outputs":{"o":{"from":"per.results"}}}`,
		map[string]any{"names": []any{"a", "b"}})
	if res.Err != nil {
		t.Fatalf("res=%+v", res)
	}
	want := map[string]bool{"node:per[0].t": true, "node:per[1].t": true}
	for _, k := range keys {
		if k == "node:per" {
			t.Fatal("map node itself must not be step-wrapped")
		}
		delete(want, k)
	}
	if len(want) != 0 {
		t.Fatalf("missing keys %v in %v", want, keys)
	}
}
```

(Requires `"sync"` and `"context"` imports in the test file; drop the illustrative broken variant entirely.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ -run TestMap -v` and `go test ./internal/warpp/ -run TestNested -v`
Expected: FAIL — `control.map` has no execution path yet.

- [ ] **Step 3: Implement `executor_map.go`**

```go
// internal/warpp/executor_map.go
package warpp

import (
	"context"
	"fmt"
	"sync"
)

func (e *Engine) runMapNode(ctx context.Context, node *Node, in NodeInputs, scope *execScope, path string, defaults Policy) (map[string]Value, error) {
	itemsVal, ok := in.Values["items"]
	if !ok {
		return nil, fmt.Errorf("map %s: items input missing", node.ID)
	}
	items, ok := itemsVal.Data.([]any)
	if !ok {
		return nil, fmt.Errorf("map %s: items is not a list", node.ID)
	}
	elem := Type{Kind: itemsVal.Type.Elem}
	if elem.Kind == "" || elem.Kind == KindVar {
		elem = Type{Kind: KindJSON}
	}
	concurrency := 4
	if c, ok := in.Values["concurrency"]; ok {
		if n, ok := asNumber(c.Data); ok && n >= 1 {
			concurrency = int(n)
		}
	}
	onItemError := "fail"
	if v, ok := in.Values["on_item_error"]; ok {
		if s, ok := v.Data.(string); ok && s != "" {
			onItemError = s
		}
	}

	type itemResult struct {
		value   Value
		fired   bool
		skipped bool
		err     error
	}
	results := make([]itemResult, len(items))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	iterCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i := range items {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-iterCtx.Done():
				results[i] = itemResult{err: iterCtx.Err()}
				return
			}
			itemScope := &execScope{
				parent: scope,
				outputs: map[string]map[string]Value{
					ReservedItemNode: {
						"value": Value{Type: elem, Data: items[i]},
						"index": Value{Type: Type{Kind: KindNumber}, Data: float64(i)},
					},
				},
				terminal: map[string]bool{ReservedItemNode: true},
				skipped:  map[string]bool{},
			}
			prefix := fmt.Sprintf("%s[%d].", path, i)
			bodySkipped, err := e.runScope(iterCtx, node.Body.Nodes, itemScope, prefix, defaults, e.maxConc())
			if err != nil {
				results[i] = itemResult{err: err}
				if onItemError != "skip" {
					cancel()
				}
				return
			}
			_ = bodySkipped
			binding := node.Body.Outputs["result"]
			switch {
			case binding.HasValue:
				results[i] = itemResult{value: Value{Type: InferLiteral(binding.Value), Data: binding.Value}, fired: true}
			default:
				ref, refErr := ParseRef(binding.From)
				if refErr != nil {
					results[i] = itemResult{err: refErr}
					return
				}
				v, fired, known := itemScope.lookup(ref)
				if !known || !fired {
					results[i] = itemResult{skipped: true}
					return
				}
				results[i] = itemResult{value: v, fired: true}
			}
		}(i)
	}
	wg.Wait()

	gathered := make([]any, 0, len(items))
	anySkipped := false
	for i, r := range results {
		if r.err != nil {
			if onItemError == "skip" {
				anySkipped = true
				continue
			}
			return nil, fmt.Errorf("item %d: %w", i, r.err)
		}
		if !r.fired {
			anySkipped = true
			continue
		}
		gathered = append(gathered, r.value.Data)
	}
	if anySkipped {
		e.noteSkipped() // see step note below
	}
	outType := InferLiteral(gathered)
	return map[string]Value{"results": {Type: outType, Data: gathered}}, nil
}
```

**Integration notes for this step (concrete, not optional):**
- `executeNode` (Task 7) must route `node.Type == "control.map"` here BEFORE the runner lookup and WITHOUT `e.Step` wrapping.
- `e.maxConc()` is a small helper on Engine returning `MaxConcurrency` (default 4 when <= 0) — add it in `executor.go` if Task 7 didn't.
- `e.noteSkipped()`: the "anySkipped" signal must propagate to the run status. Implement it as a field on the per-run state, not the Engine (Engine is shared): have `runScope` return `anySkipped` and have `runMapNode` return a third value OR record skipped-ness via the scope. Concretely: change `runMapNode` to return `(map[string]Value, bool /*anySkipped*/, error)` and have `executeNode` OR the boolean into the scope-run's skip accumulator. Delete the `noteSkipped` call sketch above in favor of that return value.
- Both `runScope` and `runMapNode` must tolerate `Emit` being called concurrently — the service serializes appends, but the engine's own `capture` in tests appends from one goroutine only if `runScope` sends events from its scheduler loop. Keep ALL `emit` calls on the scheduler goroutine of each scope (results funnel through the result channel), and note that concurrent map iterations each run their own scheduler — so `Emit` implementations must be goroutine-safe; the test `capture` gets a mutex.

Update the Task 7/8 test helper `capture` to be mutex-guarded:

```go
type capture struct {
	mu     sync.Mutex
	events []Event
}

func (c *capture) emit(ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}
```

(and guard the readers similarly).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/ -race -v`
Expected: PASS, including all Task 7 tests, no races.

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/executor.go internal/warpp/executor_map.go internal/warpp/executor_map_test.go internal/warpp/executor_test.go
git commit -m "feat(warpp): add control.map fan-out with lexical scoping and durable iteration keys"
```

### Task 9: Tool adapters (`toolnode` package)

**Files:**
- Create: `internal/warpp/toolnode/toolnode.go`
- Create: `internal/warpp/toolnode/adapters.go`
- Test: `internal/warpp/toolnode/toolnode_test.go`

**Interfaces:**
- Consumes: `manifold/internal/warpp` (Value/Type/Manifest/NodeRunner/NodeInputs/SelectPath/CoerceRaw), `manifold/internal/tools` (`tools.Registry`).
- Produces:
  - `type ArgMap struct { Port, Arg string }` (empty `Arg` means same as `Port`)
  - `type OutMap struct { Port, Path string }` (`Path ""` = whole parsed result)
  - `type Adapter struct { NodeType, Tool string; Manifest warpp.Manifest; Args []ArgMap; Outs []OutMap; Post func(map[string]any) map[string]any }`
  - `func Builtin() []Adapter` — the 9 curated adapters
  - `func Manifests(adapters []Adapter) []warpp.Manifest` (includes `GenericManifest()`)
  - `func Resolver(adapters []Adapter) warpp.Resolver`
  - `func Runners(reg tools.Registry, adapters []Adapter) map[string]warpp.NodeRunner` (includes `tool.generic`)
  - `func GenericManifest() warpp.Manifest` — type `tool.generic`, inputs `tool: text` req + `args: json` default `{}`, output `result: json`

**Curated adapter table (exact contract).** Before finalizing each adapter's `Outs`, read the named tool source file and confirm the result field names; the table below records the expected fields and where to verify:

| NodeType | Tool | Inputs (port: type) | Outs (port ← result path) | Verify in |
|---|---|---|---|---|
| `tool.web_search` | `web_search` | `query: text` req; `max_results: number` def 5 | `results: list<json>` ← `results`; `results_text: text` ← `results_text` (added by Post); `raw: json` ← whole | `internal/tools/web/search.go` (`{"ok":true,"results":[{title,url}]}`) |
| `tool.web_fetch` | `web_fetch` | `url: text` req | `markdown: text` ← `markdown`; `url: text` ← `url`; `raw: json` ← whole | `internal/tools/web/fetch_tool.go` (`fetchSingle` result fields) |
| `tool.file_read` | `file_read` | `path: file` req; `max_bytes: number` opt | `content: text` ← `content`; `raw: json` ← whole | `internal/tools/filetool/tool.go` (read result struct) |
| `tool.file_write` | `file_write` | `path: file` req; `content: text` req; `encoding: text` opt | `path: file` ← `path`; `raw: json` ← whole | `internal/tools/filetool/tool.go` (write result struct) |
| `tool.run_cli` | `run_cli` | `command: text` req; `args: list<text>` opt; `stdin: text` opt; `timeout_seconds: number` opt | `stdout: text` ← `stdout`; `exit_code: number` ← `exit_code`; `raw: json` ← whole | `internal/tools/cli/exec.go` |
| `tool.rag_retrieve` | `rag_retrieve` | `query: text` req; `k: number` opt; `include_text: boolean` opt | `results: list<json>` ← `results`; `raw: json` ← whole | `internal/tools/rag/tool.go` (retrieve result) |
| `tool.rag_ingest` | `rag_ingest` | `id: text` req; `text: text` req; `title: text` opt; `url: text` opt | `raw: json` ← whole | `internal/tools/rag/tool.go` |
| `tool.agent_call` | `agent_call` | `prompt: text` req | `raw: json` ← whole; `text: text` ← (confirm field, expected `response` or `content`) | `internal/tools/agents/agent_call.go` |
| `tool.matrix_room_message` | `matrix_room_message` | `text: text` req | `raw: json` ← whole | `internal/tools/matrixroom/tool.go` |

`tool.web_search` Post (exact):

```go
func webSearchPost(result map[string]any) map[string]any {
	items, _ := result["results"].([]any)
	var b strings.Builder
	for i, it := range items {
		m, _ := it.(map[string]any)
		title, _ := m["title"].(string)
		url, _ := m["url"].(string)
		fmt.Fprintf(&b, "%d. %s — %s\n", i+1, title, url)
	}
	result["results_text"] = strings.TrimRight(b.String(), "\n")
	return result
}
```

**Adapter runner behavior (exact):**
1. Build `args map[string]any`: for each ArgMap whose port is present in `in.Values`, set `args[argName] = value.Data`. Absent optional ports are omitted entirely (no null args).
2. `payload, err := reg.Dispatch(ctx, a.Tool, mustMarshal(args))` — dispatch error → node error.
3. Parse payload: `json.Unmarshal` to `any`; non-JSON → `map[string]any{"text": string(payload)}`.
4. If parsed is an object with `ok == false` and a non-empty `error` string → return `fmt.Errorf("tool %s: %s", a.Tool, errMsg)`.
5. Apply `Post` when set (on object results only).
6. For each OutMap: `Path == ""` → `warpp.Value{json, parsed}`. Else `SelectPath(parsed, Path)`; missing → `fmt.Errorf("tool %s result missing %q for port %q (contract violation)", a.Tool, m.Path, m.Port)`; then `warpp.CoerceRaw(found, declaredType)` — mismatch → contract violation error.
7. `tool.generic`: tool name from `in.Values["tool"]`, args object from `in.Values["args"]` (default `{}`); steps 2–4 identical; output `{"result": {json, parsed}}`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/warpp/toolnode/toolnode_test.go
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
	// every manifest port type must parse
	for _, m := range Manifests(adapters) {
		for _, p := range append(append([]warpp.PortSpec{}, m.Inputs...), m.Outputs...) {
			if _, err := warpp.ParseType(p.Type); err != nil {
				t.Fatalf("%s port %s: %v", m.Type, p.Name, err)
			}
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/toolnode/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `toolnode.go` (framework) and `adapters.go` (curated table)**

`toolnode.go`: `ArgMap`, `OutMap`, `Adapter`, `Runners`, `Resolver`, `Manifests`, `GenericManifest`, generic runner, and the adapter runner exactly per "Adapter runner behavior" above. `adapters.go`: `Builtin()` returning the 9 adapters with manifests built from the table (Category `"tool"`, Title = humanized tool name, Description copied from each tool's JSONSchema description string). While writing each adapter, open the "Verify in" file from the table and confirm/adjust the `Outs` paths against the tool's actual result construction; adjust the canned fixtures in the test to the REAL shapes you find (the fixtures above match `web_search` today).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/toolnode/
git commit -m "feat(warpp): add curated tool adapters and generic tool node"
```

# Phase B — Service wiring (`internal/persistence`, `internal/agentd`)

File map for the phase:

| File | Responsibility |
|---|---|
| `internal/persistence/store.go` (modify) | `WarppWorkflowRecord` + `WarppWorkflowStore` interface |
| `internal/persistence/databases/warpp_store_postgres.go` (create) | Postgres store; Init drops `flow_v2_workflows` |
| `internal/persistence/databases/warpp_store_sqlite.go` (create) | SQLite store |
| `internal/persistence/databases/interfaces.go` + `factory.go` (modify) | Manager field + wiring |
| `internal/agentd/warpp_runtime.go` (create) | In-memory runs/events/SSE state + durable hydration |
| `internal/agentd/warpp_service.go` (create) | Engine assembly, resolver chain, subflow runner, sync execution |
| `internal/agentd/handlers_warpp.go` (create) | HTTP handlers `/api/warpp/*` |
| `internal/agentd/warpp_agent_tools.go` (create) | Agent tools + published workflow tool sync |
| `internal/agentd/durable_warpp.go` (create) | Durable queue task `warpp.run` |
| `internal/warpp/manifest_workflow.go` (create) | `WorkflowManifest` (workflow-as-node) |

### Task 10: Persistence stores

**Files:**
- Modify: `internal/persistence/store.go` (add records/interface next to the FlowV2 ones at `store.go:415-431`; do NOT remove FlowV2 yet — Task 14 does)
- Create: `internal/persistence/databases/warpp_store_postgres.go`
- Create: `internal/persistence/databases/warpp_store_sqlite.go`
- Modify: `internal/persistence/databases/interfaces.go` (Manager field `Warpp` at the `FlowV2` field, `interfaces.go:78`; add to `closeIfPossible` list at `:105`)
- Modify: `internal/persistence/databases/factory.go` (wire like FlowV2 at `factory.go:406-412`)
- Test: `internal/persistence/databases/warpp_store_sqlite_test.go`

**Interfaces:**
- Consumes: `manifold/internal/warpp` (`Document`, `Canvas`).
- Produces:

```go
// in internal/persistence/store.go
// WarppWorkflowRecord is the persisted representation of a WARPP workflow.
type WarppWorkflowRecord struct {
	UserID    int64          `json:"user_id"`
	Document  warpp.Document `json:"document"`
	Canvas    warpp.Canvas   `json:"canvas"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// WarppWorkflowStore persists WARPP workflows by workflow id.
type WarppWorkflowStore interface {
	Init(ctx context.Context) error
	ListWorkflows(ctx context.Context, userID int64) ([]WarppWorkflowRecord, error)
	GetWorkflow(ctx context.Context, userID int64, workflowID string) (WarppWorkflowRecord, bool, error)
	UpsertWorkflow(ctx context.Context, userID int64, record WarppWorkflowRecord) (WarppWorkflowRecord, bool, error)
	DeleteWorkflow(ctx context.Context, userID int64, workflowID string) error
}
```

- [ ] **Step 1: Write the failing SQLite store test**

```go
// internal/persistence/databases/warpp_store_sqlite_test.go
package databases

import (
	"context"
	"testing"

	persist "manifold/internal/persistence"
	"manifold/internal/warpp"
)

func TestSQLiteWarppStoreCRUD(t *testing.T) {
	db := newTestSQLite(t) // mirror the helper used by flow_v2_store / other sqlite store tests in this package; if none exists, open ":memory:" the same way NewSQLiteFlowV2Store's tests do
	store := NewSQLiteWarppStore(db)
	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	doc := warpp.Document{ID: "wf1", Name: "One", Nodes: []warpp.Node{
		{ID: "a", Type: "data.parse", Inputs: map[string]warpp.Input{
			"text": {One: &warpp.Binding{Value: "{}", HasValue: true}}}}}}
	rec, created, err := store.UpsertWorkflow(ctx, 7, persist.WarppWorkflowRecord{UserID: 7, Document: doc})
	if err != nil || !created {
		t.Fatalf("upsert: %v created=%v", err, created)
	}
	if rec.Document.ID != "wf1" {
		t.Fatalf("roundtrip doc: %+v", rec.Document)
	}
	got, found, err := store.GetWorkflow(ctx, 7, "wf1")
	if err != nil || !found || got.Document.Name != "One" {
		t.Fatalf("get: %v %v %+v", err, found, got)
	}
	if _, found, _ := store.GetWorkflow(ctx, 8, "wf1"); found {
		t.Fatal("user scoping broken")
	}
	doc.Name = "Two"
	_, created, err = store.UpsertWorkflow(ctx, 7, persist.WarppWorkflowRecord{UserID: 7, Document: doc})
	if err != nil || created {
		t.Fatalf("update should not report created: %v %v", err, created)
	}
	list, err := store.ListWorkflows(ctx, 7)
	if err != nil || len(list) != 1 || list[0].Document.Name != "Two" {
		t.Fatalf("list: %v %+v", err, list)
	}
	if err := store.DeleteWorkflow(ctx, 7, "wf1"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.GetWorkflow(ctx, 7, "wf1"); found {
		t.Fatal("delete failed")
	}
}
```

Before writing it, open `internal/persistence/databases/flow_v2_store_sqlite.go` (referenced by `factory.go:406`) and the sqlite test helpers used in this package (`store_sqlite_test.go` in `internal/durable` shows the pattern; find the databases-package equivalent with `grep -rn "sql.Open\|:memory:" internal/persistence/databases/*_test.go`). Use the same helper; if the package has none, create `newTestSQLite(t)` in the new test file using the same driver import the sqlite stores use.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/persistence/databases/ -run TestSQLiteWarppStore -v`
Expected: FAIL — undefined: NewSQLiteWarppStore.

- [ ] **Step 3: Implement both stores + wiring**

Port `flow_v2_store_sqlite.go` → `warpp_store_sqlite.go` and `flow_v2_store_postgres.go` → `warpp_store_postgres.go` with this exact mapping (mechanical rename, same method bodies):
- type/constructor: `SQLiteFlowV2Store`→`SQLiteWarppStore`, `NewSQLiteFlowV2Store`→`NewSQLiteWarppStore`, `PostgresFlowV2Store`→`PostgresWarppStore`, `NewPostgresFlowV2Store`→`NewPostgresWarppStore`
- record: `persist.FlowV2WorkflowRecord`→`persist.WarppWorkflowRecord`; JSON columns `workflow`→`document` (marshals `warpp.Document`), `canvas` (marshals `warpp.Canvas`)
- table: `flow_v2_workflows`→`warpp_workflows`; index `warpp_workflows_user_workflow_idx`

Postgres DDL (in `Init`):

```sql
CREATE TABLE IF NOT EXISTS warpp_workflows (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  workflow_id TEXT NOT NULL,
  document JSONB NOT NULL,
  canvas JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS warpp_workflows_user_workflow_idx ON warpp_workflows(user_id, workflow_id);
DROP TABLE IF EXISTS flow_v2_workflows;
```

SQLite DDL: same columns with `INTEGER PRIMARY KEY AUTOINCREMENT`, `TEXT` for JSON columns, same unique index, same `DROP TABLE IF EXISTS flow_v2_workflows;` (clean break, spec §12).

Wire the Manager exactly like FlowV2: `interfaces.go` — add `Warpp persistence.WarppWorkflowStore` field and `closeIfPossible(m.Warpp)`; `factory.go` — in the same switch where `m.FlowV2` is set (`factory.go:406-412`), add `m.Warpp = NewSQLiteWarppStore(m.SQLite)` / `m.Warpp = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPostgresWarppStore)` and `initStore(ctx, "warpp store", m.Warpp)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/persistence/... -v -run Warpp && go build ./...`
Expected: PASS, clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/persistence/
git commit -m "feat(warpp): add workflow persistence stores (postgres/sqlite), drop flow_v2 table"
```

### Task 11: Run-state runtime (`warpp_runtime.go`)

**Files:**
- Create: `internal/agentd/warpp_runtime.go`
- Test: `internal/agentd/warpp_runtime_test.go`

**Interfaces:**
- Consumes: `warpp.Event`/statuses, `persist.WarppWorkflowStore`, `durable.Client`.
- Produces (all consumed by Tasks 12–14):
  - `type warppWorkflowSummary struct { ID, Name, Description string; PublishTool bool }` (JSON: `id, name, description, publish_tool`)
  - `type warppRuntime struct { ... }` with `newWarppRuntime(store persist.WarppWorkflowStore, durableClients ...*durable.Client) *warppRuntime`
  - Methods (same shapes as `flowV2Runtime` in the current `internal/agentd/flow_v2_runtime.go`, which this file replaces): `listWorkflowSummaries(ctx, userID) ([]warppWorkflowSummary, error)`, `getWorkflow(ctx, userID, id) (warpp.Document, warpp.Canvas, bool, error)`, `upsertWorkflow(ctx, userID, doc, canvas) (persist.WarppWorkflowRecord, bool, error)`, `deleteWorkflow(ctx, userID, id) (bool, error)`, `createRun(userID, workflowID string, input map[string]any) string` (run IDs `warpprun_<unixnano>`), `createRunWithID`, `appendRunEvent(userID, runID string, ev warpp.Event) bool`, `getRunEvents(userID, runID) ([]warpp.Event, string, bool)`, `subscribeRun`, `unsubscribeRun`, `hydrateRunFromDurable` (durable event prefix `"warpp."`), plus `warppEventPayload(ev) map[string]any` / `warppEventFromDurable(durable.Event) (warpp.Event, bool)` / `warppStatusFromDurable`.
  - `(*app).warppState() *warppRuntime` — lazy accessor mirroring today's `flowV2State()` (`handlers_flow_v2.go:391-400`), backed by `a.mgr.Warpp`; requires the `warpp *warppRuntime` field added to `app` (this task adds the field; the old `flowV2` field is removed in Task 14).

**Adaptation rules vs the old runtime (write the file fresh, using `flow_v2_runtime.go` as the reference):**
- Events use `warpp.Event`; the node identifier field is `NodePath` (payload key `"node_path"`).
- `appendRunEvent` status transitions: `EventRunCompleted` → `run.Status = ev.Status` (this is how `completed_with_skips` lands — the engine sets it); empty `ev.Status` falls back to `"completed"`. `EventRunFailed` → `"failed"` (+Error), `EventRunCancelled` → `"cancelled"`.
- Event payload includes `"outputs"` (map) instead of `"output"`.
- `Subs` non-blocking fan-out, sequence assignment, and durable hydration logic stay identical.
- If `store == nil`, workflow CRUD methods return `fmt.Errorf("warpp store unavailable")` (no silent Postgres default like the old code).

- [ ] **Step 1: Write the failing tests**

```go
// internal/agentd/warpp_runtime_test.go
package agentd

import (
	"context"
	"testing"

	persist "manifold/internal/persistence"
	"manifold/internal/warpp"
)

type fakeWarppStore struct {
	recs map[string]persist.WarppWorkflowRecord
}

func newFakeWarppStore() *fakeWarppStore {
	return &fakeWarppStore{recs: map[string]persist.WarppWorkflowRecord{}}
}
func (f *fakeWarppStore) key(userID int64, id string) string {
	return string(rune(userID)) + "/" + id
}
func (f *fakeWarppStore) Init(ctx context.Context) error { return nil }
func (f *fakeWarppStore) ListWorkflows(ctx context.Context, userID int64) ([]persist.WarppWorkflowRecord, error) {
	var out []persist.WarppWorkflowRecord
	for _, r := range f.recs {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeWarppStore) GetWorkflow(ctx context.Context, userID int64, id string) (persist.WarppWorkflowRecord, bool, error) {
	r, ok := f.recs[f.key(userID, id)]
	return r, ok, nil
}
func (f *fakeWarppStore) UpsertWorkflow(ctx context.Context, userID int64, rec persist.WarppWorkflowRecord) (persist.WarppWorkflowRecord, bool, error) {
	_, existed := f.recs[f.key(userID, rec.Document.ID)]
	f.recs[f.key(userID, rec.Document.ID)] = rec
	return rec, !existed, nil
}
func (f *fakeWarppStore) DeleteWorkflow(ctx context.Context, userID int64, id string) error {
	delete(f.recs, f.key(userID, id))
	return nil
}

func TestWarppRuntimeRunLifecycle(t *testing.T) {
	rt := newWarppRuntime(newFakeWarppStore())
	runID := rt.createRun(1, "wf", map[string]any{"a": 1})
	if !rt.appendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunStarted, Status: warpp.StatusRunning}) {
		t.Fatal("append to own run failed")
	}
	if rt.appendRunEvent(2, runID, warpp.Event{Type: warpp.EventRunStarted}) {
		t.Fatal("cross-user append must fail")
	}
	rt.appendRunEvent(1, runID, warpp.Event{Type: warpp.EventNodeCompleted, NodePath: "n",
		Outputs: map[string]any{"text": "x"}})
	rt.appendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunCompleted, Status: warpp.StatusCompletedWithSkips})
	events, status, ok := rt.getRunEvents(1, runID)
	if !ok || status != warpp.StatusCompletedWithSkips || len(events) != 3 {
		t.Fatalf("events=%d status=%s ok=%v", len(events), status, ok)
	}
	if events[0].Sequence != 1 || events[2].Sequence != 3 {
		t.Fatalf("sequence assignment wrong: %+v", events)
	}
}

func TestWarppRuntimeSubscribe(t *testing.T) {
	rt := newWarppRuntime(newFakeWarppStore())
	runID := rt.createRun(1, "wf", nil)
	rt.appendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunStarted, Status: warpp.StatusRunning})
	snapshot, ch, done, ok := rt.subscribeRun(1, runID)
	if !ok || done || len(snapshot) != 1 || ch == nil {
		t.Fatalf("subscribe: %v %v %d", ok, done, len(snapshot))
	}
	rt.appendRunEvent(1, runID, warpp.Event{Type: warpp.EventRunCompleted, Status: warpp.StatusCompleted})
	ev := <-ch
	if ev.Type != warpp.EventRunCompleted {
		t.Fatalf("live event: %+v", ev)
	}
	rt.unsubscribeRun(runID, ch)
	// subscribing to a finished run returns done snapshot
	_, ch2, done2, ok2 := rt.subscribeRun(1, runID)
	if !ok2 || !done2 || ch2 != nil {
		t.Fatalf("finished-run subscribe: %v %v", ok2, done2)
	}
}

func TestWarppRuntimeWorkflowCRUD(t *testing.T) {
	rt := newWarppRuntime(newFakeWarppStore())
	ctx := context.Background()
	doc := warpp.Document{ID: "w", Name: "W", Publish: warpp.Publish{Tool: true},
		Nodes: []warpp.Node{{ID: "a", Type: "data.parse",
			Inputs: map[string]warpp.Input{"text": {One: &warpp.Binding{Value: "{}", HasValue: true}}}}}}
	if _, _, err := rt.upsertWorkflow(ctx, 1, doc, warpp.Canvas{}); err != nil {
		t.Fatal(err)
	}
	sums, err := rt.listWorkflowSummaries(ctx, 1)
	if err != nil || len(sums) != 1 || !sums[0].PublishTool {
		t.Fatalf("summaries: %v %+v", err, sums)
	}
	got, _, found, err := rt.getWorkflow(ctx, 1, "w")
	if err != nil || !found || got.Name != "W" {
		t.Fatalf("get: %v %v", err, found)
	}
	deleted, err := rt.deleteWorkflow(ctx, 1, "w")
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", err, deleted)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/agentd/ -run TestWarppRuntime -v`
Expected: FAIL — undefined: newWarppRuntime.

- [ ] **Step 3: Implement `warpp_runtime.go`** per the adaptation rules (use `internal/agentd/flow_v2_runtime.go` as the structural reference — same mutex/fan-out/hydration design, new types). Also add the `warpp *warppRuntime` field to `app` in `internal/agentd/app.go` (next to the existing `flowV2` field at `app.go:61`) and the `warppState()` accessor.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/agentd/ -run TestWarppRuntime -v -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agentd/warpp_runtime.go internal/agentd/warpp_runtime_test.go internal/agentd/app.go
git commit -m "feat(warpp): add run-state runtime with SSE fan-out and durable hydration"
```

### Task 12: Engine assembly + HTTP handlers

**Files:**
- Create: `internal/agentd/warpp_service.go`
- Create: `internal/agentd/handlers_warpp.go`
- Modify: `internal/warpp/document.go` (add `ProjectID string \`json:"project_id,omitempty"\`` to `Document` — project sandbox scoping, mirrors today's behavior in `warpp_tools.go:70-76`)
- Modify: `internal/agentd/router.go:112-117` (swap routes)
- Test: `internal/agentd/handlers_warpp_test.go`

**Interfaces:**
- Consumes: Tasks 1–11; `toolnode`; `durable`; existing helpers `requireUserID`, `withMaybeTimeout`, `systemUserID`, and `workflowToolContext` (move it from `warpp_tools.go` into `warpp_service.go` verbatim — Task 14 deletes its old home).
- Produces:
  - `(*app).warppResolver(ctx context.Context, userID int64) warpp.Resolver` — `ChainResolvers(BuiltinResolver(), toolnode.Resolver(toolnode.Builtin()), subflowResolver)` where `subflowResolver` resolves `flow.<id>` via `warppState().getWorkflow` + `warpp.WorkflowManifest` (Task 13 adds `WorkflowManifest`; for THIS task stub subflowResolver to always miss and leave a `// flow.* resolution lands with WorkflowManifest` note — the chain structure is in place)
  - `(*app).warppRunners(userID int64) map[string]warpp.NodeRunner` — `BuiltinRunners()` + `toolnode.Runners(a.warppExecutionRegistry(), toolnode.Builtin())` + `llm.generate` via `warpp.LLMRunner(a.warppChat)` where `warppChat(ctx, instruction, input, model)` calls `a.llm.Chat(ctx, msgs, nil, model)` with a system message (instruction, when non-empty) + user message, returning `msg.Content` (see `llm.Provider` in `internal/llm/provider.go:76`)
  - `(*app).warppExecutionRegistry() tools.Registry` — policy-aware registry first, base fallback (mirror `flowV2ExecutionRegistry`, `flow_v2_expression.go:277-284`)
  - `(*app).executeWarppRun(ctx context.Context, userID int64, runID string, doc warpp.Document, input map[string]any)` — builds `warpp.Engine{Resolve, Runners, Emit, Step, MaxConcurrency: doc.Settings.MaxConcurrency}` where `Emit` records to durable (`durable.RecordEvent(ctx, "warpp."+type, warppEventPayload(ev))` for sequence/timestamps) then `appendRunEvent` (mirror `emitRunEvent`, `flow_v2_execution.go:120-126`), `Step` = `durable.Step[map[string]warpp.Value]` passthrough, then `eng.Execute`; applies `workflowToolContext` when `doc.ProjectID` non-empty
  - HTTP handlers: `warppWorkflowsHandler` (GET list), `warppWorkflowDetailHandler` (GET/PUT/DELETE `/api/warpp/workflows/{id}`), `warppValidateHandler` (POST), `warppRunsHandler` (POST `/api/warpp/runs`), `warppRunEventsHandler` (GET `/api/warpp/runs/{id}/events`, SSE + JSON), `warppCatalogHandler` (GET)
  - Wire in `internal/agentd/router.go` replacing lines 112–117:

```go
	mux.HandleFunc("/api/warpp/workflows", a.warppWorkflowsHandler())
	mux.HandleFunc("/api/warpp/workflows/", a.warppWorkflowDetailHandler())
	mux.HandleFunc("/api/warpp/validate", a.warppValidateHandler())
	mux.HandleFunc("/api/warpp/runs", a.warppRunsHandler())
	mux.HandleFunc("/api/warpp/runs/", a.warppRunEventsHandler())
	mux.HandleFunc("/api/warpp/catalog", a.warppCatalogHandler())
```

(The old `/api/flows/v2/*` lines are REMOVED here; `flowV2ToolsHandler` in `handlers_tools.go:440` loses its route now and is deleted in Task 14.)

**Handler contracts (mirror the old handler structure in `handlers_flow_v2.go` for auth, body limits, SSE writer):**
- `GET /api/warpp/workflows` → `{"workflows": [warppWorkflowSummary]}`
- `GET /api/warpp/workflows/{id}` → `{"document": ..., "canvas": ...}` (404 when missing)
- `PUT /api/warpp/workflows/{id}`: body `{"document": ..., "canvas": ...}` (1MB limit); fill empty `document.id` from URL, reject mismatch (400); `warpp.Validate` with the full resolver — errors → 400 `{"valid": false, "diagnostics": [...]}`; save; when `userID == systemUserID` → `a.syncPublishedWorkflowTools(ctx)` (Task 13; for THIS task call a stub method defined in `warpp_service.go` with a TODO-free empty body and a doc comment "published-tool sync lands in warpp_agent_tools.go"); 200/201 with saved payload
- `DELETE` → 204, same sync hook
- `POST /api/warpp/validate`: `{"document": ...}` → 200 `{"valid": bool, "diagnostics": [...]}`
- `POST /api/warpp/runs`: `{"workflow_id": "...", "input": {...}}` (64KB limit) → load + validate (422 with diagnostics when invalid) → durable spawn when `a.durableClient != nil` (queue `warppDurableQueue = "warpp"`, task `warppDurableRunTaskName = "warpp.run"`, retry policy `{MaxAttempts: 3, Backoff: "exponential", BaseDelaySeconds: 1, MaxDelaySeconds: 30}` — mirror `spawnDurableFlowV2Run`, `handlers_flow_v2.go:254-272`) else local goroutine via `withMaybeTimeout(ctx, a.cfg.WorkflowTimeoutSeconds || a.cfg.AgentRunTimeoutSeconds)` → 202 `{"run_id": "...", "status": "running"}`
- `GET /api/warpp/runs/{id}/events`: `Accept: text/event-stream` → SSE (snapshot then live until a terminal run event; mirror `flowV2RunEventsHandler` including unsubscribe); otherwise JSON `{"run_id", "status", "events"}`
- `GET /api/warpp/catalog` → `{"manifests": [...builtins, ...toolnode.Manifests(toolnode.Builtin()), ...published flow.* manifests...], "coercions": [["number","text"],["boolean","text"]], "workflows": [summaries]}` (flow.* manifests only after Task 13; emit builtins+tools now)

- [ ] **Step 1: Write the failing handler tests**

Follow the conventions in `internal/agentd/handlers_flow_v2_test.go` (httptest against the app's mux; check how that file constructs the test app with `newTestApp`-style helpers and reuse the same construction — `grep -n "func Test" internal/agentd/handlers_flow_v2_test.go` and read the setup helper before writing). Cover, as separate test functions:

```go
// internal/agentd/handlers_warpp_test.go — test names and assertions:
// TestWarppPutAndGetWorkflow: PUT a valid doc (the two-node template document
//   from Task 7's TestExecuteLinearPipeline) -> 201; GET -> same document; PUT
//   with mismatched body/URL id -> 400.
// TestWarppPutRejectsInvalidDocument: PUT doc with unknown node type -> 400,
//   body decodes to {"valid":false} with diagnostics containing code
//   "node.type.unknown".
// TestWarppValidateEndpoint: POST /api/warpp/validate with the invalid doc ->
//   200 {"valid":false}; with the valid doc -> {"valid":true}.
// TestWarppRunLifecycleLocal: PUT valid doc; POST /api/warpp/runs -> 202 with
//   run_id; poll GET /api/warpp/runs/{id}/events (JSON mode) until status is
//   "completed" (timeout 5s); assert events include node_completed for "tpl"
//   with outputs.text == "about go" and final run_completed.
// TestWarppCatalog: GET /api/warpp/catalog -> 200; manifests include
//   "data.extract", "control.map", "llm.generate", "tool.web_search",
//   "tool.generic"; coercions == [["number","text"],["boolean","text"]].
// TestWarppRunsRequireWorkflowID: POST /api/warpp/runs {} -> 400.
```

Write them fully (each ~15 lines using the discovered test-app helper), not as comments.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/agentd/ -run TestWarpp -v`
Expected: FAIL — undefined handlers.

- [ ] **Step 3: Implement `warpp_service.go` + `handlers_warpp.go` + router swap** per the contracts above. Note: `Document.ProjectID` addition requires no validator change (unknown JSON keys are already tolerated by encoding/json; the field is additive).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/agentd/ -run TestWarpp -v && go build ./...`
Expected: PASS. (Old flow v2 tests still pass — nothing deleted yet.)

- [ ] **Step 5: Commit**

```bash
git add internal/agentd/warpp_service.go internal/agentd/handlers_warpp.go internal/agentd/router.go internal/warpp/document.go internal/agentd/handlers_warpp_test.go
git commit -m "feat(warpp): add engine assembly and /api/warpp HTTP surface"
```

### Task 13: Workflow-as-node + agent tools + published tool sync

**Files:**
- Create: `internal/warpp/manifest_workflow.go` (+ tests in `internal/warpp/manifest_workflow_test.go`)
- Create: `internal/agentd/warpp_agent_tools.go`
- Modify: `internal/agentd/warpp_service.go` (fill the subflow resolver + runner, replace the sync stub)
- Test: `internal/agentd/warpp_agent_tools_test.go`

**Interfaces:**
- Produces (engine): `func WorkflowManifest(doc Document, resolve Resolver) (Manifest, []Diagnostic)` — Type `"flow."+doc.ID`, Title doc.Name, Category `"flow"`, Inputs = doc.Inputs verbatim, Outputs = one PortSpec per doc.Outputs entry typed via `ResolveOutputTypes` (literal bindings type via `InferLiteral`).
- Produces (service):
  - `(*app).subflowRunner(userID int64) warpp.NodeRunner` — strips `flow.` prefix, loads the doc, builds input map from `in.Values` (port → `.Data`), runs `a.executeWarppDocSync` (below) WITHOUT event emission (inner events are not streamed to the parent run — v1 decision), returns outputs re-typed via the subflow's `WorkflowManifest` port types (`warpp.CoerceRaw`)
  - `(*app).executeWarppDocSync(ctx, userID int64, doc warpp.Document, input map[string]any) (warpp.Result, error)` — validate + engine run with runtime-recorded run (creates a run record so it's inspectable), returns Result
  - `(*app).ExecuteWarppSync(ctx, userID int64, workflowID string, input map[string]any) (map[string]any, error)` — load by ID → `executeWarppDocSync` → `{"ok": true, "run_id", "status", "outputs": Result.Outputs}` or error (replaces old `ExecuteWorkflowSync` semantics with declared outputs — no `unwrapWorkflowPayload` guessing)
  - `(*app).syncPublishedWorkflowTools(ctx)` — lists system-user workflows, unregisters `a.warppToolNames`, registers one `tools.Tool` per `Publish.Tool` workflow named `"warpp_" + sanitize(id)` (port the small `sanitize` func from `internal/tools/warpptool/warpp_tool.go` before it's deleted); tool schema derived from `doc.Inputs` via `portSpecJSONSchema` (text/file→string, number→number, boolean→boolean, json→object-no-props, `list<E>`→array of mapped E); `Call` decodes args → `ExecuteWarppSync`
  - Five authoring tools registered once at startup (same registration point where `syncWarppTools` is called today — find with `grep -n syncWarppTools internal/agentd/app_init*.go`): `workflow_catalog` (returns manifests+coercions), `workflow_list`, `workflow_get` (by id), `workflow_save` (validate → on errors `{"ok": false, "diagnostics": [...]}` → else upsert + sync), `workflow_run` (by id + input → ExecuteWarppSync). All operate as `systemUserID`. Subflow cycle guard: `workflow_save` and the PUT handler both call `(*app).checkSubflowCycles(ctx, userID, doc) []warpp.Diagnostic` — DFS over `flow.*` node types via the store, diagnostic code `workflow.subflow.cycle`.

- [ ] **Step 1: Write the failing tests** — engine: `TestWorkflowManifest` (valid doc from Task 5's `validDoc` → manifest has Inputs `topic:text`, Outputs `echo:text` + `mapped:list<...>`; diagnostics empty). Service: `TestPublishedToolRegistrationAndCall` (fake registry counts Register/Unregister; save doc with `publish.tool` → tool `warpp_<id>` registered; Call with `{"topic":"go"}` returns `outputs.out == "about go"`), `TestWorkflowSaveToolReturnsDiagnostics` (invalid doc → ok:false with codes), `TestSubflowRunnerExecutesChildWorkflow` (save child; parent doc with node `type: "flow.<child-id>"` wired end-to-end → parent run completes with child's output), `TestSubflowCycleRejected` (A includes B, B includes A → save B fails with `workflow.subflow.cycle`). Write them fully with the same test-app helper as Task 12.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/warpp/ ./internal/agentd/ -run 'TestWorkflowManifest|TestPublished|TestWorkflowSaveTool|TestSubflow' -v`
Expected: FAIL.

- [ ] **Step 3: Implement** engine `WorkflowManifest`, then the service pieces per the interface block. The five authoring tools are small structs implementing `tools.Tool` (see `internal/tools/warpptool/warpp_tool.go:21-87` for the pattern: Name/JSONSchema/Call returning `map[string]any` with `ok` field).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/warpp/... ./internal/agentd/ -run 'TestWorkflowManifest|TestPublished|TestWorkflowSaveTool|TestSubflow|TestWarpp' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warpp/manifest_workflow.go internal/warpp/manifest_workflow_test.go internal/agentd/warpp_agent_tools.go internal/agentd/warpp_agent_tools_test.go internal/agentd/warpp_service.go
git commit -m "feat(warpp): workflows as nodes — agent tools, subflows, published tool sync"
```

### Task 14: Durable task, app wiring, and the clean-break deletion

**Files:**
- Create: `internal/agentd/durable_warpp.go`
- Modify: `internal/agentd/app.go` (remove `flowV2` field), `internal/agentd/app_init*.go` (swap `syncWarppTools` call → `syncPublishedWorkflowTools` + authoring-tool registration), `internal/agentd/handlers_tools.go` (delete `flowV2ToolsHandler`, `handlers_tools.go:440`)
- Delete: `internal/flow/` (whole package), `internal/agentd/flow_v2_execution.go`, `flow_v2_eval.go`, `flow_v2_expression.go` (move `cloneMap`/`withMaybeTimeout`-style helpers still referenced elsewhere into a small `internal/agentd/util_clone.go` first — check with `grep -rn "cloneMap\|cloneWorkflow\|asBool\|parseFlowDuration" internal/agentd/ | grep -v flow_v2`), `flow_v2_runtime.go`, `flow_v2_runtime_test.go`, `handlers_flow_v2.go`, `handlers_flow_v2_test.go`, `warpp_tools.go`, `warpp_tools_test.go`, `durable_flow.go` (port `registerDurableHandlers`'s chat/pulse registrations into `durable_warpp.go` — it is the same function, re-homed), `internal/tools/warpptool/`
- Modify: `internal/persistence/store.go` (remove `FlowV2WorkflowRecord`/`FlowV2WorkflowStore` and the `manifold/internal/flow` import), `internal/persistence/databases/` (delete `flow_v2_store_postgres.go`, `flow_v2_store_sqlite.go`, `flow_v2_store_postgres_test.go`; remove Manager field + factory wiring)
- Test: existing suites (this task is verified by the full build+test gates)

**Interfaces:**
- Produces: `registerDurableHandlers` now registers `("warpp", "warpp.run", a.runDurableWarppTask)` plus the existing chat/pulse handlers; `runDurableWarppTask(ctx, params)` mirrors the old `runDurableFlowV2Task` (`durable_flow.go:27-70`): read `workflow_id`/`input` from params, load + validate, `createRunWithID(userID, docID, tc.Task.ID, input)`, `executeWarppRun`, then return `{"run_id", "status", "outputs"}` from the run's terminal state.

- [ ] **Step 1: Write `durable_warpp.go`** (constants `warppDurableQueue = "warpp"`, `warppDurableRunTaskName = "warpp.run"`; the handler; the re-homed `registerDurableHandlers`).

- [ ] **Step 2: Execute the deletion list** exactly as in the Files block, then chase compile errors:

```bash
git rm -r internal/flow internal/tools/warpptool
git rm internal/agentd/flow_v2_execution.go internal/agentd/flow_v2_eval.go \
  internal/agentd/flow_v2_expression.go internal/agentd/flow_v2_runtime.go \
  internal/agentd/flow_v2_runtime_test.go internal/agentd/handlers_flow_v2.go \
  internal/agentd/handlers_flow_v2_test.go internal/agentd/warpp_tools.go \
  internal/agentd/warpp_tools_test.go internal/agentd/durable_flow.go
git rm internal/persistence/databases/flow_v2_store_postgres.go \
  internal/persistence/databases/flow_v2_store_sqlite.go \
  internal/persistence/databases/flow_v2_store_postgres_test.go
go build ./... 2>&1 | head -50
```

Fix every remaining reference (expected: `app.go` field, `app_init*` calls, `handlers_tools.go:440`, persistence Manager/factory, any `manifold/internal/flow` import). Verify zero stragglers:

```bash
grep -rn "manifold/internal/flow\"" --include="*.go" . ; \
grep -rn "flowV2\|FlowV2\|warpptool" --include="*.go" internal/ cmd/ ; \
grep -rn "flows/v2" --include="*.go" internal/
```

Expected: no output from all three.

- [ ] **Step 3: Run the full backend gates**

Run: `go build ./... && go vet ./... && go test ./internal/... -count=1`
Expected: everything green.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat(warpp)!: wire durable runs and delete flow v2 / legacy WARPP (clean break)"
```

### Task 15: Rewrite `docs/warpp.md`

**Files:**
- Modify: `docs/warpp.md` (full replacement)

- [ ] **Step 1: Replace the file** with documentation of the new system covering, in order: the one-rule mental model (typed ports, data only on wires); the document format (the Task 7 linear example + the Map example from Task 8, verbatim working JSON); the type system + the two-row coercion table; the builtin node table (copy from Task 3); the curated tool adapter table (copy from Task 9, plus `tool.generic` + `data.extract` escape-hatch recipe); execution semantics (skip rule, policies, statuses); the API table (copy from spec §10); agent tools (`workflow_catalog/list/get/save/run`, published `warpp_<id>` tools); subflows. State explicitly: legacy `${A.*}`, expressions, edge mappings, and `/api/flows/v2/*` are gone.

- [ ] **Step 2: Verify the JSON examples** by piping each through the validator in a throwaway test or `go run` snippet — every example in the doc must validate cleanly.

- [ ] **Step 3: Commit**

```bash
git add docs/warpp.md
git commit -m "docs(warpp): document the typed-port dataflow system"
```

# Phase C — Editor rebuild (`web/agentd-ui`)

File map for the phase:

| File | Responsibility |
|---|---|
| `src/types/warpp.ts` | TS mirrors of Document/Manifest/Event/Catalog JSON |
| `src/lib/warppTypes.ts` | Type parsing, drag-time compatibility, port colors |
| `src/api/warpp.ts` | REST client + run SSE (EventSource) |
| `src/stores/warppEditor.ts` | Document state, node/wire ops, derived Vue Flow graph |
| `src/stores/warppRun.ts` | Run/event state driven by SSE |
| `src/components/warpp/NodeCard.vue` | Node rendering: typed handles, widgets summary, status chip |
| `src/components/warpp/CatalogPanel.vue` | Palette from catalog, search, drag-to-add |
| `src/components/warpp/InspectorPanel.vue` | Manifest-driven widgets for unwired ports |
| `src/components/warpp/CanvasPane.vue` | Vue Flow instance, connect validation, Map containers |
| `src/components/warpp/RunTimeline.vue` | Event list for the active run |
| `src/views/WarppView.vue` | Shell: toolbar, panels, save/validate/run |

Editor decisions locked here:
- Vue Flow node IDs are scope paths joined with `"::"` (root node `tpl` → `"tpl"`; map body node → `"per::t"`). Map nodes render as Vue Flow parents (`parentNode` on children, `extent: 'parent'`).
- Drag-time connection checks validate PORT TYPE compatibility only (using the catalog's coercion table; `T` and `dynamic:*` ports accept anything). Scope legality, required ports, and cycles are the server validator's job — diagnostics render in the toolbar after Save/Validate and highlight offending nodes.
- Canvas wiring sets scalar bindings (`One`) and appends to list-variadic ports; named-variadic wiring is done in the Inspector (add-row UI names the key).
- No expression UI exists anywhere.

### Task 16: Types + type-compat library

**Files:**
- Create: `web/agentd-ui/src/types/warpp.ts`
- Create: `web/agentd-ui/src/lib/warppTypes.ts`
- Test: `web/agentd-ui/src/lib/__tests__/warppTypes.spec.ts`

**Interfaces (produces, consumed by every later task):**

```ts
// src/types/warpp.ts — exact shapes (mirror Go JSON tags)
export interface WarppBinding { from?: string; value?: unknown }
export type WarppInput = WarppBinding | WarppBinding[] | Record<string, WarppBinding>;
export interface WarppPolicy { timeout?: string; retries?: { max?: number; backoff?: string }; on_error?: string }
export interface WarppNode {
  id: string; type: string;
  inputs?: Record<string, WarppInput>;
  policy?: WarppPolicy;
  body?: { nodes: WarppNode[]; outputs: Record<string, WarppBinding> };
}
export interface WarppPortSpec {
  name: string; type: string; required?: boolean; default?: unknown;
  variadic?: "" | "list" | "named"; description?: string;
}
export interface WarppDocument {
  id: string; name: string; description?: string; project_id?: string;
  inputs?: WarppPortSpec[]; nodes: WarppNode[];
  outputs?: Record<string, WarppBinding>;
  settings?: { max_concurrency?: number; default_policy?: WarppPolicy };
  publish?: { tool?: boolean };
}
export interface WarppManifest {
  type: string; title: string; category: string; description?: string;
  inputs: WarppPortSpec[]; outputs: WarppPortSpec[];
}
export interface WarppCanvasNode { x: number; y: number; width?: number; height?: number; label?: string }
export interface WarppCanvas { nodes?: Record<string, WarppCanvasNode>; groups?: unknown[]; notes?: unknown[] }
export interface WarppCatalog { manifests: WarppManifest[]; coercions: [string, string][]; workflows: WarppWorkflowSummary[] }
export interface WarppWorkflowSummary { id: string; name: string; description?: string; publish_tool?: boolean }
export interface WarppDiagnostic { severity: "error" | "warning"; code: string; message: string; path?: string }
export interface WarppRunEvent {
  run_id: string; sequence: number; type: string; node_path?: string;
  status?: string; message?: string; outputs?: Record<string, unknown>;
  error?: string; occurred_at: string;
}
```

```ts
// src/lib/warppTypes.ts
export interface PortType { kind: string; elem?: string }
export function parseType(s: string): PortType;          // "list<text>" -> {kind:"list",elem:"text"}; "T" -> {kind:"T"}; "dynamic:as"/"dynamic:body" -> {kind:"dynamic"}
export function typeLabel(t: PortType): string;
export function isWildcard(t: PortType): boolean;        // T, list<T>, dynamic
export function assignable(from: string, to: string, coercions: [string, string][]): boolean;
export function portColor(typeString: string): string;   // stable palette: text #8bd17c, number #6fb3ff, boolean #ffb46f, json #c792ea, file #62d2c5, list -> its elem color, wildcard #9aa4b2
```

`assignable` rules (drag-time): parse both; wildcard on either side → true; identical → true; `list<a>`→`list<b>` requires a==b; else `[from.kind, to.kind]` present in `coercions`.

- [ ] **Step 1: Write the failing tests**

```ts
// src/lib/__tests__/warppTypes.spec.ts
import { describe, expect, it } from "vitest";
import { assignable, isWildcard, parseType, portColor } from "../warppTypes";

const coercions: [string, string][] = [["number", "text"], ["boolean", "text"]];

describe("parseType", () => {
  it("parses scalars, lists, wildcards", () => {
    expect(parseType("text")).toEqual({ kind: "text" });
    expect(parseType("list<json>")).toEqual({ kind: "list", elem: "json" });
    expect(parseType("T")).toEqual({ kind: "T" });
    expect(parseType("list<T>")).toEqual({ kind: "list", elem: "T" });
    expect(parseType("dynamic:as")).toEqual({ kind: "dynamic" });
  });
});

describe("assignable", () => {
  it("identity and coercions", () => {
    expect(assignable("text", "text", coercions)).toBe(true);
    expect(assignable("number", "text", coercions)).toBe(true);
    expect(assignable("boolean", "text", coercions)).toBe(true);
    expect(assignable("text", "number", coercions)).toBe(false);
    expect(assignable("json", "text", coercions)).toBe(false);
  });
  it("lists match on element", () => {
    expect(assignable("list<text>", "list<text>", coercions)).toBe(true);
    expect(assignable("list<number>", "list<text>", coercions)).toBe(false);
    expect(assignable("text", "list<text>", coercions)).toBe(false);
  });
  it("wildcards accept anything", () => {
    expect(assignable("json", "T", coercions)).toBe(true);
    expect(assignable("list<json>", "list<T>", coercions)).toBe(true);
    expect(assignable("dynamic:as", "text", coercions)).toBe(true);
    expect(isWildcard(parseType("list<T>"))).toBe(true);
  });
  it("port colors are stable and list follows element", () => {
    expect(portColor("text")).toBe(portColor("text"));
    expect(portColor("list<number>")).toBe(portColor("number"));
    expect(portColor("text")).not.toBe(portColor("json"));
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web/agentd-ui && npx vitest run src/lib/__tests__/warppTypes.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `types/warpp.ts` and `lib/warppTypes.ts`** exactly per the interface block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web/agentd-ui && npx vitest run src/lib/__tests__/warppTypes.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/agentd-ui/src/types/warpp.ts web/agentd-ui/src/lib/warppTypes.ts web/agentd-ui/src/lib/__tests__/warppTypes.spec.ts
git commit -m "feat(warpp-ui): add document types and drag-time type compatibility"
```

### Task 17: API client + run SSE

**Files:**
- Create: `web/agentd-ui/src/api/warpp.ts`

**Interfaces:**
- Consumes: `src/types/warpp.ts`; follows the fetch conventions of the current `src/api/flow.ts:13-28` (baseURL from `VITE_AGENTD_BASE_URL`, `handleResponse<T>`).
- Produces:

```ts
export function fetchCatalog(): Promise<WarppCatalog>;
export function listWorkflows(): Promise<{ workflows: WarppWorkflowSummary[] }>;
export function getWorkflow(id: string): Promise<{ document: WarppDocument; canvas: WarppCanvas }>;
export function saveWorkflow(id: string, payload: { document: WarppDocument; canvas: WarppCanvas }): Promise<{ document: WarppDocument; canvas: WarppCanvas }>;   // throws WarppValidationError{diagnostics} on 400 body {valid:false}
export function deleteWorkflow(id: string): Promise<void>;
export function validateWorkflow(document: WarppDocument): Promise<{ valid: boolean; diagnostics?: WarppDiagnostic[] }>;
export function startRun(workflowId: string, input: Record<string, unknown>): Promise<{ run_id: string; status: string }>;
export function streamRunEvents(runId: string, onEvent: (ev: WarppRunEvent) => void, onDone: () => void): () => void;  // EventSource on GET /api/warpp/runs/{id}/events; parses each message as WarppRunEvent; calls onDone + closes on run_completed/run_failed/run_cancelled; returns a cancel fn
export class WarppValidationError extends Error { diagnostics: WarppDiagnostic[] }
```

- [ ] **Step 1: Implement the module** (pure client; no unit test — exercised by store tests via mocked fetch and by the smoke test in Task 21).
- [ ] **Step 2: Typecheck**

Run: `cd web/agentd-ui && npx vue-tsc --noEmit 2>/dev/null || npx tsc --noEmit -p tsconfig.json`
(use whichever typecheck command `package.json`/CI uses — check `.github/workflows` for the frontend job; `npm run build` also gates)
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add web/agentd-ui/src/api/warpp.ts
git commit -m "feat(warpp-ui): add API client with SSE run streaming"
```

### Task 18: Editor store

**Files:**
- Create: `web/agentd-ui/src/stores/warppEditor.ts`
- Test: `web/agentd-ui/src/stores/__tests__/warppEditor.spec.ts`

**Interfaces:**
- Consumes: types, `warppTypes`, `api/warpp` (inject-mockable via dynamic import or module mock in tests).
- Produces (Pinia store `useWarppEditor`):

```ts
state: {
  doc: WarppDocument | null; canvas: WarppCanvas; catalog: WarppCatalog | null;
  selectedPath: string | null;           // "tpl" or "per::t"
  diagnostics: WarppDiagnostic[]; dirty: boolean; workflows: WarppWorkflowSummary[];
}
getters: {
  manifestByType(type): WarppManifest | undefined;
  flowNodes(): VfNode[];   // flattened doc nodes incl. map children (parentNode set), positions from canvas (fallback grid), data: { node, scopePath }
  flowEdges(): VfEdge[];   // derived from bindings: for each from-ref -> { id: `${src}->${dst}::${port}`, source, sourceHandle: srcPort, target, targetHandle: dstPort }; refs to "in"/"item" render no edge (v1)
  nodeAtPath(path): WarppNode | undefined;
}
actions: {
  loadCatalog(); loadList(); load(id); create(id, name);
  addNode(type: string, pos: {x,y}, parentPath?: string): string;  // id = short type tail + N (unique in scope); map nodes get an empty body {nodes:[],outputs:{}}
  removeNode(path): void;      // strips any binding in any scope that references the removed node id within that scope
  setLiteral(path, port, value: unknown): void;      // sets {value}, marks dirty
  clearInput(path, port): void;
  wire(fromPath, fromPort, toPath, toPort): boolean; // same-scope or outer->inner only (by path prefix); scalar -> {from}; list-variadic -> append; returns false when port unknown
  unwire(toPath, toPort, index?: number): void;
  setNamedVar(path, port, key, binding: WarppBinding): void;
  setPosition(path, x, y): void;
  save(): Promise<boolean>;    // PUT; on WarppValidationError -> diagnostics populated, returns false
  runValidate(): Promise<void>;
}
```

Ref-string rule: a wire from node `s` port `text` to node `t` port `value` writes `t.inputs.value = { from: "s.text" }` — path scope prefixes are STRIPPED in the document (documents know only scope-local ids + `in`/`item`; the `::` paths exist only in the editor/Vue Flow layer).

- [ ] **Step 1: Write the failing tests**

```ts
// src/stores/__tests__/warppEditor.spec.ts
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";

vi.mock("@/api/warpp", () => ({
  fetchCatalog: vi.fn(async () => ({
    manifests: [
      { type: "data.stringify", title: "Stringify", category: "data",
        inputs: [{ name: "value", type: "T", required: true }],
        outputs: [{ name: "text", type: "text" }] },
      { type: "logic.coalesce", title: "Coalesce", category: "logic",
        inputs: [{ name: "values", type: "T", required: true, variadic: "list" }],
        outputs: [{ name: "value", type: "T" }] },
      { type: "control.map", title: "Map", category: "control",
        inputs: [{ name: "items", type: "list<T>", required: true }],
        outputs: [{ name: "results", type: "dynamic:body" }] },
    ],
    coercions: [["number", "text"], ["boolean", "text"]],
    workflows: [],
  })),
  listWorkflows: vi.fn(async () => ({ workflows: [] })),
  saveWorkflow: vi.fn(async (_id: string, p: unknown) => p),
  validateWorkflow: vi.fn(async () => ({ valid: true })),
}));

import { useWarppEditor } from "../warppEditor";

describe("warppEditor", () => {
  beforeEach(async () => {
    setActivePinia(createPinia());
    const store = useWarppEditor();
    await store.loadCatalog();
    store.create("wf", "WF");
  });

  it("adds nodes with unique scope-local ids and derives edges from wires", () => {
    const store = useWarppEditor();
    const a = store.addNode("data.stringify", { x: 0, y: 0 });
    const b = store.addNode("data.stringify", { x: 200, y: 0 });
    expect(a).not.toBe(b);
    expect(store.wire(a, "text", b, "value")).toBe(true);
    const node = store.nodeAtPath(b)!;
    expect(node.inputs!.value).toEqual({ from: `${a}.text` });
    const edge = store.flowEdges.find((e) => e.target === b);
    expect(edge).toBeTruthy();
    expect(edge!.sourceHandle).toBe("text");
  });

  it("appends to list-variadic ports", () => {
    const store = useWarppEditor();
    const a = store.addNode("data.stringify", { x: 0, y: 0 });
    const b = store.addNode("data.stringify", { x: 0, y: 100 });
    const c = store.addNode("logic.coalesce", { x: 200, y: 50 });
    store.wire(a, "text", c, "values");
    store.wire(b, "text", c, "values");
    const bindings = store.nodeAtPath(c)!.inputs!.values as { from: string }[];
    expect(bindings).toHaveLength(2);
  });

  it("map children live in the body with scope-local refs", () => {
    const store = useWarppEditor();
    const m = store.addNode("control.map", { x: 0, y: 0 });
    const child = store.addNode("data.stringify", { x: 10, y: 10 }, m);
    expect(child).toBe(`${m}::${child.split("::")[1]}`);
    const mapNode = store.nodeAtPath(m)!;
    expect(mapNode.body!.nodes).toHaveLength(1);
    const vf = store.flowNodes.find((n) => n.id === child)!;
    expect(vf.parentNode).toBe(m);
  });

  it("removeNode strips dangling bindings", () => {
    const store = useWarppEditor();
    const a = store.addNode("data.stringify", { x: 0, y: 0 });
    const b = store.addNode("data.stringify", { x: 200, y: 0 });
    store.wire(a, "text", b, "value");
    store.removeNode(a);
    expect(store.nodeAtPath(b)!.inputs!.value).toBeUndefined();
    expect(store.flowEdges).toHaveLength(0);
  });

  it("setLiteral marks dirty and survives save payload", async () => {
    const store = useWarppEditor();
    const a = store.addNode("data.stringify", { x: 0, y: 0 });
    store.setLiteral(a, "value", "hello");
    expect(store.dirty).toBe(true);
    expect(store.nodeAtPath(a)!.inputs!.value).toEqual({ value: "hello" });
    await store.save();
    expect(store.dirty).toBe(false);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web/agentd-ui && npx vitest run src/stores/__tests__/warppEditor.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the store** per the interface block (positions default to a simple grid `x = 80 + 240*(count%4), y = 80 + 160*floor(count/4)` when canvas has no entry; `addNode` id generation: last segment of type + counter, e.g. `stringify1`, `map1`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web/agentd-ui && npx vitest run`
Expected: PASS (both spec files).

- [ ] **Step 5: Commit**

```bash
git add web/agentd-ui/src/stores/warppEditor.ts web/agentd-ui/src/stores/__tests__/warppEditor.spec.ts
git commit -m "feat(warpp-ui): add editor store with derived graph and wire ops"
```

### Task 19: Run store

**Files:**
- Create: `web/agentd-ui/src/stores/warppRun.ts`
- Test: `web/agentd-ui/src/stores/__tests__/warppRun.spec.ts`

**Interfaces:**
- Produces (Pinia store `useWarppRun`): state `{ runId: string | null; status: string; events: WarppRunEvent[]; nodeStatus: Record<string, string>; nodeOutputs: Record<string, Record<string, unknown>> }`; actions `start(workflowId, input)` (calls `startRun` then `streamRunEvents`, folding each event: node events update `nodeStatus[node_path]` to started/completed/failed/skipped/retrying and stash `outputs`; run events update `status`), `ingest(ev: WarppRunEvent)` (the pure folding function — exported for tests), `reset()`, `stop()` (cancels the stream).

- [ ] **Step 1: Write the failing test** — feed `ingest` a scripted sequence (run_started, node_started tpl, node_completed tpl with outputs, node_skipped down, run_completed with status completed_with_skips) and assert `nodeStatus`, `nodeOutputs.tpl.text`, and final `status`. Mock `@/api/warpp` like Task 18.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web/agentd-ui && npx vitest run src/stores/__tests__/warppRun.spec.ts`
Expected: FAIL.

- [ ] **Step 3: Implement the store.**

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web/agentd-ui && npx vitest run`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/agentd-ui/src/stores/warppRun.ts web/agentd-ui/src/stores/__tests__/warppRun.spec.ts
git commit -m "feat(warpp-ui): add run store folding SSE events into node state"
```

### Task 20: Components + view shell

**Files:**
- Create: `web/agentd-ui/src/components/warpp/NodeCard.vue`
- Create: `web/agentd-ui/src/components/warpp/CatalogPanel.vue`
- Create: `web/agentd-ui/src/components/warpp/InspectorPanel.vue`
- Create: `web/agentd-ui/src/components/warpp/CanvasPane.vue`
- Create: `web/agentd-ui/src/components/warpp/RunTimeline.vue`
- Create: `web/agentd-ui/src/views/WarppView.vue`

**Interfaces & component contracts:**
- `NodeCard.vue` — custom Vue Flow node (registered as type `warpp`). Props via `data: { node: WarppNode; scopePath: string }`. Renders: title (manifest title + node id), one `<Handle type="target" :id="port.name">` per manifest input (left side) and `<Handle type="source" :id="port.name">` per output (right side), each with `:style="{ background: portColor(port.type) }"` and a small type label; unwired-literal summary chips; run status class from `useWarppRun().nodeStatus[path]` (`is-running/is-completed/is-failed/is-skipped`); completed output values rendered under the card (first 120 chars, `JSON.stringify` for objects).
- `CatalogPanel.vue` — groups `catalog.manifests` by category; search input filters by type/title; click adds via `store.addNode(type, centerOfViewport())`; map-targeted add uses the selected map node as parent when one is selected.
- `InspectorPanel.vue` — for `selectedPath`: name/id header, delete button; per manifest input port: wired → `wired from <ref>` + unwire button; else widget by parsed type — `text`/`file`: text input; `number`: number input; `boolean`: checkbox; `json` + `list<*>`: textarea with JSON parse-on-blur (invalid JSON → inline error, no store write); variadic list → rows with per-row unwire; variadic named → key/binding rows + "add var" (key prompt, literal input); policy section (timeout text, retries max number, backoff select fixed/exponential, on_error select fail/skip); for the workflow itself (no selection): workflow inputs editor (rows: name/type select/required checkbox) and outputs editor (name + `node.port` ref picker fed by upstream node output ports), publish-as-tool toggle.
- `CanvasPane.vue` — `<VueFlow :nodes="store.flowNodes" :edges="store.flowEdges" :node-types="{ warpp: NodeCard }">` with `<Background/>`, `<MiniMap/>`, `<Controls/>`; `:is-valid-connection` = both handles exist and `assignable(srcPortType, dstPortType, coercions)`; `@connect` → `store.wire(...)`; `@node-drag-stop` → `store.setPosition`; `@edge-double-click` (or an edge context button) → `store.unwire(target, targetHandle)`; node click → `store.selectedPath = id`. Import Vue Flow the same way the old `FlowView.vue:827-850` does.
- `RunTimeline.vue` — table of `useWarppRun().events` (badge per type, node_path, message/error, elapsed).
- `WarppView.vue` — three-pane layout (Catalog | Canvas | Inspector) + toolbar: workflow dropdown (`store.workflows` + load), name input, New / Save / Validate / Run buttons, diagnostics strip (each diagnostic: severity badge + code + message; click selects `path`'s node when derivable), run input dialog (one field per workflow input, typed like Inspector widgets) → `useWarppRun().start`, RunTimeline drawer toggle. Follow the chrome/classes of an existing view (open `src/views/PulseView.vue` and reuse its panel/toolbar classes) — no new design system.

- [ ] **Step 1: Implement all six files.** No component unit tests (stores and lib carry the tested logic); the gate is typecheck + build + the Task 21 smoke.

- [ ] **Step 2: Typecheck + build**

Run: `cd web/agentd-ui && npm run build`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add web/agentd-ui/src/components/warpp/ web/agentd-ui/src/views/WarppView.vue
git commit -m "feat(warpp-ui): typed-port canvas editor, inspector, catalog, run overlay"
```

### Task 21: Router swap, frontend deletions, full verification

**Files:**
- Modify: `web/agentd-ui/src/router/index.ts:80-91` (same `/flow` path and nav meta; `component: () => import("@/views/WarppView.vue")`, `purpose: "Typed-port workflow builder"`)
- Delete: `web/agentd-ui/src/views/FlowView.vue`, `src/lib/flowEditorCompat.ts`, `src/components/flow/` (entire directory), `src/types/flow.ts`, `src/types/flowV2.ts`, `src/types/flowEditor.ts`, `src/constants/flowNodes.ts`, `src/stores/flowRun.ts`, `src/api/flow.ts`

- [ ] **Step 1: Swap the route and delete**

```bash
cd web/agentd-ui
git rm -r src/components/flow src/views/FlowView.vue src/lib/flowEditorCompat.ts \
  src/types/flow.ts src/types/flowV2.ts src/types/flowEditor.ts \
  src/constants/flowNodes.ts src/stores/flowRun.ts src/api/flow.ts
grep -rn "flowEditorCompat\|types/flowV2\|types/flowEditor\|api/flow\"\|stores/flowRun\|constants/flowNodes\|FlowView" src/
```

Expected grep: no output (fix any straggler imports it reveals — e.g. icon components or view registries).

- [ ] **Step 2: Full frontend gates**

Run: `cd web/agentd-ui && npx vitest run && npm run lint && npm run build`
Expected: all green.

- [ ] **Step 3: Full-stack smoke (manual, driven via the browser preview)** — start agentd (however this repo's dev flow runs it — check `README.md`/`Makefile`/`.claude/launch.json` for the dev command) and `npm run dev`; in the app at `/flow`: create workflow `smoke`, add `data.constant` (value `["a","b"]`, as `list<json>`) → `control.map` (wire items) with body `data.stringify` (wire `item.value`) → set body output; add workflow output from `map.results`; Save (expect no diagnostics), Run, watch node chips go running→completed and values appear; confirm `GET /api/warpp/runs/{id}/events` returned the node paths `map1[0].stringify1` style. Screenshot the completed run for the PR.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat(warpp-ui)!: swap /flow to the new editor and delete the legacy flow UI"
```

### Task 22: Final review gate

- [ ] **Step 1: Full-repo verification**

```bash
go build ./... && go vet ./... && go test ./... -count=1
cd web/agentd-ui && npx vitest run && npm run build
```

Expected: everything green.

- [ ] **Step 2: Spec conformance sweep** — reread `docs/superpowers/specs/2026-07-12-warpp-workflows-redesign-design.md` section by section and check each promise against the code (§4 document format, §5 types/coercions, §6 node tables incl. every builtin and adapter port name, §7 semantics, §8 events/validation, §9 agent tools/subflows, §10 API table, §11 editor behaviors, §12 deletions — re-run the Task 14 and Task 21 greps, §13 test inventory). Fix gaps before proceeding.

- [ ] **Step 3: Use superpowers:requesting-code-review** for the branch, then superpowers:finishing-a-development-branch (target branch: `idev`).

---

## Plan self-review notes (already applied)

- `Document.ProjectID` is added in Task 12 (not Task 2) — the engine never reads it; only the service does for sandbox context.
- The subflow resolver is stubbed in Task 12 and completed in Task 13; the published-tool sync stub in Task 12 is likewise completed in Task 13. Neither stub contains dead placeholders — they are empty hooks with doc comments, replaced one task later.
- Event `Sequence` is always assigned by `warppRuntime.appendRunEvent` (or durable recording), never by the engine.



