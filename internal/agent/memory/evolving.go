package memory

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"manifold/internal/config"
	"manifold/internal/embedding"
	"manifold/internal/llm"
	"manifold/internal/observability"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// PhaseType represents the phases of the Search → Synthesis → Evolve loop.
// It is used for callbacks/observability.
type PhaseType string

const (
	PhaseSearch    PhaseType = "search"    // Retrieve relevant memories
	PhaseSynthesis PhaseType = "synthesis" // Construct context from memories
	PhaseEvolve    PhaseType = "evolve"    // Store new experiences
)

// MemoryEvent is emitted for observability hooks around memory operations.
// It is designed to be stable and cheap to populate (no large payloads).
type MemoryEvent struct {
	Phase         PhaseType
	Timestamp     time.Time
	Input         string
	RetrievedIDs  []string
	OutputSize    int
	Error         error
	DurationMs    int64
	MemorySize    int
	RelevanceInfo map[string]float64 // memory_id -> similarity score (if available)
}

// MemoryCallbacks allow embedding the memory system into higher-level
// observability pipelines (tracing, logging, UI debugging, etc).
type MemoryCallbacks struct {
	OnSearch      func(*MemoryEvent)
	OnSynthesized func(*MemoryEvent)
	OnEvolve      func(*MemoryEvent)
}

// EmbedFunc is an injectable embedding function used by EvolvingMemory.
// In production it defaults to embedding.EmbedText; in tests it can be stubbed.
type EmbedFunc func(ctx context.Context, cfg config.EmbeddingConfig, texts []string) ([][]float32, error)

// FeedbackType represents structured feedback categories from the paper.
type FeedbackType string

const (
	FeedbackSuccess    FeedbackType = "success"     // Task completed successfully
	FeedbackFailure    FeedbackType = "failure"     // Task failed
	FeedbackPartial    FeedbackType = "partial"     // Partial success/progress
	FeedbackInProgress FeedbackType = "in_progress" // Multi-turn task ongoing
)

// MemoryType distinguishes between factual recall and procedural/strategic memories.
// The paper emphasizes the distinction between "What" (conversational recall) and
// "How" (experience reuse/procedural knowledge).
type MemoryType string

const (
	MemoryFactual    MemoryType = "factual"    // Facts, data, static knowledge
	MemoryProcedural MemoryType = "procedural" // Strategies, workflows, how-to
	MemoryEpisodic   MemoryType = "episodic"   // Specific task episodes
)

// MemoryScope controls whether a memory is session-local or promoted for reuse
// across a user's sessions.
type MemoryScope string

const (
	MemoryScopeSession MemoryScope = "session"
	MemoryScopeUser    MemoryScope = "user"
	MemoryScopeTenant  MemoryScope = "tenant"
)

const (
	maxStoredInputBytes    = 8 * 1024
	maxStoredOutputBytes   = 4 * 1024
	maxStoredRawTraceBytes = 16 * 1024

	defaultPromotionAccessThreshold = 5
	defaultPruneQualityFloor        = 3
	defaultMemoryJanitorInterval    = time.Hour
)

var (
	proceduralMemoryPattern = regexp.MustCompile(`(?i)\b(?:how\s+to|steps|procedure|workflow|strategy|algorithm|method|approach|technique|process|when\s+confronted|do\s+this|avoid|pattern)\b`)
	factualMemoryPattern    = regexp.MustCompile(`(?i)\b(?:what\s+is|define|meaning\s+of|value\s+of|answer\s+is|result\s+is|equals|fact)\b`)
	emailPattern            = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	awsAccessKeyPattern     = regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)
	jwtPattern              = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
	apiKeyPattern           = regexp.MustCompile(`(?i)\b(?:sk|pk|rk|ghp|gho|github_pat|xox[baprs]|AIza)[A-Za-z0-9_\-]{16,}\b`)
)

// StructuredFeedback provides detailed feedback signals beyond simple success/failure.
type StructuredFeedback struct {
	Type         FeedbackType `json:"type"`
	Correct      bool         `json:"correct"`       // Binary correctness
	ProgressRate float64      `json:"progress_rate"` // 0.0-1.0 for multi-turn tasks
	StepsUsed    int          `json:"steps_used"`    // Step efficiency metric
	StepsOptimal int          `json:"steps_optimal"` // Optimal steps (if known)
	Message      string       `json:"message"`       // Human-readable feedback
}

// MemoryEntry represents a structured experience from task execution.
// Implements the paper's m_i = h(x_i, ŷ_i, f_i) abstraction.
type MemoryEntry struct {
	ID        string                 `json:"id"`
	Input     string                 `json:"input"`     // x_i: task/query
	Output    string                 `json:"output"`    // ŷ_i: model's answer
	Feedback  string                 `json:"feedback"`  // f_i: success/failure signal (legacy)
	Summary   string                 `json:"summary"`   // distilled lesson
	RawTrace  string                 `json:"raw_trace"` // optional detailed reasoning
	Embedding []float32              `json:"embedding"` // for retrieval
	Metadata  map[string]interface{} `json:"metadata"`  // timestamp, domain, task_id, etc.
	CreatedAt time.Time              `json:"created_at"`

	// Enhanced fields from paper review
	StructuredFeedback *StructuredFeedback `json:"structured_feedback,omitempty"` // Detailed feedback
	MemoryType         MemoryType          `json:"memory_type"`                   // Factual vs procedural
	StrategyCard       string              `json:"strategy_card,omitempty"`       // Reusable strategy pattern
	Scope              MemoryScope         `json:"scope,omitempty"`               // session/user/tenant promotion scope
	ExpiresAt          *time.Time          `json:"expires_at,omitempty"`          // optional retention expiry
	AccessCount        int                 `json:"access_count"`                  // For relevance-based pruning
	LastAccessedAt     time.Time           `json:"last_accessed_at"`              // For recency-based pruning
	RelevanceScore     float64             `json:"relevance_score"`               // Cumulative relevance metric
}

// ScoredMemoryEntry represents a memory entry paired with its similarity score
// to a particular query. Higher scores indicate closer matches.
type ScoredMemoryEntry struct {
	Entry *MemoryEntry `json:"entry"`
	Score float64      `json:"score"`
}

