package ingest

import (
    "context"
    "testing"

    "manifold/internal/persistence/databases"
)

func TestUpsertDocAndChunksGraph_HasChunkNeighbors(t *testing.T) {
    ctx := context.Background()
    g := databases.NewMemoryGraph()
    in := IngestRequest{
        ID:     "doc:acme:alpha",
        Title:  "Alpha",
        URL:    "https://example.com/alpha",
        Source: "test",
        Tenant: "acme",
    }
    pre := PreprocessedDoc{Text: "hello world", Language: "english", Hash: "h"}
    chunks := []ChunkRecord{{Index:0, Text:"c0"},{Index:1, Text:"c1"},{Index:2, Text:"c2"}}

    ids, err := UpsertDocAndChunksGraph(ctx, g, in.ID, pre, in, chunks, 1)
    if err != nil { t.Fatalf("graph upsert failed: %v", err) }
    if len(ids) != len(chunks) { t.Fatalf("expected %d chunk ids, got %d", len(chunks), len(ids)) }

    neigh, err := g.Neighbors(ctx, in.ID, "HAS_CHUNK")
    if err != nil { t.Fatalf("neighbors failed: %v", err) }
    if len(neigh) != len(chunks) {
        t.Fatalf("expected %d neighbors, got %d", len(chunks), len(neigh))
    }

    // Idempotency: re-run and ensure neighbor count does not increase (memory graph overwrites edges by key)
    _, err = UpsertDocAndChunksGraph(ctx, g, in.ID, pre, in, chunks, 1)
    if err != nil { t.Fatalf("second upsert failed: %v", err) }
    neigh2, err := g.Neighbors(ctx, in.ID, "HAS_CHUNK")
    if err != nil { t.Fatalf("neighbors2 failed: %v", err) }
    if len(neigh2) != len(neigh) {
        t.Fatalf("expected idempotent edges, got %d then %d", len(neigh), len(neigh2))
    }
}

