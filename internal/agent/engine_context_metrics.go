package agent

import (
	"context"
	"strings"

	"manifold/internal/llm"
)

const (
	ContextMetricPhaseAssembled      = "assembled"
	ContextMetricPhaseRuntimeAdded   = "runtime_added"
	ContextMetricPhaseMemoryAdded    = "memory_added"
	ContextMetricPhasePreModel       = "pre_model"
	ContextMetricPhaseAssistantAdded = "assistant_added"
	ContextMetricPhaseToolAdded      = "tool_added"
	ContextMetricPhaseSummarizing    = "summarizing"
	ContextMetricPhaseSummarized     = "summarized"

	ContextMetricKindSystem    = "system"
	ContextMetricKindHistory   = "history"
	ContextMetricKindUser      = "user"
	ContextMetricKindMemory    = "memory"
	ContextMetricKindTools     = "tools"
	ContextMetricKindSummary   = "summary"
	ContextMetricKindAssistant = "assistant"
)

type ContextMetricSegment struct {
	Kind   string
	Tokens int
}

type ContextMetrics struct {
	Phase            string
	InputTokens      int
	ContextWindow    int
	SummaryThreshold int
	ReserveTokens    int
	MessageCount     int
	SummarizedCount  int
	WillSummarize    bool
	Segments         []ContextMetricSegment
}

func (e *Engine) emitContextMetrics(ctx context.Context, msgs []llm.Message, phase string, extraSegments []ContextMetricSegment, summarizedCount int) {
	if e == nil || e.OnContextMetrics == nil || len(msgs) == 0 {
		return
	}
	cfg := e.summaryBudget()
	inputTokens := e.countMessagesTokens(ctx, msgs)
	e.OnContextMetrics(ContextMetrics{
		Phase:            phase,
		InputTokens:      inputTokens,
		ContextWindow:    cfg.contextSize,
		SummaryThreshold: cfg.tokenBudget,
		ReserveTokens:    cfg.reserveBuffer,
		MessageCount:     len(msgs),
		SummarizedCount:  summarizedCount,
		WillSummarize:    e.SummaryEnabled && inputTokens > cfg.tokenBudget,
		Segments:         e.contextMetricSegments(msgs, extraSegments),
	})
}

func (e *Engine) contextMetricSegments(msgs []llm.Message, extraSegments []ContextMetricSegment) []ContextMetricSegment {
	currentUser := latestUserMessageIndex(msgs)
	totals := map[string]int{}
	for i, msg := range msgs {
		kind := contextMetricKind(msg, i == currentUser)
		totals[kind] += llm.EstimateTokens(msg.Content) + 8
	}
	extraTotal := 0
	for _, segment := range extraSegments {
		if segment.Tokens <= 0 || strings.TrimSpace(segment.Kind) == "" {
			continue
		}
		totals[segment.Kind] += segment.Tokens
		extraTotal += segment.Tokens
	}
	if extraTotal > 0 && totals[ContextMetricKindUser] > 0 {
		totals[ContextMetricKindUser] = max(totals[ContextMetricKindUser]-extraTotal, 0)
	}
	orderedKinds := []string{
		ContextMetricKindSystem,
		ContextMetricKindHistory,
		ContextMetricKindSummary,
		ContextMetricKindUser,
		ContextMetricKindMemory,
		ContextMetricKindTools,
		ContextMetricKindAssistant,
	}
	segments := make([]ContextMetricSegment, 0, len(orderedKinds))
	for _, kind := range orderedKinds {
		if tokens := totals[kind]; tokens > 0 {
			segments = append(segments, ContextMetricSegment{Kind: kind, Tokens: tokens})
		}
	}
	return segments
}

func latestUserMessageIndex(msgs []llm.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return i
		}
	}
	return -1
}

func contextMetricKind(msg llm.Message, currentUser bool) string {
	switch msg.Role {
	case "system":
		return ContextMetricKindSystem
	case "tool":
		return ContextMetricKindTools
	case "assistant":
		if strings.HasPrefix(strings.TrimSpace(msg.Content), "[SUMMARY]") {
			return ContextMetricKindSummary
		}
		return ContextMetricKindAssistant
	case "user":
		if currentUser {
			return ContextMetricKindUser
		}
		return ContextMetricKindHistory
	default:
		return ContextMetricKindHistory
	}
}
