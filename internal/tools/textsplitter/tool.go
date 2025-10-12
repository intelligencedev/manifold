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
        "description": "Split input text into chunks using a chosen strategy (fixed, sentences, paragraphs, markdown, code, semantic, texttiling, rolling_sentences, hybrid, layout, recursive).",
        "parameters": map[string]any{
            "type": "object",
            "properties": map[string]any{
                "text":     map[string]any{"type": "string", "description": "Input text to split"},
                "kind":     map[string]any{"type": "string", "description": "Splitter kind", "enum": []any{"fixed","sentences","paragraphs","markdown","code","semantic","texttiling","rolling_sentences","hybrid","layout","recursive"}},
                "unit":     map[string]any{"type": "string", "description": "Unit for fixed splitter: 'chars' or 'tokens'", "enum": []any{"chars", "tokens"}},
                "size":     map[string]any{"type": "integer", "description": "Chunk size in the chosen unit", "minimum": 1},
                "overlap":  map[string]any{"type": "integer", "description": "Overlap between adjacent chunks (same unit)", "minimum": 0},
                "tokenizer": map[string]any{"type": "string", "description": "Tokenizer to use when unit='tokens' (default 'whitespace')"},
                // additional knobs (optional, minimal exposure)
                "window": map[string]any{"type": "integer", "description": "Semantic/TextTiling/rolling sentence window size"},
                "threshold": map[string]any{"type": "number", "description": "Semantic/TextTiling threshold (0..1)"},
                "language": map[string]any{"type": "string", "description": "Code language hint (go, python, js)"},
                "headers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Markdown header levels to split on (e.g., ['#','##'])"},
                "rolling_step": map[string]any{"type": "integer", "description": "Rolling sentences step"},
                "page_delimiter": map[string]any{"type": "string", "description": "Layout page delimiter (regex)."},
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
        Window    int     `json:"window"`
        Threshold float64 `json:"threshold"`
        Language  string  `json:"language"`
        Headers   []string `json:"headers"`
        RollingStep int   `json:"rolling_step"`
        PageDelimiter string `json:"page_delimiter"`
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
    case textsplitters.KindSentences:
        cfg.Boundary = textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}
    case textsplitters.KindParagraphs:
        cfg.Boundary = textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}
    case textsplitters.KindHybrid:
        cfg.Boundary = textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}
    case textsplitters.KindMarkdown:
        cfg.Markdown = textsplitters.MarkdownConfig{Headers: args.Headers, Within: textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}}
    case textsplitters.KindCode:
        cfg.Code = textsplitters.CodeConfig{Language: args.Language, Within: textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}}
    case textsplitters.KindSemantic:
        cfg.Semantic = textsplitters.SemanticConfig{Window: args.Window, Threshold: args.Threshold, Within: textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}}
    case textsplitters.KindTextTiling:
        cfg.TextTiling = textsplitters.TextTilingConfig{BlockSize: args.Window, Threshold: args.Threshold, Within: textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}}
    case textsplitters.KindRollingSentences:
        cfg.Rolling = textsplitters.RollingConfig{Window: args.Window, Step: args.RollingStep}
    case textsplitters.KindLayout:
        cfg.Layout = textsplitters.LayoutConfig{PageDelimiter: args.PageDelimiter, Within: textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}}
    case textsplitters.KindRecursive:
        cfg.Recursive = textsplitters.RecursiveConfig{
            Markdown:   textsplitters.MarkdownConfig{Headers: args.Headers, Within: textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap}},
            Paragraphs: textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap},
            Sentences:  textsplitters.BoundaryConfig{Unit: unit, Size: size, Overlap: overlap},
            Fallback:   textsplitters.FixedConfig{Unit: unit, Size: size, Overlap: overlap},
        }
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

