package memory

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
)

func TestKeywordQueryTermsDropsStopwordsAndShortTokens(t *testing.T) {
	t.Parallel()

	terms := KeywordQueryTerms("how to fix the test file")
	if len(terms) != 0 {
		t.Fatalf("expected no terms for stopword-heavy query, got %#v", terms)
	}
	terms = KeywordQueryTerms("canary deploy rollback rehearsal")
	if len(terms) < 3 {
		t.Fatalf("expected substantive terms, got %#v", terms)
	}
	if _, ok := terms["the"]; ok {
		t.Fatalf("stopword leaked into terms: %#v", terms)
	}
}

func TestRRFFusePreservesDenseAndKeywordComponents(t *testing.T) {
	t.Parallel()

	entry := &MemoryEntry{ID: "shared", Input: "shared"}
	dense := []ScoredMemoryEntry{{
		Entry: entry, Score: 0.12, Dense: 0.12, HasDense: true,
	}}
	keyword := []ScoredMemoryEntry{{
		Entry: entry, Score: 0.8, Keyword: 0.8, HasKeyword: true,
	}}
	fused := rrfFuse([][]ScoredMemoryEntry{dense, keyword}, 2, 60)
	if len(fused) != 1 {
		t.Fatalf("expected 1 fused result, got %d", len(fused))
	}
	item := fused[0]
	if !item.HasDense || item.Dense != 0.12 {
		t.Fatalf("expected dense component preserved, got %#v", item)
	}
	if !item.HasKeyword || item.Keyword != 0.8 {
		t.Fatalf("expected keyword component preserved, got %#v", item)
	}
	if item.RRF != item.Score || item.Score != 1 {
		t.Fatalf("expected normalized RRF on Score/RRF, got score=%f rrf=%f", item.Score, item.RRF)
	}
}

func TestScoreMemoryCandidatesUsesDenseNotRRF(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EnableRAG: true, TopK: 1})
	now := time.Now()
	entry := &MemoryEntry{
		ID:             "low-cos-top-rrf",
		Input:          "noise",
		CreatedAt:      now,
		LastAccessedAt: now,
	}
	candidates := []ScoredMemoryEntry{{
		Entry:    entry,
		Score:    1.0, // RRF-normalized fusion score
		Dense:    0.05,
		HasDense: true,
		RRF:      1.0,
	}}
	ranked := em.scoreMemoryCandidates(candidates)
	if len(ranked) != 1 {
		t.Fatalf("expected one ranked candidate")
	}
	// With default weights, quality=1, accessBoost=1, decay≈1 for now: composite ~ dense.
	if math.Abs(ranked[0].Composite-0.05) > 1e-9 || math.Abs(ranked[0].Score-0.05) > 1e-9 {
		t.Fatalf("expected composite≈0.05 from dense, got composite=%f score=%f", ranked[0].Composite, ranked[0].Score)
	}
}

func TestSearchRejectsKeywordOnlyWhenDenseExistsButFailsThreshold(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:                      testEmbedFn,
		TopK:                         3,
		EnableRAG:                    true,
		RetrievalSimilarityThreshold: 0.5,
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "weak", Input: "unrelated worker retry E_BUSY note", Embedding: []float32{0.1, 0}, Summary: "retry E_BUSY carefully"},
	}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "E_BUSY worker retry")
	if err != nil {
		t.Fatalf("SearchWithDiagnostics failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty results when all dense below threshold, got %#v", results)
	}
	if diag.Mode != "vector_below_threshold" {
		t.Fatalf("expected vector_below_threshold mode, got %#v", diag)
	}
	if diag.DenseBeforeFilter == 0 || diag.VectorFiltered == 0 {
		t.Fatalf("expected dense candidates filtered, got %#v", diag)
	}
}

func TestSearchStillFallsBackToKeywordWhenDenseUnavailable(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
			return nil, errors.New("embedding unavailable")
		},
		EnableRAG: true,
		TopK:      1,
	})
	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:      "keyword-only",
		Input:   "worker failed with E_BUSY",
		Summary: "Retry E_BUSY after releasing the worker lock",
	}}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "E_BUSY")
	if err != nil {
		t.Fatalf("expected keyword fallback: %v", err)
	}
	if len(results) != 1 || results[0].Entry.ID != "keyword-only" {
		t.Fatalf("expected keyword-only result, got %#v", results)
	}
	if diag.Mode != "keyword" || diag.EmbeddingError == "" {
		t.Fatalf("expected keyword mode with embedding error, got %#v", diag)
	}
}

type scoringReranker struct {
	scores map[string]float64
	err    error
}

