package retrieve

import (
    "math"
    "sort"
    "strings"

    "manifold/internal/persistence/databases"
)

// fusedCandidate is an internal structure used during fusion.
type fusedCandidate struct {
    ID        string
    DocID     string
    Source    string
    FtRank    int // 1-based; 0 if absent
    VecRank   int // 1-based; 0 if absent
    FtScore   float64
    VecScore  float64
    Fused     float64
    Snippet   string
    Text      string
    Metadata  map[string]string
}

// FuseRRF performs Reciprocal Rank Fusion over FTS and vector candidates.
// Weights are derived from options.Alpha: w_ft=Alpha, w_vec=1-Alpha.
// kRRf sets the denominator constant (typical default ~60).
func FuseRRF(fts []databases.SearchResult, vec []databases.VectorResult, opt RetrieveOptions) []fusedCandidate {
    // ranks are 1-based; if absent, contribution is 0 from that source
    wft := opt.Alpha
    if wft < 0 { wft = 0 }
    if wft > 1 { wft = 1 }
    wvec := 1 - wft
    krrf := opt.RRFK
    if krrf <= 0 { krrf = 60 }

    // Index positions
    ftPos := make(map[string]int, len(fts))
    ftByID := make(map[string]databases.SearchResult, len(fts))
    for i, r := range fts {
        ftPos[r.ID] = i + 1
        ftByID[r.ID] = r
    }
    vecPos := make(map[string]int, len(vec))
    vecByID := make(map[string]databases.VectorResult, len(vec))
    for i, r := range vec {
        vecPos[r.ID] = i + 1
        vecByID[r.ID] = r
    }

    // Collect union of IDs
    seen := map[string]struct{}{}
    ids := make([]string, 0, len(fts)+len(vec))
    add := func(id string) {
        if _, ok := seen[id]; !ok {
            seen[id] = struct{}{}
            ids = append(ids, id)
        }
    }
    for _, r := range fts { add(r.ID) }
    for _, r := range vec { add(r.ID) }

    out := make([]fusedCandidate, 0, len(ids))
    for _, id := range ids {
        fr := ftPos[id]
        vr := vecPos[id]
        // Compute RRF contributions only for present ranks
        fContrib := 0.0
        vContrib := 0.0
        if fr > 0 { fContrib = 1.0 / float64(krrf+fr) }
        if vr > 0 { vContrib = 1.0 / float64(krrf+vr) }
        fused := wft*fContrib + wvec*vContrib

        // Aggregate fields
        var snippet, text string
        md := map[string]string{}
        if r, ok := ftByID[id]; ok {
            snippet = r.Snippet
            text = r.Text
            for k, v := range r.Metadata { md[k] = v }
        }
        if r, ok := vecByID[id]; ok {
            for k, v := range r.Metadata { if _, exists := md[k]; !exists { md[k] = v } }
        }
        docID := deriveDocID(id, md)
        source := md["source"]

        out = append(out, fusedCandidate{
            ID: id, DocID: docID, Source: source,
            FtRank: fr, VecRank: vr,
            FtScore: fContrib, VecScore: vContrib,
            Fused: fused,
            Snippet: snippet, Text: text,
            Metadata: md,
        })
    }

    // Sort by fused desc, deterministic tie-breakers
    sort.Slice(out, func(i, j int) bool {
        if out[i].Fused != out[j].Fused {
            return out[i].Fused > out[j].Fused
        }
        // Prefer lower sum of ranks (better across lists)
        sri := safeRankSum(out[i].FtRank, out[i].VecRank)
        srj := safeRankSum(out[j].FtRank, out[j].VecRank)
        if sri != srj { return sri < srj }
        return out[i].ID < out[j].ID
    })
    return out
}

func safeRankSum(a, b int) int {
    if a == 0 { a = 1000000000 }
    if b == 0 { b = 1000000000 }
    // prevent overflow but keep large
    if a > 500000000 { a = 500000000 }
    if b > 500000000 { b = 500000000 }
    return a + b
}

// Diversify re-ranks a fused list to reduce dominance by the same DocID/Source.
// It applies multiplicative penalties as counts increase. When diversify=false,
// the input order is returned.
func Diversify(fused []fusedCandidate, k int, diversify bool) []fusedCandidate {
    if !diversify || k <= 0 || len(fused) <= 1 {
        if k > 0 && k < len(fused) { return fused[:k] }
        return fused
    }
    // Penalty strengths tuned for visible diversification in small K.
    lambdaDoc := 0.75
    lambdaSrc := 0.25
    docCount := map[string]int{}
    srcCount := map[string]int{}
    selected := make([]fusedCandidate, 0, min(k, len(fused)))
    used := make([]bool, len(fused))
    for len(selected) < k {
        bestIdx := -1
        bestAdj := -1.0
        for i, c := range fused {
            if used[i] { continue }
            d := docCount[c.DocID]
            s := srcCount[c.Source]
            denom := 1.0 + lambdaDoc*float64(max(0, d)) + lambdaSrc*float64(max(0, s))
            adj := c.Fused / denom
            if adj > bestAdj || (almostEqual(adj, bestAdj) && c.ID < fused[bestIdx].ID) {
                bestAdj = adj
                bestIdx = i
            }
        }
        if bestIdx == -1 { break }
        pick := fused[bestIdx]
        selected = append(selected, pick)
        used[bestIdx] = true
        docCount[pick.DocID]++
        srcCount[pick.Source]++
        if len(selected) == len(fused) { break }
    }
    return selected
}

// FuseAndDiversify is the exported helper to produce final RetrievedItems.
func FuseAndDiversify(fts []databases.SearchResult, vec []databases.VectorResult, plan QueryPlan, opt RetrieveOptions) []RetrievedItem {
    fused := FuseRRF(fts, vec, opt)
    // Apply diversification if requested and cap to K
    diversified := Diversify(fused, plan.FtK+plan.VecK, opt.Diversify)
    // Convert to RetrievedItem
    items := make([]RetrievedItem, 0, len(diversified))
    for _, c := range diversified {
        items = append(items, RetrievedItem{
            ID: c.ID,
            DocID: c.DocID,
            Score: c.Fused,
            Snippet: c.Snippet,
            Text: c.Text,
            Metadata: c.Metadata,
            Explanation: map[string]any{
                "fused":   c.Fused,
                "ft_rank": c.FtRank,
                "vec_rank": c.VecRank,
                "ft_rrf": c.FtScore,
                "vec_rrf": c.VecScore,
            },
        })
    }
    // Final cap by requested K
    k := opt.K
    if k <= 0 { k = 10 }
    if len(items) > k { items = items[:k] }
    return items
}

func deriveDocID(chunkID string, md map[string]string) string {
    if d := md["doc_id"]; d != "" { return d }
    // best-effort: if chunk:<doc-id>:<i>
    if strings.HasPrefix(chunkID, "chunk:") {
        rest := strings.TrimPrefix(chunkID, "chunk:")
        // remove trailing index by cutting last ':' if present
        if idx := strings.LastIndex(rest, ":"); idx != -1 {
            return rest[:idx]
        }
    }
    // passthrough: maybe the ID is itself a doc id
    return chunkID
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

func min(a, b int) int { if a < b { return a } ; return b }
func max(a, b int) int { if a > b { return a } ; return b }

// DeriveDocIDPublic exposes internal doc-id derivation for other packages.
func DeriveDocIDPublic(chunkID string, md map[string]string) string { return deriveDocID(chunkID, md) }

