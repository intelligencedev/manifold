package memory

import (
	"context"
	"fmt"
	"manifold/internal/observability"
	"math"
	"sort"
	"time"

	"github.com/rs/zerolog"
)

func (em *EvolvingMemory) SetCallbacks(cb *MemoryCallbacks) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.callbacks = cb
}

// Search implements R(M_t, x_t): retrieve top-k similar experiences via cosine similarity.
func (em *EvolvingMemory) Search(ctx context.Context, query string) ([]*MemoryEntry, error) {
	scored, err := em.SearchWithScores(ctx, query)
	if err != nil {
		return nil, err
	}
	results := make([]*MemoryEntry, len(scored))
	for i, s := range scored {
		results[i] = s.Entry
	}
	return results, nil
}

// SearchWithScores is like Search but also returns the similarity score for
// each retrieved memory entry. This is used by debug/observability surfaces to
// explain *why* a particular memory was selected for a given query.
func (em *EvolvingMemory) SearchWithScores(ctx context.Context, query string) ([]ScoredMemoryEntry, error) {
	scored, _, err := em.SearchWithDiagnostics(ctx, query)
	return scored, err
}

// SearchWithDiagnostics returns scored memories plus information about which
// retrieval path was used. Like SearchWithScores, it updates access metrics for
// returned memories.
func (em *EvolvingMemory) SearchWithDiagnostics(ctx context.Context, query string) ([]ScoredMemoryEntry, SearchDiagnostics, error) {
	return em.searchWithScores(ctx, query, true)
}

func (em *EvolvingMemory) searchWithScores(ctx context.Context, query string, updateAccess bool) ([]ScoredMemoryEntry, SearchDiagnostics, error) {
	start := time.Now()
	entries, cb, ragEnabled := em.searchSnapshot()
	diag := SearchDiagnostics{
		EnableRAG: ragEnabled,
		Mode:      "none",
	}
	if !ragEnabled {
		diag.Mode = "disabled"
		em.finishSearch(ctx, searchReport{start: start, query: query, entries: entries, diag: diag, callbacks: cb})
		return nil, diag, nil
	}

	_, hasServerSearch := em.store.(EvolvingMemorySearchStore)
	_, hasKeywordSearch := em.store.(EvolvingMemoryKeywordStore)
	if len(entries) == 0 && !hasServerSearch && !hasKeywordSearch {
		em.finishSearch(ctx, searchReport{start: start, query: query, entries: entries, diag: diag, callbacks: cb})
		return nil, diag, nil
	}

	log := observability.LoggerWithTrace(ctx)
	fetchK := em.searchFetchK()
	keywordCandidates := em.keywordCandidates(ctx, query, entries, fetchK, log)
	diag.KeywordCandidates = len(keywordCandidates)
	_, diag.UsedKeywordStore = em.store.(EvolvingMemoryKeywordStore)

	queryVec, err := em.queryVector(ctx, query, &diag)
	if err != nil {
		log.Warn().Err(err).Msg("evolving_memory_embed_query_failed")
		if len(keywordCandidates) == 0 {
			em.finishSearch(ctx, searchReport{start: start, query: query, entries: entries, diag: diag, callbacks: cb, err: err})
			return nil, diag, fmt.Errorf("embed query: %w", err)
		}
	}
	if err == nil && len(queryVec) == 0 {
		err := fmt.Errorf("empty query embedding")
		diag.EmbeddingError = err.Error()
		if len(keywordCandidates) == 0 {
			em.finishSearch(ctx, searchReport{start: start, query: query, entries: entries, diag: diag, callbacks: cb, err: err})
			return nil, diag, err
		}
	}

	denseCandidates := em.denseCandidates(ctx, queryVec, entries, fetchK, &diag, log)
	denseBeforeFilter := len(denseCandidates)
	diag.DenseBeforeFilter = denseBeforeFilter
	denseCandidates = em.filterDenseCandidatesByThreshold(denseCandidates, &diag)
	diag.VectorCandidates = len(denseCandidates)
	if denseBeforeFilter > 0 && len(denseCandidates) == 0 {
		// Dense retrieval ran and nothing cleared the similarity bar: do not
		// degrade to ungated keyword-only noise.
		diag.Mode = "vector_below_threshold"
		em.finishSearch(ctx, searchReport{start: start, query: query, entries: entries, diag: diag, callbacks: cb})
		return nil, diag, nil
	}
	candidates := mergeMemoryCandidates(denseCandidates, keywordCandidates, fetchK, &diag)
	if len(candidates) == 0 {
		em.finishSearch(ctx, searchReport{start: start, query: query, entries: entries, diag: diag, callbacks: cb})
		return nil, diag, nil
	}

	ranked := em.scoreMemoryCandidates(candidates)
	ranked = em.rerankMemoryCandidates(ctx, query, ranked, &diag, log)
	out := applyMMR(ranked, min(em.topK, len(ranked)), em.mmrLambda)
	retrievedIDs := scoredMemoryIDs(out)
	if updateAccess && diag.Mode != "keyword" {
		go em.updateAccessMetrics(retrievedIDs)
	}
	em.finishSearch(ctx, searchReport{start: start, query: query, entries: entries, out: out, retrievedIDs: retrievedIDs, diag: diag, callbacks: cb})
	log.Debug().Int("candidates", len(entries)).Int("top_k", len(out)).Str("mode", diag.Mode).Msg("evolving_memory_search")
	return out, diag, nil
}

