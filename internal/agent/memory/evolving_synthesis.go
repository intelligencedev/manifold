package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (em *EvolvingMemory) Synthesize(ctx context.Context, currentTask string, retrieved []*MemoryEntry) string {
	scored := make([]ScoredMemoryEntry, 0, len(retrieved))
	for _, entry := range retrieved {
		scored = append(scored, ScoredMemoryEntry{Entry: entry})
	}
	return em.SynthesizeScored(ctx, currentTask, scored)
}

func (em *EvolvingMemory) SynthesizeScored(ctx context.Context, currentTask string, retrieved []ScoredMemoryEntry) string {
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

	var result strings.Builder
	result.WriteString("## Past Relevant Experiences\n\n")

	successes, cautions := partitionScoredRetrievedByOutcome(retrieved)
	if len(successes) > 0 {
		result.WriteString("## Strategies That Worked\n\n")
		for i, entry := range successes {
			result.WriteString(fmt.Sprintf("### Experience %d\n", i+1))
			result.WriteString(formatScoredExperience(entry) + "\n\n")
		}
	}
	if len(cautions) > 0 {
		result.WriteString("## Mistakes to Avoid\n\n")
		for i, entry := range cautions {
			result.WriteString(fmt.Sprintf("### Experience %d\n", i+1))
			result.WriteString(formatScoredExperience(entry) + "\n\n")
		}
	}

	if cb != nil && cb.OnSynthesized != nil {
		retrievedIDs := make([]string, 0, len(retrieved))
		for _, r := range retrieved {
			if r.Entry != nil {
				retrievedIDs = append(retrievedIDs, r.Entry.ID)
			}
		}
		cb.OnSynthesized(&MemoryEvent{
			Phase:        PhaseSynthesis,
			Timestamp:    start,
			Input:        currentTask,
			RetrievedIDs: retrievedIDs,
			OutputSize:   len(result.String()),
			DurationMs:   time.Since(start).Milliseconds(),
		})
	}

	return result.String()
}

func partitionScoredRetrievedByOutcome(retrieved []ScoredMemoryEntry) ([]ScoredMemoryEntry, []ScoredMemoryEntry) {
	successes := make([]ScoredMemoryEntry, 0, len(retrieved))
	cautions := make([]ScoredMemoryEntry, 0, len(retrieved))
	for _, item := range retrieved {
		entry := item.Entry
		if entry == nil {
			continue
		}
		if entry.StructuredFeedback != nil && entry.StructuredFeedback.Type == FeedbackFailure {
			cautions = append(cautions, item)
			continue
		}
		if entry.StructuredFeedback != nil && (entry.StructuredFeedback.Type == FeedbackPartial || entry.StructuredFeedback.Type == FeedbackInProgress) {
			cautions = append(cautions, item)
			continue
		}
		if strings.EqualFold(entry.Feedback, string(FeedbackFailure)) || strings.EqualFold(entry.Feedback, string(FeedbackPartial)) {
			cautions = append(cautions, item)
			continue
		}
		successes = append(successes, item)
	}
	return successes, cautions
}

func formatScoredExperience(item ScoredMemoryEntry) string {
	if item.Entry == nil {
		return ""
	}
	s := formatExperience(item.Entry)
	if item.Score != 0 {
		s += fmt.Sprintf("**Retrieval Score:** %.3f\n", item.Score)
	}
	return s
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
