package memory

import (
	"context"
	"errors"
	"fmt"
	"manifold/internal/agent/prompts"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/persistence"
	"strings"
	"sync"
)

func (m *Manager) summarizeChunk(ctx context.Context, existingSummary string, chunk []persistence.ChatMessage, targetCompactor llm.CompactionProvider, targetModel string) (string, error) {
	if m.summary == nil {
		return existingSummary, fmt.Errorf("llm provider unavailable")
	}

	// Decode existing summary to get both compaction and plain components.
	existing := decodeDualSummary(existingSummary)
	log := observability.LoggerWithTrace(ctx)

	if m.responsesCompactionEnabled(targetCompactor) {
		// Run both compaction and plain text summarization in parallel.
		// This ensures we have a plain text fallback if the user switches to a non-OpenAI model.
		var (
			wg            sync.WaitGroup
			compactionErr error
			plainErr      error
			compactionRes string
			plainRes      string
		)

		wg.Add(2)

		// Goroutine 1: Compaction summarization (OpenAI Responses API).
		go func() {
			defer wg.Done()
			res, err := m.compactChunk(ctx, targetCompactor, targetModel, existing.Compaction, chunk)
			if err != nil {
				compactionErr = err
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					log.Warn().Err(err).Msg("compaction_summarization_cancelled")
				} else {
					log.Warn().Err(err).Msg("compaction_summarization_failed")
				}
				return
			}
			compactionRes = res
		}()

		// Goroutine 2: Plain text summarization (for non-OpenAI model fallback).
		go func() {
			defer wg.Done()
			res, err := m.plainSummarize(ctx, existing.Plain, chunk)
			if err != nil {
				plainErr = err
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					log.Warn().Err(err).Msg("plain_summarization_cancelled")
				} else {
					log.Warn().Err(err).Msg("plain_summarization_failed")
				}
				return
			}
			plainRes = res
		}()

		wg.Wait()

		ds := dualSummary{
			Compaction: compactionRes,
			Plain:      plainRes,
		}

		// If compaction failed but plain succeeded, use plain only.
		if compactionErr != nil && plainErr == nil {
			return ds.Plain, nil
		}
		// If both failed, return error.
		if compactionErr != nil && plainErr != nil {
			return existingSummary, fmt.Errorf("both summarization methods failed: compaction: %v, plain: %v", compactionErr, plainErr)
		}
		// If plain failed but compaction succeeded, still return dual (plain will be empty).
		// Future requests to non-OpenAI models will get no summary, which is safer than garbage.

		return encodeDualSummary(ds), nil
	}

	// Non-compaction mode: just do plain text summarization.
	plainRes, err := m.plainSummarize(ctx, existing.Plain, chunk)
	if err != nil {
		return existingSummary, err
	}
	return plainRes, nil
}

func (m *Manager) responsesCompactionEnabled(targetCompactor llm.CompactionProvider) bool {
	return targetCompactor != nil && !m.compactionUnavailable.Load()
}

// plainSummarize generates a plain text summary of the conversation chunk.
// This is used for non-OpenAI models and as a fallback when compaction is enabled.
func (m *Manager) plainSummarize(ctx context.Context, existingPlainSummary string, chunk []persistence.ChatMessage) (string, error) {
	sections := m.buildPlainSummarySections(existingPlainSummary, chunk)
	summary, err := m.reducePlainSummarySections(ctx, sections)
	if err != nil {
		return existingPlainSummary, err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return existingPlainSummary, fmt.Errorf("empty summary returned")
	}
	return summary, nil
}

