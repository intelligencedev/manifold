package memory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/embedding"
	"manifold/internal/llm"
)

// mockLLMProvider is a simple mock for testing.
type mockLLMProvider struct {
	response string
}

type countingEmbedder struct {
	mu    sync.Mutex
	calls int
}

type recordingEmbedder struct {
	mu    sync.Mutex
	texts []string
}

type recordingMagmaSink struct {
	userID    int64
	sessionID string
	entry     *MemoryEntry
}

type fakeEvolvingReranker struct {
	order []string
	err   error
	seen  []string
}

func (c *countingEmbedder) embed(ctx context.Context, cfg config.EmbeddingConfig, texts []string) ([][]float32, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return testEmbedFn(ctx, cfg, texts)
}

func (c *countingEmbedder) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (r *recordingEmbedder) embed(ctx context.Context, cfg config.EmbeddingConfig, texts []string) ([][]float32, error) {
	r.mu.Lock()
	r.texts = append(r.texts, texts...)
	r.mu.Unlock()
	return testEmbedFn(ctx, cfg, texts)
}

func (r *recordingEmbedder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.texts...)
}

func (m *mockLLMProvider) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: m.response}, nil
}

func (m *mockLLMProvider) ChatStream(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, model string, handler llm.StreamHandler) error {
	handler.OnDelta(m.response)
	return nil
}

func (s *recordingMagmaSink) IngestEvolvingMemory(_ context.Context, userID int64, sessionID string, entry *MemoryEntry) (string, error) {
	s.userID = userID
	s.sessionID = sessionID
	s.entry = cloneEntry(entry)
	return "event:user:7:evolving:" + entry.ID, nil
}

func (r *fakeEvolvingReranker) RerankEvolvingMemory(_ context.Context, _ string, items []ScoredMemoryEntry) ([]ScoredMemoryEntry, error) {
	r.seen = r.seen[:0]
	for _, item := range items {
		if item.Entry != nil {
			r.seen = append(r.seen, item.Entry.ID)
		}
	}
	if r.err != nil {
		return items, r.err
	}
	byID := make(map[string]ScoredMemoryEntry, len(items))
	for _, item := range items {
		if item.Entry != nil {
			byID[item.Entry.ID] = item
		}
	}
	out := make([]ScoredMemoryEntry, 0, len(items))
	used := make(map[string]struct{}, len(items))
	for i, id := range r.order {
		item, ok := byID[id]
		if !ok {
			continue
		}
		item.Rerank = 1 - float64(i)*0.01
		item.HasRerank = true
		item.Score = item.Rerank
		out = append(out, item)
		used[id] = struct{}{}
	}
	for _, item := range items {
		if item.Entry == nil {
			continue
		}
		if _, ok := used[item.Entry.ID]; ok {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func testEmbedFn(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
	// Deterministic, cheap embedding for tests.
	out := make([][]float32, len(texts))
	for i, s := range texts {
		v := make([]float32, 8)
		for j := 0; j < len(s); j++ {
			v[j%len(v)] += float32(s[j]) / 255.0
		}
		out[i] = v
	}
	return out, nil
}

func TestEvolveEnhancedMirrorsToMagmaSink(t *testing.T) {
	t.Parallel()

	sink := &recordingMagmaSink{}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:    testEmbedFn,
		LLM:        &mockLLMProvider{response: "Remember the deployment workflow."},
		Model:      "test-model",
		EnableRAG:  true,
		UserID:     7,
		SessionID:  "session-1",
		MagmaSink:  sink,
		MaxSize:    100,
		TopK:       3,
		WindowSize: 10,
	})

	if err := em.EvolveEnhanced(context.Background(), "deploy service", "deployment succeeded", "success", nil, nil, ""); err != nil {
		t.Fatalf("EvolveEnhanced failed: %v", err)
	}

	if sink.userID != 7 || sink.sessionID != "session-1" {
		t.Fatalf("unexpected MAGMA sink scope: user=%d session=%q", sink.userID, sink.sessionID)
	}
	if sink.entry == nil {
		t.Fatal("expected MAGMA sink entry")
	}
	if sink.entry.Input != "deploy service" || sink.entry.Summary != "Remember the deployment workflow." {
		t.Fatalf("unexpected MAGMA sink entry: %#v", sink.entry)
	}
	if sink.entry == em.entries[0] {
		t.Fatal("expected MAGMA sink to receive a cloned entry")
	}
}

func TestEvolvingMemory_SearchSynthesizeEvolve(t *testing.T) {
	ctx := context.Background()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: config.EmbeddingConfig{},
		EmbedFn:         testEmbedFn,
		LLM:             &mockLLMProvider{response: "Key lesson: Always check inputs first."},
		Model:           "test-model",
		MaxSize:         100,
		TopK:            3,
		WindowSize:      10,
		EnableRAG:       true,
	})

	// Evolve (adding memories)
	if err := em.EvolveEnhanced(ctx, "test task 1", "solution 1", "success", nil, nil, ""); err != nil {
		t.Fatalf("Evolve failed: %v", err)
	}
	if got := len(em.entries); got != 1 {
		t.Fatalf("expected 1 entry, got %d", got)
	}

	// Search should find the memory.
	res, err := em.Search(ctx, "test task")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("expected search to return results")
	}

	// Synthesize should produce non-empty context.
	ctxStr := em.Synthesize(ctx, "current task", res)
	if ctxStr == "" {
		t.Fatalf("expected synthesized context")
	}
	if strings.Contains(ctxStr, "## Current Task") {
		t.Fatalf("synthesized context should not duplicate current task: %s", ctxStr)
	}
}

func TestGenerateSummaryPromptRequestsReusableMemory(t *testing.T) {
	t.Parallel()

	provider := &recordingLLMProvider{responses: []string{"summary"}}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: testEmbedFn,
		LLM:     provider,
		Model:   "test-model",
	})

	summary, err := em.generateSummary(context.Background(), "task", "output", "success")
	if err != nil {
		t.Fatalf("generateSummary failed: %v", err)
	}
	if summary != "summary" {
		t.Fatalf("expected provider response, got %q", summary)
	}
	systemPrompt := provider.lastSystemMessage()
	for _, want := range []string{
		"task pattern",
		"reusable lesson or strategy",
		"mistake or risk to avoid",
		"when the lesson should not be applied",
		"Do not include secrets",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("expected summary prompt to contain %q, got %q", want, systemPrompt)
		}
	}
}

func TestEvolveWithRAGDisabledDoesNotEmbedAndExpRecentStillWorks(t *testing.T) {
	t.Parallel()

	counting := &countingEmbedder{}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: counting.embed,
		LLM:     &mockLLMProvider{response: "remember the lesson"},
	})

	if err := em.EvolveEnhanced(context.Background(), "recent task", "recent output", "success", nil, nil, ""); err != nil {
		t.Fatalf("EvolveEnhanced failed: %v", err)
	}
	if got := counting.count(); got != 0 {
		t.Fatalf("expected no embedding calls when RAG is disabled, got %d", got)
	}
	memories := em.ExportMemories()
	if len(memories) != 1 {
		t.Fatalf("expected one memory, got %d", len(memories))
	}
	if len(memories[0].Embedding) != 0 {
		t.Fatalf("expected no stored embedding when RAG is disabled, got %#v", memories[0].Embedding)
	}
	results, err := em.Search(context.Background(), "recent task")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected semantic search disabled, got %#v", results)
	}
	if got := em.BuildExpRecentContext(); !strings.Contains(got, "recent task") {
		t.Fatalf("expected ExpRecent context to include stored task, got %q", got)
	}
}

