package agentd

import (
	"context"
	"strings"
	"unicode/utf8"

	"manifold/internal/agent/memory"
	"manifold/internal/rag/retrieve"
)

type evolvingMemoryRAGReranker struct {
	reranker retrieve.Reranker
}

func (r evolvingMemoryRAGReranker) RerankEvolvingMemory(ctx context.Context, query string, items []memory.ScoredMemoryEntry) ([]memory.ScoredMemoryEntry, error) {
	if r.reranker == nil || len(items) <= 1 {
		return items, nil
	}
	retrieved := make([]retrieve.RetrievedItem, 0, len(items))
	byID := make(map[string]memory.ScoredMemoryEntry, len(items))
	for _, item := range items {
		if item.Entry == nil {
			continue
		}
		text := memoryRerankText(item.Entry)
		retrieved = append(retrieved, retrieve.RetrievedItem{
			ID:    item.Entry.ID,
			Score: item.Score,
			Text:  text,
			Metadata: map[string]string{
				"memory_type": string(item.Entry.MemoryType),
				"scope":       string(item.Entry.Scope),
			},
		})
		byID[item.Entry.ID] = item
	}
	if len(retrieved) <= 1 {
		return items, nil
	}

	reranked, err := r.reranker.Rerank(ctx, query, retrieved)
	if err != nil {
		return items, err
	}
	out := make([]memory.ScoredMemoryEntry, 0, len(items))
	used := make(map[string]struct{}, len(reranked))
	for _, item := range reranked {
		original, ok := byID[item.ID]
		if !ok {
			continue
		}
		original.Score = item.Score
		out = append(out, original)
		used[item.ID] = struct{}{}
	}
	for _, item := range items {
		if item.Entry == nil {
			continue
		}
		if _, ok := used[item.Entry.ID]; ok {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func memoryRerankText(entry *memory.MemoryEntry) string {
	if entry == nil {
		return ""
	}
	parts := make([]string, 0, 5)
	if text := strings.TrimSpace(entry.Input); text != "" {
		parts = append(parts, "Task: "+truncateMemoryRerankText(text, 800))
	}
	if text := strings.TrimSpace(entry.Feedback); text != "" {
		parts = append(parts, "Outcome: "+truncateMemoryRerankText(text, 120))
	}
	if text := strings.TrimSpace(entry.Summary); text != "" {
		parts = append(parts, "Reusable lesson: "+truncateMemoryRerankText(text, 1200))
	}
	if text := strings.TrimSpace(entry.StrategyCard); text != "" {
		parts = append(parts, "Strategy: "+truncateMemoryRerankText(text, 1200))
	}
	if text := strings.TrimSpace(entry.Output); text != "" {
		parts = append(parts, "Result: "+truncateMemoryRerankText(text, 800))
	}
	return strings.Join(parts, "\n")
}

func truncateMemoryRerankText(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	truncated := s[:maxBytes]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + "..."
}