func (m *Manager) buildPlainSummarySections(existingPlainSummary string, chunk []persistence.ChatMessage) []string {
	sections := make([]string, 0, len(chunk)+1)
	limit := maxSummarizeChunkSize
	if m.maxSummaryChunkTokens > 0 {
		limit = m.maxSummaryChunkTokens
	}

	if existing := strings.TrimSpace(existingPlainSummary); existing != "" {
		sections = append(sections, "Existing summary:\n"+truncateForSummary(existing, limit))
	}

	for _, msg := range chunk {
		summaryMsg := buildSummaryPromptMessage(msg)
		var section strings.Builder
		section.WriteString("Role: ")
		section.WriteString(summaryMsg.Role)
		section.WriteString("\n")
		if len(summaryMsg.ToolCalls) > 0 {
			section.WriteString("Tool calls: ")
			section.WriteString(strings.Join(summaryMsg.ToolCalls, ", "))
			section.WriteString("\n")
		}
		if strings.TrimSpace(summaryMsg.ToolID) != "" {
			section.WriteString("Tool ID: ")
			section.WriteString(summaryMsg.ToolID)
			section.WriteString("\n")
		}
		content := truncateForSummary(strings.TrimSpace(summaryMsg.Content), limit)
		if content == "" {
			content = "(no content)"
		}
		section.WriteString(content)
		sections = append(sections, section.String())
	}

	return sections
}

func (m *Manager) reducePlainSummarySections(ctx context.Context, sections []string) (string, error) {
	current := make([]string, 0, len(sections))
	chunkBudget := m.summaryReductionChunkBudget()
	for _, section := range sections {
		current = append(current, splitSummarySection(section, chunkBudget)...)
	}
	if len(current) == 0 {
		return "", fmt.Errorf("empty summary input")
	}

	if len(current) == 1 {
		summary, err := m.runPlainSummaryPass(ctx, current)
		if err != nil {
			return "", err
		}
		summary = strings.TrimSpace(summary)
		if summary == "" {
			return "", fmt.Errorf("empty summary returned")
		}
		return summary, nil
	}

	for range 8 {
		chunks := packSummarySections(current, chunkBudget)
		if len(chunks) == 1 {
			summary, err := m.runPlainSummaryPass(ctx, chunks[0])
			if err != nil {
				return "", err
			}
			summary = strings.TrimSpace(summary)
			if summary == "" {
				return "", fmt.Errorf("empty summary returned")
			}
			return summary, nil
		}
		next := make([]string, 0, len(chunks))
		for _, chunk := range chunks {
			summary, err := m.runPlainSummaryPass(ctx, chunk)
			if err != nil {
				return "", err
			}
			summary = strings.TrimSpace(summary)
			if summary == "" {
				return "", fmt.Errorf("empty summary returned")
			}
			next = append(next, summary)
		}

		if len(next) == 1 && estimateSummaryTextTokens(next[0]) <= chunkBudget {
			return next[0], nil
		}

		if len(next) == 1 && len(current) == 1 && strings.TrimSpace(next[0]) == strings.TrimSpace(current[0]) {
			current = splitSummarySection(next[0], maxInt(1, chunkBudget/2))
			continue
		}

		current = current[:0]
		for _, summary := range next {
			current = append(current, splitSummarySection(summary, chunkBudget)...)
		}
	}

	return truncateForSummary(strings.Join(current, "\n\n"), chunkBudget*4), nil
}

