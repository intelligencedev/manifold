package db

import (
	"context"
	"encoding/json"

	"singularityio/internal/persistence/databases"
)

type searchIndexTool struct{ s databases.FullTextSearch }
type searchQueryTool struct{ s databases.FullTextSearch }
type searchRemoveTool struct{ s databases.FullTextSearch }

func NewSearchIndexTool(s databases.FullTextSearch) *searchIndexTool { return &searchIndexTool{s: s} }
func NewSearchQueryTool(s databases.FullTextSearch) *searchQueryTool { return &searchQueryTool{s: s} }
func NewSearchRemoveTool(s databases.FullTextSearch) *searchRemoveTool {
	return &searchRemoveTool{s: s}
}

func (t *searchIndexTool) Name() string { return "search_index" }
func (t *searchIndexTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Index a text document for full-text search.",
		"parameters": map[string]any{
			"type":       "object",
			"required":   []string{"id", "text"},
			"properties": map[string]any{"id": map[string]any{"type": "string", "description": "Unique document ID"}, "text": map[string]any{"type": "string"}, "metadata": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}},
		},
	}
}
func (t *searchIndexTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID       string            `json:"id"`
		Text     string            `json:"text"`
		Metadata map[string]string `json:"metadata"`
	}
	_ = json.Unmarshal(raw, &args)
	if err := t.s.Index(ctx, args.ID, args.Text, args.Metadata); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true}, nil
}

func (t *searchQueryTool) Name() string { return "search_query" }
func (t *searchQueryTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Query the full-text index and return matching documents.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer", "minimum": 1, "default": 5},
			},
		},
	}
}
func (t *searchQueryTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(raw, &args)
	res, err := t.s.Search(ctx, args.Query, args.Limit)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "results": res}, nil
}

func (t *searchRemoveTool) Name() string { return "search_remove" }
func (t *searchRemoveTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Remove a document from the full-text index.",
		"parameters": map[string]any{
			"type":       "object",
			"required":   []string{"id"},
			"properties": map[string]any{"id": map[string]any{"type": "string"}},
		},
	}
}
func (t *searchRemoveTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &args)
	if err := t.s.Remove(ctx, args.ID); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true}, nil
}
