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
	em.mu.RLock()
	entries := filterExpiredEntries(em.snapshotEntriesLocked(), time.Now())
	cb := em.callbacks
	ragEnabled := em.enableRAG
	em.mu.RUnlock()

	diag := SearchDiagnostics{
		EnableRAG: ragEnabled,
		Mode:      "none",
	}
	if !ragEnabled {
		diag.Mode = "disabled"
		if cb != nil && cb.OnSearch != nil {
			cb.OnSearch(&MemoryEvent{
				Phase:      PhaseSearch,
				Timestamp:  start,
				Input:      query,
				MemorySize: len(entries),
				DurationMs: time.Since(start).Milliseconds(),
				Search:     diag,
			})
		}
		if em.metrics != nil {
			em.metrics.RecordSearch(ctx, time.Since(start), 0, len(entries), em.userID, em.sessionID)
		}
		return nil, diag, nil
	}

	_, hasServerSearch := em.store.(EvolvingMemorySearchStore)
	_, hasKeywordSearch := em.store.(EvolvingMemoryKeywordStore)
	if len(entries) == 0 && !hasServerSearch && !hasKeywordSearch {
		if cb != nil && cb.OnSearch != nil {
			cb.OnSearch(&MemoryEvent{
				Phase:      PhaseSearch,
				Timestamp:  start,
				Input:      query,
				MemorySize: 0,
				DurationMs: time.Since(start).Milliseconds(),
				Search:     diag,
			})
		}
		if em.metrics != nil {
			em.metrics.RecordSearch(ctx, time.Since(start), 0, 0, em.userID, em.sessionID)
		}
		return nil, diag, nil
	}

	log := observability.LoggerWithTrace(ctx)
	fetchK := em.searchFetchK()
	keywordCandidates := em.keywordCandidates(ctx, query, entries, fetchK, log)
	diag.KeywordCandidates = len(keywordCandidates)
	_, diag.UsedKeywordStore = em.store.(EvolvingMemoryKeywordStore)

	queryVec, instruction, err := em.embedQuery(ctx, query)
	diag.EmbeddingInstructionUsed = true
	diag.EmbeddingInstructionApplied = instruction.Applied
	diag.EmbeddingInstructionUseCase = instruction.UseCase
	diag.EmbeddingInstructionFormat = instruction.Format
	diag.EmbeddingInstructionMode = instruction.Mode
	diag.EmbeddingInstructionSource = instruction.Source
	if err != nil {
		diag.EmbeddingError = err.Error()
		log.Warn().Err(err).Msg("evolving_memory_embed_query_failed")
		if len(keywordCandidates) == 0 {
			if cb != nil && cb.OnSearch != nil {
				cb.OnSearch(&MemoryEvent{
					Phase:      PhaseSearch,
					Timestamp:  start,
					Input:      query,
					Error:      err,
					MemorySize: len(entries),
					DurationMs: time.Since(start).Milliseconds(),
					Search:     diag,
				})
			}
			if em.metrics != nil {
				em.metrics.RecordSearch(ctx, time.Since(start), 0, len(entries), em.userID, em.sessionID)
			}
			return nil, diag, fmt.Errorf("embed query: %w", err)
		}
	} else if len(queryVec) == 0 {
		err = fmt.Errorf("empty query embedding")
		diag.EmbeddingError = err.Error()
		if len(keywordCandidates) == 0 {
			if cb != nil && cb.OnSearch != nil {
				cb.OnSearch(&MemoryEvent{
					Phase:      PhaseSearch,
					Timestamp:  start,
					Input:      query,
					Error:      err,
					MemorySize: len(entries),
					DurationMs: time.Since(start).Milliseconds(),
					Search:     diag,
				})
			}
			if em.metrics != nil {
				em.metrics.RecordSearch(ctx, time.Since(start), 0, len(entries), em.userID, em.sessionID)
			}
			return nil, diag, err
		}
	}

	denseCandidates := make([]ScoredMemoryEntry, 0, len(entries))
	if len(queryVec) > 0 {
		if searchStore, ok := em.store.(EvolvingMemorySearchStore); ok {
			storeCandidates, err := searchStore.SearchTopK(ctx, em.userID, em.sessionID, queryVec, fetchK)
			if err != nil {
				log.Warn().Err(err).Msg("evolving_memory_store_search_failed")
			} else {
				denseCandidates = storeCandidates
				diag.UsedServerVector = true
			}
		}

		if len(denseCandidates) == 0 {
			denseCandidates = make([]ScoredMemoryEntry, 0, len(entries))
			for _, e := range entries {
				if len(e.Embedding) == 0 {
					continue
				}
				sim := dotProduct(queryVec, e.Embedding)
				denseCandidates = append(denseCandidates, ScoredMemoryEntry{Entry: e, Score: sim})
			}
		}
	}
	diag.VectorCandidates = len(denseCandidates)

	candidates := denseCandidates
	switch {
	case len(denseCandidates) > 0 && len(keywordCandidates) > 0:
		candidates = rrfFuse([][]ScoredMemoryEntry{denseCandidates, keywordCandidates}, fetchK, 60)
		diag.Mode = "hybrid"
	case len(denseCandidates) > 0:
		diag.Mode = "vector"
	case len(keywordCandidates) > 0:
		candidates = keywordCandidates
		diag.Mode = "keyword"
	}

	if len(candidates) == 0 {
		if cb != nil && cb.OnSearch != nil {
			cb.OnSearch(&MemoryEvent{
				Phase:      PhaseSearch,
				Timestamp:  start,
				Input:      query,
				MemorySize: len(entries),
				DurationMs: time.Since(start).Milliseconds(),
				Search:     diag,
			})
		}
		if em.metrics != nil {
			em.metrics.RecordSearch(ctx, time.Since(start), 0, len(entries), em.userID, em.sessionID)
		}
		return nil, diag, nil
	}

	// Score all entries with dense similarity plus quality, recency, and access signals.
	now := time.Now()
	for i, candidate := range candidates {
		if candidate.Entry == nil {
			continue
		}
		candidates[i].Score = em.compositeScore(candidate.Score, candidate.Entry, now)
	}

	// Sort descending by score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	k := em.topK
	if k > len(candidates) {
		k = len(candidates)
	}
	out := applyMMR(candidates, k, em.mmrLambda)

	retrievedIDs := make([]string, 0, len(out))
	for _, item := range out {
		if item.Entry != nil {
			retrievedIDs = append(retrievedIDs, item.Entry.ID)
		}
	}

	// Update access metrics for retrieved entries (async to not block search)
	if updateAccess {
		go em.updateAccessMetrics(retrievedIDs)
	}

	if cb != nil && cb.OnSearch != nil {
		relevance := make(map[string]float64, len(out))
		for _, o := range out {
			if o.Entry != nil {
				relevance[o.Entry.ID] = o.Score
			}
		}
		cb.OnSearch(&MemoryEvent{
			Phase:         PhaseSearch,
			Timestamp:     start,
			Input:         query,
			RetrievedIDs:  retrievedIDs,
			MemorySize:    len(entries),
			DurationMs:    time.Since(start).Milliseconds(),
			RelevanceInfo: relevance,
			Search:        diag,
		})
	}
	if em.metrics != nil {
		em.metrics.RecordSearch(ctx, time.Since(start), len(out), len(entries), em.userID, em.sessionID)
	}

	log.Debug().Int("candidates", len(entries)).Int("top_k", k).Str("mode", diag.Mode).Msg("evolving_memory_search")
	return out, diag, nil
}