// MemoryScoreExplanation exposes the ranking components used by debug APIs.
type MemoryScoreExplanation struct {
	Entry         *MemoryEntry `json:"entry"`
	Similarity    float64      `json:"similarity"`
	Decay         float64      `json:"decay"`
	QualityWeight float64      `json:"qualityWeight"`
	AccessBoost   float64      `json:"accessBoost"`
	Composite     float64      `json:"composite"`
	MMRPenalty    float64      `json:"mmrPenalty"`
	FinalScore    float64      `json:"finalScore"`
}

// RankingWeights controls how dense similarity is blended with memory quality,
// recency, and usage signals during retrieval.
type RankingWeights struct {
	DecayHalfLifeDays float64
	SuccessWeight     float64
	FailureWeight     float64
	PartialWeight     float64
	AccessCountWeight float64
}

type embeddingCacheEntry struct {
	vec       []float32
	expiresAt time.Time
	lastUsed  time.Time
}

// EvolvingMemoryStore defines a persistence backend for evolving memory.
// Implementations should be safe for concurrent use.
type EvolvingMemoryStore interface {
	Load(ctx context.Context, userID int64, sessionID string) ([]*MemoryEntry, error)
	Save(ctx context.Context, userID int64, sessionID string, entries []*MemoryEntry) error
}

// EvolvingMemoryDeltaStore is an optional extension for stores that can persist
// only changed rows instead of rewriting the entire session snapshot.
type EvolvingMemoryDeltaStore interface {
	Upsert(ctx context.Context, userID int64, sessionID string, entries []*MemoryEntry) error
	Delete(ctx context.Context, userID int64, sessionID string, ids []string) error
	TouchAccess(ctx context.Context, ids []string, at time.Time) error
}

// EvolvingMemorySearchStore is an optional extension for stores that can run
// vector retrieval close to the data instead of requiring an in-process scan.
type EvolvingMemorySearchStore interface {
	SearchTopK(ctx context.Context, userID int64, sessionID string, queryVec []float32, k int) ([]ScoredMemoryEntry, error)
}

// EvolvingMemoryKeywordStore is an optional extension for lexical retrieval.
// It complements dense embeddings for exact identifiers, paths, and error codes.
type EvolvingMemoryKeywordStore interface {
	KeywordSearch(ctx context.Context, userID int64, sessionID string, query string, k int) ([]ScoredMemoryEntry, error)
}

// EvolvingMemoryPromotionStore is an optional extension for stores that can
// promote high-quality procedural memories beyond one session.
type EvolvingMemoryPromotionStore interface {
	PromoteToUserScope(ctx context.Context, userID int64, entryID string) error
}

// EvolvingMemoryJanitorStore is an optional extension for stores that can purge
// expired memories without loading the full session.
type EvolvingMemoryJanitorStore interface {
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

// EvolvingMemory implements the Search → Synthesis → Evolve loop from the paper.
// It provides:
// - R: retrieval function (top-k similarity search)
// - C: context constructor (builds prompts from retrieved memories)
// - U: memory update function (append, prune, merge)
type EvolvingMemory struct {
	mu        sync.RWMutex
	entries   []*MemoryEntry
	embedCfg  config.EmbeddingConfig
	embedFn   EmbedFunc
	llm       llm.Provider
	model     string
	maxSize   int // max number of entries to keep
	topK      int // number of similar entries to retrieve (default 4)
	windowSz  int // for ExpRecent sliding window (default 20)
	enableRAG bool

	// Similarity-based pruning configuration (from paper analysis)
	pruneThreshold           float64 // similarity threshold for auto-pruning duplicates
	relevanceDecay           float64 // decay factor for relevance scores over time
	minRelevance             float64 // minimum relevance to keep entry during pruning
	enableSmartPrune         bool    // enable similarity-based pruning
	rankingWeights           RankingWeights
	mmrLambda                float64
	promotionAccessThreshold int
	pruneQualityFloor        int
	janitorInterval          time.Duration
	metrics                  *MemoryMetrics

	queryCacheMu  sync.Mutex
	queryCache    map[string]embeddingCacheEntry
	queryCacheTTL time.Duration
	queryCacheMax int

	// Optional persistent backing store; when set, entries are loaded on
	// construction and persisted after each mutation.
	store          EvolvingMemoryStore
	userID         int64
	sessionID      string
	persistDelay   time.Duration
	persistVersion uint64
	pendingPersist []*MemoryEntry
	dirtyIDs       map[string]struct{}
	deletedIDs     map[string]struct{}

	callbacks *MemoryCallbacks
}

// Introspection helpers for debug APIs.

// TopK returns the configured top-k retrieval size.
func (em *EvolvingMemory) TopK() int { return em.topK }

// MaxSize returns the maximum number of entries kept in memory.
func (em *EvolvingMemory) MaxSize() int { return em.maxSize }

// WindowSize returns the sliding window size used by ExpRecent.
func (em *EvolvingMemory) WindowSize() int { return em.windowSz }

// EvolvingMemoryConfig configures the evolving memory system.
type EvolvingMemoryConfig struct {
	EmbeddingConfig config.EmbeddingConfig
	EmbedFn         EmbedFunc
	LLM             llm.Provider
	Model           string
	MaxSize         int  // 0 = unlimited
	TopK            int  // default 4
	WindowSize      int  // default 20 for ExpRecent
	EnableRAG       bool // enable ExpRAG retrieval

	// Similarity-based pruning configuration
	PruneThreshold           float64 // default 0.95 - entries above this similarity are candidates for merge
	RelevanceDecay           float64 // default 0.99 - daily decay factor for relevance scores
	MinRelevance             float64 // default 0.1 - entries below this relevance may be pruned
	EnableSmartPrune         bool    // default false - enable intelligent pruning
	RankingWeights           RankingWeights
	MMRLambda                float64 // default 0.7
	PromotionAccessThreshold int
	PruneQualityFloor        int
	JanitorInterval          time.Duration
	Metrics                  *MemoryMetrics

	QueryEmbeddingCacheTTL  time.Duration // default 5 minutes
	QueryEmbeddingCacheSize int           // default 128

	// Optional persistent store. When non-nil, NewEvolvingMemory will load
	// existing entries for the given userID and persist updates.
	Store           EvolvingMemoryStore
	UserID          int64
	SessionID       string
	PersistDebounce time.Duration

	Callbacks *MemoryCallbacks
}

// NewEvolvingMemory creates a new evolving memory system.
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
// Safe to call concurrently with Search/Evolve operations.
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
	start := time.Now()
	em.mu.RLock()
	entries := filterExpiredEntries(em.snapshotEntriesLocked(), time.Now())
	cb := em.callbacks
	em.mu.RUnlock()

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
			})
		}
		return nil, nil
	}

	log := observability.LoggerWithTrace(ctx)

	queryVec, err := em.embedQuery(ctx, query)
	if err != nil {
		log.Error().Err(err).Msg("evolving_memory_embed_query_failed")
		if cb != nil && cb.OnSearch != nil {
			cb.OnSearch(&MemoryEvent{
				Phase:      PhaseSearch,
				Timestamp:  start,
				Input:      query,
				Error:      err,
				MemorySize: len(entries),
				DurationMs: time.Since(start).Milliseconds(),
			})
		}
		return nil, fmt.Errorf("embed query: %w", err)
	}

	denseCandidates := make([]ScoredMemoryEntry, 0, len(entries))
	fetchK := em.searchFetchK()
	if searchStore, ok := em.store.(EvolvingMemorySearchStore); ok {
		storeCandidates, err := searchStore.SearchTopK(ctx, em.userID, em.sessionID, queryVec, fetchK)
		if err != nil {
			log.Warn().Err(err).Msg("evolving_memory_store_search_failed")
		} else {
			denseCandidates = storeCandidates
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

	keywordCandidates := em.keywordCandidates(ctx, query, entries, fetchK, log)
	candidates := denseCandidates
	if len(keywordCandidates) > 0 {
		candidates = rrfFuse([][]ScoredMemoryEntry{denseCandidates, keywordCandidates}, fetchK, 60)
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

	retrievedIDs := make([]string, k)
	for i := 0; i < k; i++ {
		retrievedIDs[i] = out[i].Entry.ID
	}

	// Update access metrics for retrieved entries (async to not block search)
	go em.updateAccessMetrics(retrievedIDs)

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
		})
	}
	if em.metrics != nil {
		em.metrics.RecordSearch(ctx, time.Since(start), len(out), len(entries), em.userID, em.sessionID)
	}

	log.Debug().Int("candidates", len(entries)).Int("top_k", k).Msg("evolving_memory_search")
	return out, nil
}

