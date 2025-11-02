package retrieve

import (
    "context"
    "fmt"
    "testing"

    "manifold/internal/persistence/databases"
)

func TestExpandWithGraph_AddsNeighbors(t *testing.T) {
    ctx := context.Background()
    g := databases.NewMemoryGraph()
    // Setup a small doc with 3 chunks
    docID := "doc:acme:alpha"
    _ = g.UpsertNode(ctx, docID, []string{"Doc"}, nil)
    for i := 0; i < 3; i++ {
        cid := fmt.Sprintf("chunk:%s:%d", docID, i)
        _ = g.UpsertNode(ctx, cid, []string{"Chunk"}, nil)
        _ = g.UpsertEdge(ctx, docID, "HAS_CHUNK", cid, nil)
    }

    fused := []RetrievedItem{
        {ID: fmt.Sprintf("chunk:%s:%d", docID, 0), Score: 1.0, Metadata: map[string]string{"doc_id": docID}},
    }
    out, diag := ExpandWithGraph(ctx, g, fused, GraphExpandOptions{TopN: 1, MaxPerSeed: 2, Hops: 1, Boost: 0.01})
    if len(out) <= len(fused) {
        t.Fatalf("expected expansion to add neighbors, got %d", len(out))
    }
    if diag.Expanded == 0 {
        t.Fatalf("expected non-zero expanded count")
    }
}

func TestAssembleResults_NoRerankMatchesOrder(t *testing.T) {
    ctx := context.Background()
    items := []RetrievedItem{{ID: "a", Score: 2}, {ID: "b", Score: 1}}
    plan := QueryPlan{Query: "q"}
    opt := RetrieveOptions{K: 2, GraphAugment: false, Rerank: false}
    out, _, err := AssembleResults(ctx, nil, nil, plan, opt, items)
    if err != nil { t.Fatalf("assemble failed: %v", err) }
    if len(out) != 2 || out[0].ID != "a" || out[1].ID != "b" {
        t.Fatalf("expected same order, got %#v", out)
    }
}

