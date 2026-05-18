package durable

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type Handler func(ctx context.Context, params map[string]any) (map[string]any, error)

type HandlerSpec struct {
	Queue string
	Name  string
	Fn    Handler
}

type Registry struct {
	mu       sync.RWMutex
	handlers map[string]HandlerSpec
}

func NewRegistry() *Registry {
	return &Registry{handlers: map[string]HandlerSpec{}}
}

func (r *Registry) Register(queue, name string, fn Handler) {
	if r == nil || fn == nil {
		return
	}
	queue = normalizeQueue(queue)
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[handlerKey(queue, name)] = HandlerSpec{Queue: queue, Name: name, Fn: fn}
}

func (r *Registry) Get(queue, name string) (HandlerSpec, bool) {
	if r == nil {
		return HandlerSpec{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.handlers[handlerKey(normalizeQueue(queue), strings.TrimSpace(name))]
	return spec, ok
}

func (r *Registry) Queues() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	set := map[string]struct{}{}
	for _, spec := range r.handlers {
		set[spec.Queue] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for queue := range set {
		out = append(out, queue)
	}
	sort.Strings(out)
	return out
}

func normalizeQueue(queue string) string {
	queue = strings.TrimSpace(queue)
	if queue == "" {
		return DefaultQueue
	}
	return queue
}

func handlerKey(queue, name string) string {
	return queue + "\x00" + name
}