func (m *Manager) runPlainSummaryPass(ctx context.Context, sections []string) (string, error) {
	msgs := m.buildPlainSummaryPassMessages(sections)
	if limit := m.summaryPromptTokenLimit(); limit > 0 && estimateMessagesTokens(msgs) > limit {
		if m.summaryPromptSectionBudget(limit) <= 0 {
			return truncateForSummary(strings.TrimSpace(strings.Join(sections, "\n\n")), maxInt(32, limit*4)), nil
		}

		if len(sections) > 1 {
			mid := len(sections) / 2
			left, err := m.runPlainSummaryPass(ctx, sections[:mid])
			if err != nil {
				return "", err
			}
			right, err := m.runPlainSummaryPass(ctx, sections[mid:])
			if err != nil {
				return "", err
			}

			reduced := []string{left, right}
			if estimateMessagesTokens(m.buildPlainSummaryPassMessages(reduced)) > limit {
				merged := m.truncateSectionForPromptLimit(strings.Join(reduced, "\n\n"), limit)
				return m.runPlainSummaryPass(ctx, []string{merged})
			}
			return m.runPlainSummaryPass(ctx, reduced)
		}

		splitBudget := minInt(m.summaryReductionChunkBudget()/2, m.summaryPromptSectionBudget(limit))
		split := splitSummarySection(sections[0], maxInt(1, splitBudget))
		if len(split) > 1 {
			compressed := make([]string, 0, len(split))
			for _, part := range split {
				summary, err := m.runPlainSummaryPass(ctx, []string{part})
				if err != nil {
					return "", err
				}
				compressed = append(compressed, summary)
			}
			return m.runPlainSummaryPass(ctx, compressed)
		}

		truncated := m.truncateSectionForPromptLimit(sections[0], limit)
		if strings.TrimSpace(truncated) != strings.TrimSpace(sections[0]) {
			return m.runPlainSummaryPass(ctx, []string{truncated})
		}

		return truncateForSummary(strings.TrimSpace(sections[0]), maxInt(32, limit*4)), nil
	}

	var (
		resp llm.Message
		err  error
	)
	if m.summaryCallTimeout > 0 {
		callCtx, cancel := context.WithTimeout(ctx, m.summaryCallTimeout)
		defer cancel()
		resp, err = m.summary.Chat(callCtx, msgs, nil, m.summaryModel)
	} else {
		resp, err = m.summary.Chat(ctx, msgs, nil, m.summaryModel)
	}
	if err != nil {
		return "", fmt.Errorf("summarize chat: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}

func (m *Manager) buildPlainSummaryPassMessages(sections []string) []llm.Message {
	return prompts.BuildRunningSummaryMessages(sections)
}

func (m *Manager) summaryPromptSectionBudget(limit int) int {
	if limit <= 0 {
		return 0
	}
	overhead := estimateMessagesTokens(m.buildPlainSummaryPassMessages([]string{""}))
	return limit - overhead
}

func (m *Manager) truncateSectionForPromptLimit(section string, limit int) string {
	if limit <= 0 {
		return strings.TrimSpace(section)
	}
	available := m.summaryPromptSectionBudget(limit)
	if available <= 0 {
		available = maxInt(1, limit/4)
	}
	return truncateForSummary(strings.TrimSpace(section), available*4)
}

func (m *Manager) summaryReductionChunkBudget() int {
	budget := m.summaryPromptTokenLimit()
	if budget <= 0 {
		budget = maxSummarizeChunkSize
	}
	budget -= 256
	if budget < 64 {
		budget = 64
	}
	return budget
}

func (m *Manager) summaryPromptTokenLimit() int {
	ctxSize := m.resolvePlainTextContextWindowTokens(SummaryPolicy{})

	reserveBuffer := m.reserveBufferTokens
	if reserveBuffer <= 0 {
		reserveBuffer = defaultReserveBuffer
	}

	budget := ctxSize - reserveBuffer
	if budget <= 0 {
		budget = ctxSize / 2
	}
	if budget <= 0 {
		budget = maxSummarizeChunkSize
	}
	return budget
}

func packSummarySections(sections []string, maxTokens int) [][]string {
	if maxTokens <= 0 {
		maxTokens = 1
	}
	chunks := make([][]string, 0, len(sections))
	current := make([]string, 0, len(sections))
	currentTokens := 0
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		sectionTokens := estimateSummaryTextTokens(section)
		if len(current) > 0 && currentTokens+sectionTokens > maxTokens {
			chunks = append(chunks, current)
			current = make([]string, 0, len(sections))
			currentTokens = 0
		}
		current = append(current, section)
		currentTokens += sectionTokens
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func splitSummarySection(section string, maxTokens int) []string {
	section = strings.TrimSpace(section)
	if section == "" {
		return nil
	}
	if maxTokens <= 0 || estimateSummaryTextTokens(section) <= maxTokens {
		return []string{section}
	}

	parts := strings.Split(section, "\n\n")
	if len(parts) > 1 {
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			result = append(result, splitSummarySection(part, maxTokens)...)
		}
		if len(result) > 1 {
			return result
		}
	}

	maxRunes := maxTokens * 4
	if maxRunes <= 0 {
		maxRunes = len([]rune(section))
	}
	runes := []rune(section)
	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := min(start+maxRunes, len(runes))
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func estimateSummaryTextTokens(content string) int {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return 1
	}
	return len([]rune(trimmed))/4 + 1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func estimateMessagesTokens(msgs []llm.Message) int {
	est := 0
	for _, m := range msgs {
		c := strings.TrimSpace(m.Content)
		if c == "" {
			est++
			continue
		}
		est += len([]rune(c))/4 + 1
	}
	return est
}