func (r scoringReranker) RerankEvolvingMemory(_ context.Context, _ string, items []ScoredMemoryEntry) ([]ScoredMemoryEntry, error) {
	if r.err != nil {
		return items, r.err
	}
	out := make([]ScoredMemoryEntry, 0, len(items))
	for _, item := range items {
		if item.Entry == nil {
			continue
		}
		score, ok := r.scores[item.Entry.ID]
		if !ok {
			score = item.Score
		}
		item.Rerank = score
		item.HasRerank = true
		item.Score = score
		out = append(out, item)
	}
	return out, nil
}

func TestRerankFloorDropsLowRelevanceCandidates(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:        testEmbedFn,
		EnableRAG:      true,
		TopK:           4,
		MinRerankScore: 0.3,
		Reranker: scoringReranker{scores: map[string]float64{
			"good":  0.9,
			"ok":    0.4,
			"bad":   0.001,
			"worse": 0.0004,
		}},
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "good", Input: "good", Embedding: []float32{1, 0}},
		{ID: "ok", Input: "ok", Embedding: []float32{0.99, 0}},
		{ID: "bad", Input: "bad", Embedding: []float32{0.98, 0}},
		{ID: "worse", Input: "worse", Embedding: []float32{0.97, 0}},
	}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "q")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 survivors, got %#v", results)
	}
	if diag.RerankFiltered != 2 {
		t.Fatalf("expected RerankFiltered=2, got %#v", diag)
	}
	ids := map[string]bool{}
	for _, item := range results {
		ids[item.Entry.ID] = true
		if !item.HasRerank {
			t.Fatalf("expected HasRerank on admitted item: %#v", item)
		}
	}
	if !ids["good"] || !ids["ok"] {
		t.Fatalf("unexpected survivors: %#v", results)
	}
}

func TestRerankFloorDropsAllResults(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:        testEmbedFn,
		EnableRAG:      true,
		TopK:           2,
		MinRerankScore: 0.3,
		Reranker: scoringReranker{scores: map[string]float64{
			"a": 0.01,
			"b": 0.02,
		}},
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "a", Input: "a", Embedding: []float32{1, 0}},
		{ID: "b", Input: "b", Embedding: []float32{0.99, 0}},
	}
	em.mu.Unlock()

	results, _, err := em.SearchWithDiagnostics(context.Background(), "q")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty after rerank floor, got %#v", results)
	}
	if got := em.SynthesizeScored(context.Background(), "task", results); got != "" {
		t.Fatalf("expected empty synthesis, got %q", got)
	}
}

func TestRerankSigmoidNormalizesRawLogits(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:        testEmbedFn,
		EnableRAG:      true,
		TopK:           2,
		MinRerankScore: 0.5,
		Reranker: scoringReranker{scores: map[string]float64{
			"strong": 3.2,
			"weak":   -4.1,
		}},
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "strong", Input: "strong", Embedding: []float32{1, 0}},
		{ID: "weak", Input: "weak", Embedding: []float32{0.99, 0}},
	}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "q")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 || results[0].Entry.ID != "strong" {
		t.Fatalf("expected only strong after sigmoid floor, got %#v", results)
	}
	if results[0].Rerank <= 0.5 || results[0].Rerank > 1 {
		t.Fatalf("expected sigmoid-normalized rerank in (0.5,1], got %f", results[0].Rerank)
	}
	if diag.RerankFiltered != 1 {
		t.Fatalf("expected 1 filtered, got %#v", diag)
	}
}

func TestRerankErrorKeepsPreRerankCandidates(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:        testEmbedFn,
		EnableRAG:      true,
		TopK:           2,
		MinRerankScore: 0.99,
		Reranker:       scoringReranker{err: errors.New("down")},
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "a", Input: "a", Embedding: []float32{1, 0}},
		{ID: "b", Input: "b", Embedding: []float32{0.99, 0}},
	}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "q")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected pre-rerank candidates to remain when reranker errors")
	}
	if diag.RerankApplied || !strings.Contains(diag.RerankError, "down") {
		t.Fatalf("expected rerank error without apply, got %#v", diag)
	}
}

func TestExplainSearchMatchesLivePipelineIDs(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		EnableRAG: true,
		TopK:      2,
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "near", Input: "near", Embedding: []float32{0.9, 0.1}},
		{ID: "far", Input: "far", Embedding: []float32{0.1, 0.9}},
		{ID: "best", Input: "best", Embedding: []float32{1, 0}},
	}
	em.mu.Unlock()

	live, _, err := em.SearchWithDiagnostics(context.Background(), "q")
	if err != nil {
		t.Fatalf("live search failed: %v", err)
	}
	explained, err := em.ExplainSearch(context.Background(), "q")
	if err != nil {
		t.Fatalf("explain failed: %v", err)
	}
	if len(live) != len(explained) {
		t.Fatalf("length mismatch live=%d explain=%d", len(live), len(explained))
	}
	for i := range live {
		if live[i].Entry.ID != explained[i].Entry.ID {
			t.Fatalf("order mismatch at %d: live=%s explain=%s", i, live[i].Entry.ID, explained[i].Entry.ID)
		}
	}
}

