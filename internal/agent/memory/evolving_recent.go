package memory

import (
	"fmt"
	"strings"
)

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

	var result strings.Builder
	result.WriteString("## Recent Task History\n\n")
	for i, entry := range recent {
		result.WriteString(fmt.Sprintf("%d. Task: %s | Outcome: %s\n",
			i+1, truncate(entry.Input, 80), entry.Feedback))
	}
	return result.String() + "\n"
}