func TestEvolveWithRAGEnabledStoresTextMemoryWhenEmbeddingFails(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
			return nil, errors.New("embedding unavailable")
		},
		LLM:       &mockLLMProvider{response: "fallback lesson"},
		EnableRAG: true,
	})

	if err := em.EvolveEnhanced(context.Background(), "task", "output", "success", nil, nil, ""); err != nil {
		t.Fatalf("EvolveEnhanced should store without embedding: %v", err)
	}
	memories := em.ExportMemories()
	if len(memories) != 1 {
		t.Fatalf("expected one memory, got %d", len(memories))
	}
	if len(memories[0].Embedding) != 0 {
		t.Fatalf("expected no embedding after embed failure, got %#v", memories[0].Embedding)
	}
	if memories[0].Metadata["embedding_error"] == nil {
		t.Fatalf("expected embedding_error metadata, got %#v", memories[0].Metadata)
	}
}

func TestSearchFallsBackToKeywordWhenEmbeddingFails(t *testing.T) {
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
		t.Fatalf("SearchWithDiagnostics should use keyword fallback: %v", err)
	}
	if len(results) != 1 || results[0].Entry.ID != "keyword-only" {
		t.Fatalf("expected keyword-only result, got %#v", results)
	}
	if diag.Mode != "keyword" || diag.EmbeddingError == "" {
		t.Fatalf("expected keyword mode with embedding error, got %#v", diag)
	}
}

func TestRebuildEmbeddingsBackfillsExistingMemories(t *testing.T) {
	t.Parallel()

	var embedded []string
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: func(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
			embedded = append(embedded, texts...)
			return [][]float32{{1, 0}}, nil
		},
		EnableRAG: true,
	})
	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:           "needs-embedding",
		Input:        "task",
		Output:       "output",
		Feedback:     "success",
		Summary:      "summary lesson",
		StrategyCard: "strategy card",
		Metadata:     map[string]any{},
	}}
	em.mu.Unlock()

	if err := em.RebuildEmbeddings(context.Background()); err != nil {
		t.Fatalf("RebuildEmbeddings failed: %v", err)
	}
	memories := em.ExportMemories()
	if len(memories) != 1 || len(memories[0].Embedding) == 0 {
		t.Fatalf("expected rebuilt embedding, got %#v", memories)
	}
	if len(embedded) != 1 || !strings.Contains(embedded[0], "summary lesson") || !strings.Contains(embedded[0], "strategy card") {
		t.Fatalf("expected retrieval text embedding, got %#v", embedded)
	}
	if memories[0].Metadata["embedding_text_basis"] != memoryEmbeddingTextBasis || memories[0].Metadata["has_embedding"] != true {
		t.Fatalf("expected embedding metadata, got %#v", memories[0].Metadata)
	}
}

func TestEvolveEmbedsReusableLessonText(t *testing.T) {
	t.Parallel()

	var embeddedText string
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: func(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
			if len(texts) > 0 {
				embeddedText = texts[0]
			}
			return [][]float32{{1, 0}}, nil
		},
		LLM:       &mockLLMProvider{response: "prefer the stable retry strategy"},
		EnableRAG: true,
	})

	if err := em.EvolveEnhanced(context.Background(), "fix flaky job", "passed", "success", nil, nil, "When jobs are flaky, retry once after checking logs."); err != nil {
		t.Fatalf("EvolveEnhanced failed: %v", err)
	}
	for _, want := range []string{"fix flaky job", "prefer the stable retry strategy", "When jobs are flaky"} {
		if !strings.Contains(embeddedText, want) {
			t.Fatalf("expected embedded text to contain %q, got %q", want, embeddedText)
		}
	}
}

func TestSearchCompositeScorePrefersSuccessfulRecentMemory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		TopK:      1,
		EnableRAG: true,
		RankingWeights: RankingWeights{
			DecayHalfLifeDays: 30,
			SuccessWeight:     1.5,
			FailureWeight:     0.2,
			PartialWeight:     0.8,
			AccessCountWeight: 0.1,
		},
	})

	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{
			ID:                 "failed",
			Embedding:          []float32{1, 0},
			StructuredFeedback: &StructuredFeedback{Type: FeedbackFailure},
			LastAccessedAt:     time.Now(),
		},
		{
			ID:                 "success",
			Embedding:          []float32{0.95, 0.05},
			StructuredFeedback: &StructuredFeedback{Type: FeedbackSuccess},
			LastAccessedAt:     time.Now(),
		},
	}
	em.mu.Unlock()

	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}

	results, err := em.SearchWithScores(ctx, "same query")
	if err != nil {
		t.Fatalf("SearchWithScores failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Entry.ID != "success" {
		t.Fatalf("expected success to outrank slightly more similar failure, got %q", results[0].Entry.ID)
	}
}

func TestApplyMMRDiversifiesTopK(t *testing.T) {
	t.Parallel()

	candidates := []ScoredMemoryEntry{
		{Entry: &MemoryEntry{ID: "near-1", Embedding: []float32{1, 0}}, Score: 0.99},
		{Entry: &MemoryEntry{ID: "near-2", Embedding: []float32{1, 0}}, Score: 0.98},
		{Entry: &MemoryEntry{ID: "diverse", Embedding: []float32{0, 1}}, Score: 0.8},
	}

	selected := applyMMR(candidates, 2, 0.7)
	if len(selected) != 2 {
		t.Fatalf("expected two selected candidates, got %d", len(selected))
	}
	if selected[0].Entry.ID != "near-1" || selected[1].Entry.ID != "diverse" {
		t.Fatalf("expected MMR to choose the best match and a diverse second result, got %q and %q", selected[0].Entry.ID, selected[1].Entry.ID)
	}
}

func TestSynthesizeGroupsSuccessesAndFailures(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn})
	retrieved := []ScoredMemoryEntry{
		{
			Entry: &MemoryEntry{
				ID:                 "ok",
				Input:              "fixed build",
				Output:             "ran tests",
				Feedback:           string(FeedbackSuccess),
				StructuredFeedback: &StructuredFeedback{Type: FeedbackSuccess},
			},
			Score: 0.91,
		},
		{
			Entry: &MemoryEntry{
				ID:                 "bad",
				Input:              "skipped tests",
				Output:             "regression",
				Feedback:           string(FeedbackFailure),
				StructuredFeedback: &StructuredFeedback{Type: FeedbackFailure},
			},
			Score: 0.73,
		},
	}

	got := em.SynthesizeScored(context.Background(), "current task", retrieved)
	if !strings.Contains(got, "## Strategies That Worked") {
		t.Fatalf("expected success section, got %s", got)
	}
	if !strings.Contains(got, "## Mistakes to Avoid") {
		t.Fatalf("expected failure section, got %s", got)
	}
	if !strings.Contains(got, "**Retrieval Score:** 0.910") {
		t.Fatalf("expected retrieval score, got %s", got)
	}
	if strings.Contains(got, "## Current Task") || strings.Contains(got, "current task") {
		t.Fatalf("expected synthesized context to omit current task, got %s", got)
	}
}