func TestInMemoryKeywordSearchRequiresStrongMatch(t *testing.T) {
	t.Parallel()

	entries := []*MemoryEntry{
		{ID: "weak", Input: "generic task background", Summary: "retry something"},
		{ID: "strong", Input: "worker failed with code E_BUSY", StrategyCard: "Release lock then retry E_BUSY"},
		{ID: "raw-noise", Input: "unrelated", Summary: "nothing", RawTrace: "debug path mentions E_BUSY twice"},
	}
	// stopword-only query -> no terms
	if got := inMemoryKeywordSearch(entries, "how to fix the test file", 4); len(got) != 0 {
		t.Fatalf("expected no results for stopword query, got %#v", got)
	}
	// single strong token still admitted via score>=0.5
	got := inMemoryKeywordSearch(entries, "E_BUSY", 4)
	if len(got) != 1 || got[0].Entry.ID != "strong" {
		t.Fatalf("expected strong keyword hit, got %#v", got)
	}
}

func TestKeywordOnlyResultsDoNotUpdateAccessMetrics(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
			return nil, errors.New("embedding unavailable")
		},
		EnableRAG: true,
		TopK:      1,
	})
	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:          "keyword-only",
		Input:       "worker failed with E_BUSY",
		Summary:     "Retry E_BUSY after releasing the worker lock",
		AccessCount: 0,
	}}
	em.mu.Unlock()

	if _, _, err := em.SearchWithDiagnostics(context.Background(), "E_BUSY"); err != nil {
		t.Fatalf("search failed: %v", err)
	}
	// Access updates are async when enabled; keyword mode skips them entirely so value stays 0 immediately.
	em.mu.RLock()
	count := em.entries[0].AccessCount
	em.mu.RUnlock()
	if count != 0 {
		t.Fatalf("expected access count unchanged for keyword-only retrieval, got %d", count)
	}
}

func TestSearchAdmitsParaphraseHitAboveDenseThreshold(t *testing.T) {
	t.Parallel()

	// Controlled cosine via fixed vectors: query and channel-aligned entry share unit x-axis.
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:                      testEmbedFn,
		EnableRAG:                    true,
		TopK:                         1,
		RetrievalSimilarityThreshold: 0.35,
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:        "vite",
		Input:     "kill the vite dev server on port 5173",
		Embedding: []float32{1, 0},
	}, {
		ID:        "unrelated",
		Input:     "summarize quarterly paper backlog",
		Embedding: []float32{0, 1},
	}}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "stop the running vite process")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 || results[0].Entry.ID != "vite" {
		t.Fatalf("expected vite memory admitted, got %#v", results)
	}
	if !results[0].HasDense || results[0].Dense < 0.35 {
		t.Fatalf("expected dense admitting score, got %#v", results[0])
	}
	if diag.SimilarityThreshold != 0.35 {
		t.Fatalf("expected threshold recorded, got %#v", diag)
	}
}

func TestNoRelevantCorpusReturnsEmptySynthesis(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:                      testEmbedFn,
		EnableRAG:                    true,
		TopK:                         4,
		RetrievalSimilarityThreshold: 0.35,
		MinRerankScore:               0.30,
		Reranker: scoringReranker{scores: map[string]float64{
			"a": 0.01,
			"b": 0.02,
		}},
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "a", Input: "bake sourdough bread", Embedding: []float32{0, 1}},
		{ID: "b", Input: "water house plants", Embedding: []float32{0.1, 0.9}},
	}
	em.mu.Unlock()

	results, _, err := em.SearchWithDiagnostics(context.Background(), "debug kubernetes pod crashloop")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results for unrelated corpus, got %#v", results)
	}
	if got := em.SynthesizeScored(context.Background(), "debug kubernetes pod crashloop", results); got != "" {
		t.Fatalf("expected empty synthesis, got %q", got)
	}
}

func TestFormatScoredExperienceShowsDenseAndRerank(t *testing.T) {
	t.Parallel()

	got := formatScoredExperience(ScoredMemoryEntry{
		Entry:     &MemoryEntry{Input: "task", Feedback: "success"},
		Dense:     0.42,
		HasDense:  true,
		Rerank:    0.77,
		HasRerank: true,
		Score:     0.77,
	})
	if !strings.Contains(got, "**Relevance:** similarity 0.420 rerank 0.770") {
		t.Fatalf("unexpected format: %s", got)
	}
}
