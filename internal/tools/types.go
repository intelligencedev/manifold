package tools

import (
    "context"
    "encoding/json"

    "gptagent/internal/llm"
    "gptagent/internal/observability"
)

// Tool is an executable capability the agent can call.
type Tool interface {
	Name() string
	JSONSchema() map[string]any
	Call(ctx context.Context, raw json.RawMessage) (any, error)
}

// Registry keeps track of tools and dispatches calls by name.
type Registry interface {
	Schemas() []llm.ToolSchema
	Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error)
	Register(t Tool)
}

type defaultRegistry struct {
    byName map[string]Tool
    logPayloads bool
}

// NewRegistry returns a basic in-memory registry.
func NewRegistry() Registry { return NewRegistryWithLogging(false) }

// NewRegistryWithLogging allows enabling redacted payload logging for tool args/results.
func NewRegistryWithLogging(logPayloads bool) Registry {
    return &defaultRegistry{byName: make(map[string]Tool), logPayloads: logPayloads}
}

func (r *defaultRegistry) Register(t Tool) { r.byName[t.Name()] = t }

func (r *defaultRegistry) Schemas() []llm.ToolSchema {
	out := make([]llm.ToolSchema, 0, len(r.byName))
	for name, t := range r.byName {
		schema := t.JSONSchema()
		out = append(out, llm.ToolSchema{
			Name:        name,
			Description: strFrom(schema["description"]),
			Parameters:  mapFrom(schema["parameters"]),
		})
	}
	return out
}

func (r *defaultRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
    t := r.byName[name]
    if t == nil {
        observability.LoggerWithTrace(ctx).Error().Str("tool", name).Msg("tool_not_found")
        return []byte(`{"error":"tool not found"}`), nil
    }
    if r.logPayloads {
        observability.LoggerWithTrace(ctx).Debug().Str("tool", name).RawJSON("args", observability.RedactJSON(raw)).Msg("tool_dispatch")
    }
    val, err := t.Call(ctx, raw)
    if err != nil {
        // return structured error payload
        b, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
        observability.LoggerWithTrace(ctx).Error().Str("tool", name).Err(err).Msg("tool_error")
        return b, nil
    }
    b, _ := json.Marshal(val)
    if r.logPayloads {
        observability.LoggerWithTrace(ctx).Debug().Str("tool", name).RawJSON("payload", observability.RedactJSON(b)).Msg("tool_ok")
    }
    return b, nil
}

func strFrom(v any) string         { s, _ := v.(string); return s }
func mapFrom(v any) map[string]any { m, _ := v.(map[string]any); return m }
