package textsplitter

import (
    "context"
    "encoding/json"

    "manifold/internal/textsplitters"
)

// Tool exposes text splitting strategies as a registry tool.
type Tool struct{}

func New() *Tool { return &Tool{} }

func (t *Tool) Name() string { return "split_text" }

func (t *Tool) JSONSchema() map[string]any {
    return map[string]any{
        "name":        t.Name(),
        "description": "Split input text into chunks using a chosen strategy (fixed length by chars or tokens with optional overlap).",
        "parameters": map[string]any{
            "type": "object",
            "properties": map[string]any{
                "text":     map[string]any{"type": "string", "description": "Input text to split"},
                "kind":     map[string]any{"type": "string", "description": "Splitter kind (currently only 'fixed')", "enum": []any{"fixed"}},
                "unit":     map[string]any{"type": "string", "description": "Unit for fixed splitter: 'chars' or 'tokens'", "enum": []any{"chars", "tokens"}},
                "size":     map[string]any{"type": "integer", "description": "Chunk size in the chosen unit", "minimum": 1},
                "overlap":  map[string]any{"type": "integer", "description": "Overlap between adjacent chunks (same unit)", "minimum": 0},
                "tokenizer": map[string]any{"type": "string", "description": "Tokenizer to use when unit='tokens' (default 'whitespace')"},
            },
            "required": []any{"text"},
        },
    }
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) { //nolint:revive // ctx kept for future use
    var args struct {
        Text      string `json:"text"`
        Kind      string `json:"kind"`
        Unit      string `json:"unit"`
        Size      int    `json:"size"`
        Overlap   int    `json:"overlap"`
        Tokenizer string `json:"tokenizer"`
    }
    if err := json.Unmarshal(raw, &args); err != nil {
        return nil, err
    }

    // Defaults
    kind := textsplitters.Kind(args.Kind)
    if kind == "" {
        kind = textsplitters.KindFixed
    }
    unit := textsplitters.Unit(args.Unit)
    if unit == "" {
        unit = textsplitters.UnitChars
    }
    size := args.Size
    if size <= 0 {
        size = 100
    }
    overlap := args.Overlap
    if overlap < 0 {
        overlap = 0
    }

    cfg := textsplitters.Config{Kind: kind}
    switch kind {
    case textsplitters.KindFixed:
        f := textsplitters.FixedConfig{Unit: unit, Size: size, Overlap: overlap}
        // For now, only whitespace tokenizer is supported when tokens.
        if unit == textsplitters.UnitTokens {
            f.Tokenizer = textsplitters.WhitespaceTokenizer{}
        }
        cfg.Fixed = f
    }

    splitter, err := textsplitters.NewFromConfig(cfg)
    if err != nil {
        return map[string]any{"ok": false, "error": err.Error()}, nil
    }
    chunks := splitter.Split(args.Text)
    return map[string]any{
        "ok":      true,
        "chunks":  chunks,
        "count":   len(chunks),
        "kind":    string(kind),
        "unit":    string(unit),
        "size":    size,
        "overlap": overlap,
    }, nil
}

