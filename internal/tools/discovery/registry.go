package discovery

import (
	"context"
	"encoding/json"
	"sync"

	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/tools"
)

const defaultMaxDiscoveredTools = 20

type DiscoverableRegistry struct {
	base          tools.Registry
	index         *ToolIndex
	searchTool    tools.Tool
	maxDiscovered int

	mu       sync.RWMutex
	active   map[string]bool
	promoted map[string]bool
}

func NewDiscoverableRegistry(base tools.Registry, index *ToolIndex, bootstrapNames []string, maxDiscovered int) *DiscoverableRegistry {
	if maxDiscovered <= 0 {
		maxDiscovered = defaultMaxDiscoveredTools
	}
	reg := &DiscoverableRegistry{
		base:          base,
		index:         index,
		maxDiscovered: maxDiscovered,
		active:        make(map[string]bool, len(bootstrapNames)),
		promoted:      make(map[string]bool),
	}
	if index != nil {
		for _, name := range bootstrapNames {
			if result, ok := index.Lookup(name); ok {
				reg.active[result.Name] = true
			}
		}
	}
	reg.searchTool = NewSearchTool(index, reg)
	return reg
}

func (r *DiscoverableRegistry) Register(t tools.Tool) {
	r.base.Register(t)
}

func (r *DiscoverableRegistry) Unregister(name string) {
	r.base.Unregister(name)
	r.mu.Lock()
	delete(r.active, name)
	delete(r.promoted, name)
	r.mu.Unlock()
}

func (r *DiscoverableRegistry) Schemas() []llm.ToolSchema {
	baseSchemas := r.base.Schemas()
	r.mu.RLock()
	out := make([]llm.ToolSchema, 0, len(baseSchemas)+1)
	for _, schema := range baseSchemas {
		if r.active[schema.Name] {
			out = append(out, schema)
		}
	}
	r.mu.RUnlock()
	searchSchema := r.searchTool.JSONSchema()
	out = append(out, llm.ToolSchema{
		Name:        r.searchTool.Name(),
		Description: schemaString(searchSchema["description"]),
		Parameters:  schemaMap(searchSchema["parameters"]),
	})
	return out
}

func (r *DiscoverableRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
	if name == r.searchTool.Name() {
		result, err := r.searchTool.Call(ctx, raw)
		if err != nil {
			payload, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
			return payload, nil
		}
		payload, _ := json.Marshal(result)
		return payload, nil
	}
	return r.base.Dispatch(ctx, name, raw)
}

func (r *DiscoverableRegistry) Promote(names []string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	promoted := make([]string, 0, len(names))
	for _, name := range names {
		if r.index != nil {
			if _, ok := r.index.Lookup(name); !ok {
				continue
			}
		}
		if r.active[name] {
			promoted = append(promoted, name)
			continue
		}
		if len(r.promoted) >= r.maxDiscovered {
			observability.LoggerWithTrace(context.Background()).Warn().Int("maxDiscovered", r.maxDiscovered).Str("tool", name).Msg("tool_discovery_cap_reached")
			continue
		}
		r.active[name] = true
		r.promoted[name] = true
		promoted = append(promoted, name)
	}
	if len(promoted) > 0 {
		observability.LoggerWithTrace(context.Background()).Info().Strs("tools", promoted).Msg("tool_promoted")
	}
	return promoted
}

func (r *DiscoverableRegistry) IsActive(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == r.searchTool.Name() {
		return true
	}
	return r.active[name]
}

func schemaString(value any) string {
	text, _ := value.(string)
	return text
}

func schemaMap(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}
