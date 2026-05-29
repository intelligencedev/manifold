package databases

import (
	"context"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"
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
	maps.Copy(cp, props)
	m.nodes[id] = Node{ID: id, Labels: append([]string{}, labels...), Props: cp}
	return nil
}

func (m *memoryGraph) UpsertEdge(_ context.Context, srcID, rel, dstID string, props map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := edgeKey{src: srcID, rel: rel}
	m.ensureEdgeKey(key)
	cp := make(map[string]any, len(props))
	maps.Copy(cp, props)
	m.edges[key][dstID] = cp
	return nil
}

func (m *memoryGraph) TypedUpsertEdge(ctx context.Context, srcID, graphType, rel, dstID string, props map[string]any) error {
	cp := make(map[string]any, len(props)+1)
	for k, v := range props {
		cp[k] = v
	}
	cp["graph_type"] = graphType
	return m.UpsertEdge(ctx, srcID, typedRel(graphType, rel), dstID, cp)
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

func (m *memoryGraph) TypedNeighbors(ctx context.Context, id, graphType, rel string) ([]string, error) {
	return m.Neighbors(ctx, id, typedRel(graphType, rel))
}

func (m *memoryGraph) GetNode(_ context.Context, id string) (Node, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.nodes[id]
	return n, ok
}

func (m *memoryGraph) ListMagmaEvents(_ context.Context) ([]MagmaEventSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MagmaEventSummary, 0)
	for _, node := range m.nodes {
		if !hasLabel(node.Labels, "MagmaEvent") {
			continue
		}
		event := MagmaEventSummary{ID: node.ID}
		if tenant, ok := node.Props["tenant"].(string); ok {
			event.Tenant = tenant
		}
		if session, ok := node.Props["session"].(string); ok {
			event.Session = session
		}
		if created, ok := node.Props["created_at"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, created); err == nil {
				event.CreatedAt = parsed
			}
		}
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memoryGraph) ListMagmaEdges(_ context.Context) ([]TypedEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]TypedEdge, 0)
	for key, dsts := range m.edges {
		graphType, rel, ok := splitTypedRel(key.rel)
		if !ok {
			continue
		}
		for dst, props := range dsts {
			cp := make(map[string]any, len(props))
			maps.Copy(cp, props)
			out = append(out, TypedEdge{
				Source:    key.src,
				GraphType: graphType,
				Rel:       rel,
				Target:    dst,
				Weight:    floatPropFromMap(cp, "weight"),
				Props:     cp,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].GraphType != out[j].GraphType {
			return out[i].GraphType < out[j].GraphType
		}
		if out[i].Rel != out[j].Rel {
			return out[i].Rel < out[j].Rel
		}
		return out[i].Target < out[j].Target
	})
	return out, nil
}

func (m *memoryGraph) UpsertMagmaEdgeProps(ctx context.Context, edge TypedEdge) error {
	props := make(map[string]any, len(edge.Props)+1)
	maps.Copy(props, edge.Props)
	if edge.Weight != 0 {
		props["weight"] = edge.Weight
	}
	return m.TypedUpsertEdge(ctx, edge.Source, edge.GraphType, edge.Rel, edge.Target, props)
}

func (m *memoryGraph) DeleteMagmaEdge(_ context.Context, srcID, graphType, rel, dstID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := edgeKey{src: srcID, rel: typedRel(graphType, rel)}
	if dsts, ok := m.edges[key]; ok {
		delete(dsts, dstID)
		if len(dsts) == 0 {
			delete(m.edges, key)
		}
	}
	return nil
}

func (m *memoryGraph) DeleteMagmaEvent(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nodes, id)
	for key, dsts := range m.edges {
		if key.src == id {
			delete(m.edges, key)
			continue
		}
		delete(dsts, id)
		if len(dsts) == 0 {
			delete(m.edges, key)
		}
	}
	return nil
}

func (m *memoryGraph) ensureEdgeKey(k edgeKey) {
	if _, ok := m.edges[k]; !ok {
		m.edges[k] = make(map[string]map[string]any)
	}
}

func hasLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

func splitTypedRel(rel string) (string, string, bool) {
	graphType, remainder, ok := strings.Cut(rel, ":")
	if !ok || graphType == "" || remainder == "" {
		return "", "", false
	}
	switch graphType {
	case "semantic", "temporal", "causal", "entity":
		return graphType, remainder, true
	default:
		return "", "", false
	}
}

func floatPropFromMap(props map[string]any, key string) float64 {
	switch value := props[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}
