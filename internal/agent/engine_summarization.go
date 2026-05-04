package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"strings"
)

func (e *Engine) consumeSkipInitialSummarization() bool {
	if !e.SkipInitialSummarization {
		return false
	}
	e.SkipInitialSummarization = false
	return true
}

func (e *Engine) maybeSummarize(ctx context.Context, msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}

	// Determine context window size
	ctxSize := e.ContextWindowTokens
	if ctxSize <= 0 {
		if sz, _ := llm.ContextSize(e.model()); sz > 0 {
			ctxSize = sz
		}
	}
	if ctxSize <= 0 {
		ctxSize = 128_000 // Conservative default for modern models
	}

	// Reserve buffer for output tokens (including reasoning tokens for reasoning models)
	// OpenAI recommends ~25,000 when experimenting with reasoning models
	reserveBuffer := e.SummaryReserveBufferTokens
	if reserveBuffer <= 0 {
		reserveBuffer = 25_000
	}

	minTail := e.SummaryMinKeepLastMessages
	if minTail <= 0 {
		minTail = 4
	}

	// Calculate available budget for input
	tokenBudget := ctxSize - reserveBuffer
	if tokenBudget <= 0 {
		tokenBudget = ctxSize / 2 // Fallback if reserve is too large
	}

	// Count actual input tokens
	inputTokens := e.countMessagesTokens(ctx, msgs)
	if inputTokens <= tokenBudget {
		// No summarization needed; fits within budget
		return msgs
	}

	log := observability.LoggerWithTrace(ctx)
	log.Info().
		Int("messages", len(msgs)).
		Int("input_tokens", inputTokens).
		Int("token_budget", tokenBudget).
		Int("context_window", ctxSize).
		Int("reserve_buffer", reserveBuffer).
		Msg("summarization_triggered")

	// Preserve leading system message if present
	start := 0
	var sysMsg *llm.Message
	if msgs[0].Role == "system" {
		sysMsg = &msgs[0]
		start = 1
	}

	// Work backwards from the end, keeping as many recent messages as will fit
	// in roughly half the budget (leaving room for system prompt, tools, and summary)
	recent := make([]llm.Message, 0, len(msgs))
	remaining := tokenBudget / 2
	for i := len(msgs) - 1; i >= start; i-- {
		msgTokens := e.countTokens(ctx, msgs[i].Content)
		if len(recent) >= minTail && remaining-msgTokens <= 0 {
			break
		}
		recent = append(recent, msgs[i])
		remaining -= msgTokens
		if remaining <= 0 {
			break
		}
	}

	// Restore chronological order (we appended in reverse)
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}

	// Everything between start and the beginning of recent becomes summary input
	cutIndex := len(msgs) - len(recent)
	if cutIndex < start {
		cutIndex = start
	}
	cutIndex = e.adjustCutIndexForToolDeps(msgs, start, cutIndex)
	cutIndex = e.adjustCutIndexForLatestUser(msgs, start, cutIndex)
	if cutIndex < start {
		cutIndex = start
	}
	// If we adjusted the cut index, expand the recent tail to keep message order.
	recent = msgs[cutIndex:]
	toSummarize := msgs[start:cutIndex]
	if len(toSummarize) == 0 {
		return msgs
	}

	// Notify callback that summarization is occurring
	if e.OnSummaryTriggered != nil {
		e.OnSummaryTriggered(inputTokens, tokenBudget, len(msgs), len(toSummarize))
	}

	return e.buildSummarizedMessages(ctx, sysMsg, toSummarize, recent, len(recent))
}

