package memory

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const memoryEmbeddingTextBasis = "input_summary_strategy_outcome"

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

func retrievalTextForMemory(input, output, feedback, summary, strategyCard string) string {
	parts := make([]string, 0, 5)
	if strings.TrimSpace(input) != "" {
		parts = append(parts, "Task: "+truncate(input, 800))
	}
	if strings.TrimSpace(feedback) != "" {
		parts = append(parts, "Outcome: "+truncate(feedback, 120))
	}
	if strings.TrimSpace(summary) != "" {
		parts = append(parts, "Reusable lesson: "+truncate(summary, 1200))
	}
	if strings.TrimSpace(strategyCard) != "" {
		parts = append(parts, "Strategy: "+truncate(strategyCard, 1200))
	}
	if strings.TrimSpace(output) != "" {
		parts = append(parts, "Result: "+truncate(output, 800))
	}
	return strings.Join(parts, "\n")
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
