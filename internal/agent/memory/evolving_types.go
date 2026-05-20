package memory

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"manifold/internal/config"
	"manifold/internal/llm"
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
	Search        SearchDiagnostics
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
	ID        string         `json:"id"`
	Input     string         `json:"input"`     // x_i: task/query
	Output    string         `json:"output"`    // ŷ_i: model's answer
	Feedback  string         `json:"feedback"`  // f_i: success/failure signal (legacy)
	Summary   string         `json:"summary"`   // distilled lesson
	RawTrace  string         `json:"raw_trace"` // optional detailed reasoning
	Embedding []float32      `json:"embedding"` // for retrieval
	Metadata  map[string]any `json:"metadata"`  // timestamp, domain, task_id, etc.
	CreatedAt time.Time      `json:"created_at"`

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

// SearchDiagnostics describes how a memory search was executed. It is kept
// small so it can be returned by debug APIs and attached to MemoryEvents.
type SearchDiagnostics struct {
	EnableRAG                   bool   `json:"enableRAG"`
	Mode                        string `json:"mode"`
	VectorCandidates            int    `json:"vectorCandidates"`
	KeywordCandidates           int    `json:"keywordCandidates"`
	UsedServerVector            bool   `json:"usedServerVector"`
	UsedKeywordStore            bool   `json:"usedKeywordStore"`
	EmbeddingInstructionUsed    bool   `json:"embeddingInstructionUsed"`
	EmbeddingInstructionApplied bool   `json:"embeddingInstructionApplied"`
	EmbeddingInstructionUseCase string `json:"embeddingInstructionUseCase,omitempty"`
	EmbeddingInstructionFormat  string `json:"embeddingInstructionFormat,omitempty"`
	EmbeddingInstructionMode    string `json:"embeddingInstructionMode,omitempty"`
	EmbeddingInstructionSource  string `json:"embeddingInstructionSource,omitempty"`
	EmbeddingError              string `json:"embeddingError,omitempty"`
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

type entryIDContextKey struct{}

// WithEntryID reserves the ID EvolveEnhanced should use for the next memory entry.
func WithEntryID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(id) == "" {
		return ctx
	}
	return context.WithValue(ctx, entryIDContextKey{}, strings.TrimSpace(id))
}

func entryIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(entryIDContextKey{}).(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// Introspection helpers for debug APIs.

// TopK returns the configured top-k retrieval size.
func (em *EvolvingMemory) TopK() int { return em.topK }

// MaxSize returns the maximum number of entries kept in memory.
func (em *EvolvingMemory) MaxSize() int { return em.maxSize }

// WindowSize returns the sliding window size used by ExpRecent.
func (em *EvolvingMemory) WindowSize() int { return em.windowSz }

// RAGEnabled reports whether embedding/vector retrieval is enabled.
func (em *EvolvingMemory) RAGEnabled() bool { return em.enableRAG }

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
