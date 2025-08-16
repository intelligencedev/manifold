package databases

import (
	"context"
	"sort"
	"sync"
)

type edgeKey struct{ src, rel string }

type memoryGraph struct {
	mu    sync.RWMutex
	nodes map[string]Node
	edges map[edgeKey]map[string]map[string]any // key:(src,rel) -> dst -> props
}

func NewMemoryGraph() GraphDB {
	return &memoryGraph{
		nodes: make(map[string]Node),
		edges: make(map[edgeKey]map[string]map[string]any),
	}
}

func (m *memoryGraph) UpsertNode(_ context.Context, id string, labels []string, props map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make(map[string]any, len(props))
	for k, v := range props {
		cp[k] = v
	}
	m.nodes[id] = Node{ID: id, Labels: append([]string{}, labels...), Props: cp}
	return nil
}

func (m *memoryGraph) UpsertEdge(_ context.Context, srcID, rel, dstID string, props map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := edgeKey{src: srcID, rel: rel}
	m.ensureEdgeKey(key)
	cp := make(map[string]any, len(props))
	for k, v := range props {
		cp[k] = v
	}
	m.edges[key][dstID] = cp
	return nil
}

func (m *memoryGraph) Neighbors(_ context.Context, id string, rel string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := edgeKey{src: id, rel: rel}
	var out []string
	if dsts, ok := m.edges[key]; ok {
		for dst := range dsts {
			out = append(out, dst)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *memoryGraph) GetNode(_ context.Context, id string) (Node, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.nodes[id]
	return n, ok
}

func (m *memoryGraph) ensureEdgeKey(k edgeKey) {
	if _, ok := m.edges[k]; !ok {
		m.edges[k] = make(map[string]map[string]any)
	}
}
