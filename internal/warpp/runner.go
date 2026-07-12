package warpp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// NodeInputs carries the resolved inputs for one node execution. Scalar ports
// land in Values; list-variadic ports in List; named-variadic ports in Named.
type NodeInputs struct {
	Values map[string]Value
	List   map[string][]Value
	Named  map[string]map[string]Value
}

// RunnerCtx carries per-execution context to a node runner.
type RunnerCtx struct {
	Path     string
	Node     *Node
	Manifest Manifest
}

// NodeRunner executes one node, returning typed values per output port.
type NodeRunner func(ctx context.Context, rc RunnerCtx, in NodeInputs) (map[string]Value, error)

// SelectPath traverses structured data by dot/index path. It never parses
// strings mid-path (unlike the legacy system).
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
// implicit coercion table when the value does not already conform.
func CoerceRaw(data any, to Type) (Value, error) {
	if err := Conforms(data, to); err == nil {
		return Value{Type: to, Data: data}, nil
	}
	return Coerce(Value{Type: InferLiteral(data), Data: data}, to)
}

func renderScalar(v Value) string {
	switch v.Type.Kind {
	case KindText, KindFile:
		s, _ := v.Data.(string)
		return s
	case KindNumber, KindBoolean:
		s, _ := stringify(v)
		return s
	default:
		b, err := json.Marshal(v.Data)
		if err != nil {
			return fmt.Sprintf("%v", v.Data)
		}
		return string(b)
	}
}

func parseAs(in NodeInputs) (Type, error) {
	as := "json"
	if v, ok := in.Values["as"]; ok {
		if s, ok := v.Data.(string); ok && s != "" {
			as = s
		}
	}
	if !dynamicAsEnum[as] {
		return Type{}, fmt.Errorf("invalid as %q", as)
	}
	return ParseType(as)
}

var templateSlot = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// BuiltinRunners returns runners for every data.* and logic.* node type.
// control.map is executed by the engine; llm.generate is injected by the
// service via LLMRunner.
func BuiltinRunners() map[string]NodeRunner {
	return map[string]NodeRunner{
		"data.extract":       runExtract,
		"data.template":      runTemplate,
		"data.merge":         runMerge,
		"data.stringify":     runStringify,
		"data.parse":         runParse,
		"data.constant":      runConstant,
		"logic.if":           runIf,
		"logic.coalesce":     runCoalesce,
		"logic.equals":       runEquals,
		"logic.contains":     runContains,
		"logic.not":          runNot,
		"logic.greater_than": runGreaterThan,
	}
}

func runExtract(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	source := in.Values["source"]
	path, _ := in.Values["path"].Data.(string)
	found, ok := SelectPath(source.Data, path)
	if !ok {
		return nil, fmt.Errorf("path not found: %s", path)
	}
	asType, err := parseAs(in)
	if err != nil {
		return nil, err
	}
	v, err := CoerceRaw(found, asType)
	if err != nil {
		return nil, err
	}
	return map[string]Value{"value": v}, nil
}

func runTemplate(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	tmpl, _ := in.Values["template"].Data.(string)
	vars := in.Named["vars"]
	var missing string
	out := templateSlot.ReplaceAllStringFunc(tmpl, func(slot string) string {
		name := slot[1 : len(slot)-1]
		v, ok := vars[name]
		if !ok {
			if missing == "" {
				missing = name
			}
			return slot
		}
		return renderScalar(v)
	})
	if missing != "" {
		return nil, fmt.Errorf("template var %q not provided", missing)
	}
	return map[string]Value{"text": NewText(out)}, nil
}

func runMerge(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	merged := map[string]any{}
	for i, v := range in.List["objects"] {
		m, ok := v.Data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("merge input %d is not an object", i)
		}
		for k, val := range m {
			merged[k] = val
		}
	}
	return map[string]Value{"json": {Type: Type{Kind: KindJSON}, Data: merged}}, nil
}

func runStringify(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	v := in.Values["value"]
	switch v.Type.Kind {
	case KindText, KindFile, KindNumber, KindBoolean:
		s, err := stringify(v)
		if err != nil {
			return nil, err
		}
		return map[string]Value{"text": NewText(s)}, nil
	default:
		b, err := json.MarshalIndent(v.Data, "", "  ")
		if err != nil {
			return nil, err
		}
		return map[string]Value{"text": NewText(string(b))}, nil
	}
}

func runParse(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	text, _ := in.Values["text"].Data.(string)
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return map[string]Value{"json": {Type: Type{Kind: KindJSON}, Data: parsed}}, nil
}

func runConstant(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	asType, err := parseAs(in)
	if err != nil {
		return nil, err
	}
	v, err := CoerceRaw(in.Values["value"].Data, asType)
	if err != nil {
		return nil, err
	}
	return map[string]Value{"value": v}, nil
}

func runIf(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	cond, _ := in.Values["condition"].Data.(bool)
	value := in.Values["value"]
	if cond {
		return map[string]Value{"then": value}, nil
	}
	return map[string]Value{"else": value}, nil
}

func runCoalesce(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	values := in.List["values"]
	if len(values) == 0 {
		return nil, fmt.Errorf("coalesce received no fired values")
	}
	return map[string]Value{"value": values[0]}, nil
}

func runEquals(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	a := in.Values["a"]
	b := in.Values["b"]
	return boolResult(reflect.DeepEqual(a.Data, b.Data)), nil
}

func runContains(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	haystack, _ := in.Values["haystack"].Data.(string)
	needle, _ := in.Values["needle"].Data.(string)
	return boolResult(strings.Contains(haystack, needle)), nil
}

func runNot(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	b, _ := in.Values["value"].Data.(bool)
	return boolResult(!b), nil
}

func runGreaterThan(_ context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
	a, _ := asNumber(in.Values["a"].Data)
	b, _ := asNumber(in.Values["b"].Data)
	return boolResult(a > b), nil
}

func boolResult(v bool) map[string]Value {
	return map[string]Value{"result": {Type: Type{Kind: KindBoolean}, Data: v}}
}

// ChatFunc is the single-completion callback the LLM node runs on.
type ChatFunc func(ctx context.Context, instruction, input, model string) (string, error)

// LLMRunner builds the runner registered under llm.generate.
func LLMRunner(chat ChatFunc) NodeRunner {
	return func(ctx context.Context, _ RunnerCtx, in NodeInputs) (map[string]Value, error) {
		instruction, _ := in.Values["instruction"].Data.(string)
		input, _ := in.Values["input"].Data.(string)
		model, _ := in.Values["model"].Data.(string)
		out, err := chat(ctx, instruction, input, model)
		if err != nil {
			return nil, err
		}
		return map[string]Value{"text": NewText(out)}, nil
	}
}
