package retrieve

import (
    "context"
)

// AssembleResults runs the post-fusion pipeline: optional graph expansion,
// optional reranking, and final pruning to K.
func AssembleResults(ctx context.Context, g GraphFacade, rr Reranker, plan QueryPlan, opt RetrieveOptions, fused []RetrievedItem) ([]RetrievedItem, map[string]any, error) {
    debug := map[string]any{}

    // Graph expansion
    items := fused
    if opt.GraphAugment && g != nil {
        geOpt := GraphExpandOptions{TopN: min(opt.K, 10), MaxPerSeed: 3, Hops: 1, Boost: 0.01}
        var diag GraphDiagnostics
        var err error
        items, diag = ExpandWithGraph(ctx, g, items, geOpt)
        debug["graph"] = map[string]any{"expanded": diag.Expanded, "ms": diag.Duration.Milliseconds()}
        if err != nil { // currently ExpandWithGraph cannot error, reserved
            return items, debug, err
        }
    }

    // Reranking
    if opt.Rerank {
        if rr == nil {
            rr = NoopReranker{}
        }
        out, err := rr.Rerank(ctx, plan.Query, items)
        if err != nil {
            return items, debug, err
        }
        items = out
    }

    // Prune to K
    k := opt.K
    if k <= 0 { k = 10 }
    if len(items) > k { items = items[:k] }
    return items, debug, nil
}

// GraphFacade is the minimal surface we require from graph within this package.
type GraphFacade interface {
    Neighbors(ctx context.Context, id, rel string) ([]string, error)
}

