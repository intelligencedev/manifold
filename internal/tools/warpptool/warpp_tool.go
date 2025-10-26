package warpptool

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "manifold/internal/tools"
    "manifold/internal/warpp"
)

// ToolPrefix is the name prefix used for WARPP workflow tools.
const ToolPrefix = "warpp_"

// tool is a tools.Tool implementation that runs a specific WARPP workflow
// when invoked. It exposes a single required parameter: query.
type tool struct {
    name        string
    intent      string
    description string
    runner      *warpp.Runner
}

// Name returns the registered tool name.
func (t *tool) Name() string { return t.name }

func (t *tool) JSONSchema() map[string]any {
    desc := t.description
    if strings.TrimSpace(desc) == "" {
        desc = fmt.Sprintf("Run WARPP workflow '%s' with a natural language query", t.intent)
    }
    return map[string]any{
        "name":        t.name,
        "description": desc,
        "parameters": map[string]any{
            "type": "object",
            "properties": map[string]any{
                "query": map[string]any{
                    "type":        "string",
                    "description": "Natural language request passed to the workflow (available as A.utter/query)",
                },
            },
            "required": []string{"query"},
        },
    }
}

func (t *tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    var args struct {
        Query string `json:"query"`
    }
    if err := json.Unmarshal(raw, &args); err != nil {
        return map[string]any{"ok": false, "error": "invalid args"}, nil
    }
    q := strings.TrimSpace(args.Query)
    if q == "" {
        return map[string]any{"ok": false, "error": "query required"}, nil
    }

    if t.runner == nil || t.runner.Workflows == nil {
        return map[string]any{"ok": false, "error": "warpp runner not initialized"}, nil
    }
    // Lookup workflow by intent (stored on tool)
    wf, err := t.runner.Workflows.Get(t.intent)
    if err != nil {
        return map[string]any{"ok": false, "error": "workflow not found"}, nil
    }
    // Build attributes and personalize
    attrs := warpp.Attrs{"utter": q}
    wfStar, _, A, err := t.runner.Personalize(ctx, wf, attrs)
    if err != nil {
        return map[string]any{"ok": false, "error": err.Error()}, nil
    }
    // Build allowed tool allowlist from personalized workflow
    allowed := map[string]bool{}
    for _, s := range wfStar.Steps {
        if s.Tool != nil {
            allowed[s.Tool.Name] = true
        }
    }
    summary, err := t.runner.Execute(ctx, wfStar, allowed, A, nil)
    if err != nil {
        return map[string]any{"ok": false, "error": err.Error()}, nil
    }
    return map[string]any{"ok": true, "summary": summary, "intent": t.intent}, nil
}

// RegisterAll registers one tool per workflow in wfreg into reg, using the
// provided runner to execute them. Tool names are prefixed with "warpp_".
func RegisterAll(reg tools.Registry, runner *warpp.Runner) {
    if reg == nil || runner == nil || runner.Workflows == nil {
        return
    }
    for _, w := range runner.Workflows.All() {
        name := ToolPrefix + sanitize(w.Intent)
        reg.Register(&tool{name: name, intent: w.Intent, description: w.Description, runner: runner})
    }
}

func sanitize(intent string) string {
    s := strings.TrimSpace(strings.ToLower(intent))
    var b strings.Builder
    b.Grow(len(s))
    for _, r := range s {
        switch {
        case r >= 'a' && r <= 'z':
            b.WriteRune(r)
        case r >= '0' && r <= '9':
            b.WriteRune(r)
        case r == '-' || r == '_':
            b.WriteRune(r)
        case r == ' ':
            b.WriteByte('_')
        }
    }
    out := b.String()
    if out == "" {
        return "workflow"
    }
    return out
}

