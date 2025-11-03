package ragtool

import (
    "context"
    "encoding/json"

    "manifold/internal/persistence/databases"
    ragservice "manifold/internal/rag/service"
    "manifold/internal/rag/ingest"
    "manifold/internal/rag/retrieve"
)

// Ingest tool
type ingestTool struct { s *ragservice.Service }

// NewIngestTool constructs the rag_ingest tool backed by the RAG service.
func NewIngestTool(mgr databases.Manager, opts ...ragservice.Option) *ingestTool {
    s := ragservice.New(mgr, opts...)
    return &ingestTool{s: s}
}

func (t *ingestTool) Name() string { return "rag_ingest" }

func (t *ingestTool) JSONSchema() map[string]any {
    return map[string]any{
        "name":        t.Name(),
        "description": "Ingest a document into RAG (chunk -> search/vector/graph).",
        "parameters": map[string]any{
            "type": "object",
            "required": []string{"id", "text"},
            "properties": map[string]any{
                "id":       map[string]any{"type": "string", "description": "Unified document ID (doc:<ns>:<slug|hash>)"},
                "title":    map[string]any{"type": "string"},
                "url":      map[string]any{"type": "string"},
                "source":   map[string]any{"type": "string"},
                "text":     map[string]any{"type": "string", "description": "Full document text to ingest"},
                "metadata": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
                "language": map[string]any{"type": "string"},
                "tenant":   map[string]any{"type": "string"},
                "acl":      map[string]any{"type": "object"},
                "options": map[string]any{
                    "type": "object",
                    "properties": map[string]any{
                        "chunking": map[string]any{
                            "type": "object",
                            "properties": map[string]any{
                                "strategy":   map[string]any{"type": "string"},
                                "max_tokens": map[string]any{"type": "integer"},
                                "overlap":    map[string]any{"type": "integer"},
                            },
                        },
                        "embedding": map[string]any{
                            "type": "object",
                            "properties": map[string]any{
                                "enabled":    map[string]any{"type": "boolean"},
                                "model":      map[string]any{"type": "string"},
                                "dimensions": map[string]any{"type": "integer"},
                            },
                        },
                        "graph": map[string]any{
                            "type": "object",
                            "properties": map[string]any{
                                "enabled":          map[string]any{"type": "boolean"},
                                "extract_entities": map[string]any{"type": "boolean"},
                                "external_refs":    map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
                            },
                        },
                        "reingest_policy": map[string]any{"type": "string", "enum": []any{"skip_if_unchanged", "overwrite", "new_version"}},
                        "version":         map[string]any{"type": "integer"},
                        "idempotency_key": map[string]any{"type": "string"},
                    },
                },
            },
        },
    }
}

func (t *ingestTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    // Define a mirror of ingest.IngestRequest that is JSON-friendly
    var args struct {
        ID       string           `json:"id"`
        Title    string           `json:"title"`
        URL      string           `json:"url"`
        Source   string           `json:"source"`
        Text     string           `json:"text"`
        Metadata map[string]any   `json:"metadata"`
        Language string           `json:"language"`
        Tenant   string           `json:"tenant"`
        ACL      map[string]any   `json:"acl"`
        Options  struct {
            Chunking struct {
                Strategy  string `json:"strategy"`
                MaxTokens int    `json:"max_tokens"`
                Overlap   int    `json:"overlap"`
            } `json:"chunking"`
            Embedding struct {
                Enabled    bool   `json:"enabled"`
                Model      string `json:"model"`
                Dimensions int    `json:"dimensions"`
            } `json:"embedding"`
            Graph struct {
                Enabled         bool              `json:"enabled"`
                ExtractEntities bool              `json:"extract_entities"`
                ExternalRefs    map[string]string `json:"external_refs"`
            } `json:"graph"`
            ReingestPolicy string `json:"reingest_policy"`
            Version        int    `json:"version"`
            IdempotencyKey string `json:"idempotency_key"`
        } `json:"options"`
    }
    if err := json.Unmarshal(raw, &args); err != nil {
        return nil, err
    }

    // Map to ingest.IngestRequest
    pol := ingest.ReingestSkipIfUnchanged
    switch args.Options.ReingestPolicy {
    case string(ingest.ReingestOverwrite):
        pol = ingest.ReingestOverwrite
    case string(ingest.ReingestNewVersion):
        pol = ingest.ReingestNewVersion
    }
    req := ingest.IngestRequest{
        ID:       args.ID,
        Title:    args.Title,
        URL:      args.URL,
        Source:   args.Source,
        Text:     args.Text,
        Metadata: args.Metadata,
        Language: args.Language,
        Tenant:   args.Tenant,
        ACL:      args.ACL,
        Options: ingest.IngestOptions{
            Chunking: ingest.ChunkingOptions{Strategy: args.Options.Chunking.Strategy, MaxTokens: args.Options.Chunking.MaxTokens, Overlap: args.Options.Chunking.Overlap},
            Embedding: ingest.EmbeddingOptions{Enabled: args.Options.Embedding.Enabled, Model: args.Options.Embedding.Model, Dimensions: args.Options.Embedding.Dimensions},
            Graph: ingest.GraphOptions{Enabled: args.Options.Graph.Enabled, ExtractEntities: args.Options.Graph.ExtractEntities, ExternalRefs: args.Options.Graph.ExternalRefs},
            ReingestPolicy: pol,
            Version: args.Options.Version,
            IdempotencyKey: args.Options.IdempotencyKey,
        },
    }

    resp, err := t.s.Ingest(ctx, req)
    if err != nil {
        return map[string]any{"ok": false, "error": err.Error()}, nil
    }
    return map[string]any{"ok": true, "doc_id": resp.DocID, "version": resp.Version, "chunk_ids": resp.ChunkIDs, "stats": resp.Stats, "warnings": resp.Warnings}, nil
}

