package memory

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/persistence"
	"strings"
)

type summaryRequest struct {
	UserID          *int64
	Session         persistence.ChatSession
	Messages        []persistence.ChatMessage
	TargetCompactor llm.CompactionProvider
	TargetModel     string
	Policy          SummaryPolicy
}

func (m *Manager) ensureSummary(ctx context.Context, req summaryRequest) (string, int, *SummaryResult) {
	if !m.enabled || m.summary == nil {
		return req.Session.Summary, req.Session.SummarizedCount, nil
	}

	total := len(req.Messages)
	if total == 0 {
		return req.Session.Summary, req.Session.SummarizedCount, nil
	}

	plan := m.summaryPlan(req)
	if !plan.shouldSummarize() {
		return req.Session.Summary, req.Session.SummarizedCount, nil
	}
	chunk, target, summarizedCount, ok := summaryChunk(req.Messages, req.Session.SummarizedCount)
	if !ok {
		return req.Session.Summary, summarizedCount, nil
	}

	log := observability.LoggerWithTrace(ctx)
	logSummaryPlan(ctx, req, plan, len(chunk))
	result := summaryResult(plan, total, len(chunk))

	summaryCtx, cancel := AuxiliarySummaryContext(ctx, m.summaryCallTimeout)
	defer cancel()

	summary, err := m.summarizeChunk(summaryCtx, req.Session.Summary, chunk, req.TargetCompactor, req.TargetModel)
	if err != nil {
		log.Error().Err(err).Str("session", req.Session.ID).Msg("chat_summary_failed")
		return req.Session.Summary, summarizedCount, result
	}

	if err := m.store.UpdateSummary(summaryCtx, req.UserID, req.Session.ID, summary, target); err != nil {
		log.Error().Err(err).Str("session", req.Session.ID).Msg("chat_summary_persist_failed")
		return req.Session.Summary, summarizedCount, result
	}

	log.Info().Str("session", req.Session.ID).Int("messages", target).Msg("chat_summary_updated")
	return summary, target, result
}

type summaryPlan struct {
	targetContextSize int
	plainContextSize  int
	contextSize       int
	reserveBuffer     int
	budget            int
	summaryTokens     int
	effectiveTokens   int
	unsummarizedCount int
	maxTail           int
	compactionEnabled bool
	forceByCount      bool
	compactionReady   bool
}

func (p summaryPlan) shouldSummarize() bool {
	if !p.forceByCount && p.effectiveTokens <= p.budget {
		if !p.compactionEnabled {
			return false
		}
		return p.compactionReady
	}
	if p.forceByCount && p.effectiveTokens <= p.budget {
		return p.unsummarizedCount-p.maxTail >= minForceCountBatch
	}
	return true
}

func (m *Manager) summaryPlan(req summaryRequest) summaryPlan {
	total := len(req.Messages)
	summarizedCursor := min(max(req.Session.SummarizedCount, 0), total)
	compactionEnabled := m.responsesCompactionEnabled(req.TargetCompactor)
	plan := summaryPlan{
		targetContextSize: m.resolveTargetContextWindowTokens(req.Policy, req.TargetModel),
		plainContextSize:  m.resolvePlainTextContextWindowTokens(req.Policy),
		reserveBuffer:     defaultIntValue(m.reserveBufferTokens, defaultReserveBuffer),
		summaryTokens:     estimateStoredSummaryTokens(req.Session.Summary),
		unsummarizedCount: total - summarizedCursor,
		maxTail:           max(m.maxKeepLastMessages, m.minKeepLastMessages),
		compactionEnabled: compactionEnabled,
	}
	plan.contextSize = m.summaryContextSize(plan)
	plan.budget = summaryTokenBudget(plan.contextSize, plan.reserveBuffer)
	plan.effectiveTokens = plan.summaryTokens + estimateChatMessagesTokens(req.Messages[summarizedCursor:])
	plan.forceByCount = !plan.compactionEnabled && plan.maxTail > 0 && plan.unsummarizedCount > plan.maxTail
	plan.compactionReady = compactionMilestoneReached(req.Messages, req.Session.SummarizedCount)
	return plan
}

func estimateStoredSummaryTokens(summary string) int {
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return 0
	}
	return len([]rune(trimmed))/4 + 1
}

func (m *Manager) summaryContextSize(plan summaryPlan) int {
	ctxSize := plan.targetContextSize
	if !plan.compactionEnabled {
		ctxSize = minPositiveInt(plan.targetContextSize, plan.plainContextSize)
		if ctxSize <= 0 {
			ctxSize = plan.targetContextSize
		}
	}
	if ctxSize <= 0 {
		return 32_000
	}
	return ctxSize
}

