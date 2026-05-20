package memory

import (
	"context"
	"fmt"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"time"

	"github.com/google/uuid"
)

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
	summary = limitUTF8Bytes(redactPII(summary), maxStoredOutputBytes)
	strategyCard = limitUTF8Bytes(redactPII(strategyCard), maxStoredOutputBytes)

	// Classify memory type based on content analysis.
	memType := em.classifyMemoryType(input, output, summary, strategyCard)

	// Build raw trace from reasoning trace
	rawTrace := ""
	if len(reasoningTrace) > 0 {
		for i, t := range reasoningTrace {
			rawTrace += fmt.Sprintf("Step %d: %s\n", i+1, t)
		}
	}
	rawTrace = limitUTF8Bytes(redactPII(rawTrace), maxStoredRawTraceBytes)

	entryID := entryIDFromContext(ctx)
	if entryID == "" {
		entryID = uuid.New().String()
	}
	entry := &MemoryEntry{
		ID:                 entryID,
		Input:              input,
		Output:             output,
		Feedback:           feedback,
		Summary:            summary,
		RawTrace:           rawTrace,
		MemoryType:         memType,
		StrategyCard:       strategyCard,
		Scope:              MemoryScopeSession,
		StructuredFeedback: structuredFB,
		AccessCount:        0,
		LastAccessedAt:     time.Now(),
		RelevanceScore:     1.0, // Start with full relevance
		Metadata: map[string]any{
			"domain":                "general",
			"embedding_enabled":     em.enableRAG,
			"embedding_text_basis":  memoryEmbeddingTextBasis,
			"embedding_text_length": len(retrievalTextForMemory(input, output, feedback, summary, strategyCard)),
		},
		CreatedAt: time.Now(),
	}
	if em.enableRAG {
		retrievalText := retrievalTextForMemory(input, output, feedback, summary, strategyCard)
		vecs, err := em.embedFn(ctx, em.embedCfg, []string{retrievalText})
		if err != nil {
			log.Warn().Err(err).Msg("evolving_memory_embed_failed_storing_without_embedding")
			entry.Metadata["embedding_error"] = err.Error()
		} else if len(vecs) == 0 {
			log.Warn().Msg("evolving_memory_embed_empty_storing_without_embedding")
			entry.Metadata["embedding_error"] = "empty embedding"
		} else {
			entry.Embedding = normalizeVector(vecs[0])
		}
	}
	entry.Metadata["has_embedding"] = len(entry.Embedding) > 0

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
func (em *EvolvingMemory) classifyMemoryType(input, output, summary, strategyCard string) MemoryType {
	// Simple heuristic-based classification
	// In production, this could use an LLM call for more accurate classification

	combined := input + " " + output + " " + summary + " " + strategyCard
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

	retrievalText := retrievalTextForMemory(newEntry.Input, newEntry.Output, newEntry.Feedback, mergedSummary, newEntry.StrategyCard)
	vecs, err := em.embedFn(ctx, em.embedCfg, []string{retrievalText})
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
		newEntry.Metadata = make(map[string]any)
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