// Retrieve tool
type retrieveTool struct { s *ragservice.Service }

// NewRetrieveTool constructs the rag_retrieve tool backed by the RAG service.
func NewRetrieveTool(mgr databases.Manager, opts ...ragservice.Option) *retrieveTool {
    s := ragservice.New(mgr, opts...)
    return &retrieveTool{s: s}
}

func (t *retrieveTool) Name() string { return "rag_retrieve" }

func (t *retrieveTool) JSONSchema() map[string]any {
    return map[string]any{
        "name":        t.Name(),
        "description": "Execute a hybrid RAG retrieval over Search/Vector/Graph and return fused results.",
        "parameters": map[string]any{
            "type": "object",
            "required": []string{"query"},
            "properties": map[string]any{
                "query":           map[string]any{"type": "string"},
                "k":               map[string]any{"type": "integer"},
                "ft_k":            map[string]any{"type": "integer"},
                "vec_k":           map[string]any{"type": "integer"},
                "alpha":           map[string]any{"type": "number"},
                "use_rrf":         map[string]any{"type": "boolean"},
                "rrf_k":           map[string]any{"type": "integer"},
                "include_text":    map[string]any{"type": "boolean"},
                "include_snippet": map[string]any{"type": "boolean"},
                "diversify":       map[string]any{"type": "boolean"},
                "rerank":          map[string]any{"type": "boolean"},
                "graph_augment":   map[string]any{"type": "boolean"},
                "tenant":          map[string]any{"type": "string"},
                "filter":          map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
            },
        },
    }
}

func (t *retrieveTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    var args struct {
        Query          string            `json:"query"`
        K              int               `json:"k"`
        FtK            int               `json:"ft_k"`
        VecK           int               `json:"vec_k"`
        Alpha          float64           `json:"alpha"`
        UseRRF         bool              `json:"use_rrf"`
        RRFK           int               `json:"rrf_k"`
        IncludeText    bool              `json:"include_text"`
        IncludeSnippet bool              `json:"include_snippet"`
        Diversify      bool              `json:"diversify"`
        Rerank         bool              `json:"rerank"`
        GraphAugment   bool              `json:"graph_augment"`
        Tenant         string            `json:"tenant"`
        Filter         map[string]string `json:"filter"`
    }
    if err := json.Unmarshal(raw, &args); err != nil {
        return nil, err
    }
    opt := retrieve.RetrieveOptions{
        K: args.K, FtK: args.FtK, VecK: args.VecK, Alpha: args.Alpha,
        UseRRF: args.UseRRF, RRFK: args.RRFK,
        IncludeText: args.IncludeText, IncludeSnippet: args.IncludeSnippet,
        Diversify: args.Diversify, Rerank: args.Rerank, GraphAugment: args.GraphAugment,
        Tenant: args.Tenant, Filter: args.Filter,
    }
    resp, err := t.s.Retrieve(ctx, args.Query, opt)
    if err != nil {
        return map[string]any{"ok": false, "error": err.Error()}, nil
    }
    return map[string]any{"ok": true, "query": resp.Query, "items": resp.Items, "debug": resp.Debug}, nil
}

