package db

import (
	"context"
	"encoding/json"

	"singularityio/internal/persistence/databases"
)

type graphUpsertNodeTool struct{ g databases.GraphDB }
type graphUpsertEdgeTool struct{ g databases.GraphDB }
type graphNeighborsTool struct{ g databases.GraphDB }
type graphGetNodeTool struct{ g databases.GraphDB }

func NewGraphUpsertNodeTool(g databases.GraphDB) *graphUpsertNodeTool {
	return &graphUpsertNodeTool{g: g}
}
func NewGraphUpsertEdgeTool(g databases.GraphDB) *graphUpsertEdgeTool {
	return &graphUpsertEdgeTool{g: g}
}
func NewGraphNeighborsTool(g databases.GraphDB) *graphNeighborsTool { return &graphNeighborsTool{g: g} }
func NewGraphGetNodeTool(g databases.GraphDB) *graphGetNodeTool     { return &graphGetNodeTool{g: g} }

func (t *graphUpsertNodeTool) Name() string { return "graph_upsert_node" }
func (t *graphUpsertNodeTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Create or update a node in the graph.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id":     map[string]any{"type": "string"},
				"labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"props":  map[string]any{"type": "object"},
			},
		},
	}
}
func (t *graphUpsertNodeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID     string         `json:"id"`
		Labels []string       `json:"labels"`
		Props  map[string]any `json:"props"`
	}
	_ = json.Unmarshal(raw, &args)
	if err := t.g.UpsertNode(ctx, args.ID, args.Labels, args.Props); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true}, nil
}

func (t *graphUpsertEdgeTool) Name() string { return "graph_upsert_edge" }
func (t *graphUpsertEdgeTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Create or update a relationship between two nodes.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"src", "rel", "dst"},
			"properties": map[string]any{
				"src":   map[string]any{"type": "string"},
				"rel":   map[string]any{"type": "string"},
				"dst":   map[string]any{"type": "string"},
				"props": map[string]any{"type": "object"},
			},
		},
	}
}
func (t *graphUpsertEdgeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Src   string         `json:"src"`
		Rel   string         `json:"rel"`
		Dst   string         `json:"dst"`
		Props map[string]any `json:"props"`
	}
	_ = json.Unmarshal(raw, &args)
	if err := t.g.UpsertEdge(ctx, args.Src, args.Rel, args.Dst, args.Props); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true}, nil
}

func (t *graphNeighborsTool) Name() string { return "graph_neighbors" }
func (t *graphNeighborsTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "List outbound neighbor IDs for a node by relationship type.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"id", "rel"},
			"properties": map[string]any{
				"id":  map[string]any{"type": "string"},
				"rel": map[string]any{"type": "string"},
			},
		},
	}
}
func (t *graphNeighborsTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct{ ID, Rel string }
	_ = json.Unmarshal(raw, &args)
	out, err := t.g.Neighbors(ctx, args.ID, args.Rel)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "neighbors": out}, nil
}

func (t *graphGetNodeTool) Name() string { return "graph_get_node" }
func (t *graphGetNodeTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Fetch a node by ID.",
		"parameters": map[string]any{
			"type":       "object",
			"required":   []string{"id"},
			"properties": map[string]any{"id": map[string]any{"type": "string"}},
		},
	}
}
func (t *graphGetNodeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &args)
	n, ok := t.g.GetNode(ctx, args.ID)
	if !ok {
		return map[string]any{"ok": false, "error": "not found"}, nil
	}
	return map[string]any{"ok": true, "node": n}, nil
}