func TestRAGEnabledReportsConfig(t *testing.T) {
	t.Parallel()

	if NewEvolvingMemory(EvolvingMemoryConfig{}).RAGEnabled() {
		t.Fatal("expected RAG disabled by default")
	}
	if !NewEvolvingMemory(EvolvingMemoryConfig{EnableRAG: true}).RAGEnabled() {
		t.Fatal("expected RAG enabled from config")
	}
}

func TestClassifyMemoryTypeUsesWholeWords(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn})

	if got := em.classifyMemoryType("factory reset failed", "output", "summary", ""); got != MemoryEpisodic {
		t.Fatalf("expected factory not to match factual keyword, got %q", got)
	}
	if got := em.classifyMemoryType("how to retry flaky tests", "output", "summary", ""); got != MemoryProcedural {
		t.Fatalf("expected procedural classification, got %q", got)
	}
	if got := em.classifyMemoryType("what is the cache key", "output", "summary", ""); got != MemoryFactual {
		t.Fatalf("expected factual classification, got %q", got)
	}
	if got := em.classifyMemoryType("debug flaky tests", "output", "summary", "Reusable strategy: retry only the failed shard first"); got != MemoryProcedural {
		t.Fatalf("expected strategy card to drive procedural classification, got %q", got)
	}
}

func TestSearchCachesQueryEmbeddings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	counting := &countingEmbedder{}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:                 counting.embed,
		QueryEmbeddingCacheTTL:  time.Minute,
		QueryEmbeddingCacheSize: 4,
		TopK:                    1,
		EnableRAG:               true,
	})

	vecs, err := testEmbedFn(ctx, config.EmbeddingConfig{}, []string{"same task"})
	if err != nil {
		t.Fatalf("test embed failed: %v", err)
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{{ID: "entry", Embedding: normalizeVector(vecs[0])}}
	em.mu.Unlock()

	if _, err := em.Search(ctx, "same task"); err != nil {
		t.Fatalf("first search failed: %v", err)
	}
	if _, err := em.Search(ctx, "same task"); err != nil {
		t.Fatalf("second search failed: %v", err)
	}
	if got := counting.count(); got != 1 {
		t.Fatalf("expected one embedding call for repeated query, got %d", got)
	}
}

func TestSearchAppliesQueryInstruction(t *testing.T) {
	t.Parallel()

	recorder := &recordingEmbedder{}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: config.EmbeddingConfig{
			Model: "Qwen3-Embedding-0.6B-f16.gguf",
			Instructions: config.EmbeddingInstructionConfig{
				Mode:   "auto",
				Format: "qwen",
			},
		},
		EmbedFn:   recorder.embed,
		TopK:      1,
		EnableRAG: true,
	})
	em.mu.Lock()
	em.entries = []*MemoryEntry{{ID: "entry", Embedding: []float32{1, 0, 0, 0, 0, 0, 0, 0}}}
	em.mu.Unlock()

	_, diag, err := em.SearchWithDiagnostics(context.Background(), "fix flaky tests")
	if err != nil {
		t.Fatalf("SearchWithDiagnostics failed: %v", err)
	}
	texts := recorder.snapshot()
	if len(texts) != 1 {
		t.Fatalf("expected one embedding call, got %#v", texts)
	}
	want := "Instruct: " + embedding.DefaultMemoryQueryInstruction + "\nQuery: fix flaky tests"
	if texts[0] != want {
		t.Fatalf("unexpected query embedding text:\n got %q\nwant %q", texts[0], want)
	}
	if !diag.EmbeddingInstructionApplied || diag.EmbeddingInstructionUseCase != embedding.UseCaseEvolvingMemoryQuery {
		t.Fatalf("unexpected instruction diagnostics: %+v", diag)
	}
}

func TestSearchReranksCandidatePoolBeforeSelection(t *testing.T) {
	t.Parallel()

	reranker := &fakeEvolvingReranker{order: []string{"wanted", "near", "far"}}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
			return [][]float32{{1, 0, 0, 0, 0, 0, 0, 0}}, nil
		},
		TopK:      1,
		EnableRAG: true,
		MMRLambda: 1,
		Reranker:  reranker,
	})
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "near", Summary: "near vector match", Embedding: []float32{1, 0, 0, 0, 0, 0, 0, 0}},
		{ID: "wanted", Summary: "reranker says this is relevant", Embedding: []float32{0.8, 0, 0, 0, 0, 0, 0, 0}},
		{ID: "far", Summary: "less relevant", Embedding: []float32{0.6, 0, 0, 0, 0, 0, 0, 0}},
	}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "query")
	if err != nil {
		t.Fatalf("SearchWithDiagnostics failed: %v", err)
	}
	if len(results) != 1 || results[0].Entry.ID != "wanted" {
		t.Fatalf("expected reranked candidate to be selected, got %#v", results)
	}
	if !diag.RerankEnabled || !diag.RerankApplied || diag.RerankCandidates != 3 {
		t.Fatalf("unexpected rerank diagnostics: %+v", diag)
	}
	if strings.Join(reranker.seen, ",") != "near,wanted,far" {
		t.Fatalf("expected reranker to see preselection pool, got %#v", reranker.seen)
	}
}

func TestSearchFallsBackWhenRerankerFails(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
			return [][]float32{{1, 0, 0, 0, 0, 0, 0, 0}}, nil
		},
		TopK:      1,
		EnableRAG: true,
		MMRLambda: 1,
		Reranker:  &fakeEvolvingReranker{err: errors.New("reranker unavailable")},
	})
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "near", Summary: "near vector match", Embedding: []float32{1, 0, 0, 0, 0, 0, 0, 0}},
		{ID: "far", Summary: "less relevant", Embedding: []float32{0.5, 0, 0, 0, 0, 0, 0, 0}},
	}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "query")
	if err != nil {
		t.Fatalf("SearchWithDiagnostics failed: %v", err)
	}
	if len(results) != 1 || results[0].Entry.ID != "near" {
		t.Fatalf("expected fallback local ranking, got %#v", results)
	}
	if !diag.RerankEnabled || diag.RerankApplied || !strings.Contains(diag.RerankError, "reranker unavailable") {
		t.Fatalf("unexpected rerank diagnostics: %+v", diag)
	}
}

