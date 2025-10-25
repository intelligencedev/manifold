package specialists_tool

import (
	"context"
	"encoding/json"

	"manifold/internal/specialists"
)

type registryContextKey struct{}

// WithRegistry attaches a per-request specialists registry to the context so
// tool invocations can resolve user-specific specialists.
func WithRegistry(ctx context.Context, reg *specialists.Registry) context.Context {
	if reg == nil {
		return ctx
	}
	return context.WithValue(ctx, registryContextKey{}, reg)
}

func registryFromContext(ctx context.Context) *specialists.Registry {
	if v := ctx.Value(registryContextKey{}); v != nil {
		if reg, ok := v.(*specialists.Registry); ok {
			return reg
		}
	}
	return nil
}

// Tool exposes configured specialists as a single callable tool.
type Tool struct {
	reg *specialists.Registry
}

func New(reg *specialists.Registry) *Tool { return &Tool{reg: reg} }

func (t *Tool) Name() string { return "specialists_infer" }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Invoke a configured specialist (inference-only). Use this for domain-specific expertise like code review or structured extraction.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"specialist": map[string]any{
					"type":        "string",
					"description": "Name of the specialist to invoke (use /api/specialists to discover available names).",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "The input for the specialist",
				},
				"override_reasoning_effort": map[string]any{
					"type":        "string",
					"description": "Optional override for reasoning effort",
					"enum":        []string{"low", "medium", "high"},
				},
			},
			"required": []string{"specialist", "prompt"},
		},
	}
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Specialist string `json:"specialist"`
		Prompt     string `json:"prompt"`
		OverrideRE string `json:"override_reasoning_effort"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	reg := registryFromContext(ctx)
	if reg == nil {
		reg = t.reg
	}
	if reg == nil {
		return map[string]any{"ok": false, "error": "no specialists configured"}, nil
	}
	a, ok := reg.Get(args.Specialist)
	if !ok {
		return map[string]any{"ok": false, "error": "unknown specialist"}, nil
	}
	// Shallow copy to apply temporary override without affecting registry entry.
	a2 := *a
	if args.OverrideRE != "" {
		a2.ReasoningEffort = args.OverrideRE
	}
	out, err := a2.Inference(ctx, args.Prompt, nil)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{
		"ok":                    true,
		"specialist":            a.Name,
		"model":                 a.Model,
		"used_reasoning_effort": a2.ReasoningEffort,
		"output":                out,
	}, nil
}
