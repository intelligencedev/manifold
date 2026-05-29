package memory

import (
	"context"
	"manifold/internal/embedding"
	"strings"
	"time"
)

func NewEvolvingMemory(cfg EvolvingMemoryConfig) *EvolvingMemory {
	topK := cfg.TopK
	if topK <= 0 {
		topK = 4
	}
	windowSz := cfg.WindowSize
	if windowSz <= 0 {
		windowSz = 20
	}
	maxSz := cfg.MaxSize
	if maxSz <= 0 {
		maxSz = 1000 // reasonable default
	}

	// Pruning defaults from paper analysis
	pruneThreshold := cfg.PruneThreshold
	if pruneThreshold <= 0 {
		pruneThreshold = 0.95 // high similarity = near duplicate
	}
	relevanceDecay := cfg.RelevanceDecay
	if relevanceDecay <= 0 {
		relevanceDecay = 0.99 // 1% daily decay
	}
	minRelevance := cfg.MinRelevance
	if minRelevance <= 0 {
		minRelevance = 0.1
	}
	rankingWeights := normalizeRankingWeights(cfg.RankingWeights)
	mmrLambda := cfg.MMRLambda
	if mmrLambda <= 0 || mmrLambda > 1 {
		mmrLambda = 0.7
	}
	queryCacheTTL := cfg.QueryEmbeddingCacheTTL
	if queryCacheTTL <= 0 {
		queryCacheTTL = 5 * time.Minute
	}
	queryCacheMax := cfg.QueryEmbeddingCacheSize
	if queryCacheMax <= 0 {
		queryCacheMax = 128
	}
	promotionAccessThreshold := cfg.PromotionAccessThreshold
	if promotionAccessThreshold <= 0 {
		promotionAccessThreshold = defaultPromotionAccessThreshold
	}
	pruneQualityFloor := cfg.PruneQualityFloor
	if pruneQualityFloor <= 0 {
		pruneQualityFloor = defaultPruneQualityFloor
	}
	janitorInterval := cfg.JanitorInterval
	if janitorInterval < 0 {
		janitorInterval = 0
	} else if janitorInterval == 0 {
		janitorInterval = defaultMemoryJanitorInterval
	}

	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	embedFn := cfg.EmbedFn
	if embedFn == nil {
		embedFn = embedding.EmbedText
	}
	persistDelay := cfg.PersistDebounce
	if persistDelay <= 0 {
		persistDelay = 250 * time.Millisecond
	}

	em := &EvolvingMemory{
		entries:                  make([]*MemoryEntry, 0),
		embedCfg:                 cfg.EmbeddingConfig,
		embedFn:                  embedFn,
		llm:                      cfg.LLM,
		model:                    cfg.Model,
		maxSize:                  maxSz,
		topK:                     topK,
		windowSz:                 windowSz,
		enableRAG:                cfg.EnableRAG,
		pruneThreshold:           pruneThreshold,
		relevanceDecay:           relevanceDecay,
		minRelevance:             minRelevance,
		enableSmartPrune:         cfg.EnableSmartPrune,
		rankingWeights:           rankingWeights,
		mmrLambda:                mmrLambda,
		promotionAccessThreshold: promotionAccessThreshold,
		pruneQualityFloor:        pruneQualityFloor,
		janitorInterval:          janitorInterval,
		metrics:                  cfg.Metrics,
		queryCache:               make(map[string]embeddingCacheEntry),
		queryCacheTTL:            queryCacheTTL,
		queryCacheMax:            queryCacheMax,
		store:                    cfg.Store,
		userID:                   cfg.UserID,
		sessionID:                sessionID,
		persistDelay:             persistDelay,
		magmaSink:                cfg.MagmaSink,
		dirtyIDs:                 make(map[string]struct{}),
		deletedIDs:               make(map[string]struct{}),
		callbacks:                cfg.Callbacks,
	}

	// If a store is provided, preload entries for the configured user.
	// Note: systemUserID is 0 in agentd; we still want persistence for it.
	if em.store != nil {
		if entries, err := em.store.Load(context.Background(), em.userID, em.sessionID); err == nil && len(entries) > 0 {
			// Respect maxSize by keeping only the newest maxSize entries.
			if len(entries) > em.maxSize {
				entries = entries[len(entries)-em.maxSize:]
			}
			for _, entry := range entries {
				if entry != nil {
					entry.Embedding = normalizeVector(entry.Embedding)
					if entry.Scope == "" {
						entry.Scope = MemoryScopeSession
					}
				}
			}
			em.entries = filterExpiredEntries(entries, time.Now())
		}
		if janitorInterval > 0 {
			em.startStoreJanitor(janitorInterval)
		}
	}

	return em
}

// SetCallbacks sets (or clears) callbacks for observability.