func TestSearchCacheSeparatesEmbeddingInstructions(t *testing.T) {
	t.Parallel()

	counting := &countingEmbedder{}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: config.EmbeddingConfig{
			Model: "text-embedding-3-small",
			Instructions: config.EmbeddingInstructionConfig{
				Mode:                "enabled",
				Format:              "qwen",
				EvolvingMemoryQuery: "First instruction.",
			},
		},
		EmbedFn:                 counting.embed,
		QueryEmbeddingCacheTTL:  time.Minute,
		QueryEmbeddingCacheSize: 4,
		TopK:                    1,
		EnableRAG:               true,
	})
	em.mu.Lock()
	em.entries = []*MemoryEntry{{ID: "entry", Embedding: []float32{1, 0, 0, 0, 0, 0, 0, 0}}}
	em.mu.Unlock()

	if _, err := em.Search(context.Background(), "same task"); err != nil {
		t.Fatalf("first search failed: %v", err)
	}
	em.embedCfg.Instructions.EvolvingMemoryQuery = "Second instruction."
	if _, err := em.Search(context.Background(), "same task"); err != nil {
		t.Fatalf("second search failed: %v", err)
	}
	if got := counting.count(); got != 2 {
		t.Fatalf("expected cache separation by effective instruction, got %d calls", got)
	}
}

func TestEvolveEmbedsRawRetrievalText(t *testing.T) {
	t.Parallel()

	recorder := &recordingEmbedder{}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: config.EmbeddingConfig{
			Model: "Qwen3-Embedding-0.6B-f16.gguf",
			Instructions: config.EmbeddingInstructionConfig{
				Mode:   "auto",
				Format: "qwen",
			},
		},
		EmbedFn:   recorder.embed,
		LLM:       &mockLLMProvider{response: "check inputs before retrying"},
		Model:     "test-model",
		EnableRAG: true,
	})

	if err := em.EvolveEnhanced(context.Background(), "fix flaky tests", "reran package tests", "success", nil, nil, ""); err != nil {
		t.Fatalf("EvolveEnhanced failed: %v", err)
	}
	texts := recorder.snapshot()
	if len(texts) != 1 {
		t.Fatalf("expected one write-time embedding call, got %#v", texts)
	}
	if strings.HasPrefix(texts[0], "Instruct: ") {
		t.Fatalf("expected raw write-time retrieval text, got %q", texts[0])
	}
	if !strings.Contains(texts[0], "Task: fix flaky tests") {
		t.Fatalf("expected retrieval text to include task label, got %q", texts[0])
	}
}

func TestEvolvingMemory_ExpRecent(t *testing.T) {
	ctx := context.Background()
	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn})

	// Add entries directly to avoid summarizer/embeddings.
	em.mu.Lock()
	for range 5 {
		em.entries = append(em.entries, &MemoryEntry{Input: "task", Output: "out", Feedback: "success"})
	}
	em.mu.Unlock()

	recent := em.BuildExpRecentContext()
	if recent == "" {
		t.Fatalf("expected recent context")
	}
	_ = ctx
}

func TestEvolvingMemory_CallbacksFire(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var (
		mu          sync.Mutex
		searchHits  int
		synthHits   int
		evolveHits  int
		lastPhase   PhaseType
		lastEventID string
	)

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: config.EmbeddingConfig{},
		EmbedFn:         testEmbedFn,
		LLM:             &mockLLMProvider{response: "Lesson."},
		Model:           "test-model",
		TopK:            2,
		EnableRAG:       true,
		Callbacks: &MemoryCallbacks{
			OnSearch: func(evt *MemoryEvent) {
				mu.Lock()
				defer mu.Unlock()
				searchHits++
				lastPhase = evt.Phase
				if len(evt.RetrievedIDs) > 0 {
					lastEventID = evt.RetrievedIDs[0]
				}
			},
			OnSynthesized: func(evt *MemoryEvent) {
				mu.Lock()
				defer mu.Unlock()
				synthHits++
				lastPhase = evt.Phase
			},
			OnEvolve: func(evt *MemoryEvent) {
				mu.Lock()
				defer mu.Unlock()
				evolveHits++
				lastPhase = evt.Phase
			},
		},
	})

	if err := em.EvolveEnhanced(ctx, "do thing", "done", "success", nil, nil, ""); err != nil {
		t.Fatalf("evolve failed: %v", err)
	}

	res, err := em.Search(ctx, "do")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	_ = em.Synthesize(ctx, "cur", res)

	mu.Lock()
	defer mu.Unlock()
	if evolveHits == 0 {
		t.Fatalf("expected evolve callback")
	}
	if searchHits == 0 {
		t.Fatalf("expected search callback")
	}
	if synthHits == 0 {
		t.Fatalf("expected synth callback")
	}
	if lastPhase == "" {
		t.Fatalf("expected phase to be set")
	}
	if lastEventID == "" {
		// It's okay if empty depending on topK and embeddings; but in practice it should be populated.
		t.Logf("no retrieved id captured")
	}
}

func TestEvolvingMemorySnapshotsReturnCopies(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn})

	em.mu.Lock()
	em.entries = append(em.entries, &MemoryEntry{
		Input:     "original input",
		Output:    "original output",
		Feedback:  "success",
		Embedding: []float32{1, 2, 3},
		Metadata: map[string]any{
			"tag": "original",
		},
		StructuredFeedback: &StructuredFeedback{Message: "keep me"},
	})
	em.mu.Unlock()

	recent := em.GetRecentWindow()
	exported := em.ExportMemories()

	recent[0].Input = "mutated recent"
	recent[0].Embedding[0] = 99
	recent[0].Metadata["tag"] = "mutated"
	recent[0].StructuredFeedback.Message = "mutated"

	exported[0].Output = "mutated export"

	fresh := em.ExportMemories()
	if fresh[0].Input != "original input" {
		t.Fatalf("expected internal input to remain unchanged, got %q", fresh[0].Input)
	}
	if fresh[0].Output != "original output" {
		t.Fatalf("expected internal output to remain unchanged, got %q", fresh[0].Output)
	}
	if fresh[0].Embedding[0] != 1 {
		t.Fatalf("expected internal embedding to remain unchanged, got %v", fresh[0].Embedding)
	}
	if fresh[0].Metadata["tag"] != "original" {
		t.Fatalf("expected internal metadata to remain unchanged, got %#v", fresh[0].Metadata)
	}
	if fresh[0].StructuredFeedback == nil || fresh[0].StructuredFeedback.Message != "keep me" {
		t.Fatalf("expected structured feedback to remain unchanged, got %#v", fresh[0].StructuredFeedback)
	}
}

func TestEvolvingMemoryRelevanceScoreDoesNotCompound(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:          testEmbedFn,
		MaxSize:          1,
		EnableSmartPrune: true,
		RelevanceDecay:   0.99,
		MinRelevance:     0.1,
		PruneThreshold:   0.95,
	})

	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{
			ID:             "stale",
			CreatedAt:      now.Add(-14 * 24 * time.Hour),
			LastAccessedAt: now.Add(-14 * 24 * time.Hour),
			AccessCount:    0,
		},
		{
			ID:             "fresh",
			CreatedAt:      now,
			LastAccessedAt: now,
			AccessCount:    0,
		},
	}
	em.mu.Unlock()

	em.mu.Lock()
	entry := em.entries[0]
	first := em.computeRelevanceScore(now, entry)
	entry.RelevanceScore = first
	second := em.computeRelevanceScore(now, entry)
	em.mu.Unlock()

	if math.Abs(first-second) > 1e-9 {
		t.Fatalf("expected stable relevance score, got first=%f second=%f", first, second)
	}

	em.mu.Lock()
	em.relevanceBasedPrune(context.Background())
	remaining := em.entries
	em.mu.Unlock()

	if len(remaining) != 1 {
		t.Fatalf("expected prune to keep one entry, got %d", len(remaining))
	}
	if remaining[0].ID != "fresh" {
		t.Fatalf("expected freshest entry to remain, got %q", remaining[0].ID)
	}
}

