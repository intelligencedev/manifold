package memory

import (
	"context"
	"encoding/json"
	"errors"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/persistence"
	"strings"

	"github.com/rs/zerolog"
)

func (m *Manager) BuildContextForProvider(ctx context.Context, userID *int64, sessionID string, targetProvider llm.Provider, targetModel string, policy SummaryPolicy) ([]llm.Message, *SummaryResult, error) {
	log := observability.LoggerWithTrace(ctx)
	if sessionID == "" {
		return nil, nil, nil
	}
	targetCompactor, targetSupportsCompaction := llm.ProviderCompactor(targetProvider)
	messages, err := m.listContextMessages(ctx, userID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	log.Info().Str("session_id", sessionID).Int("messages_count", len(messages)).Msg("build_context_list_messages")

	session, err := m.contextSession(ctx, userID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	summary, summarizedCount, summaryResult := m.contextSummary(ctx, contextSummaryRequest{
		userID:          userID,
		session:         session,
		messages:        messages,
		targetCompactor: targetCompactor,
		targetModel:     targetModel,
		policy:          policy,
	})
	total := len(messages)
	tailStart := m.contextTailStart(messages, summarizedCount, summary, targetModel, policy)
	if total > 0 {
		tailStart = adjustedContextTailStart(log, sessionID, messages, summarizedCount, tailStart)
	}
	history := make([]llm.Message, 0, (total-tailStart)+1)
	if targetSupportsCompaction {
		history = append(history, llm.Message{Role: "system", Content: compactionContinuationRule})
	}
	history, tailStart = appendContextSummary(log, sessionID, history, summary, targetSupportsCompaction, tailStart)
	history = appendStoredContextMessages(log, history, messages[tailStart:])
	return history, summaryResult, nil
}

func (m *Manager) listContextMessages(ctx context.Context, userID *int64, sessionID string) ([]persistence.ChatMessage, error) {
	messages, err := m.store.ListMessages(ctx, userID, sessionID, 0)
	if errors.Is(err, persistence.ErrNotFound) {
		return nil, nil
	}
	return messages, err
}

func (m *Manager) contextSession(ctx context.Context, userID *int64, sessionID string) (persistence.ChatSession, error) {
	session, err := m.store.GetSession(ctx, userID, sessionID)
	if errors.Is(err, persistence.ErrNotFound) {
		return persistence.ChatSession{ID: sessionID}, nil
	}
	return session, err
}

type contextSummaryRequest struct {
	userID          *int64
	session         persistence.ChatSession
	messages        []persistence.ChatMessage
	targetCompactor llm.CompactionProvider
	targetModel     string
	policy          SummaryPolicy
}

func (m *Manager) contextSummary(ctx context.Context, req contextSummaryRequest) (string, int, *SummaryResult) {
	summary := req.session.Summary
	summarizedCount := req.session.SummarizedCount
	if !m.enabled {
		return summary, summarizedCount, nil
	}
	updatedSummary, updatedCount, result := m.ensureSummary(ctx, summaryRequest{
		UserID:          req.userID,
		Session:         req.session,
		Messages:        req.messages,
		TargetCompactor: req.targetCompactor,
		TargetModel:     req.targetModel,
		Policy:          req.policy,
	})
	if updatedSummary != "" || updatedCount != summarizedCount {
		summary = updatedSummary
		summarizedCount = updatedCount
	}
	if result != nil && result.Triggered {
		return summary, summarizedCount, result
	}
	return summary, summarizedCount, nil
}

func (m *Manager) contextTailStart(messages []persistence.ChatMessage, summarizedCount int, summary, targetModel string, policy SummaryPolicy) int {
	total := len(messages)
	if !m.enabled {
		return clampTailStart(0, total)
	}
	tailStart := m.tokenBudgetTailStart(messages, targetModel, policy)
	if tailStart < summarizedCount {
		tailStart = summarizedCount
	}
	maxTail := m.maxKeepLastMessages
	if maxTail > 0 && total-tailStart > maxTail {
		tailStart = max(total-maxTail, summarizedCount)
	}
	if summary == "" && summarizedCount > 0 {
		tailStart = 0
	}
	return clampTailStart(tailStart, total)
}

func (m *Manager) tokenBudgetTailStart(messages []persistence.ChatMessage, targetModel string, policy SummaryPolicy) int {
	tailBudget := m.tailTokenBudget(targetModel, policy)
	minTail := max(m.minKeepLastMessages, 4)
	remaining := tailBudget
	kept := 0
	tailStart := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := len([]rune(strings.TrimSpace(messages[i].Content)))/4 + 1
		if kept >= minTail && remaining-msgTokens <= 0 {
			break
		}
		remaining -= msgTokens
		kept++
		tailStart = i
		if remaining <= 0 {
			break
		}
	}
	return tailStart
}

func (m *Manager) tailTokenBudget(targetModel string, policy SummaryPolicy) int {
	ctxSize := m.resolveTargetContextWindowTokens(policy, targetModel)
	reserveBuffer := m.reserveBufferTokens
	if reserveBuffer <= 0 {
		reserveBuffer = defaultReserveBuffer
	}
	budget := ctxSize - reserveBuffer
	if budget <= 0 {
		budget = ctxSize / 2
	}
	tailBudget := budget / 2
	if tailBudget <= 0 {
		return budget
	}
	return tailBudget
}

func clampTailStart(tailStart, total int) int {
	if tailStart < 0 {
		return 0
	}
	if tailStart > total {
		return total
	}
	return tailStart
}

func adjustedContextTailStart(log interface {
	Warn() *zerolog.Event
}, sessionID string, messages []persistence.ChatMessage, summarizedCount, tailStart int) int {
	adjusted := adjustIndexForToolDeps(messages, 0, tailStart)
	if adjusted >= tailStart {
		return tailStart
	}
	if adjusted < max(summarizedCount, 0) {
		log.Warn().
			Str("session_id", sessionID).
			Int("summarized_count", summarizedCount).
			Int("tail_start", tailStart).
			Int("adjusted_tail_start", adjusted).
			Msg("tail_start_crosses_tool_chain_including_pre_summarized_tool_calls")
	}
	return adjusted
}

func appendContextSummary(log interface {
	Warn() *zerolog.Event
}, sessionID string, history []llm.Message, summary string, targetSupportsCompaction bool, tailStart int) ([]llm.Message, int) {
	if summary == "" {
		return history, tailStart
	}
	ds := decodeDualSummary(summary)
	switch {
	case targetSupportsCompaction && ds.Compaction != "":
		return appendCompactionSummary(history, ds), tailStart
	case ds.Plain != "":
		return appendPlainSummary(history, ds.Plain), tailStart
	case ds.Compaction != "" && !targetSupportsCompaction:
		log.Warn().Str("session_id", sessionID).Msg("compaction_summary_incompatible_with_target_provider_no_plain_fallback")
		return history, 0
	default:
		return history, tailStart
	}
}

func appendCompactionSummary(history []llm.Message, ds dualSummary) []llm.Message {
	if item, ok := decodeCompactionSummary(ds.Compaction); ok {
		return append(history, llm.Message{Role: "assistant", Compaction: &item})
	}
	if ds.Plain != "" {
		return appendPlainSummary(history, ds.Plain)
	}
	return history
}

func appendPlainSummary(history []llm.Message, plain string) []llm.Message {
	return append(history, llm.Message{
		Role:    "system",
		Content: "Conversation summary (for context only):\n" + plain,
	})
}

func appendStoredContextMessages(log interface {
	Debug() *zerolog.Event
}, history []llm.Message, messages []persistence.ChatMessage) []llm.Message {
	for i, msg := range messages {
		log.Debug().Int("index", i).Str("role", msg.Role).Int("content_len", len(msg.Content)).Str("content_preview", truncate(msg.Content, 100)).Msg("build_context_message")
		history = append(history, storedContextMessage(msg))
	}
	return history
}

func storedContextMessage(msg persistence.ChatMessage) llm.Message {
	if decoded, ok := decodeAssistantContextMessage(msg); ok {
		return decoded
	}
	if decoded, ok := decodeToolContextMessage(msg); ok {
		return decoded
	}
	return llm.Message{Role: msg.Role, Content: msg.Content}
}

func decodeAssistantContextMessage(msg persistence.ChatMessage) (llm.Message, bool) {
	if msg.Role != "assistant" || !strings.HasPrefix(strings.TrimSpace(msg.Content), "{") {
		return llm.Message{}, false
	}
	var data struct {
		Content   string         `json:"content"`
		ToolCalls []llm.ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(msg.Content), &data); err != nil || len(data.ToolCalls) == 0 {
		return llm.Message{}, false
	}
	return llm.Message{Role: msg.Role, Content: data.Content, ToolCalls: data.ToolCalls}, true
}

func decodeToolContextMessage(msg persistence.ChatMessage) (llm.Message, bool) {
	if msg.Role != "tool" || !strings.HasPrefix(strings.TrimSpace(msg.Content), "{") {
		return llm.Message{}, false
	}
	var data struct {
		Content string `json:"content"`
		ToolID  string `json:"tool_id"`
	}
	if err := json.Unmarshal([]byte(msg.Content), &data); err != nil || data.ToolID == "" {
		return llm.Message{}, false
	}
	return llm.Message{Role: msg.Role, Content: data.Content, ToolID: data.ToolID}, true
}

// adjustIndexForToolDeps ensures that if the kept tail includes any tool response
// messages, it also includes the preceding assistant message(s) that contain the
// corresponding ToolCalls.
//
// This prevents provider-specific request validation failures (e.g., Anthropic
// requires tool_result to reference a prior tool_use) and avoids losing metadata
// chains required by some providers (e.g., Gemini thought signatures).
//