func (em *EvolvingMemory) searchSnapshot() ([]*MemoryEntry, *MemoryCallbacks, bool) {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return filterExpiredEntries(em.snapshotEntriesLocked(), time.Now()), em.callbacks, em.enableRAG
}

func (em *EvolvingMemory) queryVector(ctx context.Context, query string, diag *SearchDiagnostics) ([]float32, error) {
	queryVec, instruction, err := em.embedQuery(ctx, query)
	diag.EmbeddingInstructionUsed = true
	diag.EmbeddingInstructionApplied = instruction.Applied
	diag.EmbeddingInstructionUseCase = instruction.UseCase
	diag.EmbeddingInstructionFormat = instruction.Format
	diag.EmbeddingInstructionMode = instruction.Mode
	diag.EmbeddingInstructionSource = instruction.Source
	if err != nil {
		diag.EmbeddingError = err.Error()
	}
	return queryVec, err
}

func (em *EvolvingMemory) denseCandidates(ctx context.Context, queryVec []float32, entries []*MemoryEntry, fetchK int, diag *SearchDiagnostics, log *zerolog.Logger) []ScoredMemoryEntry {
	if len(queryVec) == 0 {
		return nil
	}
	var candidates []ScoredMemoryEntry
	if searchStore, ok := em.store.(EvolvingMemorySearchStore); ok {
		storeCandidates, err := searchStore.SearchTopK(ctx, em.userID, em.sessionID, queryVec, fetchK)
		if err != nil {
			log.Warn().Err(err).Msg("evolving_memory_store_search_failed")
		} else {
			diag.UsedServerVector = true
			if len(storeCandidates) > 0 {
				candidates = storeCandidates
			}
		}
	}
	if candidates == nil {
		candidates = localDenseCandidates(queryVec, entries)
	}
	return annotateDenseCandidates(candidates)
}

func localDenseCandidates(queryVec []float32, entries []*MemoryEntry) []ScoredMemoryEntry {
	candidates := make([]ScoredMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || len(entry.Embedding) == 0 {
			continue
		}
		candidates = append(candidates, ScoredMemoryEntry{Entry: entry, Score: dotProduct(queryVec, entry.Embedding)})
	}
	return candidates
}

func annotateDenseCandidates(candidates []ScoredMemoryEntry) []ScoredMemoryEntry {
	for i := range candidates {
		candidates[i].Dense = candidates[i].Score
		candidates[i].HasDense = true
	}
	return candidates
}

func (em *EvolvingMemory) filterDenseCandidatesByThreshold(candidates []ScoredMemoryEntry, diag *SearchDiagnostics) []ScoredMemoryEntry {
	threshold := em.retrievalSimilarityThreshold
	if diag != nil {
		diag.SimilarityThreshold = threshold
	}
	if threshold <= 0 {
		return candidates
	}
	filtered := make([]ScoredMemoryEntry, 0, len(candidates))
	for _, candidate := range candidates {
		sim := candidate.Score
		if candidate.HasDense {
			sim = candidate.Dense
		}
		if sim >= threshold {
			filtered = append(filtered, candidate)
			continue
		}
		if diag != nil {
			diag.VectorFiltered++
		}
	}
	return filtered
}