// ExplainSearch returns the same high-level retrieval candidates as SearchWithScores,
// plus the ranking components used to score and diversify them. It does not update
// access counters.
func (em *EvolvingMemory) ExplainSearch(ctx context.Context, query string) ([]MemoryScoreExplanation, error) {
	em.mu.RLock()
	entries := filterExpiredEntries(em.snapshotEntriesLocked(), time.Now())
	em.mu.RUnlock()

	queryVec, err := em.embedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	fetchK := em.searchFetchK()
	candidates := make([]ScoredMemoryEntry, 0, len(entries))
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
// Returns a formatted string suitable for injection into the system prompt or context.
func (em *EvolvingMemory) Synthesize(ctx context.Context, currentTask string, retrieved []*MemoryEntry) string {
	start := time.Now()
	em.mu.RLock()
	cb := em.callbacks
	em.mu.RUnlock()

	if len(retrieved) == 0 {
		if cb != nil && cb.OnSynthesized != nil {
			cb.OnSynthesized(&MemoryEvent{
				Phase:      PhaseSynthesis,
				Timestamp:  start,
				Input:      currentTask,
				OutputSize: 0,
				DurationMs: time.Since(start).Milliseconds(),
			})
		}
		return ""
	}

	var result string
	result += "## Past Relevant Experiences\n\n"

	successes, cautions := partitionRetrievedByOutcome(retrieved)
	if len(successes) > 0 {
		result += "## Strategies That Worked\n\n"
		for i, entry := range successes {
			result += fmt.Sprintf("### Experience %d\n", i+1)
			result += formatExperience(entry) + "\n\n"
		}
	}
	if len(cautions) > 0 {
		result += "## Mistakes to Avoid\n\n"
		for i, entry := range cautions {
			result += fmt.Sprintf("### Experience %d\n", i+1)
			result += formatExperience(entry) + "\n\n"
		}
	}

	if cb != nil && cb.OnSynthesized != nil {
		retrievedIDs := make([]string, 0, len(retrieved))
		for _, r := range retrieved {
			if r != nil {
				retrievedIDs = append(retrievedIDs, r.ID)
			}
		}
		cb.OnSynthesized(&MemoryEvent{
			Phase:        PhaseSynthesis,
			Timestamp:    start,
			Input:        currentTask,
			RetrievedIDs: retrievedIDs,
			OutputSize:   len(result),
			DurationMs:   time.Since(start).Milliseconds(),
		})
	}

	return result
}

// formatExperience converts a memory entry into a structured textual block (template S from paper).
func formatExperience(entry *MemoryEntry) string {
	var s string
	s += fmt.Sprintf("**Task:** %s\n", truncate(entry.Input, 200))
	s += fmt.Sprintf("**Outcome:** %s\n", entry.Feedback)
	if entry.MemoryType != "" {
		s += fmt.Sprintf("**Type:** %s\n", entry.MemoryType)
	}
	if entry.Summary != "" {
		s += fmt.Sprintf("**Key Lesson:** %s\n", entry.Summary)
	}
	if entry.StrategyCard != "" {
		s += fmt.Sprintf("**Strategy:** %s\n", entry.StrategyCard)
	}
	if entry.Output != "" {
		s += fmt.Sprintf("**Solution:** %s\n", truncate(entry.Output, 150))
	}
	return s
}

// EvolveEnhanced is the full-featured Evolve that accepts structured feedback,
// reasoning trace, and strategy card. This implements the paper's complete
// experience storage with distinction between factual and procedural memory.
func (em *EvolvingMemory) EvolveEnhanced(
	ctx context.Context,
	input, output, feedback string,
	structuredFB *StructuredFeedback,
	reasoningTrace []string,
	strategyCard string,
) error {
	start := time.Now()
	log := observability.LoggerWithTrace(ctx)
	em.mu.RLock()
	cb := em.callbacks
	memorySize := len(em.entries)
	em.mu.RUnlock()

	// Generate summary via LLM
	input = limitUTF8Bytes(redactPII(input), maxStoredInputBytes)
	output = limitUTF8Bytes(redactPII(output), maxStoredOutputBytes)
	summary, err := em.generateSummary(ctx, input, output, feedback)
	if err != nil {
		log.Warn().Err(err).Msg("evolving_memory_summarize_failed")
		summary = "(summary unavailable)"
	}

	// Embed the input for retrieval
	vecs, err := em.embedFn(ctx, em.embedCfg, []string{input})
	if err != nil {
		log.Error().Err(err).Msg("evolving_memory_embed_failed")
		if cb != nil && cb.OnEvolve != nil {
			cb.OnEvolve(&MemoryEvent{
				Phase:      PhaseEvolve,
				Timestamp:  start,
				Input:      input,
				Error:      err,
				MemorySize: memorySize,
				DurationMs: time.Since(start).Milliseconds(),
			})
		}
		if em.metrics != nil {
			em.metrics.RecordEvolve(ctx, "error", memorySize, em.userID, em.sessionID)
		}
		return fmt.Errorf("embed input: %w", err)
	}

	// Classify memory type based on content analysis
	memType := em.classifyMemoryType(input, output, summary)

	// Build raw trace from reasoning trace
	rawTrace := ""
	if len(reasoningTrace) > 0 {
		for i, t := range reasoningTrace {
			rawTrace += fmt.Sprintf("Step %d: %s\n", i+1, t)
		}
	}
	rawTrace = limitUTF8Bytes(redactPII(rawTrace), maxStoredRawTraceBytes)

	entry := &MemoryEntry{
		ID:                 uuid.New().String(),
		Input:              input,
		Output:             output,
		Feedback:           feedback,
		Summary:            summary,
		RawTrace:           rawTrace,
		Embedding:          normalizeVector(vecs[0]),
		MemoryType:         memType,
		StrategyCard:       strategyCard,
		Scope:              MemoryScopeSession,
		StructuredFeedback: structuredFB,
		AccessCount:        0,
		LastAccessedAt:     time.Now(),
		RelevanceScore:     1.0, // Start with full relevance
		Metadata: map[string]interface{}{
			"domain": "general",
		},
		CreatedAt: time.Now(),
	}

	var mergePlan *smartMergePlan
	if em.enableSmartPrune {
		em.mu.RLock()
		existingEntries := em.snapshotEntriesLocked()
		em.mu.RUnlock()

		mergePlan, err = em.prepareSmartMerge(ctx, existingEntries, entry)
		if err != nil {
			log.Warn().Err(err).Msg("evolving_memory_prepare_smart_merge_failed")
			mergePlan = nil
		}
	}

	em.mu.Lock()
	cb = em.callbacks

	// Smart pruning: check for near-duplicates before adding
	if mergePlan != nil {
		em.applySmartMergePlan(ctx, mergePlan, entry)
	}

	em.entries = append(em.entries, entry)
	em.markDirtyLocked(entry.ID)

	// Apply relevance-based pruning if enabled and over capacity
	if em.enableSmartPrune && len(em.entries) > em.maxSize {
		em.relevanceBasedPrune(ctx)
	} else if len(em.entries) > em.maxSize {
		// Fallback to FIFO pruning
		removed := em.entries[:len(em.entries)-em.maxSize]
		removedIDs := make([]string, 0, len(removed))
		for _, removedEntry := range removed {
			if removedEntry != nil {
				removedIDs = append(removedIDs, removedEntry.ID)
			}
		}
		em.entries = em.entries[len(em.entries)-em.maxSize:]
		em.markDeletedLocked(removedIDs)
		log.Info().Int("pruned_to", em.maxSize).Msg("evolving_memory_fifo_pruned")
	}

	memorySize = len(em.entries)
	entriesSnapshot := em.snapshotEntriesLocked()
	em.mu.Unlock()

	if cb != nil && cb.OnEvolve != nil {
		cb.OnEvolve(&MemoryEvent{
			Phase:      PhaseEvolve,
			Timestamp:  start,
			Input:      input,
			OutputSize: len(output),
			MemorySize: memorySize,
			DurationMs: time.Since(start).Milliseconds(),
		})
	}
	if em.metrics != nil {
		em.metrics.RecordEvolve(ctx, "success", memorySize, em.userID, em.sessionID)
	}

	// Persist in the background if a store is configured.
	// Note: systemUserID is 0 in agentd; we still want persistence for it.
	if em.store != nil {
		em.persistEntriesAsync(entriesSnapshot)
	}

	log.Info().
		Str("entry_id", entry.ID).
		Str("memory_type", string(memType)).
		Bool("has_strategy_card", strategyCard != "").
		Msg("evolving_memory_entry_added")
	return nil
}

// classifyMemoryType determines if the memory is factual, procedural, or episodic.
// This implements the paper's distinction between conversational recall and experience reuse.
func (em *EvolvingMemory) classifyMemoryType(input, output, summary string) MemoryType {
	// Simple heuristic-based classification
	// In production, this could use an LLM call for more accurate classification

	combined := input + " " + output + " " + summary
	if proceduralMemoryPattern.MatchString(combined) {
		return MemoryProcedural
	}

	if factualMemoryPattern.MatchString(combined) {
		return MemoryFactual
	}

	// Default to episodic (specific task instance)
	return MemoryEpisodic
}

type smartMergePlan struct {
	mergedIDs       []string
	mergedSummary   string
	mergedEmbedding []float32
}

// prepareSmartMerge plans any smart-merge operation using a snapshot of the
// existing entries so expensive embedding work stays outside the write lock.
func (em *EvolvingMemory) prepareSmartMerge(ctx context.Context, existingEntries []*MemoryEntry, newEntry *MemoryEntry) (*smartMergePlan, error) {
	log := observability.LoggerWithTrace(ctx)

	if len(newEntry.Embedding) == 0 {
		return nil, nil
	}

	var toMerge []string
	mergedSummaries := make([]string, 0, len(existingEntries)+1)
	for _, existing := range existingEntries {
		if len(existing.Embedding) == 0 {
			continue
		}
		sim := dotProduct(newEntry.Embedding, existing.Embedding)
		if sim >= em.pruneThreshold {
			toMerge = append(toMerge, existing.ID)
			if existing.Summary != "" {
				mergedSummaries = append(mergedSummaries, existing.Summary)
			}
			log.Debug().
				Str("existing_id", existing.ID).
				Float64("similarity", sim).
				Msg("evolving_memory_found_duplicate")
		}
	}

	if len(toMerge) == 0 {
		return nil, nil
	}
	if newEntry.Summary != "" {
		mergedSummaries = append(mergedSummaries, newEntry.Summary)
	}

	plan := &smartMergePlan{mergedIDs: toMerge}
	mergedSummary := mergeSummaryText(mergedSummaries)
	if mergedSummary == "" {
		return plan, nil
	}

	plan.mergedSummary = mergedSummary
	if mergedSummary == newEntry.Summary {
		return plan, nil
	}

	vecs, err := em.embedFn(ctx, em.embedCfg, []string{mergedSummary})
	if err != nil {
		return nil, fmt.Errorf("embed merged summary: %w", err)
	}
	if len(vecs) > 0 {
		plan.mergedEmbedding = vecs[0]
	}

	return plan, nil
}

func (em *EvolvingMemory) applySmartMergePlan(ctx context.Context, plan *smartMergePlan, newEntry *MemoryEntry) {
	if plan == nil {
		return
	}
	if newEntry.Metadata == nil {
		newEntry.Metadata = make(map[string]interface{})
	}
	if len(plan.mergedIDs) > 0 {
		newEntry.Metadata["merged_from"] = append([]string(nil), plan.mergedIDs...)
		newEntry.Metadata["merge_count"] = len(plan.mergedIDs) + 1
	}
	if plan.mergedSummary != "" {
		newEntry.Summary = plan.mergedSummary
	}
	if len(plan.mergedEmbedding) > 0 {
		newEntry.Embedding = normalizeVector(plan.mergedEmbedding)
	}

	em.pruneEntries(plan.mergedIDs)
	observability.LoggerWithTrace(ctx).Info().
		Int("merged_count", len(plan.mergedIDs)).
		Msg("evolving_memory_smart_merged")
	if em.metrics != nil {
		em.metrics.RecordSmartMerge(ctx, len(plan.mergedIDs))
	}
}

// relevanceBasedPrune removes entries based on relevance scores.
// Uses a combination of access frequency, recency, and base relevance.
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

// generateSummary asks the LLM to distill a key lesson from the experience.
func (em *EvolvingMemory) generateSummary(ctx context.Context, input, output, feedback string) (string, error) {
	if em.llm == nil {
		return "", fmt.Errorf("no LLM provider configured")
	}

	sys := `You are a concise experience summarizer. Extract a reusable memory from this task experience.

Return only a short summary under 100 words. Preserve:
- task pattern
- outcome
- reusable lesson or strategy
- mistake or risk to avoid
- when the lesson should not be applied

Do not include secrets, credentials, private user data, or transient one-off details.`
	user := fmt.Sprintf("Task: %s\nOutcome: %s\nSolution: %s\n\nWrite the reusable memory summary.",
		truncate(input, 300), feedback, truncate(output, 200))

	msgs := []llm.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: user},
	}

	resp, err := em.llm.Chat(ctx, msgs, nil, em.model)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// GetRecentWindow returns the most recent N entries for ExpRecent.
