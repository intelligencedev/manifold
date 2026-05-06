package memory

import (
	"context"
	"fmt"
	"time"
)

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