type recordingEvolvingMemoryStore struct {
	saveCh chan []*MemoryEntry
}

func (s *recordingEvolvingMemoryStore) Load(context.Context, int64, string) ([]*MemoryEntry, error) {
	return nil, nil
}

func (s *recordingEvolvingMemoryStore) Save(_ context.Context, _ int64, _ string, entries []*MemoryEntry) error {
	s.saveCh <- cloneEntrySlice(entries)
	return nil
}

type recordingDeltaEvolvingMemoryStore struct {
	saveCh    chan []*MemoryEntry
	upsertCh  chan []*MemoryEntry
	deleteCh  chan []string
	touchCh   chan []string
	promoteCh chan string
}

type recordingSearchEvolvingMemoryStore struct {
	searchQueries  chan []float32
	keywordQueries chan string
	results        []ScoredMemoryEntry
	keywordResults []ScoredMemoryEntry
}

func (s *recordingSearchEvolvingMemoryStore) Load(context.Context, int64, string) ([]*MemoryEntry, error) {
	return nil, nil
}

func (s *recordingSearchEvolvingMemoryStore) Save(context.Context, int64, string, []*MemoryEntry) error {
	return nil
}

func (s *recordingSearchEvolvingMemoryStore) SearchTopK(_ context.Context, _ int64, _ string, queryVec []float32, _ int) ([]ScoredMemoryEntry, error) {
	s.searchQueries <- append([]float32(nil), queryVec...)
	return s.results, nil
}

func (s *recordingSearchEvolvingMemoryStore) KeywordSearch(_ context.Context, _ int64, _ string, query string, _ int) ([]ScoredMemoryEntry, error) {
	if s.keywordQueries != nil {
		s.keywordQueries <- query
	}
	return s.keywordResults, nil
}

func newRecordingDeltaEvolvingMemoryStore() *recordingDeltaEvolvingMemoryStore {
	return &recordingDeltaEvolvingMemoryStore{
		saveCh:    make(chan []*MemoryEntry, 2),
		upsertCh:  make(chan []*MemoryEntry, 4),
		deleteCh:  make(chan []string, 4),
		touchCh:   make(chan []string, 4),
		promoteCh: make(chan string, 4),
	}
}

func (s *recordingDeltaEvolvingMemoryStore) Load(context.Context, int64, string) ([]*MemoryEntry, error) {
	return nil, nil
}

func (s *recordingDeltaEvolvingMemoryStore) Save(_ context.Context, _ int64, _ string, entries []*MemoryEntry) error {
	s.saveCh <- cloneEntrySlice(entries)
	return nil
}

func (s *recordingDeltaEvolvingMemoryStore) Upsert(_ context.Context, _ int64, _ string, entries []*MemoryEntry) error {
	s.upsertCh <- cloneEntrySlice(entries)
	return nil
}

func (s *recordingDeltaEvolvingMemoryStore) Delete(_ context.Context, _ int64, _ string, ids []string) error {
	s.deleteCh <- append([]string(nil), ids...)
	return nil
}

func (s *recordingDeltaEvolvingMemoryStore) TouchAccess(_ context.Context, ids []string, _ time.Time) error {
	s.touchCh <- append([]string(nil), ids...)
	return nil
}

func (s *recordingDeltaEvolvingMemoryStore) PromoteToUserScope(_ context.Context, _ int64, entryID string) error {
	s.promoteCh <- entryID
	return nil
}

type queuedMockLLMProvider struct {
	responses []string
	mu        sync.Mutex
	index     int
}

func (m *queuedMockLLMProvider) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string) (llm.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	resp := ""
	if m.index < len(m.responses) {
		resp = m.responses[m.index]
		m.index++
	}
	return llm.Message{Role: "assistant", Content: resp}, nil
}

func (m *queuedMockLLMProvider) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return nil
}

func TestSearchPersistsAccessMetrics(t *testing.T) {
	t.Parallel()

	store := &recordingEvolvingMemoryStore{saveCh: make(chan []*MemoryEntry, 1)}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		Store:     store,
		UserID:    1,
		SessionID: "search-session",
		EnableRAG: true,
	})

	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:        "entry-1",
		Input:     "same task",
		Output:    "result",
		Feedback:  "success",
		Embedding: [][]float32{{0}}[0],
		CreatedAt: time.Now().UTC(),
	}}
	em.entries[0].Embedding, _ = func() ([]float32, error) {
		vecs, err := testEmbedFn(context.Background(), config.EmbeddingConfig{}, []string{"same task"})
		if err != nil {
			return nil, err
		}
		return vecs[0], nil
	}()
	em.mu.Unlock()

	results, err := em.Search(context.Background(), "same task")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 {
			t.Fatalf("expected one saved entry, got %d", len(saved))
		}
		if saved[0].AccessCount != 1 {
			t.Fatalf("expected access count 1, got %d", saved[0].AccessCount)
		}
		if saved[0].LastAccessedAt.IsZero() {
			t.Fatal("expected last accessed timestamp to be persisted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for access-metric persistence")
	}
}

func TestSearchUsesTouchAccessForDeltaStore(t *testing.T) {
	t.Parallel()

	store := newRecordingDeltaEvolvingMemoryStore()
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		Store:     store,
		UserID:    1,
		SessionID: "delta-search-session",
		EnableRAG: true,
	})

	vecs, err := testEmbedFn(context.Background(), config.EmbeddingConfig{}, []string{"same task"})
	if err != nil {
		t.Fatalf("testEmbedFn failed: %v", err)
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:        "entry-1",
		Input:     "same task",
		Output:    "result",
		Feedback:  "success",
		Embedding: normalizeVector(vecs[0]),
		CreatedAt: time.Now().UTC(),
	}}
	em.mu.Unlock()

	results, err := em.Search(context.Background(), "same task")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}

	select {
	case touched := <-store.touchCh:
		if len(touched) != 1 || touched[0] != "entry-1" {
			t.Fatalf("expected touch for entry-1, got %#v", touched)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for touch access")
	}

	select {
	case saved := <-store.saveCh:
		t.Fatalf("expected delta store search to avoid full save, got %#v", saved)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSearchUsesServerSideStoreWhenLocalEntriesEmpty(t *testing.T) {
	t.Parallel()

	store := &recordingSearchEvolvingMemoryStore{
		searchQueries: make(chan []float32, 1),
		results: []ScoredMemoryEntry{{
			Entry: &MemoryEntry{
				ID:                 "server-entry",
				Input:              "server task",
				Embedding:          []float32{1, 0},
				StructuredFeedback: &StructuredFeedback{Type: FeedbackSuccess},
			},
			Score: 0.8,
		}},
	}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		Store:     store,
		TopK:      1,
		EnableRAG: true,
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}

	results, err := em.Search(context.Background(), "server query")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "server-entry" {
		t.Fatalf("expected server-side result, got %#v", results)
	}

	select {
	case query := <-store.searchQueries:
		if len(query) != 2 || query[0] != 1 || query[1] != 0 {
			t.Fatalf("expected normalized query vector, got %#v", query)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server-side search")
	}
}

func TestSearchFiltersVectorCandidatesBelowRetrievalSimilarityThreshold(t *testing.T) {
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
		{ID: "strong", Input: "strong semantic match", Embedding: []float32{1, 0}},
		{ID: "weak", Input: "unrelated note", Embedding: []float32{0.2, 0}},
	}
	em.mu.Unlock()

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "semantic query")
	if err != nil {
		t.Fatalf("SearchWithDiagnostics failed: %v", err)
	}
	if len(results) != 1 || results[0].Entry.ID != "strong" {
		t.Fatalf("expected only strong match above threshold, got %#v", results)
	}
	if diag.VectorFiltered != 1 || diag.SimilarityThreshold != 0.5 {
		t.Fatalf("expected threshold diagnostics, got %#v", diag)
	}
}