func (em *EvolvingMemory) GetRecentWindow() []*MemoryEntry {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if len(em.entries) == 0 {
		return nil
	}
	start := 0
	if len(em.entries) > em.windowSz {
		start = len(em.entries) - em.windowSz
	}
	return cloneEntrySlice(em.entries[start:])
}

// BuildExpRecentContext constructs a compressed summary of recent episodes.
func (em *EvolvingMemory) BuildExpRecentContext() string {
	recent := em.GetRecentWindow()
	if len(recent) == 0 {
		return ""
	}

	var result string
	result += "## Recent Task History\n\n"
	for i, entry := range recent {
		result += fmt.Sprintf("%d. Task: %s | Outcome: %s\n",
			i+1, truncate(entry.Input, 80), entry.Feedback)
	}
	return result + "\n"
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func normalizeVector(v []float32) []float32 {
	if len(v) == 0 {
		return nil
	}
	var mag float64
	for _, x := range v {
		mag += float64(x) * float64(x)
	}
	if mag == 0 {
		return append([]float32(nil), v...)
	}
	scale := 1 / math.Sqrt(mag)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) * scale)
	}
	return out
}

func dotProduct(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

func applyMMR(candidates []ScoredMemoryEntry, k int, lambda float64) []ScoredMemoryEntry {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}
	if k > len(candidates) {
		k = len(candidates)
	}
	if lambda <= 0 || lambda > 1 {
		lambda = 0.7
	}

	selected := make([]ScoredMemoryEntry, 0, k)
	used := make([]bool, len(candidates))
	for len(selected) < k {
		bestIdx := -1
		bestScore := math.Inf(-1)
		for i, candidate := range candidates {
			if used[i] || candidate.Entry == nil {
				continue
			}
			maxSimilarity := 0.0
			for _, chosen := range selected {
				if chosen.Entry == nil {
					continue
				}
				if sim := dotProduct(candidate.Entry.Embedding, chosen.Entry.Embedding); sim > maxSimilarity {
					maxSimilarity = sim
				}
			}
			mmrScore := lambda*candidate.Score - (1-lambda)*maxSimilarity
			if bestIdx == -1 || mmrScore > bestScore {
				bestIdx = i
				bestScore = mmrScore
			}
		}
		if bestIdx == -1 {
			break
		}
		used[bestIdx] = true
		selected = append(selected, candidates[bestIdx])
	}
	return selected
}

