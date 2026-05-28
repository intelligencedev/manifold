package databases

import "context"

// TypedGraphDB is an optional extension for graph backends that can store
// orthogonal graph views without encoding the graph type into the relation.
type TypedGraphDB interface {
	TypedUpsertEdge(ctx context.Context, srcID, graphType, rel, dstID string, props map[string]any) error
	TypedNeighbors(ctx context.Context, id, graphType, rel string) ([]string, error)
}

// TypedUpsertEdge writes a graph-view-specific edge. Backends that do not
// implement TypedGraphDB are supported by prefixing the relation name.
func TypedUpsertEdge(ctx context.Context, graph GraphDB, srcID, graphType, rel, dstID string, props map[string]any) error {
	if graph == nil {
		return nil
	}
	if typed, ok := graph.(TypedGraphDB); ok {
		return typed.TypedUpsertEdge(ctx, srcID, graphType, rel, dstID, props)
	}
	cp := make(map[string]any, len(props)+1)
	for k, v := range props {
		cp[k] = v
	}
	cp["graph_type"] = graphType
	return graph.UpsertEdge(ctx, srcID, typedRel(graphType, rel), dstID, cp)
}

// TypedNeighbors reads graph-view-specific neighbors with the same fallback
// relation encoding used by TypedUpsertEdge.
func TypedNeighbors(ctx context.Context, graph GraphDB, id, graphType, rel string) ([]string, error) {
	if graph == nil {
		return nil, nil
	}
	if typed, ok := graph.(TypedGraphDB); ok {
		return typed.TypedNeighbors(ctx, id, graphType, rel)
	}
	return graph.Neighbors(ctx, id, typedRel(graphType, rel))
}

func typedRel(graphType, rel string) string {
	if graphType == "" {
		return rel
	}
	return graphType + ":" + rel
}