func TestSearchFiltersServerVectorCandidatesBelowRetrievalSimilarityThreshold(t *testing.T) {
	t.Parallel()

	store := &recordingSearchEvolvingMemoryStore{
		searchQueries: make(chan []float32, 1),
		results: []ScoredMemoryEntry{{
			Entry: &MemoryEntry{ID: "weak-server-entry", Input: "weak server task", Embedding: []float32{1, 0}},
			Score: 0.3,
		}},
	}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:                      testEmbedFn,
		Store:                        store,
		TopK:                         1,
		EnableRAG:                    true,
		RetrievalSimilarityThreshold: 0.5,
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}

	results, diag, err := em.SearchWithDiagnostics(context.Background(), "server query")
	if err != nil {
		t.Fatalf("SearchWithDiagnostics failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected weak server result below threshold to be filtered, got %#v", results)
	}
	if diag.VectorFiltered != 1 || diag.SimilarityThreshold != 0.5 {
		t.Fatalf("expected threshold diagnostics, got %#v", diag)
	}
}

func TestSearchFusesDenseAndKeywordStoreResults(t *testing.T) {
	t.Parallel()

	dense := &MemoryEntry{ID: "dense", Input: "semantic match", Embedding: []float32{1, 0}}
	keyword := &MemoryEntry{ID: "keyword", Input: "error E_BUSY in worker", Embedding: []float32{0, 1}}
	store := &recordingSearchEvolvingMemoryStore{
		searchQueries:  make(chan []float32, 1),
		keywordQueries: make(chan string, 1),
		results:        []ScoredMemoryEntry{{Entry: dense, Score: 0.9}},
		keywordResults: []ScoredMemoryEntry{{Entry: keyword, Score: 1.0}},
	}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		Store:     store,
		TopK:      2,
		EnableRAG: true,
	})
	em.embedFn = func(context.Context, config.EmbeddingConfig, []string) ([][]float32, error) {
		return [][]float32{{1, 0}}, nil
	}

	results, err := em.Search(context.Background(), "E_BUSY")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two fused results, got %d", len(results))
	}
	seen := map[string]bool{results[0].ID: true, results[1].ID: true}
	if !seen["dense"] || !seen["keyword"] {
		t.Fatalf("expected dense and keyword results, got %#v", results)
	}

	select {
	case query := <-store.keywordQueries:
		if query != "E_BUSY" {
			t.Fatalf("expected keyword query E_BUSY, got %q", query)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for keyword search")
	}
}

func TestInMemoryKeywordSearchFindsEntityHeavyMemory(t *testing.T) {
	t.Parallel()

	entries := []*MemoryEntry{
		{ID: "generic", Input: "fix background worker", Summary: "retry transient failures"},
		{ID: "entity", Input: "debug error code E_BUSY in worker", StrategyCard: "Check lock contention."},
	}
	results := inMemoryKeywordSearch(entries, "E_BUSY", 4)
	if len(results) != 1 {
		t.Fatalf("expected one keyword result, got %d", len(results))
	}
	if results[0].Entry.ID != "entity" {
		t.Fatalf("expected entity-heavy memory, got %q", results[0].Entry.ID)
	}
}

func TestRRFFuseCombinesRankedLists(t *testing.T) {
	t.Parallel()

	a := &MemoryEntry{ID: "a"}
	b := &MemoryEntry{ID: "b"}
	c := &MemoryEntry{ID: "c"}
	fused := rrfFuse([][]ScoredMemoryEntry{
		{{Entry: a, Score: 0.9}, {Entry: b, Score: 0.8}},
		{{Entry: b, Score: 1.0}, {Entry: c, Score: 0.7}},
	}, 3, 60)
	if len(fused) != 3 {
		t.Fatalf("expected three fused results, got %d", len(fused))
	}
	if fused[0].Entry.ID != "b" {
		t.Fatalf("expected item appearing in both lists to rank first, got %q", fused[0].Entry.ID)
	}
	if fused[0].Score != 1 {
		t.Fatalf("expected normalized top fused score 1, got %f", fused[0].Score)
	}
}

func TestSmartMergeReembedsMergedSummary(t *testing.T) {
	t.Parallel()

	provider := &queuedMockLLMProvider{responses: []string{"first lesson", "second lesson"}}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:          testEmbedFn,
		LLM:              provider,
		Model:            "test-model",
		EnableRAG:        true,
		EnableSmartPrune: true,
		PruneThreshold:   0.95,
	})

	ctx := context.Background()
	if err := em.EvolveEnhanced(ctx, "same task", "first output", "success", nil, nil, ""); err != nil {
		t.Fatalf("first evolve failed: %v", err)
	}
	if err := em.EvolveEnhanced(ctx, "same task", "second output", "success", nil, nil, ""); err != nil {
		t.Fatalf("second evolve failed: %v", err)
	}

	memories := em.ExportMemories()
	if len(memories) != 1 {
		t.Fatalf("expected one merged memory, got %d", len(memories))
	}
	merged := memories[0]
	wantSummary := "first lesson\n\nsecond lesson"
	if merged.Summary != wantSummary {
		t.Fatalf("expected merged summary %q, got %q", wantSummary, merged.Summary)
	}
	mergedFrom, ok := merged.Metadata["merged_from"].([]string)
	if !ok || len(mergedFrom) != 1 {
		t.Fatalf("expected merged_from metadata, got %#v", merged.Metadata["merged_from"])
	}

	wantEmbedding, err := testEmbedFn(ctx, config.EmbeddingConfig{}, []string{
		retrievalTextForMemory(merged.Input, merged.Output, merged.Feedback, merged.Summary, merged.StrategyCard),
	})
	if err != nil {
		t.Fatalf("testEmbedFn failed: %v", err)
	}
	if len(merged.Embedding) != len(wantEmbedding[0]) {
		t.Fatalf("expected merged embedding length %d, got %d", len(wantEmbedding[0]), len(merged.Embedding))
	}
	for i := range wantEmbedding[0] {
		wantNormalized := normalizeVector(wantEmbedding[0])
		if merged.Embedding[i] != wantNormalized[i] {
			t.Fatalf("expected merged embedding %v, got %v", wantNormalized, merged.Embedding)
		}
	}
}