// ExplainSearch returns the same high-level retrieval candidates as SearchWithScores,
// plus the ranking components used to score and diversify them. It does not update
// access counters.
func (em *EvolvingMemory) ExplainSearch(ctx context.Context, query string) ([]MemoryScoreExplanation, error) {
	em.mu.RLock()
	entries := filterExpiredEntries(em.snapshotEntriesLocked(), time.Now())
	ragEnabled := em.enableRAG
	em.mu.RUnlock()
	if !ragEnabled {
		return nil, nil
	}

	fetchK := em.searchFetchK()
	log := observability.LoggerWithTrace(ctx)
	keywordCandidates := em.keywordCandidates(ctx, query, entries, fetchK, log)
	queryVec, _, err := em.embedQuery(ctx, query)
	if err != nil {
		if len(keywordCandidates) == 0 {
			return nil, fmt.Errorf("embed query: %w", err)
		}
	}
	candidates := make([]ScoredMemoryEntry, 0, len(entries))
	if len(queryVec) > 0 {
		if searchStore, ok := em.store.(EvolvingMemorySearchStore); ok {
			if storeCandidates, err := searchStore.SearchTopK(ctx, em.userID, em.sessionID, queryVec, fetchK); err == nil {
				candidates = storeCandidates
			}
		}
		if len(candidates) == 0 {
			for _, entry := range entries {
				if entry == nil || len(entry.Embedding) == 0 {
					continue
				}
				candidates = append(candidates, ScoredMemoryEntry{Entry: entry, Score: dotProduct(queryVec, entry.Embedding)})
			}
		}
	}
	if len(candidates) > 0 && len(keywordCandidates) > 0 {
		candidates = rrfFuse([][]ScoredMemoryEntry{candidates, keywordCandidates}, fetchK, 60)
	} else if len(candidates) == 0 {
		candidates = keywordCandidates
	}

	now := time.Now()
	explanations := make([]MemoryScoreExplanation, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Entry == nil {
			continue
		}
		components := em.scoreComponents(candidate.Score, candidate.Entry, now)
		explanations = append(explanations, components)
	}
	sort.Slice(explanations, func(i, j int) bool {
		return explanations[i].Composite > explanations[j].Composite
	})
	if len(explanations) > fetchK {
		explanations = explanations[:fetchK]
	}
	selected := make([]MemoryScoreExplanation, 0, em.topK)
	used := make([]bool, len(explanations))
	for len(selected) < em.topK && len(selected) < len(explanations) {
		bestIdx := -1
		bestScore := math.Inf(-1)
		for i, candidate := range explanations {
			if used[i] || candidate.Entry == nil {
				continue
			}
			maxSimilarity := 0.0
			for _, chosen := range selected {
				if chosen.Entry != nil {
					maxSimilarity = math.Max(maxSimilarity, dotProduct(candidate.Entry.Embedding, chosen.Entry.Embedding))
				}
			}
			candidate.MMRPenalty = maxSimilarity
			candidate.FinalScore = em.mmrLambda*candidate.Composite - (1-em.mmrLambda)*maxSimilarity
			if bestIdx == -1 || candidate.FinalScore > bestScore {
				bestIdx = i
				bestScore = candidate.FinalScore
				explanations[i] = candidate
			}
		}
		if bestIdx == -1 {
			break
		}
		used[bestIdx] = true
		selected = append(selected, explanations[bestIdx])
	}
	return selected, nil
}

func (em *EvolvingMemory) searchFetchK() int {
	fetchK := em.topK * 4
	if fetchK < em.topK {
		fetchK = em.topK
	}
	if fetchK <= 0 {
		fetchK = 4
	}
	return fetchK
}

func (em *EvolvingMemory) keywordCandidates(ctx context.Context, query string, entries []*MemoryEntry, k int, log *zerolog.Logger) []ScoredMemoryEntry {
	if keywordStore, ok := em.store.(EvolvingMemoryKeywordStore); ok {
		storeCandidates, err := keywordStore.KeywordSearch(ctx, em.userID, em.sessionID, query, k)
		if err != nil {
			if log != nil {
				log.Warn().Err(err).Msg("evolving_memory_keyword_search_failed")
			}
		} else if len(storeCandidates) > 0 {
			return storeCandidates
		}
	}
	return inMemoryKeywordSearch(entries, query, k)
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
