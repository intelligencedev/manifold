package tools

import (
	"context"
	"encoding/json"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

type overlayRegistry struct {
	base  Registry
	extra map[string]Tool
	order []string
}

// NewOverlayRegistry exposes the base registry plus additional ad hoc tools
// scoped to the current engine/run.
func NewOverlayRegistry(base Registry, extras ...Tool) Registry {
	reg := &overlayRegistry{
		base:  base,
		extra: make(map[string]Tool, len(extras)),
		order: make([]string, 0, len(extras)),
	}
	for _, extra := range extras {
		if extra == nil {
			continue
		}
		name := extra.Name()
		if name == "" {
			continue
		}
		if _, exists := reg.extra[name]; !exists {
			reg.order = append(reg.order, name)
		}
		reg.extra[name] = extra
	}
	return reg
}

func (r *overlayRegistry) Schemas() []llm.ToolSchema {
	baseSchemas := r.base.Schemas()
	known := make(map[string]bool, len(baseSchemas)+len(r.extra))
	out := make([]llm.ToolSchema, 0, len(baseSchemas)+len(r.extra))
	for _, schema := range baseSchemas {
		known[schema.Name] = true
		out = append(out, schema)
	}
	for _, name := range r.order {
		extra := r.extra[name]
		if extra == nil || known[name] {
			continue
		}
		schema := addCommonWarppIO(extra.JSONSchema())
		out = append(out, llm.ToolSchema{
			Name:        name,
			Description: strFrom(schema["description"]),
			Parameters:  mapFrom(schema["parameters"]),
		})
	}
	return out
}

func (r *overlayRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
	if extra := r.extra[name]; extra != nil {
		val, err := extra.Call(ctx, raw)
		if err != nil {
			payload, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
			observability.LoggerWithTrace(ctx).Error().Str("tool", name).Err(err).Msg("tool_error")
			return payload, nil
		}
		payload, _ := json.Marshal(val)
		return payload, nil
	}
	return r.base.Dispatch(ctx, name, raw)
}

func (r *overlayRegistry) Register(t Tool) {
	r.base.Register(t)
}

func (r *overlayRegistry) Unregister(name string) {
	r.base.Unregister(name)
	if _, exists := r.extra[name]; !exists {
		return
	}
	delete(r.extra, name)
	filtered := make([]string, 0, len(r.order))
	for _, existing := range r.order {
		if existing != name {
			filtered = append(filtered, existing)
		}
	}
	r.order = filtered
}