func TestMergeEntriesPreservesBestSignals(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn})
	now := time.Now().UTC()
	em.entries = []*MemoryEntry{
		{
			ID:                 "failure",
			Input:              "bad input",
			Output:             "failed output",
			Feedback:           string(FeedbackFailure),
			StructuredFeedback: &StructuredFeedback{Type: FeedbackFailure, Correct: false, Message: "failed"},
			StrategyCard:       "Avoid skipping validation.",
			RawTrace:           "short",
			MemoryType:         MemoryEpisodic,
			AccessCount:        2,
			LastAccessedAt:     now.Add(-time.Hour),
			RelevanceScore:     0.5,
		},
		{
			ID:                 "success",
			Input:              "good input",
			Output:             "successful detailed output",
			Feedback:           string(FeedbackSuccess),
			StructuredFeedback: &StructuredFeedback{Type: FeedbackSuccess, Correct: true, Message: "worked"},
			StrategyCard:       "Run validation first.",
			RawTrace:           "longer trace with useful details",
			MemoryType:         MemoryProcedural,
			AccessCount:        3,
			LastAccessedAt:     now,
			RelevanceScore:     0.9,
		},
	}

	if err := em.mergeEntries(context.Background(), []string{"failure", "success"}, "merged lesson"); err != nil {
		t.Fatalf("mergeEntries failed: %v", err)
	}

	if len(em.entries) != 1 {
		t.Fatalf("expected one merged entry, got %d", len(em.entries))
	}
	merged := em.entries[0]
	if merged.Feedback != string(FeedbackSuccess) {
		t.Fatalf("expected success feedback to win, got %q", merged.Feedback)
	}
	if merged.StructuredFeedback == nil || merged.StructuredFeedback.Type != FeedbackSuccess || !merged.StructuredFeedback.Correct {
		t.Fatalf("expected successful structured feedback, got %#v", merged.StructuredFeedback)
	}
	if merged.Output != "successful detailed output" {
		t.Fatalf("expected representative successful output, got %q", merged.Output)
	}
	if !strings.Contains(merged.StrategyCard, "Avoid skipping validation.") || !strings.Contains(merged.StrategyCard, "Run validation first.") {
		t.Fatalf("expected unioned strategy cards, got %q", merged.StrategyCard)
	}
	if merged.RawTrace != "longer trace with useful details" {
		t.Fatalf("expected richer raw trace, got %q", merged.RawTrace)
	}
	if merged.MemoryType != MemoryProcedural {
		t.Fatalf("expected procedural memory type, got %q", merged.MemoryType)
	}
	if merged.AccessCount != 5 {
		t.Fatalf("expected summed access count 5, got %d", merged.AccessCount)
	}
	if !merged.LastAccessedAt.Equal(now) {
		t.Fatalf("expected latest access time, got %s", merged.LastAccessedAt)
	}
	if merged.RelevanceScore != 0.9 {
		t.Fatalf("expected best relevance 0.9, got %f", merged.RelevanceScore)
	}
	mergedFrom, ok := merged.Metadata["merged_from"].([]string)
	if !ok || len(mergedFrom) != 2 {
		t.Fatalf("expected merged_from metadata with both IDs, got %#v", merged.Metadata["merged_from"])
	}
}

func TestPersistEntriesAsyncCoalescesRapidUpdates(t *testing.T) {
	t.Parallel()

	store := &recordingEvolvingMemoryStore{saveCh: make(chan []*MemoryEntry, 4)}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		Store:     store,
		UserID:    9,
		SessionID: "debounce-session",
	})
	em.persistDelay = 30 * time.Millisecond

	em.persistEntriesAsync([]*MemoryEntry{{ID: "first", Input: "a"}})
	em.persistEntriesAsync([]*MemoryEntry{{ID: "second", Input: "b"}})

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 {
			t.Fatalf("expected one saved entry, got %d", len(saved))
		}
		if saved[0].ID != "second" {
			t.Fatalf("expected latest snapshot to be persisted, got %q", saved[0].ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for debounced persist")
	}

	select {
	case extra := <-store.saveCh:
		t.Fatalf("expected only one coalesced save, got extra %#v", extra)
	case <-time.After(120 * time.Millisecond):
	}
}

func TestDeltaPersistenceUpsertsDirtyEntries(t *testing.T) {
	t.Parallel()

	store := newRecordingDeltaEvolvingMemoryStore()
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:          testEmbedFn,
		LLM:              &mockLLMProvider{response: "delta lesson"},
		Model:            "test-model",
		Store:            store,
		UserID:           2,
		SessionID:        "delta-upsert-session",
		PersistDebounce:  10 * time.Millisecond,
		EnableSmartPrune: false,
	})

	if err := em.EvolveEnhanced(context.Background(), "new input", "new output", "success", nil, nil, ""); err != nil {
		t.Fatalf("EvolveEnhanced failed: %v", err)
	}

	select {
	case upserted := <-store.upsertCh:
		if len(upserted) != 1 {
			t.Fatalf("expected one upserted entry, got %d", len(upserted))
		}
		if upserted[0].Input != "new input" {
			t.Fatalf("expected dirty entry to be upserted, got %#v", upserted[0])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delta upsert")
	}

	select {
	case saved := <-store.saveCh:
		t.Fatalf("expected delta store evolve to avoid full save, got %#v", saved)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDeltaPersistenceDeletesPrunedEntries(t *testing.T) {
	t.Parallel()

	store := newRecordingDeltaEvolvingMemoryStore()
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:         testEmbedFn,
		Store:           store,
		UserID:          3,
		SessionID:       "delta-delete-session",
		PersistDebounce: 10 * time.Millisecond,
	})
	em.mu.Lock()
	em.entries = []*MemoryEntry{{ID: "entry-to-delete", Input: "old"}}
	em.mu.Unlock()

	if err := em.ApplyEdits(context.Background(), []MemoryEditOp{{Type: "PRUNE", IDs: []string{"entry-to-delete"}}}); err != nil {
		t.Fatalf("ApplyEdits failed: %v", err)
	}

	select {
	case deleted := <-store.deleteCh:
		if len(deleted) != 1 || deleted[0] != "entry-to-delete" {
			t.Fatalf("expected deleted entry ID, got %#v", deleted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delta delete")
	}
}

func TestRedactPIIRemovesKnownSecretPatterns(t *testing.T) {
	t.Parallel()

	input := "email user@example.com aws AKIA1234567890ABCDEF jwt eyJhbGciOiJIUzI1NiJ9.aaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbb key sk-abcdefghijklmnopqrstuvwxyz123456"
	redacted := redactPII(input)
	for _, forbidden := range []string{"user@example.com", "AKIA1234567890ABCDEF", "eyJhbGciOiJIUzI1NiJ9", "sk-abcdefghijklmnopqrstuvwxyz123456"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("expected %q to be redacted from %q", forbidden, redacted)
		}
	}
}

func TestEvolveEnhancedRedactsPIIAtWriteTime(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: testEmbedFn,
		LLM:     &mockLLMProvider{response: "lesson"},
		Model:   "test-model",
	})
	trace := []string{"used token eyJhbGciOiJIUzI1NiJ9.aaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbb"}
	if err := em.EvolveEnhanced(context.Background(), "contact user@example.com", "key AKIA1234567890ABCDEF", "success", nil, trace, ""); err != nil {
		t.Fatalf("EvolveEnhanced failed: %v", err)
	}
	entries := em.ExportMemories()
	if len(entries) != 1 {
		t.Fatalf("expected one memory, got %d", len(entries))
	}
	joined := entries[0].Input + entries[0].Output + entries[0].RawTrace
	for _, forbidden := range []string{"user@example.com", "AKIA1234567890ABCDEF", "eyJhbGciOiJIUzI1NiJ9"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("expected stored fields to redact %q: %#v", forbidden, entries[0])
		}
	}
}

