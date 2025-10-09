package db

import (
    "context"
    "encoding/json"
    "math"
    "sort"

    "manifold/internal/config"
    "manifold/internal/embedding"
    "manifold/internal/persistence/databases"
)

// hybridQueryTool performs a hybrid retrieval over full-text and vector stores
// and returns a unified, deduplicated result set with normalized scores.
type hybridQueryTool struct {
    s      databases.FullTextSearch
    v      databases.VectorStore
    embCfg config.EmbeddingConfig
}

func NewHybridQueryTool(s databases.FullTextSearch, v databases.VectorStore, emb config.EmbeddingConfig) *hybridQueryTool {
    return &hybridQueryTool{s: s, v: v, embCfg: emb}
}

func (t *hybridQueryTool) Name() string { return "hybrid_query" }
func (t *hybridQueryTool) JSONSchema() map[string]any {
    return map[string]any{
        "description": "Hybrid search across full-text and vectors. Accepts a natural-language query or raw text (embedded). Returns fused results with per-source scores and metadata.",
        "parameters": map[string]any{
            "type":     "object",
            "required": []string{},
            "properties": map[string]any{
                "query":  map[string]any{"type": "string", "description": "Natural-language query for full-text"},
                "text":   map[string]any{"type": "string", "description": "If provided, embed to query vector"},
                "vector": map[string]any{"type": "array", "items": map[string]any{"type": "number"}},
                "k":      map[string]any{"type": "integer", "minimum": 1, "default": 8},
                "filter": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
                "alpha":  map[string]any{"type": "number", "default": 0.4},
                "beta":   map[string]any{"type": "number", "default": 0.6},
            },
        },
    }
}

func (t *hybridQueryTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    var args struct {
        Query  string             `json:"query"`
        Text   string             `json:"text"`
        Vector []float32          `json:"vector"`
        K      int                `json:"k"`
        Filter map[string]string  `json:"filter"`
        Alpha  float64            `json:"alpha"`
        Beta   float64            `json:"beta"`
    }
    _ = json.Unmarshal(raw, &args)
    if args.K <= 0 {
        args.K = 8
    }
    if args.Alpha == 0 && args.Beta == 0 {
        args.Alpha, args.Beta = 0.4, 0.6
    }

    // Prepare query vector if not provided but text exists
    if len(args.Vector) == 0 && args.Text != "" {
        if embs, err := embedding.EmbedText(ctx, t.embCfg, []string{args.Text}); err == nil && len(embs) > 0 {
            args.Vector = embs[0]
        }
    }

    // Execute both lookups (sequentially here; simple and deterministic)
    var fts []databases.SearchResult
    if args.Query != "" {
        if res, err := t.s.Search(ctx, args.Query, args.K); err == nil {
            fts = res
        }
    }
    var vres []databases.VectorResult
    if len(args.Vector) > 0 {
        if res, err := t.v.SimilaritySearch(ctx, args.Vector, args.K, args.Filter); err == nil {
            vres = res
        }
    }

    // Normalize scores to [0,1] by max for each modality
    maxFTS := 0.0
    for _, r := range fts {
        if r.Score > maxFTS {
            maxFTS = r.Score
        }
    }
    maxVec := 0.0
    for _, r := range vres {
        if r.Score > maxVec {
            maxVec = r.Score
        }
    }

    type fused struct {
        ID         string             `json:"id"`
        Score      float64            `json:"score"`
        Scores     map[string]float64 `json:"scores"`
        Snippet    string             `json:"snippet,omitempty"`
        Metadata   map[string]string  `json:"metadata,omitempty"`
        SourceRank map[string]int     `json:"source_rank,omitempty"`
    }

    byID := map[string]*fused{}
    // Add FTS results
    for i, r := range fts {
        norm := 0.0
        if maxFTS > 0 {
            norm = r.Score / maxFTS
        }
        f := byID[r.ID]
        if f == nil {
            f = &fused{ID: r.ID, Scores: map[string]float64{}, Metadata: map[string]string{}, SourceRank: map[string]int{}}
            byID[r.ID] = f
        }
        f.Scores["bm25"] = norm
        f.Snippet = r.Snippet
        // prefer existing metadata; merge if empty
        if len(f.Metadata) == 0 && r.Metadata != nil {
            f.Metadata = r.Metadata
        }
        f.SourceRank["bm25"] = i
    }
    // Add vector results
    for i, r := range vres {
        norm := 0.0
        if maxVec > 0 {
            norm = r.Score / maxVec
        }
        f := byID[r.ID]
        if f == nil {
            f = &fused{ID: r.ID, Scores: map[string]float64{}, Metadata: map[string]string{}, SourceRank: map[string]int{}}
            byID[r.ID] = f
        }
        f.Scores["cosine"] = norm
        if len(f.Metadata) == 0 && r.Metadata != nil {
            f.Metadata = r.Metadata
        }
        f.SourceRank["cosine"] = i
    }

    out := make([]fused, 0, len(byID))
    for _, f := range byID {
        bm := f.Scores["bm25"]
        cs := f.Scores["cosine"]
        // Default weights if unset to avoid NaNs
        alpha := args.Alpha
        beta := args.Beta
        // guard against both zeros
        if alpha == 0 && beta == 0 {
            alpha, beta = 0.4, 0.6
        }
        f.Score = alpha*bm + beta*cs
        // Clamp
        if f.Score < 0 {
            f.Score = 0
        }
        if f.Score > 1 || math.IsNaN(f.Score) {
            f.Score = math.Min(1, f.Score)
        }
        out = append(out, *f)
    }

    sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
    if len(out) > args.K {
        out = out[:args.K]
    }

    // Convert to generic list for JSON
    results := make([]map[string]any, 0, len(out))
    for _, r := range out {
        results = append(results, map[string]any{
            "id":       r.ID,
            "score":    r.Score,
            "scores":   r.Scores,
            "snippet":  r.Snippet,
            "metadata": r.Metadata,
        })
    }
    return map[string]any{"ok": true, "results": results}, nil
}