func inMemoryKeywordSearch(entries []*MemoryEntry, query string, k int) []ScoredMemoryEntry {
	terms := keywordTerms(query)
	if len(terms) == 0 || len(entries) == 0 || k <= 0 {
		return nil
	}
	scores := make([]ScoredMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		textTerms := keywordTerms(strings.Join([]string{entry.Input, entry.Summary, entry.StrategyCard}, " "))
		if len(textTerms) == 0 {
			continue
		}
		matches := 0
		for term := range terms {
			if _, ok := textTerms[term]; ok {
				matches++
			}
		}
		if matches == 0 {
			continue
		}
		scores = append(scores, ScoredMemoryEntry{Entry: entry, Score: float64(matches) / float64(len(terms))})
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
	if k > len(scores) {
		k = len(scores)
	}
	return scores[:k]
}

func keywordTerms(text string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == '/')
	})
	terms := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, "._-/")
		if len(field) < 2 {
			continue
		}
		terms[field] = struct{}{}
	}
	return terms
}

func rrfFuse(rankings [][]ScoredMemoryEntry, k int, constant int) []ScoredMemoryEntry {
	if k <= 0 || len(rankings) == 0 {
		return nil
	}
	if constant <= 0 {
		constant = 60
	}
	type fusedCandidate struct {
		entry *MemoryEntry
		score float64
	}
	fused := make(map[string]fusedCandidate)
	for _, ranking := range rankings {
		for rank, candidate := range ranking {
			if candidate.Entry == nil || candidate.Entry.ID == "" {
				continue
			}
			key := candidate.Entry.ID
			current := fused[key]
			if current.entry == nil {
				current.entry = candidate.Entry
			}
			current.score += 1 / float64(constant+rank+1)
			fused[key] = current
		}
	}
	if len(fused) == 0 {
		return nil
	}
	out := make([]ScoredMemoryEntry, 0, len(fused))
	maxScore := 0.0
	for _, candidate := range fused {
		if candidate.score > maxScore {
			maxScore = candidate.score
		}
		out = append(out, ScoredMemoryEntry{Entry: candidate.entry, Score: candidate.score})
	}
	if maxScore > 0 {
		for i := range out {
			out[i].Score /= maxScore
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if k > len(out) {
		k = len(out)
	}
	return out[:k]
}

func partitionRetrievedByOutcome(retrieved []*MemoryEntry) ([]*MemoryEntry, []*MemoryEntry) {
	successes := make([]*MemoryEntry, 0, len(retrieved))
	cautions := make([]*MemoryEntry, 0, len(retrieved))
	for _, entry := range retrieved {
		if entry == nil {
			continue
		}
		if entry.StructuredFeedback != nil && entry.StructuredFeedback.Type == FeedbackFailure {
			cautions = append(cautions, entry)
			continue
		}
		if entry.StructuredFeedback != nil && (entry.StructuredFeedback.Type == FeedbackPartial || entry.StructuredFeedback.Type == FeedbackInProgress) {
			cautions = append(cautions, entry)
			continue
		}
		if strings.EqualFold(entry.Feedback, string(FeedbackFailure)) || strings.EqualFold(entry.Feedback, string(FeedbackPartial)) {
			cautions = append(cautions, entry)
			continue
		}
		successes = append(successes, entry)
	}
	return successes, cautions
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func limitUTF8Bytes(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	truncated := s[:maxBytes]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

func redactPII(s string) string {
	if s == "" {
		return s
	}
	replacers := []struct {
		pattern     *regexp.Regexp
		replacement string
	}{
		{emailPattern, "[REDACTED_EMAIL]"},
		{awsAccessKeyPattern, "[REDACTED_AWS_KEY]"},
		{jwtPattern, "[REDACTED_JWT]"},
		{apiKeyPattern, "[REDACTED_API_KEY]"},
	}
	out := s
	for _, replacer := range replacers {
		out = replacer.pattern.ReplaceAllString(out, replacer.replacement)
	}
	return out
}

// MemoryEditOp represents a memory editing operation for ReMem's REFINE phase.
type MemoryEditOp struct {
	Type       string   `json:"type"`        // PRUNE, MERGE, UPDATE_TAG
	IDs        []string `json:"ids"`         // entry IDs to operate on
	NewSummary string   `json:"new_summary"` // for MERGE
	Tag        string   `json:"tag"`         // for UPDATE_TAG
	Reason     string   `json:"reason"`      // short rationale for audit/debugging
}

// ApplyEdits applies memory editing operations (for ReMem REFINE phase).
func (em *EvolvingMemory) ApplyEdits(ctx context.Context, ops []MemoryEditOp) error {
	log := observability.LoggerWithTrace(ctx)
	changed := false

	for _, op := range ops {
		switch op.Type {
		case "PRUNE":
			em.mu.Lock()
			em.pruneEntries(op.IDs)
			em.mu.Unlock()
			changed = true
			log.Info().Strs("ids", op.IDs).Msg("evolving_memory_pruned_entries")

		case "MERGE":
			em.mu.Lock()
			if err := em.mergeEntries(ctx, op.IDs, op.NewSummary); err != nil {
				em.mu.Unlock()
				log.Error().Err(err).Msg("evolving_memory_merge_failed")
				return err
			}
			em.mu.Unlock()
			changed = true
			log.Info().Strs("ids", op.IDs).Msg("evolving_memory_merged_entries")

		case "UPDATE_TAG":
			em.mu.Lock()
			em.updateTag(op.IDs, op.Tag)
			em.mu.Unlock()
			changed = true
			log.Info().Strs("ids", op.IDs).Str("tag", op.Tag).Msg("evolving_memory_updated_tag")

		default:
			log.Warn().Str("type", op.Type).Msg("evolving_memory_unknown_edit_op")
		}
	}

	// Persist after applying edits if backed by a store.
	// Note: systemUserID is 0 in agentd; we still want persistence for it.
	if changed && em.store != nil {
		em.mu.RLock()
		entriesCopy := em.snapshotEntriesLocked()
		em.mu.RUnlock()
		em.persistEntriesAsync(entriesCopy)
	}

	return nil
}

// pruneEntries removes entries by ID.
func (em *EvolvingMemory) pruneEntries(ids []string) {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	filtered := make([]*MemoryEntry, 0, len(em.entries))
	for _, e := range em.entries {
		if !idSet[e.ID] {
			filtered = append(filtered, e)
		}
	}
	em.entries = filtered
	em.markDeletedLocked(ids)
}

// mergeEntries combines multiple entries into one with a new summary.
func (em *EvolvingMemory) mergeEntries(ctx context.Context, ids []string, newSummary string) error {
	if len(ids) == 0 {
		return nil
	}

	// Find entries to merge
	var toMerge []*MemoryEntry
	for _, e := range em.entries {
		for _, id := range ids {
			if e.ID == id {
				toMerge = append(toMerge, e)
				break
			}
		}
	}

	if len(toMerge) == 0 {
		return fmt.Errorf("no entries found to merge")
	}

	representative := selectRepresentativeEntry(toMerge)
	structuredFeedback := bestStructuredFeedback(toMerge)
	feedback := "merged"
	if structuredFeedback != nil && structuredFeedback.Type != "" {
		feedback = string(structuredFeedback.Type)
	}

	merged := &MemoryEntry{
		ID:                 uuid.New().String(),
		Input:              representative.Input,
		Output:             representative.Output,
		Feedback:           feedback,
		Summary:            newSummary,
		RawTrace:           longestRawTrace(toMerge),
		MemoryType:         mergedMemoryType(toMerge),
		StrategyCard:       mergeStrategyCards(toMerge),
		Scope:              mergedMemoryScope(toMerge),
		StructuredFeedback: structuredFeedback,
		AccessCount:        mergedAccessCount(toMerge),
		LastAccessedAt:     latestAccessedAt(toMerge),
		RelevanceScore:     bestRelevanceScore(toMerge),
		Metadata: map[string]interface{}{
			"merged_from": ids,
		},
		CreatedAt: time.Now(),
	}

	// Re-embed the merged summary
	vecs, err := em.embedFn(ctx, em.embedCfg, []string{newSummary})
	if err != nil {
		return fmt.Errorf("embed merged entry: %w", err)
	}
	merged.Embedding = normalizeVector(vecs[0])

	// Remove old entries and add merged
	em.pruneEntries(ids)
	em.entries = append(em.entries, merged)
	em.markDirtyLocked(merged.ID)

	return nil
}

func mergedMemoryScope(entries []*MemoryEntry) MemoryScope {
	for _, entry := range entries {
		if entry != nil && entry.Scope == MemoryScopeUser {
			return MemoryScopeUser
		}
	}
	return MemoryScopeSession
}

func selectRepresentativeEntry(entries []*MemoryEntry) *MemoryEntry {
	if len(entries) == 0 {
		return &MemoryEntry{}
	}
	representative := entries[0]
	bestRank := feedbackRank(representative.StructuredFeedback)
	for _, entry := range entries[1:] {
		rank := feedbackRank(entry.StructuredFeedback)
		if rank > bestRank || rank == bestRank && len(entry.Output) > len(representative.Output) {
			representative = entry
			bestRank = rank
		}
	}
	return representative
}

func bestStructuredFeedback(entries []*MemoryEntry) *StructuredFeedback {
	var best *StructuredFeedback
	bestRank := -1
	for _, entry := range entries {
		if entry == nil || entry.StructuredFeedback == nil {
			continue
		}
		rank := feedbackRank(entry.StructuredFeedback)
		if rank > bestRank {
			copyFeedback := *entry.StructuredFeedback
			best = &copyFeedback
			bestRank = rank
		}
	}
	return best
}

func feedbackRank(feedback *StructuredFeedback) int {
	if feedback == nil {
		return 0
	}
	switch feedback.Type {
	case FeedbackSuccess:
		return 4
	case FeedbackPartial, FeedbackInProgress:
		return 3
	case FeedbackFailure:
		return 1
	default:
		return 2
	}
}

func longestRawTrace(entries []*MemoryEntry) string {
	var longest string
	for _, entry := range entries {
		if entry != nil && len(entry.RawTrace) > len(longest) {
			longest = entry.RawTrace
		}
	}
	return longest
}

func mergeStrategyCards(entries []*MemoryEntry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil && entry.StrategyCard != "" {
			parts = append(parts, entry.StrategyCard)
		}
	}
	return mergeSummaryText(parts)
}

func mergedMemoryType(entries []*MemoryEntry) MemoryType {
	foundFactual := false
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		switch entry.MemoryType {
		case MemoryProcedural:
			return MemoryProcedural
		case MemoryFactual:
			foundFactual = true
		}
	}
	if foundFactual {
		return MemoryFactual
	}
	return MemoryEpisodic
}

func mergedAccessCount(entries []*MemoryEntry) int {
	total := 0
	for _, entry := range entries {
		if entry != nil {
			total += entry.AccessCount
		}
	}
	return total
}

func latestAccessedAt(entries []*MemoryEntry) time.Time {
	var latest time.Time
	for _, entry := range entries {
		if entry != nil && entry.LastAccessedAt.After(latest) {
			latest = entry.LastAccessedAt
		}
	}
	return latest
}

func bestRelevanceScore(entries []*MemoryEntry) float64 {
	best := 0.0
	for _, entry := range entries {
		if entry != nil && entry.RelevanceScore > best {
			best = entry.RelevanceScore
		}
	}
	return best
}

// updateTag modifies metadata tags on entries.
func (em *EvolvingMemory) updateTag(ids []string, tag string) {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	for _, e := range em.entries {
		if idSet[e.ID] {
			if e.Metadata == nil {
				e.Metadata = make(map[string]interface{})
			}
			e.Metadata["tag"] = tag
			em.markDirtyLocked(e.ID)
		}
	}
}

// ExportMemories returns all memory entries for inspection endpoints.
func (em *EvolvingMemory) ExportMemories() []*MemoryEntry {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.snapshotEntriesLocked()
}

func (em *EvolvingMemory) shouldPromoteToUserScopeLocked(entry *MemoryEntry) bool {
	if entry == nil || entry.Scope == MemoryScopeUser || em.promotionAccessThreshold <= 0 {
		return false
	}
	if entry.AccessCount < em.promotionAccessThreshold || entry.MemoryType != MemoryProcedural {
		return false
	}
	return entry.StructuredFeedback != nil && entry.StructuredFeedback.Correct && entry.StructuredFeedback.Type == FeedbackSuccess
}

func (em *EvolvingMemory) promoteToUserScopeAsync(store EvolvingMemoryPromotionStore, entryID string) {
	if store == nil || strings.TrimSpace(entryID) == "" {
		return
	}
	go func() {
		bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.PromoteToUserScope(bgctx, em.userID, entryID); err != nil {
			observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_promote_failed")
		}
	}()
}

func (em *EvolvingMemory) startStoreJanitor(interval time.Duration) {
	store, ok := em.store.(EvolvingMemoryJanitorStore)
	if !ok || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			bgctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			removed, err := store.DeleteExpired(bgctx, time.Now().UTC())
			cancel()
			if err != nil {
				observability.LoggerWithTrace(context.Background()).Error().Err(err).Msg("evolving_memory_expired_delete_failed")
				continue
			}
			if removed > 0 && em.metrics != nil {
				em.metrics.RecordPruned(context.Background(), "expired", int(removed))
			}
		}
	}()
}