func summaryTokenBudget(ctxSize, reserveBuffer int) int {
	budget := ctxSize - reserveBuffer
	if budget <= 0 {
		return ctxSize / 2
	}
	return budget
}

func estimateChatMessagesTokens(messages []persistence.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += len([]rune(strings.TrimSpace(msg.Content)))/4 + 1
	}
	return total
}

func compactionMilestoneReached(messages []persistence.ChatMessage, summarizedCount int) bool {
	delta := len(messages) - summarizedCount
	if delta <= 0 {
		return false
	}
	return delta > compactionMinDeltaMessages || countToolOutputs(messages, summarizedCount) >= compactionToolMilestoneOutputs
}

func countToolOutputs(messages []persistence.ChatMessage, start int) int {
	if start < 0 || start >= len(messages) {
		return 0
	}
	count := 0
	for _, msg := range messages[start:] {
		if msg.Role == "tool" {
			count++
		}
	}
	return count
}

func summaryChunk(messages []persistence.ChatMessage, summarizedCount int) ([]persistence.ChatMessage, int, int, bool) {
	target := len(messages)
	summarizedCount = min(summarizedCount, target)
	if summarizedCount == target {
		return nil, target, summarizedCount, false
	}
	start := summarizedCount
	if start < 0 || start > target {
		start = 0
	}
	if target > start {
		target = adjustSummaryTargetForToolDeps(messages, start, target)
		if target <= start {
			return nil, target, summarizedCount, false
		}
	}
	chunk := messages[start:target]
	return chunk, target, summarizedCount, len(chunk) > 0
}

func adjustSummaryTargetForToolDeps(messages []persistence.ChatMessage, start, target int) int {
	adjustedTarget := adjustIndexForToolDeps(messages, start, target)
	if adjustedTarget < target {
		return adjustedTarget
	}
	return target
}

func logSummaryPlan(ctx context.Context, req summaryRequest, plan summaryPlan, summarizingCount int) {
	observability.LoggerWithTrace(ctx).Info().
		Str("session", req.Session.ID).
		Int("messages", len(req.Messages)).
		Int("effective_tokens", plan.effectiveTokens).
		Int("summary_tokens", plan.summaryTokens).
		Int("unsummarized_count", plan.unsummarizedCount).
		Int("token_budget", plan.budget).
		Int("context_window", plan.contextSize).
		Int("target_context_window", plan.targetContextSize).
		Int("plain_text_context_window", plan.plainContextSize).
		Int("reserve_buffer", plan.reserveBuffer).
		Int("summarizing_count", summarizingCount).
		Msg("summarization_triggered")
}

func summaryResult(plan summaryPlan, messageCount, summarizedCount int) *SummaryResult {
	return &SummaryResult{
		Triggered:           true,
		EstimatedTokens:     plan.effectiveTokens,
		TokenBudget:         plan.budget,
		ContextWindowTokens: plan.contextSize,
		ReserveBufferTokens: plan.reserveBuffer,
		MessageCount:        messageCount,
		SummarizedCount:     summarizedCount,
	}
}

func defaultIntValue(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func (m *Manager) resolveTargetContextWindowTokens(policy SummaryPolicy, targetModel string) int {
	ctxSize := policy.TargetContextWindowTokens
	if ctxSize <= 0 {
		ctxSize = m.contextWindowTokens
	}
	if ctxSize <= 0 && targetModel != "" {
		if size, _ := llm.ContextSize(targetModel); size > 0 {
			ctxSize = size
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000
	}
	return ctxSize
}

func (m *Manager) resolvePlainTextContextWindowTokens(policy SummaryPolicy) int {
	ctxSize := policy.PlainTextContextWindowTokens
	if ctxSize <= 0 {
		ctxSize = m.plainTextContextWindowTokens
	}
	if ctxSize <= 0 && m.summaryModel != "" {
		if size, _ := llm.ContextSize(m.summaryModel); size > 0 {
			ctxSize = size
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000
	}
	return ctxSize
}

func estimatePersistedSummaryTokens(summary string, compactionEnabled bool) int {
	if strings.TrimSpace(summary) == "" {
		return 0
	}
	ds := decodeDualSummary(summary)
	content := ds.Plain
	if compactionEnabled && ds.Compaction != "" {
		content = ds.Compaction
	}
	return estimateSummaryTextTokens(content)
}

func minPositiveInt(vals ...int) int {
	best := 0
	for _, value := range vals {
		if value <= 0 {
			continue
		}
		if best == 0 || value < best {
			best = value
		}
	}
	return best
}
