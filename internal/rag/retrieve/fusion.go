package retrieve

import (
	"maps"
	"math"
	"sort"
	"strings"

	"manifold/internal/persistence/databases"
)

// fusedCandidate is an internal structure used during fusion.
type fusedCandidate struct {
	ID       string
	DocID    string
	Source   string
	FtRank   int // 1-based; 0 if absent
	VecRank  int // 1-based; 0 if absent
	FtScore  float64
	VecScore float64
	Fused    float64
	Snippet  string
	Text     string
	Metadata map[string]string
}

// FuseRRF performs Reciprocal Rank Fusion over FTS and vector candidates.
// Weights are derived from options.Alpha: w_ft=Alpha, w_vec=1-Alpha.
// kRRf sets the denominator constant (typical default ~60).
func FuseRRF(fts []databases.SearchResult, vec []databases.VectorResult, opt RetrieveOptions) []fusedCandidate {
	wft, wvec, krrf := rrfWeights(opt)
	ftPos, ftByID := indexFTSResults(fts)
	vecPos, vecByID := indexVectorResults(vec)
	ids := fusedResultIDs(fts, vec)

	out := make([]fusedCandidate, 0, len(ids))
	ctx := fusedBuildContext{ftPos: ftPos, ftByID: ftByID, vecPos: vecPos, vecByID: vecByID, wft: wft, wvec: wvec, krrf: krrf}
	for _, id := range ids {
		out = append(out, buildFusedCandidate(id, ctx))
	}

	sortFusedCandidates(out)
	return out
}

func rrfWeights(opt RetrieveOptions) (float64, float64, int) {
	wft := max(0, min(1, opt.Alpha))
	krrf := opt.RRFK
	if krrf <= 0 {
		krrf = 60
	}
	return wft, 1 - wft, krrf
}

func indexFTSResults(fts []databases.SearchResult) (map[string]int, map[string]databases.SearchResult) {
	pos := make(map[string]int, len(fts))
	byID := make(map[string]databases.SearchResult, len(fts))
	for i, result := range fts {
		pos[result.ID] = i + 1
		byID[result.ID] = result
	}
	return pos, byID
}

func indexVectorResults(vec []databases.VectorResult) (map[string]int, map[string]databases.VectorResult) {
	pos := make(map[string]int, len(vec))
	byID := make(map[string]databases.VectorResult, len(vec))
	for i, result := range vec {
		pos[result.ID] = i + 1
		byID[result.ID] = result
	}
	return pos, byID
}

func fusedResultIDs(fts []databases.SearchResult, vec []databases.VectorResult) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(fts)+len(vec))
	add := func(id string) {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	for _, result := range fts {
		add(result.ID)
	}
	for _, result := range vec {
		add(result.ID)
	}
	return ids
}

type fusedBuildContext struct {
	ftPos   map[string]int
	ftByID  map[string]databases.SearchResult
	vecPos  map[string]int
	vecByID map[string]databases.VectorResult
	wft     float64
	wvec    float64
	krrf    int
}

func buildFusedCandidate(id string, ctx fusedBuildContext) fusedCandidate {
	fr := ctx.ftPos[id]
	vr := ctx.vecPos[id]
	fContrib := rrfContribution(ctx.krrf, fr)
	vContrib := rrfContribution(ctx.krrf, vr)
	snippet, text, metadata := fusedMetadata(id, ctx.ftByID, ctx.vecByID)
	return fusedCandidate{
		ID: id, DocID: deriveDocID(id, metadata), Source: metadata["source"],
		FtRank: fr, VecRank: vr,
		FtScore: fContrib, VecScore: vContrib,
		Fused:    ctx.wft*fContrib + ctx.wvec*vContrib,
		Snippet:  snippet,
		Text:     text,
		Metadata: metadata,
	}
}

func rrfContribution(krrf, rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1.0 / float64(krrf+rank)
}

func fusedMetadata(id string, ftByID map[string]databases.SearchResult, vecByID map[string]databases.VectorResult) (string, string, map[string]string) {
	var snippet, text string
	md := map[string]string{}
	if result, ok := ftByID[id]; ok {
		snippet = result.Snippet
		text = result.Text
		maps.Copy(md, result.Metadata)
	}
	if result, ok := vecByID[id]; ok {
		for key, value := range result.Metadata {
			if _, exists := md[key]; !exists {
				md[key] = value
			}
		}
	}
	return snippet, text, md
}

func sortFusedCandidates(out []fusedCandidate) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Fused != out[j].Fused {
			return out[i].Fused > out[j].Fused
		}
		sri := safeRankSum(out[i].FtRank, out[i].VecRank)
		srj := safeRankSum(out[j].FtRank, out[j].VecRank)
		if sri != srj {
			return sri < srj
		}
		return out[i].ID < out[j].ID
	})
}

func safeRankSum(a, b int) int {
	if a == 0 {
		a = 1000000000
	}
	if b == 0 {
		b = 1000000000
	}
	// prevent overflow but keep large
	if a > 500000000 {
		a = 500000000
	}
	if b > 500000000 {
		b = 500000000
	}
	return a + b
}

// Diversify re-ranks a fused list to reduce dominance by the same DocID/Source.
// It applies multiplicative penalties as counts increase. When diversify=false,
// the input order is returned.
func Diversify(fused []fusedCandidate, k int, diversify bool) []fusedCandidate {
	if !diversify || k <= 0 || len(fused) <= 1 {
		if k > 0 && k < len(fused) {
			return fused[:k]
		}
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
			if used[i] {
				continue
			}
			d := docCount[c.DocID]
			s := srcCount[c.Source]
			denom := 1.0 + lambdaDoc*float64(max(0, d)) + lambdaSrc*float64(max(0, s))
			adj := c.Fused / denom
			if adj > bestAdj || (almostEqual(adj, bestAdj) && c.ID < fused[bestIdx].ID) {
				bestAdj = adj
				bestIdx = i
			}
		}
		if bestIdx == -1 {
			break
		}
		pick := fused[bestIdx]
		selected = append(selected, pick)
		used[bestIdx] = true
		docCount[pick.DocID]++
		srcCount[pick.Source]++
		if len(selected) == len(fused) {
			break
		}
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
			ID:       c.ID,
			DocID:    c.DocID,
			Score:    c.Fused,
			Snippet:  c.Snippet,
			Text:     c.Text,
			Metadata: c.Metadata,
			Explanation: map[string]any{
				"fused":    c.Fused,
				"ft_rank":  c.FtRank,
				"vec_rank": c.VecRank,
				"ft_rrf":   c.FtScore,
				"vec_rrf":  c.VecScore,
			},
		})
	}
	// Final cap by requested K
	k := opt.K
	if k <= 0 {
		k = 10
	}
	if len(items) > k {
		items = items[:k]
	}
	return items
}

func deriveDocID(chunkID string, md map[string]string) string {
	if d := md["doc_id"]; d != "" {
		return d
	}
	// best-effort: if chunk:<doc-id>:<i>
	if after, ok := strings.CutPrefix(chunkID, "chunk:"); ok {
		rest := after
		if idx := strings.LastIndex(rest, ":"); idx != -1 {
			return rest[:idx]
		}
	}
	// passthrough: maybe the ID is itself a doc id
	return chunkID
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

// DeriveDocIDPublic exposes internal doc-id derivation for other packages.
func DeriveDocIDPublic(chunkID string, md map[string]string) string { return deriveDocID(chunkID, md) }
