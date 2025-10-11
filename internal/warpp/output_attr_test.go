package warpp

import (
    "context"
    "encoding/json"
    "testing"

    "manifold/internal/tools"
)

// minimal tool that returns a small JSON document
type miniEcho struct{ name string }

func (m miniEcho) Name() string               { return m.name }
func (m miniEcho) JSONSchema() map[string]any { return map[string]any{"description": "mini"} }
func (m miniEcho) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    var a any
    _ = json.Unmarshal(raw, &a)
    return map[string]any{"ok": true, "value": "XYZ", "n": 7}, nil
}

func TestGenericOutputAttrFromAnyNode_JSONAndValue(t *testing.T) {
    reg := tools.NewRegistry()
    reg.Register(miniEcho{name: "echo_json"})
    runner := Runner{Tools: reg}

    // Set from JSON payload
    step := Step{ID: "s1", Tool: &ToolRef{Name: "echo_json", Args: map[string]any{"output_attr": "left_val", "output_from": "json.value"}}}
    _, delta, _, err := runner.runStep(context.Background(), step, Attrs{"utter": "u"})
    if err != nil {
        t.Fatalf("runStep error: %v", err)
    }
    if delta["left_val"] != "XYZ" {
        t.Fatalf("expected left_val=XYZ, got %v", delta["left_val"])
    }

    // Set from explicit value with templating
    step2 := Step{ID: "s2", Tool: &ToolRef{Name: "echo_json", Args: map[string]any{"output_attr": "right_val", "output_value": "${A.utter}"}}}
    _, delta2, args2, err := runner.runStep(context.Background(), step2, Attrs{"utter": "hello"})
    if err != nil {
        t.Fatalf("runStep error: %v", err)
    }
    if delta2["right_val"] != "hello" {
        t.Fatalf("expected right_val=hello, got %v", delta2["right_val"])
    }
    if args2 == nil {
        t.Fatalf("expected rendered args present")
    }
}

