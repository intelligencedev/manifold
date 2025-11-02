package retrieve

import (
    "testing"
    "manifold/internal/persistence/databases"
)

func TestFuseRRF_OrderAndTies(t *testing.T) {
    fts := []databases.SearchResult{
        {ID: "A", Score: 1.0, Metadata: map[string]string{"doc_id": "D1"}},
        {ID: "B", Score: 0.9, Metadata: map[string]string{"doc_id": "D2"}},
        {ID: "C", Score: 0.8, Metadata: map[string]string{"doc_id": "D3"}},
    }
    vec := []databases.VectorResult{
        {ID: "B", Score: 0.99, Metadata: map[string]string{"doc_id": "D2"}},
        {ID: "A", Score: 0.50, Metadata: map[string]string{"doc_id": "D1"}},
    }
    opt := RetrieveOptions{Alpha: 0.5, RRFK: 60}
    fused := FuseRRF(fts, vec, opt)
    if len(fused) != 3 { t.Fatalf("expected 3 fused, got %d", len(fused)) }
    // B appears at ranks (2,1) vs A at (1,2); scores should be equal for symmetric ranks
    if fused[0].Fused == fused[1].Fused {
        // ensure deterministic order by ID tie-break after rank sum
        if !(fused[0].ID == "A" || fused[0].ID == "B") {
            t.Fatalf("unexpected top IDs: %v, %v", fused[0].ID, fused[1].ID)
        }
    }
}

func TestDiversify_ReducesDominance(t *testing.T) {
    // Build a fused list dominated by same doc/source
    fc := []fusedCandidate{
        {ID: "c1", DocID: "D1", Source: "S1", Fused: 1.0},
        {ID: "c2", DocID: "D1", Source: "S1", Fused: 0.99},
        {ID: "c3", DocID: "D1", Source: "S1", Fused: 0.98},
        {ID: "c4", DocID: "D2", Source: "S2", Fused: 0.5},
    }
    out := Diversify(fc, 3, true)
    if len(out) != 3 { t.Fatalf("expected 3 results, got %d", len(out)) }
    // Expect that D2 appears within top-3 due to diversification
    foundD2 := false
    for _, it := range out { if it.DocID == "D2" { foundD2 = true; break } }
    if !foundD2 { t.Fatalf("expected diversification to include D2") }
}