// adjustCutIndexForToolDeps ensures that if the kept "recent" tail includes any
// tool response messages, it also includes the preceding assistant message(s)
// that contain the corresponding ToolCalls.
//
// This matters for providers like Gemini 3 where tool responses may need to
// echo provider-specific metadata (e.g., thought signatures) that are carried on
// the original ToolCall message. Summarization must not split that chain.
func (e *Engine) adjustCutIndexForToolDeps(msgs []llm.Message, start, cutIndex int) int {
	if cutIndex <= start || cutIndex >= len(msgs) {
		return cutIndex
	}

	required := make(map[string]struct{})
	for i := cutIndex; i < len(msgs); i++ {
		if msgs[i].Role == "tool" {
			id := strings.TrimSpace(msgs[i].ToolID)
			if id != "" {
				required[id] = struct{}{}
			}
		}
	}
	if len(required) == 0 {
		return cutIndex
	}

	earliestNeeded := cutIndex
	for toolID := range required {
		foundIdx := -1
		for i := cutIndex - 1; i >= start; i-- {
			if msgs[i].Role != "assistant" {
				continue
			}
			for _, tc := range msgs[i].ToolCalls {
				if strings.TrimSpace(tc.ID) == toolID {
					foundIdx = i
					break
				}
			}
			if foundIdx != -1 {
				break
			}
		}
		if foundIdx != -1 && foundIdx < earliestNeeded {
			earliestNeeded = foundIdx
		}
	}

	return earliestNeeded
}

// adjustCutIndexForLatestUser ensures the kept "recent" tail contains at least
// one user message when one exists. Some model/provider prompt templates expect
// a user query to be present and can fail when the tail contains only
// assistant/tool turns after summarization.
func (e *Engine) adjustCutIndexForLatestUser(msgs []llm.Message, start, cutIndex int) int {
	if cutIndex <= start || cutIndex >= len(msgs) {
		return cutIndex
	}

	latestUserIdx := -1
	for i := len(msgs) - 1; i >= start; i-- {
		if msgs[i].Role == "user" {
			latestUserIdx = i
			break
		}
	}

	if latestUserIdx == -1 || latestUserIdx >= cutIndex {
		return cutIndex
	}

	return latestUserIdx
}

// buildSummarizedMessages constructs a summary prompt, calls the LLM, and
// returns the new message list (system + [summary] + recent).
func (e *Engine) buildSummarizedMessages(
	ctx context.Context,
	sysMsg *llm.Message,
	toSummarize []llm.Message,
	recent []llm.Message,
	keep int,
) []llm.Message {
	maxChunkTokens := e.SummaryMaxSummaryChunkTokens
	if maxChunkTokens <= 0 {
		maxChunkTokens = 4096
	}

	var b strings.Builder
	currentTokens := 0
	for _, m := range toSummarize {
		// Approximate token cost per message and cap at maxChunkTokens.
		msgTokens := e.countTokens(ctx, m.Content) + 8 // overhead for role/formatting
		if currentTokens+msgTokens > maxChunkTokens {
			break
		}
		b.WriteString("Role: ")
		b.WriteString(m.Role)
		b.WriteString("\n")
		content := m.Content
		// Hard safety cap in characters as a backstop.
		if len(content) > maxChunkTokens*4 {
			content = content[:maxChunkTokens*4] + "\n[TRUNCATED]"
		}
		b.WriteString(content)
		b.WriteString("\n\n")
		currentTokens += msgTokens
	}

	sys := "You are a concise summarizer. Produce a short, factual summary (<= 300 characters) of the conversation that follows. Keep important facts, omit chit-chat. Return only the summary text."
	user := "Summarize the following conversation:\n\n" + b.String()

	summReq := []llm.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}}
	summReq = e.enforceContextBudget(ctx, summReq)
	sumMsg, err := e.LLM.Chat(ctx, summReq, nil, e.model())
	if err != nil {
		observability.LoggerWithTrace(ctx).Error().Err(err).Msg("summary_failed")
		return append([]llm.Message{}, append(toSummarize, recent...)...)
	}

	summaryContent := "[SUMMARY] " + strings.TrimSpace(sumMsg.Content)
	summary := llm.Message{Role: "assistant", Content: summaryContent}

	newMsgs := make([]llm.Message, 0, 1+keep+2)
	if sysMsg != nil {
		newMsgs = append(newMsgs, *sysMsg)
	}
	newMsgs = append(newMsgs, summary)
	newMsgs = append(newMsgs, recent...)

	observability.LoggerWithTrace(ctx).Info().
		Int("orig_messages", len(toSummarize)+len(recent)).
		Int("new_messages", len(newMsgs)).
		Msg("history_summarized")
	return newMsgs
}

// augmentWithMemory appends evolving memory context to the system prompt (ExpRAG or ExpRecent).