func filterExpiredEntries(entries []*MemoryEntry, now time.Time) []*MemoryEntry {
	if len(entries) == 0 {
		return entries
	}
	out := entries[:0]
	for _, entry := range entries {
		if entry == nil || (entry.ExpiresAt != nil && !entry.ExpiresAt.After(now)) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (em *EvolvingMemory) snapshotEntriesLocked() []*MemoryEntry {
	return cloneEntrySlice(em.entries)
}

func (em *EvolvingMemory) persistEntriesAsync(entries []*MemoryEntry) {
	if em.store == nil {
		return
	}
	if _, ok := em.store.(EvolvingMemoryDeltaStore); ok {
		em.persistDirtyEntriesAsync()
		return
	}

	em.mu.Lock()
	em.pendingPersist = entries
	em.persistVersion++
	version := em.persistVersion
	delay := em.persistDelay
	uid := em.userID
	sid := em.sessionID
	em.mu.Unlock()

	go func(targetVersion uint64) {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			<-timer.C
		}

		em.mu.Lock()
		if em.persistVersion != targetVersion {
			em.mu.Unlock()
			return
		}
		entries := em.pendingPersist
		em.pendingPersist = nil
		em.mu.Unlock()

		bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := em.store.Save(bgctx, uid, sid, entries); err != nil {
			observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_persist_failed")
		}
	}(version)
}

func (em *EvolvingMemory) persistDirtyEntriesAsync() {
	em.mu.Lock()
	if len(em.dirtyIDs) == 0 && len(em.deletedIDs) == 0 {
		em.mu.Unlock()
		return
	}
	em.persistVersion++
	version := em.persistVersion
	delay := em.persistDelay
	uid := em.userID
	sid := em.sessionID
	em.mu.Unlock()

	go func(targetVersion uint64) {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			<-timer.C
		}

		em.mu.Lock()
		if em.persistVersion != targetVersion {
			em.mu.Unlock()
			return
		}
		dirtyIDs := em.dirtyIDs
		deletedIDs := em.deletedIDs
		em.dirtyIDs = make(map[string]struct{})
		em.deletedIDs = make(map[string]struct{})
		entries := em.entriesForIDsLocked(dirtyIDs)
		idsToDelete := stringSetKeys(deletedIDs)
		em.mu.Unlock()

		deltaStore, ok := em.store.(EvolvingMemoryDeltaStore)
		if !ok {
			return
		}

		bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if len(entries) > 0 {
			if err := deltaStore.Upsert(bgctx, uid, sid, entries); err != nil {
				observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_delta_upsert_failed")
				return
			}
		}
		if len(idsToDelete) > 0 {
			if err := deltaStore.Delete(bgctx, uid, sid, idsToDelete); err != nil {
				observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_delta_delete_failed")
			}
		}
	}(version)
}

