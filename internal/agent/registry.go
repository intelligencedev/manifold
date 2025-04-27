package agent

import (
	"context"
	"fmt"
	"sync"
)

// Registry is threadsafe and lazily initialises tools once.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

func (r *Registry) Register(name string, t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = t
}

func (r *Registry) Spec() []ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolSpec, 0, len(r.tools))
	for n, t := range r.tools {
		spec := t.Describe()
		spec.Name = n
		out = append(out, spec)
	}
	return out
}

func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (any, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	return t.Execute(ctx, args)
}
