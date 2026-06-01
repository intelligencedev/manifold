package memory

import (
	"context"
	"manifold/internal/embedding"
	"strings"
	"time"
)

func NewEvolvingMemory(cfg EvolvingMemoryConfig) *EvolvingMemory {
	resolved := resolveEvolvingMemoryConfig(cfg)
	em := &EvolvingMemory{
		entries:                      make([]*MemoryEntry, 0),
		embedCfg:                     cfg.EmbeddingConfig,
		embedFn:                      resolved.embedFn,
		llm:                          cfg.LLM,
		model:                        cfg.Model,
		maxSize:                      resolved.maxSize,
		topK:                         resolved.topK,
		windowSz:                     resolved.windowSize,
		enableRAG:                    cfg.EnableRAG,
		pruneThreshold:               resolved.pruneThreshold,
		retrievalSimilarityThreshold: resolved.retrievalSimilarityThreshold,
		relevanceDecay:               resolved.relevanceDecay,
		minRelevance:                 resolved.minRelevance,
		enableSmartPrune:             cfg.EnableSmartPrune,
		rankingWeights:               resolved.rankingWeights,
		mmrLambda:                    resolved.mmrLambda,
		reranker:                     cfg.Reranker,
		promotionAccessThreshold:     resolved.promotionAccessThreshold,
		pruneQualityFloor:            resolved.pruneQualityFloor,
		janitorInterval:              resolved.janitorInterval,
		metrics:                      cfg.Metrics,
		queryCache:                   make(map[string]embeddingCacheEntry),
		queryCacheTTL:                resolved.queryCacheTTL,
		queryCacheMax:                resolved.queryCacheMax,
		store:                        cfg.Store,
		userID:                       cfg.UserID,
		sessionID:                    resolved.sessionID,
		persistDelay:                 resolved.persistDelay,
		magmaSink:                    cfg.MagmaSink,
		dirtyIDs:                     make(map[string]struct{}),
		deletedIDs:                   make(map[string]struct{}),
		callbacks:                    cfg.Callbacks,
	}
	em.loadPersistedEntries()
	return em
}

type resolvedEvolvingMemoryConfig struct {
	topK                         int
	windowSize                   int
	maxSize                      int
	pruneThreshold               float64
	retrievalSimilarityThreshold float64
	relevanceDecay               float64
	minRelevance                 float64
	rankingWeights               RankingWeights
	mmrLambda                    float64
	queryCacheTTL                time.Duration
	queryCacheMax                int
	promotionAccessThreshold     int
	pruneQualityFloor            int
	janitorInterval              time.Duration
	sessionID                    string
	embedFn                      EmbedFunc
	persistDelay                 time.Duration
}

func resolveEvolvingMemoryConfig(cfg EvolvingMemoryConfig) resolvedEvolvingMemoryConfig {
	return resolvedEvolvingMemoryConfig{
		topK:                         defaultInt(cfg.TopK, 4),
		windowSize:                   defaultInt(cfg.WindowSize, 20),
		maxSize:                      defaultInt(cfg.MaxSize, 1000),
		pruneThreshold:               defaultFloat(cfg.PruneThreshold, 0.95),
		retrievalSimilarityThreshold: defaultUnitThreshold(cfg.RetrievalSimilarityThreshold),
		relevanceDecay:               defaultFloat(cfg.RelevanceDecay, 0.99),
		minRelevance:                 defaultFloat(cfg.MinRelevance, 0.1),
		rankingWeights:               normalizeRankingWeights(cfg.RankingWeights),
		mmrLambda:                    defaultUnitFloat(cfg.MMRLambda, 0.7),
		queryCacheTTL:                defaultDuration(cfg.QueryEmbeddingCacheTTL, 5*time.Minute),
		queryCacheMax:                defaultInt(cfg.QueryEmbeddingCacheSize, 128),
		promotionAccessThreshold:     defaultInt(cfg.PromotionAccessThreshold, defaultPromotionAccessThreshold),
		pruneQualityFloor:            defaultInt(cfg.PruneQualityFloor, defaultPruneQualityFloor),
		janitorInterval:              resolveJanitorInterval(cfg.JanitorInterval),
		sessionID:                    defaultString(strings.TrimSpace(cfg.SessionID), "default"),
		embedFn:                      resolveEmbedFn(cfg.EmbedFn),
		persistDelay:                 defaultDuration(cfg.PersistDebounce, 250*time.Millisecond),
	}
}

func defaultInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultFloat(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultUnitFloat(value, fallback float64) float64 {
	if value <= 0 || value > 1 {
		return fallback
	}
	return value
}

func defaultUnitThreshold(value float64) float64 {
	if value <= 0 || value > 1 {
		return 0
	}
	return value
}

func defaultDuration(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func resolveJanitorInterval(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	if value == 0 {
		return defaultMemoryJanitorInterval
	}
	return value
}

func resolveEmbedFn(embedFn EmbedFunc) EmbedFunc {
	if embedFn == nil {
		return embedding.EmbedText
	}
	return embedFn
}

func (em *EvolvingMemory) loadPersistedEntries() {
	if em.store == nil {
		return
	}
	entries, err := em.store.Load(context.Background(), em.userID, em.sessionID)
	if err == nil && len(entries) > 0 {
		em.entries = filterExpiredEntries(normalizeLoadedEntries(entries, em.maxSize), time.Now())
	}
	if em.janitorInterval > 0 {
		em.startStoreJanitor(em.janitorInterval)
	}
}

func normalizeLoadedEntries(entries []*MemoryEntry, maxSize int) []*MemoryEntry {
	if len(entries) > maxSize {
		entries = entries[len(entries)-maxSize:]
	}
	for _, entry := range entries {
		if entry != nil {
			entry.Embedding = normalizeVector(entry.Embedding)
			if entry.Scope == "" {
				entry.Scope = MemoryScopeSession
			}
		}
	}
	return entries
}

// SetCallbacks sets (or clears) callbacks for observability.
