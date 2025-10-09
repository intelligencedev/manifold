package db

import (
    "context"
    "encoding/json"

    "manifold/internal/config"
    "manifold/internal/persistence/databases"
)

// indexDocumentTool calls both search_index and vector_upsert(text)
type indexDocumentTool struct {
    s      databases.FullTextSearch
    v      databases.VectorStore
    embCfg config.EmbeddingConfig
}

func NewIndexDocumentTool(s databases.FullTextSearch, v databases.VectorStore, emb config.EmbeddingConfig) *indexDocumentTool {
    return &indexDocumentTool{s: s, v: v, embCfg: emb}
}

func (t *indexDocumentTool) Name() string { return "index_document" }
func (t *indexDocumentTool) JSONSchema() map[string]any {
    return map[string]any{
        "description": "Index a document into both full-text and vector stores.",
        "parameters": map[string]any{
            "type":     "object",
            "required": []string{"id", "text"},
            "properties": map[string]any{
                "id":       map[string]any{"type": "string"},
                "text":     map[string]any{"type": "string"},
                "metadata": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
            },
        },
    }
}

func (t *indexDocumentTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    var args struct {
        ID       string            `json:"id"`
        Text     string            `json:"text"`
        Metadata map[string]string `json:"metadata"`
    }
    _ = json.Unmarshal(raw, &args)
    // Full-text first
    if err := t.s.Index(ctx, args.ID, args.Text, args.Metadata); err != nil {
        return map[string]any{"ok": false, "error": err.Error()}, nil
    }
    // Vector using existing vector_upsert path (embed happens in that tool)
    up := NewVectorUpsertTool(t.v, t.embCfg)
    _, _ = up.Call(ctx, mustJSON(map[string]any{"id": args.ID, "text": args.Text, "metadata": args.Metadata}))
    return map[string]any{"ok": true}, nil
}

// mustJSON is a tiny helper to marshal args inline; safe for tests/tool calls.
func mustJSON(v any) json.RawMessage {
    b, _ := json.Marshal(v)
    return json.RawMessage(b)
}

