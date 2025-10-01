package tools

import (
	"context"
	"encoding/json"

	"manifold/internal/llm"
	"manifold/internal/observability"
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
	byName      map[string]Tool
	order       []string
	logPayloads bool
}

// filteredRegistry wraps an existing Registry and exposes only a subset of tools
// specified by allowList. If allowList is empty, all tools from the wrapped
// registry are exposed.
type filteredRegistry struct {
	base  Registry
	allow map[string]bool
}

// NewFilteredRegistry builds a Registry that only exposes tool schemas and
// allows dispatch for tools listed in allowList. If allowList is empty, the
// returned registry behaves like the provided base registry.
func NewFilteredRegistry(base Registry, allowList []string) Registry {
	allow := make(map[string]bool, len(allowList))
	for _, n := range allowList {
		allow[n] = true
	}
	return &filteredRegistry{base: base, allow: allow}
}

// NewRegistry returns a basic in-memory registry.
func NewRegistry() Registry { return NewRegistryWithLogging(false) }

// NewRegistryWithLogging allows enabling redacted payload logging for tool args/results.
func NewRegistryWithLogging(logPayloads bool) Registry {
	return &defaultRegistry{byName: make(map[string]Tool), order: make([]string, 0, 64), logPayloads: logPayloads}
}

func (r *defaultRegistry) Register(t Tool) {
	name := t.Name()
	if _, exists := r.byName[name]; !exists {
		r.order = append(r.order, name)
	}
	r.byName[name] = t
}

func (r *defaultRegistry) Schemas() []llm.ToolSchema {
	const maxToolSchemas = 128
	total := len(r.order)
	n := total
	if n > maxToolSchemas {
		n = maxToolSchemas
		observability.LoggerWithTrace(context.Background()).Warn().Int("total", total).Int("using", n).Msg("tool_schemas_trimmed_for_model_limit")
	}
	out := make([]llm.ToolSchema, 0, n)
	for i := 0; i < n; i++ {
		name := r.order[i]
		t := r.byName[name]
		schema := t.JSONSchema()
		out = append(out, llm.ToolSchema{
			Name:        name,
			Description: strFrom(schema["description"]),
			Parameters:  mapFrom(schema["parameters"]),
		})
	}
	return out
}

func (f *filteredRegistry) Register(t Tool) {
	f.base.Register(t)
}

func (f *filteredRegistry) Schemas() []llm.ToolSchema {
	// If allow map is empty, expose all
	if len(f.allow) == 0 {
		return f.base.Schemas()
	}
	src := f.base.Schemas()
	out := make([]llm.ToolSchema, 0, len(src))
	for _, s := range src {
		if f.allow[s.Name] {
			out = append(out, s)
		}
	}
	return out
}

func (f *filteredRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
	if len(f.allow) != 0 && !f.allow[name] {
		observability.LoggerWithTrace(ctx).Error().Str("tool", name).Msg("tool_not_allowed")
		return []byte(`{"error":"tool not allowed"}`), nil
	}
	return f.base.Dispatch(ctx, name, raw)
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

// Context helpers for propagating an llm.Provider to tools dispatched from
// within a specialist/agent context. Tools may choose to prefer the provider
// found in context when executing (so they use the same model/baseURL/apiKey
// as the invoking agent).
type providerCtxKey struct{}

// WithProvider returns a derived context that carries the given provider.
func WithProvider(ctx context.Context, p llm.Provider) context.Context {
	return context.WithValue(ctx, providerCtxKey{}, p)
}

// ProviderFromContext extracts a provider previously set with WithProvider,
// or nil if none was set.
func ProviderFromContext(ctx context.Context) llm.Provider {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(providerCtxKey{}); v != nil {
		if p, ok := v.(llm.Provider); ok {
			return p
		}
	}
	return nil
}