func (em *EvolvingMemory) touchAccessAsync(deltaStore EvolvingMemoryDeltaStore, ids []string, at time.Time) {
	if len(ids) == 0 {
		return
	}
	idsCopy := append([]string(nil), ids...)
	go func() {
		bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := deltaStore.TouchAccess(bgctx, idsCopy, at); err != nil {
			observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_touch_access_failed")
		}
	}()
}

func (em *EvolvingMemory) markDirtyLocked(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if em.dirtyIDs == nil {
		em.dirtyIDs = make(map[string]struct{})
	}
	em.dirtyIDs[id] = struct{}{}
	delete(em.deletedIDs, id)
}

func (em *EvolvingMemory) markDeletedLocked(ids []string) {
	if len(ids) == 0 {
		return
	}
	if em.deletedIDs == nil {
		em.deletedIDs = make(map[string]struct{})
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		delete(em.dirtyIDs, id)
		em.deletedIDs[id] = struct{}{}
	}
}

func (em *EvolvingMemory) entriesForIDsLocked(ids map[string]struct{}) []*MemoryEntry {
	if len(ids) == 0 {
		return nil
	}
	entries := make([]*MemoryEntry, 0, len(ids))
	for _, entry := range em.entries {
		if entry == nil {
			continue
		}
		if _, ok := ids[entry.ID]; ok {
			entries = append(entries, cloneEntry(entry))
		}
	}
	return entries
}

func stringSetKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergeSummaryText(parts []string) string {
	if len(parts) == 0 {
		return ""
	}

	seen := make(map[string]struct{}, len(parts))
	merged := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		merged = append(merged, part)
	}

	return strings.Join(merged, "\n\n")
}

func cloneEntrySlice(entries []*MemoryEntry) []*MemoryEntry {
	if len(entries) == 0 {
		return nil
	}

	cloned := make([]*MemoryEntry, 0, len(entries))
	for _, entry := range entries {
		cloned = append(cloned, cloneEntry(entry))
	}

	return cloned
}

func cloneEntry(entry *MemoryEntry) *MemoryEntry {
	if entry == nil {
		return nil
	}

	copyEntry := *entry
	if entry.Embedding != nil {
		copyEntry.Embedding = append([]float32(nil), entry.Embedding...)
	}
	if entry.Metadata != nil {
		copyEntry.Metadata = make(map[string]interface{}, len(entry.Metadata))
		for key, value := range entry.Metadata {
			copyEntry.Metadata[key] = value
		}
	}
	if entry.StructuredFeedback != nil {
		feedbackCopy := *entry.StructuredFeedback
		copyEntry.StructuredFeedback = &feedbackCopy
	}
	if entry.ExpiresAt != nil {
		expiresCopy := *entry.ExpiresAt
		copyEntry.ExpiresAt = &expiresCopy
	}
	if copyEntry.Scope == "" {
		copyEntry.Scope = MemoryScopeSession
	}

	return &copyEntry
}
