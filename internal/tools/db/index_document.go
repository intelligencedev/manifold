package db

import (
    "context"
    "encoding/json"
    "strconv"

    "manifold/internal/config"
    "manifold/internal/persistence/databases"
    "golang.org/x/sync/errgroup"
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
            "properties": map[string]any{
                "id":          map[string]any{"type": "string", "description": "Document ID (used for single text or as base)"},
                "text":        map[string]any{"type": "string", "description": "Single input text to index"},
                "texts":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Batch input: list of texts to index"},
                "texts_json":  map[string]any{"type": "string", "description": "JSON string containing either an array of texts or an object with field 'chunks' (array of strings). Useful to pipe split_text output."},
                "id_prefix":   map[string]any{"type": "string", "description": "Prefix for generated IDs when indexing multiple texts; final ID is id_prefix + ':' + index"},
                "concurrency": map[string]any{"type": "integer", "minimum": 1, "description": "Max concurrent upserts for batch ingest"},
                "metadata":    map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
            },
        },
    }
}

func (t *indexDocumentTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    // Accept flexible inputs
    var args struct {
        ID          string            `json:"id"`
        Text        string            `json:"text"`
        Texts       []string          `json:"texts"`
        TextsJSON   string            `json:"texts_json"`
        IDPrefix    string            `json:"id_prefix"`
        Concurrency int               `json:"concurrency"`
        Metadata    map[string]string `json:"metadata"`
    }
    _ = json.Unmarshal(raw, &args)

    // Determine input mode
    batch := make([]string, 0)
    if len(args.Texts) > 0 {
        batch = append(batch, args.Texts...)
    }
    if args.TextsJSON != "" {
        // Try to parse as []string or {chunks:[]}
        var arr []string
        if err := json.Unmarshal([]byte(args.TextsJSON), &arr); err == nil && len(arr) > 0 {
            batch = append(batch, arr...)
        } else {
            var obj struct{ Chunks []string `json:"chunks"` }
            if err := json.Unmarshal([]byte(args.TextsJSON), &obj); err == nil && len(obj.Chunks) > 0 {
                batch = append(batch, obj.Chunks...)
            }
        }
    }

    // If no batch, fall back to single text
    if len(batch) == 0 {
        if args.Text == "" {
            // Try to treat TextsJSON as single object with field text
            var one struct{ Text string `json:"text"` }
            _ = json.Unmarshal([]byte(args.TextsJSON), &one)
            if one.Text != "" {
                args.Text = one.Text
            }
        }
        if args.ID == "" {
            args.ID = args.IDPrefix
        }
        if args.ID == "" || args.Text == "" {
            return map[string]any{"ok": false, "error": "missing id or text"}, nil
        }
        if err := t.s.Index(ctx, args.ID, args.Text, args.Metadata); err != nil {
            return map[string]any{"ok": false, "error": err.Error()}, nil
        }
        up := NewVectorUpsertTool(t.v, t.embCfg)
        _, _ = up.Call(ctx, mustJSON(map[string]any{"id": args.ID, "text": args.Text, "metadata": args.Metadata}))
        return map[string]any{"ok": true, "count": 1, "ids": []string{args.ID}}, nil
    }

    // Batch mode
    // Concurrency default
    conc := args.Concurrency
    if conc <= 0 {
        conc = 4
    }
    if conc > 64 {
        conc = 64
    }
    prefix := args.IDPrefix
    if prefix == "" {
        // use ID as base if provided
        prefix = args.ID
    }
    if prefix == "" {
        prefix = "doc"
    }

    up := NewVectorUpsertTool(t.v, t.embCfg)
    ids := make([]string, len(batch))
    g, gctx := errgroup.WithContext(ctx)
    g.SetLimit(conc)
    for i, txt := range batch {
        i, txt := i, txt // capture
        id := prefix + ":" + strconv.Itoa(i)
        ids[i] = id // set now for deterministic return order
        g.Go(func() error {
            if err := t.s.Index(gctx, id, txt, args.Metadata); err != nil {
                return err
            }
            // Vector using existing tool (embeddings inside)
            if _, err := up.Call(gctx, mustJSON(map[string]any{"id": id, "text": txt, "metadata": args.Metadata})); err != nil {
                return err
            }
            return nil
        })
    }
    if err := g.Wait(); err != nil {
        return map[string]any{"ok": false, "error": err.Error(), "count": len(batch), "ids": ids}, nil
    }
    return map[string]any{"ok": true, "count": len(batch), "ids": ids}, nil
}

// mustJSON is a tiny helper to marshal args inline; safe for tests/tool calls.
func mustJSON(v any) json.RawMessage {
    b, _ := json.Marshal(v)
    return json.RawMessage(b)
}

// itoa small helper avoiding strconv import in this tiny file
// no extra helpers

