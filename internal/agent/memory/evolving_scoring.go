package memory

import (
	"context"
	"fmt"
	"manifold/internal/observability"
	"math"
	"sort"
	"time"
)

func (em *EvolvingMemory) relevanceBasedPrune(ctx context.Context) {
	log := observability.LoggerWithTrace(ctx)

	now := time.Now()
	filtered := make([]*MemoryEntry, 0, len(em.entries))
	protected := make([]*MemoryEntry, 0)
	removedIDs := make([]string, 0)
	for _, e := range em.entries {
		if em.isProtectedByQualityFloor(e) {
			protected = append(protected, e)
			continue
		}
		e.RelevanceScore = em.computeRelevanceScore(now, e)
		if e.RelevanceScore >= em.minRelevance {
			filtered = append(filtered, e)
		} else {
			removedIDs = append(removedIDs, e.ID)
		}
	}
	em.entries = append(protected, filtered...)

	// Sort pruneable entries by relevance score (ascending - lowest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].RelevanceScore < filtered[j].RelevanceScore
	})

	// Calculate how many to remove
	toRemove := len(protected) + len(filtered) - em.maxSize
	if toRemove <= 0 {
		em.entries = append(protected, filtered...)
		return
	}

	// Remove lowest relevance entries
	if toRemove > len(filtered) {
		toRemove = len(filtered)
	}
	for i := 0; i < toRemove; i++ {
		removedIDs = append(removedIDs, filtered[i].ID)
	}
	em.entries = append(protected, filtered[toRemove:]...)
	em.markDeletedLocked(removedIDs)

	// Re-sort by creation time to maintain temporal order
	sort.Slice(em.entries, func(i, j int) bool {
		return em.entries[i].CreatedAt.Before(em.entries[j].CreatedAt)
	})

	log.Info().
		Int("removed_count", len(removedIDs)).
		Int("remaining", len(em.entries)).
		Msg("evolving_memory_relevance_pruned")
	if em.metrics != nil && len(removedIDs) > 0 {
		em.metrics.RecordPruned(ctx, "relevance", len(removedIDs))
	}
}

func (em *EvolvingMemory) isProtectedByQualityFloor(entry *MemoryEntry) bool {
	if entry == nil || em.pruneQualityFloor <= 0 || entry.AccessCount < em.pruneQualityFloor {
		return false
	}
	return entry.StructuredFeedback != nil && entry.StructuredFeedback.Type == FeedbackSuccess
}

func (em *EvolvingMemory) computeRelevanceScore(now time.Time, entry *MemoryEntry) float64 {
	if entry == nil {
		return 0
	}

	referenceTime := entry.LastAccessedAt
	if referenceTime.IsZero() {
		referenceTime = entry.CreatedAt
	}
	if referenceTime.IsZero() {
		referenceTime = now
	}

	daysSinceAccess := now.Sub(referenceTime).Hours() / 24
	if daysSinceAccess < 0 {
		daysSinceAccess = 0
	}

	decayFactor := math.Pow(em.relevanceDecay, daysSinceAccess)
	accessBoost := 1.0 + 0.1*math.Log1p(float64(entry.AccessCount))
	return decayFactor * accessBoost
}

func normalizeRankingWeights(weights RankingWeights) RankingWeights {
	if weights.DecayHalfLifeDays <= 0 {
		weights.DecayHalfLifeDays = 30
	}
	if weights.SuccessWeight <= 0 {
		weights.SuccessWeight = 1.2
	}
	if weights.FailureWeight <= 0 {
		weights.FailureWeight = 0.6
	}
	if weights.PartialWeight <= 0 {
		weights.PartialWeight = 0.9
	}
	if weights.AccessCountWeight <= 0 {
		weights.AccessCountWeight = 0.1
	}
	return weights
}

func (em *EvolvingMemory) embedQuery(ctx context.Context, query string) ([]float32, error) {
	now := time.Now()
	em.queryCacheMu.Lock()
	if cached, ok := em.queryCache[query]; ok && now.Before(cached.expiresAt) {
		cached.lastUsed = now
		em.queryCache[query] = cached
		vec := append([]float32(nil), cached.vec...)
		em.queryCacheMu.Unlock()
		return vec, nil
	}
	em.queryCacheMu.Unlock()

	vecs, err := em.embedFn(ctx, em.embedCfg, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}
	vec := normalizeVector(vecs[0])
	em.storeQueryEmbedding(query, vec, now)
	return vec, nil
}

func (em *EvolvingMemory) storeQueryEmbedding(query string, vec []float32, now time.Time) {
	if em.queryCacheMax <= 0 || em.queryCacheTTL <= 0 {
		return
	}
	em.queryCacheMu.Lock()
	defer em.queryCacheMu.Unlock()

	if len(em.queryCache) >= em.queryCacheMax {
		var oldestKey string
		var oldest time.Time
		for key, cached := range em.queryCache {
			if oldestKey == "" || cached.lastUsed.Before(oldest) {
				oldestKey = key
				oldest = cached.lastUsed
			}
		}
		delete(em.queryCache, oldestKey)
	}

	em.queryCache[query] = embeddingCacheEntry{
		vec:       append([]float32(nil), vec...),
		expiresAt: now.Add(em.queryCacheTTL),
		lastUsed:  now,
	}
}

func (em *EvolvingMemory) compositeScore(sim float64, entry *MemoryEntry, now time.Time) float64 {
	return em.scoreComponents(sim, entry, now).Composite
}

func (em *EvolvingMemory) scoreComponents(sim float64, entry *MemoryEntry, now time.Time) MemoryScoreExplanation {
	weights := em.rankingWeights
	referenceTime := entry.LastAccessedAt
	if referenceTime.IsZero() {
		referenceTime = entry.CreatedAt
	}
	if referenceTime.IsZero() {
		referenceTime = now
	}
	ageDays := now.Sub(referenceTime).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	decay := math.Pow(0.5, ageDays/weights.DecayHalfLifeDays)
	accessBoost := 1 + weights.AccessCountWeight*math.Log1p(float64(entry.AccessCount))
	quality := qualityWeight(entry.StructuredFeedback, weights)
	composite := sim * decay * quality * accessBoost
	return MemoryScoreExplanation{
		Entry:         entry,
		Similarity:    sim,
		Decay:         decay,
		QualityWeight: quality,
		AccessBoost:   accessBoost,
		Composite:     composite,
		FinalScore:    composite,
	}
}

func qualityWeight(feedback *StructuredFeedback, weights RankingWeights) float64 {
	if feedback == nil {
		return 1
	}
	switch feedback.Type {
	case FeedbackSuccess:
		return weights.SuccessWeight
	case FeedbackFailure:
		return weights.FailureWeight
	case FeedbackPartial, FeedbackInProgress:
		return weights.PartialWeight
	default:
		return 1
	}
}
