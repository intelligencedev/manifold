package memory

import (
	"context"
	"fmt"
	"manifold/internal/observability"
	"slices"
	"time"

	"github.com/google/uuid"
)

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
		if slices.Contains(ids, e.ID) {
			toMerge = append(toMerge, e)
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
		Metadata: map[string]any{
			"merged_from": ids,
		},
		CreatedAt: time.Now(),
	}

	if merged.Metadata == nil {
		merged.Metadata = make(map[string]any)
	}
	merged.Metadata["embedding_enabled"] = em.enableRAG
	merged.Metadata["embedding_text_basis"] = memoryEmbeddingTextBasis
	if em.enableRAG {
		retrievalText := retrievalTextForMemory(merged.Input, merged.Output, merged.Feedback, merged.Summary, merged.StrategyCard)
		merged.Metadata["embedding_text_length"] = len(retrievalText)
		vecs, err := em.embedFn(ctx, em.embedCfg, []string{retrievalText})
		if err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Msg("evolving_memory_merge_embed_failed_storing_without_embedding")
			merged.Metadata["embedding_error"] = err.Error()
		} else if len(vecs) > 0 {
			merged.Embedding = normalizeVector(vecs[0])
		}
	}
	merged.Metadata["has_embedding"] = len(merged.Embedding) > 0

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
				e.Metadata = make(map[string]any)
			}
			e.Metadata["tag"] = tag
			em.markDirtyLocked(e.ID)
		}
	}
}