func mergeMemoryCandidates(denseCandidates, keywordCandidates []ScoredMemoryEntry, fetchK int, diag *SearchDiagnostics) []ScoredMemoryEntry {
	switch {
	case len(denseCandidates) > 0 && len(keywordCandidates) > 0:
		diag.Mode = "hybrid"
		return rrfFuse([][]ScoredMemoryEntry{denseCandidates, keywordCandidates}, fetchK, 60)
	case len(denseCandidates) > 0:
		diag.Mode = "vector"
		return denseCandidates
	case len(keywordCandidates) > 0:
		diag.Mode = "keyword"
		return keywordCandidates
	default:
		return nil
	}
}

func (em *EvolvingMemory) scoreMemoryCandidates(candidates []ScoredMemoryEntry) []ScoredMemoryEntry {
	now := time.Now()
	for i, candidate := range candidates {
		if candidate.Entry == nil {
			continue
		}
		sim := candidate.Score
		if candidate.HasDense {
			sim = candidate.Dense
		}
		composite := em.compositeScore(sim, candidate.Entry, now)
		candidates[i].Composite = composite
		candidates[i].Score = composite
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	return candidates
}

func (em *EvolvingMemory) rerankMemoryCandidates(ctx context.Context, query string, candidates []ScoredMemoryEntry, diag *SearchDiagnostics, log *zerolog.Logger) []ScoredMemoryEntry {
	if em.reranker == nil {
		return candidates
	}
	if diag != nil {
		diag.RerankEnabled = true
		diag.RerankCandidates = len(candidates)
	}
	if len(candidates) <= 1 {
		return candidates
	}
	start := time.Now()
	reranked, err := em.reranker.RerankEvolvingMemory(ctx, query, candidates)
	if diag != nil {
		diag.RerankDurationMs = time.Since(start).Milliseconds()
	}
	if err != nil {
		if diag != nil {
			diag.RerankError = err.Error()
		}
		if log != nil {
			log.Warn().Err(err).Msg("evolving_memory_rerank_failed")
		}
		return candidates
	}
	if len(reranked) == 0 {
		if diag != nil {
			diag.RerankError = "reranker returned no candidates"
		}
		return candidates
	}
	if diag != nil {
		diag.RerankApplied = true
	}
	normalizeRerankScores(reranked)
	return em.filterRerankedCandidates(reranked, diag)
}

func normalizeRerankScores(candidates []ScoredMemoryEntry) {
	needsSigmoid := false
	for _, candidate := range candidates {
		if !candidate.HasRerank {
			continue
		}
		if candidate.Rerank < 0 || candidate.Rerank > 1 {
			needsSigmoid = true
			break
		}
	}
	if !needsSigmoid {
		return
	}
	for i := range candidates {
		if !candidates[i].HasRerank {
			continue
		}
		candidates[i].Rerank = 1 / (1 + math.Exp(-candidates[i].Rerank))
		candidates[i].Score = candidates[i].Rerank
	}
}

func (em *EvolvingMemory) filterRerankedCandidates(candidates []ScoredMemoryEntry, diag *SearchDiagnostics) []ScoredMemoryEntry {
	floor := em.minRerankScore
	if floor <= 0 {
		return candidates
	}
	filtered := make([]ScoredMemoryEntry, 0, len(candidates))
	for _, candidate := range candidates {
		score := candidate.Score
		if candidate.HasRerank {
			score = candidate.Rerank
		}
		if score >= floor {
			filtered = append(filtered, candidate)
			continue
		}
		if diag != nil {
			diag.RerankFiltered++
		}
	}
	return filtered
}

func scoredMemoryIDs(scored []ScoredMemoryEntry) []string {
	ids := make([]string, 0, len(scored))
	for _, item := range scored {
		if item.Entry != nil {
			ids = append(ids, item.Entry.ID)
		}
	}
	return ids
}

type searchReport struct {
	start        time.Time
	query        string
	entries      []*MemoryEntry
	out          []ScoredMemoryEntry
	retrievedIDs []string
	diag         SearchDiagnostics
	callbacks    *MemoryCallbacks
	err          error
}

func (em *EvolvingMemory) finishSearch(ctx context.Context, report searchReport) {
	if report.callbacks != nil && report.callbacks.OnSearch != nil {
		report.callbacks.OnSearch(searchEvent(report))
	}
	if em.metrics != nil {
		em.metrics.RecordSearch(ctx, time.Since(report.start), len(report.out), len(report.entries), em.userID, em.sessionID)
	}
}

func searchEvent(report searchReport) *MemoryEvent {
	return &MemoryEvent{
		Phase:         PhaseSearch,
		Timestamp:     report.start,
		Input:         report.query,
		Error:         report.err,
		RetrievedIDs:  report.retrievedIDs,
		MemorySize:    len(report.entries),
		DurationMs:    time.Since(report.start).Milliseconds(),
		RelevanceInfo: relevanceInfo(report.out),
		Search:        report.diag,
	}
}

func relevanceInfo(scored []ScoredMemoryEntry) map[string]float64 {
	if len(scored) == 0 {
		return nil
	}
	relevance := make(map[string]float64, len(scored))
	for _, item := range scored {
		if item.Entry != nil {
			relevance[item.Entry.ID] = item.Score
		}
	}
	return relevance
}

// ExplainSearch returns the same high-level retrieval candidates as SearchWithScores,
// plus the ranking components used to score and diversify them. It does not update
// access counters.
func (em *EvolvingMemory) ExplainSearch(ctx context.Context, query string) ([]MemoryScoreExplanation, error) {
	scored, _, err := em.searchWithScores(ctx, query, false)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	explanations := make([]MemoryScoreExplanation, 0, len(scored))
	for _, candidate := range scored {
		if candidate.Entry == nil {
			continue
		}
		sim := candidate.Score
		if candidate.HasDense {
			sim = candidate.Dense
		}
		components := em.scoreComponents(sim, candidate.Entry, now)
		if candidate.Composite != 0 {
			components.Composite = candidate.Composite
		}
		components.FinalScore = candidate.Score
		explanations = append(explanations, components)
	}
	return explanations, nil
}

func (em *EvolvingMemory) searchFetchK() int {
	fetchK := max(em.topK*4, em.topK)
	if fetchK <= 0 {
		fetchK = 4
	}
	return fetchK
}

func (em *EvolvingMemory) keywordCandidates(ctx context.Context, query string, entries []*MemoryEntry, k int, log *zerolog.Logger) []ScoredMemoryEntry {
	var candidates []ScoredMemoryEntry
	if keywordStore, ok := em.store.(EvolvingMemoryKeywordStore); ok {
		storeCandidates, err := keywordStore.KeywordSearch(ctx, em.userID, em.sessionID, query, k)
		if err != nil {
			if log != nil {
				log.Warn().Err(err).Msg("evolving_memory_keyword_search_failed")
			}
		} else if len(storeCandidates) > 0 {
			candidates = storeCandidates
		}
	}
	if candidates == nil {
		candidates = inMemoryKeywordSearch(entries, query, k)
	}
	return annotateKeywordCandidates(candidates)
}

func annotateKeywordCandidates(candidates []ScoredMemoryEntry) []ScoredMemoryEntry {
	for i := range candidates {
		candidates[i].Keyword = candidates[i].Score
		candidates[i].HasKeyword = true
	}
	return candidates
}

// updateAccessMetrics increments access counts and updates last accessed time.
func (em *EvolvingMemory) updateAccessMetrics(ids []string) {
	em.mu.Lock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	now := time.Now()
	for _, e := range em.entries {
		if idSet[e.ID] {
			e.AccessCount++
			e.LastAccessedAt = now
			if em.shouldPromoteToUserScopeLocked(e) {
				e.Scope = MemoryScopeUser
				em.markDirtyLocked(e.ID)
				if promotionStore, ok := em.store.(EvolvingMemoryPromotionStore); ok {
					em.promoteToUserScopeAsync(promotionStore, e.ID)
				}
			}
		}
	}
	var entriesSnapshot []*MemoryEntry
	if _, ok := em.store.(EvolvingMemoryDeltaStore); !ok {
		entriesSnapshot = em.snapshotEntriesLocked()
	}
	em.mu.Unlock()

	if deltaStore, ok := em.store.(EvolvingMemoryDeltaStore); ok {
		em.touchAccessAsync(deltaStore, ids, now)
		return
	}
	em.persistEntriesAsync(entriesSnapshot)
}

// Synthesize implements C(x_t, R_t): build context from current task + retrieved memories.