func TestRelevancePruneKeepsSuccessfulQualityFloor(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:           testEmbedFn,
		MaxSize:           1,
		EnableSmartPrune:  true,
		PruneQualityFloor: 3,
	})
	now := time.Now().UTC()
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{
			ID:                 "protected",
			Input:              "old but useful",
			StructuredFeedback: &StructuredFeedback{Type: FeedbackSuccess, Correct: true},
			MemoryType:         MemoryProcedural,
			AccessCount:        3,
			CreatedAt:          now.Add(-365 * 24 * time.Hour),
			LastAccessedAt:     now.Add(-365 * 24 * time.Hour),
		},
		{
			ID:             "unprotected",
			Input:          "new but unused",
			CreatedAt:      now,
			LastAccessedAt: now,
		},
	}
	em.relevanceBasedPrune(context.Background())
	remaining := em.entries
	em.mu.Unlock()

	if len(remaining) != 1 || remaining[0].ID != "protected" {
		t.Fatalf("expected quality-floor protected memory to remain, got %#v", remaining)
	}
}

func TestSearchPromotesSuccessfulProceduralMemory(t *testing.T) {
	t.Parallel()

	store := newRecordingDeltaEvolvingMemoryStore()
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:                  testEmbedFn,
		Store:                    store,
		UserID:                   42,
		SessionID:                "promotion-session",
		TopK:                     1,
		EnableRAG:                true,
		PromotionAccessThreshold: 5,
	})
	vecs, err := testEmbedFn(context.Background(), config.EmbeddingConfig{}, []string{"promote me"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:                 "promote-entry",
		Input:              "promote me",
		Embedding:          normalizeVector(vecs[0]),
		MemoryType:         MemoryProcedural,
		Scope:              MemoryScopeSession,
		AccessCount:        4,
		StructuredFeedback: &StructuredFeedback{Type: FeedbackSuccess, Correct: true},
	}}
	em.mu.Unlock()

	if _, err := em.Search(context.Background(), "promote me"); err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	select {
	case promoted := <-store.promoteCh:
		if promoted != "promote-entry" {
			t.Fatalf("expected promote-entry promotion, got %q", promoted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for promotion")
	}
}

func TestSearchFiltersExpiredEntries(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn, TopK: 2, EnableRAG: true})
	vecs, err := testEmbedFn(context.Background(), config.EmbeddingConfig{}, []string{"live", "expired"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	expiredAt := time.Now().Add(-time.Minute)
	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{ID: "live", Input: "live", Embedding: normalizeVector(vecs[0])},
		{ID: "expired", Input: "expired", Embedding: normalizeVector(vecs[1]), ExpiresAt: &expiredAt},
	}
	em.mu.Unlock()

	results, err := em.Search(context.Background(), "expired")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "live" {
		t.Fatalf("expected only live memory, got %#v", results)
	}
}

func TestExplainSearchIncludesScoreComponents(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn, TopK: 1, EnableRAG: true})
	vecs, err := testEmbedFn(context.Background(), config.EmbeddingConfig{}, []string{"component query"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:                 "entry",
		Input:              "component query",
		Embedding:          normalizeVector(vecs[0]),
		AccessCount:        2,
		StructuredFeedback: &StructuredFeedback{Type: FeedbackSuccess},
	}}
	em.mu.Unlock()

	explanations, err := em.ExplainSearch(context.Background(), "component query")
	if err != nil {
		t.Fatalf("ExplainSearch failed: %v", err)
	}
	if len(explanations) != 1 {
		t.Fatalf("expected one explanation, got %d", len(explanations))
	}
	got := explanations[0]
	if got.Entry.ID != "entry" || got.Similarity == 0 || got.Decay == 0 || got.QualityWeight == 0 || got.AccessBoost == 0 || got.Composite == 0 {
		t.Fatalf("expected populated score components, got %#v", got)
	}
}

func TestConcurrentSearchEvolveAndApplyEdits(t *testing.T) {
	ctx := context.Background()
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:          testEmbedFn,
		LLM:              &mockLLMProvider{response: "lesson"},
		Model:            "test-model",
		TopK:             3,
		EnableSmartPrune: false,
	})
	if err := em.EvolveEnhanced(ctx, "initial", "ok", "success", nil, nil, ""); err != nil {
		t.Fatalf("initial evolve failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := range 8 {
		idx := i
		wg.Go(func() {
			for j := range 20 {
				_, _ = em.Search(ctx, "initial")
				_ = em.ApplyEdits(ctx, []MemoryEditOp{{Type: "UPDATE_TAG", IDs: []string{"missing"}, Tag: "tag"}})
				_ = em.EvolveEnhanced(ctx, fmt.Sprintf("task-%d-%d", idx, j), "ok", "success", nil, nil, "")
			}
		})
	}
	wg.Wait()
}

func TestEvolveEnhancedCapsStoredFieldSizes(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn: testEmbedFn,
		LLM:     &mockLLMProvider{response: "lesson"},
		Model:   "test-model",
	})

	longInput := strings.Repeat("i", maxStoredInputBytes+100)
	longOutput := strings.Repeat("o", maxStoredOutputBytes+100)
	longTraceStep := strings.Repeat("t", maxStoredRawTraceBytes+100)
	if err := em.EvolveEnhanced(context.Background(), longInput, longOutput, "success", nil, []string{longTraceStep}, ""); err != nil {
		t.Fatalf("EvolveEnhanced failed: %v", err)
	}

	entries := em.ExportMemories()
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if len(entries[0].Input) > maxStoredInputBytes {
		t.Fatalf("expected capped input, got %d bytes", len(entries[0].Input))
	}
	if len(entries[0].Output) > maxStoredOutputBytes {
		t.Fatalf("expected capped output, got %d bytes", len(entries[0].Output))
	}
	if len(entries[0].RawTrace) > maxStoredRawTraceBytes {
		t.Fatalf("expected capped raw trace, got %d bytes", len(entries[0].RawTrace))
	}
}

func TestNewEvolvingMemoryUsesConfiguredPersistDebounce(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:         testEmbedFn,
		PersistDebounce: 75 * time.Millisecond,
	})

	if em.persistDelay != 75*time.Millisecond {
		t.Fatalf("expected persist delay 75ms, got %s", em.persistDelay)
	}
}
