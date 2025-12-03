package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"manifold/internal/config"
	"manifold/internal/embedding"
	"manifold/internal/llm"
	"manifold/internal/observability"

	"github.com/google/uuid"
)

// MemoryEntry represents a structured experience from task execution.
// Implements the paper's m_i = h(x_i, ŷ_i, f_i) abstraction.
type MemoryEntry struct {
	ID        string                 `json:"id"`
	Input     string                 `json:"input"`     // x_i: task/query
	Output    string                 `json:"output"`    // ŷ_i: model's answer
	Feedback  string                 `json:"feedback"`  // f_i: success/failure signal
	Summary   string                 `json:"summary"`   // distilled lesson
	RawTrace  string                 `json:"raw_trace"` // optional detailed reasoning
	Embedding []float32              `json:"embedding"` // for retrieval
	Metadata  map[string]interface{} `json:"metadata"`  // timestamp, domain, task_id, etc.
	CreatedAt time.Time              `json:"created_at"`
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
	Load(ctx context.Context, userID int64) ([]*MemoryEntry, error)
	Save(ctx context.Context, userID int64, entries []*MemoryEntry) error
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
	llm       llm.Provider
	model     string
	maxSize   int // max number of entries to keep
	topK      int // number of similar entries to retrieve (default 4)
	windowSz  int // for ExpRecent sliding window (default 20)
	enableRAG bool

	// Optional persistent backing store; when set, entries are loaded on
	// construction and persisted after each mutation.
	store  EvolvingMemoryStore
	userID int64
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
	LLM             llm.Provider
	Model           string
	MaxSize         int  // 0 = unlimited
	TopK            int  // default 4
	WindowSize      int  // default 20 for ExpRecent
	EnableRAG       bool // enable ExpRAG retrieval
	// Optional persistent store. When non-nil, NewEvolvingMemory will load
	// existing entries for the given userID and persist updates.
	Store  EvolvingMemoryStore
	UserID int64
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

	em := &EvolvingMemory{
		entries:   make([]*MemoryEntry, 0),
		embedCfg:  cfg.EmbeddingConfig,
		llm:       cfg.LLM,
		model:     cfg.Model,
		maxSize:   maxSz,
		topK:      topK,
		windowSz:  windowSz,
		enableRAG: cfg.EnableRAG,
		store:     cfg.Store,
		userID:    cfg.UserID,
	}

	// If a store is provided and a non-zero userID is set, preload entries.
	if em.store != nil && em.userID != 0 {
		if entries, err := em.store.Load(context.Background(), em.userID); err == nil && len(entries) > 0 {
			// Respect maxSize by keeping only the newest maxSize entries.
			if len(entries) > em.maxSize {
				entries = entries[len(entries)-em.maxSize:]
			}
			em.entries = entries
		}
	}

	return em
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
	em.mu.RLock()
	entries := make([]*MemoryEntry, len(em.entries))
	copy(entries, em.entries)
	em.mu.RUnlock()

	if len(entries) == 0 {
		return nil, nil
	}

	log := observability.LoggerWithTrace(ctx)

	// Embed the query
	vecs, err := embedding.EmbedText(ctx, em.embedCfg, []string{query})
	if err != nil {
		log.Error().Err(err).Msg("evolving_memory_embed_query_failed")
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
	for i := 0; i < k; i++ {
		out[i] = ScoredMemoryEntry{Entry: scores[i].entry, Score: scores[i].score}
	}

	log.Debug().Int("candidates", len(entries)).Int("top_k", k).Msg("evolving_memory_search")
	return out, nil
}

// Synthesize implements C(x_t, R_t): build context from current task + retrieved memories.
// Returns a formatted string suitable for injection into the system prompt or context.
func (em *EvolvingMemory) Synthesize(ctx context.Context, currentTask string, retrieved []*MemoryEntry) string {
	if len(retrieved) == 0 {
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

	return result
}

// formatExperience converts a memory entry into a structured textual block (template S from paper).
func formatExperience(entry *MemoryEntry) string {
	var s string
	s += fmt.Sprintf("**Task:** %s\n", truncate(entry.Input, 200))
	s += fmt.Sprintf("**Outcome:** %s\n", entry.Feedback)
	if entry.Summary != "" {
		s += fmt.Sprintf("**Key Lesson:** %s\n", entry.Summary)
	}
	if entry.Output != "" {
		s += fmt.Sprintf("**Solution:** %s\n", truncate(entry.Output, 150))
	}
	return s
}

// Evolve implements U(M_t, m_t): update memory with new experience.
// This appends the new entry and optionally prunes if max size is exceeded.
func (em *EvolvingMemory) Evolve(ctx context.Context, input, output, feedback string) error {
	log := observability.LoggerWithTrace(ctx)

	// Generate summary via LLM
	summary, err := em.generateSummary(ctx, input, output, feedback)
	if err != nil {
		log.Warn().Err(err).Msg("evolving_memory_summarize_failed")
		summary = "(summary unavailable)"
	}

	// Embed the input for retrieval
	vecs, err := embedding.EmbedText(ctx, em.embedCfg, []string{input})
	if err != nil {
		log.Error().Err(err).Msg("evolving_memory_embed_failed")
		return fmt.Errorf("embed input: %w", err)
	}

	entry := &MemoryEntry{
		ID:        uuid.New().String(),
		Input:     input,
		Output:    output,
		Feedback:  feedback,
		Summary:   summary,
		Embedding: vecs[0],
		Metadata: map[string]interface{}{
			"domain": "general",
		},
		CreatedAt: time.Now(),
	}

	em.entries = append(em.entries, entry)

	// Prune oldest if exceeding max size
	if len(em.entries) > em.maxSize {
		em.entries = em.entries[len(em.entries)-em.maxSize:]
		log.Info().Int("pruned_to", em.maxSize).Msg("evolving_memory_pruned")
	}

	// Persist in the background if a store is configured.
	if em.store != nil && em.userID != 0 {
		entriesCopy := make([]*MemoryEntry, len(em.entries))
		copy(entriesCopy, em.entries)
		go func(entries []*MemoryEntry, uid int64) {
			bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := em.store.Save(bgctx, uid, entries); err != nil {
				observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_persist_failed")
			}
		}(entriesCopy, em.userID)
	}

	log.Info().Str("entry_id", entry.ID).Msg("evolving_memory_entry_added")
	return nil
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
	if len(em.entries) == 0 {
		return nil
	}
	start := 0
	if len(em.entries) > em.windowSz {
		start = len(em.entries) - em.windowSz
	}
	return em.entries[start:]
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
	if em.store != nil && em.userID != 0 {
		entriesCopy := make([]*MemoryEntry, len(em.entries))
		copy(entriesCopy, em.entries)
		go func(entries []*MemoryEntry, uid int64) {
			bgctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := em.store.Save(bgctx, uid, entries); err != nil {
				observability.LoggerWithTrace(bgctx).Error().Err(err).Msg("evolving_memory_persist_failed")
			}
		}(entriesCopy, em.userID)
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
	vecs, err := embedding.EmbedText(ctx, em.embedCfg, []string{newSummary})
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

// ExportMemories returns all memory entries (for debugging/persistence).
func (em *EvolvingMemory) ExportMemories() []*MemoryEntry {
	return em.entries
}

// ImportMemories loads memory entries (for persistence/restore).
func (em *EvolvingMemory) ImportMemories(entries []*MemoryEntry) {
	em.entries = entries
}

// MarshalJSON serializes the memory state.
func (em *EvolvingMemory) MarshalJSON() ([]byte, error) {
	return json.Marshal(em.entries)
}

// UnmarshalJSON deserializes the memory state.
func (em *EvolvingMemory) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &em.entries)
}
