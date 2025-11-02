package retrieve

import (
    "context"
    "time"
)

// GraphExpandOptions control how we expand fused candidates via the graph.
type GraphExpandOptions struct {
    // TopN is how many top fused items to consider for expansion.
    TopN int
    // MaxPerSeed limits how many neighbors to include per seed.
    MaxPerSeed int
    // Hops is the number of expansion hops (1 = direct neighbors only).
    Hops int
    // Boost is the additive boost applied to expanded neighbors relative to their seed.
    Boost float64
}

type GraphDiagnostics struct {
    Expanded int
    Duration time.Duration
}

// ExpandWithGraph expands a fused candidate list using graph neighbors.
// It returns a new list of RetrievedItem including original items and expanded
// neighbors (deduped by ID) with small additive boosts.
func ExpandWithGraph(ctx context.Context, g GraphFacade, fused []RetrievedItem, opt GraphExpandOptions) ([]RetrievedItem, GraphDiagnostics) {
    start := time.Now()
    diag := GraphDiagnostics{}
    if g == nil || len(fused) == 0 || opt.TopN <= 0 || opt.Hops <= 0 || opt.Boost == 0 {
        // pass-through
        return fused, diag
    }
    top := opt.TopN
    if top > len(fused) { top = len(fused) }

    // Index existing items and scores for quick checks
    byID := make(map[string]RetrievedItem, len(fused))
    for _, it := range fused { byID[it.ID] = it }

    // Collect expansions
    maxPer := opt.MaxPerSeed
    if maxPer <= 0 { maxPer = 3 }

    // Helper to enqueue a neighbor
    addNeighbor := func(seed RetrievedItem, nid string) {
        if _, exists := byID[nid]; exists { return }
        // score: inherit seed score with small additive boost
        newItem := RetrievedItem{
            ID: nid,
            Score: seed.Score + opt.Boost,
            Metadata: map[string]string{"expanded_from": seed.ID},
            Explanation: map[string]any{"graph_boost": opt.Boost, "expanded_from": seed.ID},
        }
        byID[nid] = newItem
        diag.Expanded++
    }

    // We expand only via HAS_CHUNK (Doc->Chunk) for now. Future: MENTIONS, REFERS_TO.
    for i := 0; i < top; i++ {
        seed := fused[i]
        // Derive doc id from chunk id if possible
        docID := deriveDocID(seed.ID, seed.Metadata)
        // Hop 1: Doc -> HAS_CHUNK -> Chunk
        neigh, err := g.Neighbors(ctx, docID, "HAS_CHUNK")
        if err != nil { continue }
        cnt := 0
        for _, nid := range neigh {
            if nid == seed.ID { continue }
            addNeighbor(seed, nid)
            cnt++
            if cnt >= maxPer { break }
        }
        // Optional further hops: for simplicity, we do not traverse beyond HAS_CHUNK for now.
    }

    // Convert map back to slice, preserving original order then appending expansions
    // Keep original order for first len(fused)
    out := make([]RetrievedItem, 0, len(byID))
    out = append(out, fused...)
    // Append expanded that were not in original order
    for id, it := range byID {
        if !containsID(fused, id) {
            out = append(out, it)
        }
    }
    diag.Duration = time.Since(start)
    return out, diag
}

func containsID(items []RetrievedItem, id string) bool {
    for _, it := range items { if it.ID == id { return true } }
    return false
}

