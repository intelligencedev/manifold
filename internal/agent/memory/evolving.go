package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"manifold/internal/config"
	"manifold/internal/embedding"
	"manifold/internal/llm"
	"manifold/internal/observability"

	"github.com/google/uuid"
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

// EvolvingMemoryStore defines a persistence backend for evolving memory.
// Implementations should be safe for concurrent use.
type EvolvingMemoryStore interface {
	Load(ctx context.Context, userID int64, sessionID string) ([]*MemoryEntry, error)
	Save(ctx context.Context, userID int64, sessionID string, entries []*MemoryEntry) error
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
	pruneThreshold   float64 // similarity threshold for auto-pruning duplicates
	relevanceDecay   float64 // decay factor for relevance scores over time
	minRelevance     float64 // minimum relevance to keep entry during pruning
	enableSmartPrune bool    // enable similarity-based pruning

	// Optional persistent backing store; when set, entries are loaded on
	// construction and persisted after each mutation.
	store          EvolvingMemoryStore
	userID         int64
	sessionID      string
	persistDelay   time.Duration
	persistVersion uint64
	pendingPersist []*MemoryEntry

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
	PruneThreshold   float64 // default 0.95 - entries above this similarity are candidates for merge
	RelevanceDecay   float64 // default 0.99 - daily decay factor for relevance scores
	MinRelevance     float64 // default 0.1 - entries below this relevance may be pruned
	EnableSmartPrune bool    // default false - enable intelligent pruning

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
		entries:          make([]*MemoryEntry, 0),
		embedCfg:         cfg.EmbeddingConfig,
		embedFn:          embedFn,
		llm:              cfg.LLM,
		model:            cfg.Model,
		maxSize:          maxSz,
		topK:             topK,
		windowSz:         windowSz,
		enableRAG:        cfg.EnableRAG,
		pruneThreshold:   pruneThreshold,
		relevanceDecay:   relevanceDecay,
		minRelevance:     minRelevance,
		enableSmartPrune: cfg.EnableSmartPrune,
		store:            cfg.Store,
		userID:           cfg.UserID,
		sessionID:        sessionID,
		persistDelay:     persistDelay,
		callbacks:        cfg.Callbacks,
	}

	// If a store is provided, preload entries for the configured user.
	// Note: systemUserID is 0 in agentd; we still want persistence for it.
	if em.store != nil {
		if entries, err := em.store.Load(context.Background(), em.userID, em.sessionID); err == nil && len(entries) > 0 {
			// Respect maxSize by keeping only the newest maxSize entries.
			if len(entries) > em.maxSize {
				entries = entries[len(entries)-em.maxSize:]
			}
			em.entries = entries
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
	entries := make([]*MemoryEntry, len(em.entries))
	copy(entries, em.entries)
	cb := em.callbacks
	em.mu.RUnlock()

	if len(entries) == 0 {
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

	// Embed the query
	vecs, err := em.embedFn(ctx, em.embedCfg, []string{query})
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
	queryVec := vecs[0]

	// Score all entries by cosine similarity
	type scoredLocal struct {
		entry *MemoryEntry
		score float64
	}
	scores := make([]scoredLocal, 0, len(entries))
	for _, e := range entries {
		if len(e.Embedding) == 0 {
			continue
		}
		sim := cosineSimilarity(queryVec, e.Embedding)
		scores = append(scores, scoredLocal{entry: e, score: sim})
	}

	// Sort descending by score
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Return top-k
	k := em.topK
	if k > len(scores) {
		k = len(scores)
	}
	out := make([]ScoredMemoryEntry, k)
	retrievedIDs := make([]string, k)
	for i := 0; i < k; i++ {
		out[i] = ScoredMemoryEntry{Entry: scores[i].entry, Score: scores[i].score}
		retrievedIDs[i] = scores[i].entry.ID
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

	log.Debug().Int("candidates", len(entries)).Int("top_k", k).Msg("evolving_memory_search")
	return out, nil
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
		}
	}
	entriesSnapshot := em.snapshotEntriesLocked()
	em.mu.Unlock()

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
	result += "Below are similar tasks from your memory. Use them to avoid mistakes and reuse successful strategies.\n\n"

	for i, entry := range retrieved {
		result += fmt.Sprintf("### Experience %d\n", i+1)
		result += formatExperience(entry) + "\n\n"
	}

	result += "## Current Task\n"
	result += currentTask + "\n"

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

	entry := &MemoryEntry{
		ID:                 uuid.New().String(),
		Input:              input,
		Output:             output,
		Feedback:           feedback,
		Summary:            summary,
		RawTrace:           rawTrace,
		Embedding:          vecs[0],
		MemoryType:         memType,
		StrategyCard:       strategyCard,
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
		em.applySmartMergePlan(mergePlan, entry)
	}

	em.entries = append(em.entries, entry)

	// Apply relevance-based pruning if enabled and over capacity
	if em.enableSmartPrune && len(em.entries) > em.maxSize {
		em.relevanceBasedPrune(ctx)
	} else if len(em.entries) > em.maxSize {
		// Fallback to FIFO pruning
		em.entries = em.entries[len(em.entries)-em.maxSize:]
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

	// Check for procedural indicators
	proceduralKeywords := []string{
		"how to", "steps", "procedure", "workflow", "strategy",
		"algorithm", "method", "approach", "technique", "process",
		"when confronted", "do this", "avoid", "pattern",
	}
	combined := input + " " + output + " " + summary
	for _, kw := range proceduralKeywords {
		if containsIgnoreCase(combined, kw) {
			return MemoryProcedural
		}
	}

	// Check for factual indicators
	factualKeywords := []string{
		"what is", "define", "meaning of", "value of",
		"answer is", "result is", "equals", "fact",
	}
	for _, kw := range factualKeywords {
		if containsIgnoreCase(combined, kw) {
			return MemoryFactual
		}
	}

	// Default to episodic (specific task instance)
	return MemoryEpisodic
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > 0 && len(substr) > 0 &&
				containsLower(toLower(s), toLower(substr)))
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
		sim := cosineSimilarity(newEntry.Embedding, existing.Embedding)
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

func (em *EvolvingMemory) applySmartMergePlan(plan *smartMergePlan, newEntry *MemoryEntry) {
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
		newEntry.Embedding = append([]float32(nil), plan.mergedEmbedding...)
	}

	em.pruneEntries(plan.mergedIDs)
	observability.LoggerWithTrace(context.Background()).Info().
		Int("merged_count", len(plan.mergedIDs)).
		Msg("evolving_memory_smart_merged")
}

// relevanceBasedPrune removes entries based on relevance scores.
// Uses a combination of access frequency, recency, and base relevance.
func (em *EvolvingMemory) relevanceBasedPrune(ctx context.Context) {
	log := observability.LoggerWithTrace(ctx)

	now := time.Now()
	filtered := make([]*MemoryEntry, 0, len(em.entries))
	for _, e := range em.entries {
		e.RelevanceScore = em.computeRelevanceScore(now, e)
		if e.RelevanceScore >= em.minRelevance {
			filtered = append(filtered, e)
		}
	}
	em.entries = filtered

	// Sort by relevance score (ascending - lowest first)
	sort.Slice(em.entries, func(i, j int) bool {
		return em.entries[i].RelevanceScore < em.entries[j].RelevanceScore
	})

	// Calculate how many to remove
	toRemove := len(em.entries) - em.maxSize
	if toRemove <= 0 {
		return
	}

	// Remove lowest relevance entries
	var removedIDs []string
	for i := 0; i < toRemove && i < len(em.entries); i++ {
		removedIDs = append(removedIDs, em.entries[i].ID)
	}
	em.entries = em.entries[toRemove:]

	// Re-sort by creation time to maintain temporal order
	sort.Slice(em.entries, func(i, j int) bool {
		return em.entries[i].CreatedAt.Before(em.entries[j].CreatedAt)
	})

	log.Info().
		Int("removed_count", len(removedIDs)).
		Int("remaining", len(em.entries)).
		Msg("evolving_memory_relevance_pruned")
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

// generateSummary asks the LLM to distill a key lesson from the experience.
func (em *EvolvingMemory) generateSummary(ctx context.Context, input, output, feedback string) (string, error) {
	if em.llm == nil {
		return "", fmt.Errorf("no LLM provider configured")
	}

	sys := "You are a concise summarizer. Extract the key lesson or strategy from this task experience. Keep it under 100 words."
	user := fmt.Sprintf("Task: %s\nOutcome: %s\nSolution: %s\n\nWhat's the key lesson?",
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// MemoryEditOp represents a memory editing operation for ReMem's REFINE phase.
type MemoryEditOp struct {
	Type       string   `json:"type"`        // PRUNE, MERGE, UPDATE_TAG
	IDs        []string `json:"ids"`         // entry IDs to operate on
	NewSummary string   `json:"new_summary"` // for MERGE
	Tag        string   `json:"tag"`         // for UPDATE_TAG
}

// ApplyEdits applies memory editing operations (for ReMem REFINE phase).
func (em *EvolvingMemory) ApplyEdits(ctx context.Context, ops []MemoryEditOp) error {
	log := observability.LoggerWithTrace(ctx)

	for _, op := range ops {
		switch op.Type {
		case "PRUNE":
			em.pruneEntries(op.IDs)
			log.Info().Strs("ids", op.IDs).Msg("evolving_memory_pruned_entries")

		case "MERGE":
			if err := em.mergeEntries(ctx, op.IDs, op.NewSummary); err != nil {
				log.Error().Err(err).Msg("evolving_memory_merge_failed")
				return err
			}
			log.Info().Strs("ids", op.IDs).Msg("evolving_memory_merged_entries")

		case "UPDATE_TAG":
			em.updateTag(op.IDs, op.Tag)
			log.Info().Strs("ids", op.IDs).Str("tag", op.Tag).Msg("evolving_memory_updated_tag")

		default:
			log.Warn().Str("type", op.Type).Msg("evolving_memory_unknown_edit_op")
		}
	}

	// Persist after applying edits if backed by a store.
	// Note: systemUserID is 0 in agentd; we still want persistence for it.
	if em.store != nil {
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

	// Create merged entry (use first entry's input/output, new summary)
	merged := &MemoryEntry{
		ID:       uuid.New().String(),
		Input:    toMerge[0].Input,
		Output:   toMerge[0].Output,
		Feedback: "merged",
		Summary:  newSummary,
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
	merged.Embedding = vecs[0]

	// Remove old entries and add merged
	em.pruneEntries(ids)
	em.entries = append(em.entries, merged)

	return nil
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
		}
	}
}

// ExportMemories returns all memory entries for inspection endpoints.
func (em *EvolvingMemory) ExportMemories() []*MemoryEntry {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.snapshotEntriesLocked()
}

func (em *EvolvingMemory) snapshotEntriesLocked() []*MemoryEntry {
	return cloneEntrySlice(em.entries)
}

func (em *EvolvingMemory) persistEntriesAsync(entries []*MemoryEntry) {
	if em.store == nil {
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
		if entry == nil {
			cloned = append(cloned, nil)
			continue
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

		cloned = append(cloned, &copyEntry)
	}

	return cloned
}
