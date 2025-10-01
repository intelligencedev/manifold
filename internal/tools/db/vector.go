package db

import (
	"context"
	"encoding/json"
	"fmt"

	"manifold/internal/config"
	"manifold/internal/embedding"
	"manifold/internal/persistence/databases"
)

type vectorUpsertTool struct {
	v      databases.VectorStore
	embCfg config.EmbeddingConfig
}
type vectorQueryTool struct{ v databases.VectorStore }
type vectorDeleteTool struct{ v databases.VectorStore }

func NewVectorUpsertTool(v databases.VectorStore, emb config.EmbeddingConfig) *vectorUpsertTool {
	return &vectorUpsertTool{v: v, embCfg: emb}
}
func NewVectorQueryTool(v databases.VectorStore) *vectorQueryTool   { return &vectorQueryTool{v: v} }
func NewVectorDeleteTool(v databases.VectorStore) *vectorDeleteTool { return &vectorDeleteTool{v: v} }

func (t *vectorUpsertTool) Name() string { return "vector_upsert" }
func (t *vectorUpsertTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Upsert a vector embedding with optional metadata.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"id", "vector"},
			"properties": map[string]any{
				"id":       map[string]any{"type": "string"},
				"vector":   map[string]any{"type": "array", "items": map[string]any{"type": "number"}},
				"metadata": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
			},
		},
	}
}
func (t *vectorUpsertTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	// Accept either a precomputed vector or a text input to produce an embedding.
	var args struct {
		ID       string            `json:"id"`
		Vector   []float32         `json:"vector"`
		Text     string            `json:"text"`
		Metadata map[string]string `json:"metadata"`
	}
	_ = json.Unmarshal(raw, &args)

	// If caller provided text but no vector, try to generate embedding via embedding service.
	if len(args.Vector) == 0 && args.Text != "" {
		embs, err := embedding.EmbedText(ctx, t.embCfg, []string{args.Text})
		if err != nil {
			return map[string]any{"ok": false, "error": fmt.Sprintf("embed error: %v", err)}, nil
		}
		if len(embs) > 0 {
			args.Vector = embs[0]
		}
	}

	// If vector dimensions are known on the configured VectorStore, validate.
	// We only have strict enforcement for Postgres vector backend (pgVector) which
	// exposes dimensions via a concrete type. For other backends, skip validation.
	if pv, ok := t.v.(interface{ Dimension() int }); ok {
		if d := pv.Dimension(); d > 0 && len(args.Vector) != 0 && len(args.Vector) != d {
			return map[string]any{"ok": false, "error": fmt.Sprintf("vector length mismatch: expected %d, got %d", d, len(args.Vector))}, nil
		}
	}

	if err := t.v.Upsert(ctx, args.ID, args.Vector, args.Metadata); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true}, nil
}

func (t *vectorQueryTool) Name() string { return "vector_query" }
func (t *vectorQueryTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Query nearest neighbors using a query vector.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"vector"},
			"properties": map[string]any{
				"vector": map[string]any{"type": "array", "items": map[string]any{"type": "number"}},
				"k":      map[string]any{"type": "integer", "minimum": 1, "default": 5},
				"filter": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
			},
		},
	}
}
func (t *vectorQueryTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Vector []float32         `json:"vector"`
		K      int               `json:"k"`
		Filter map[string]string `json:"filter"`
	}
	_ = json.Unmarshal(raw, &args)
	res, err := t.v.SimilaritySearch(ctx, args.Vector, args.K, args.Filter)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "results": res}, nil
}

func (t *vectorDeleteTool) Name() string { return "vector_delete" }
func (t *vectorDeleteTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Delete a vector by ID.",
		"parameters": map[string]any{
			"type":       "object",
			"required":   []string{"id"},
			"properties": map[string]any{"id": map[string]any{"type": "string"}},
		},
	}
}
func (t *vectorDeleteTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &args)
	if err := t.v.Delete(ctx, args.ID); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true}, nil
}
