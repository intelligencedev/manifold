package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/persistence"
	"strings"
)

func (m *Manager) compactChunk(ctx context.Context, compactor llm.CompactionProvider, targetModel string, existingSummary string, chunk []persistence.ChatMessage) (string, error) {
	if compactor == nil {
		observability.LoggerWithTrace(ctx).Warn().Msg("responses_compaction_unavailable")
		return existingSummary, nil
	}
	if strings.TrimSpace(targetModel) == "" {
		targetModel = m.summaryModel
	}

	var prev *llm.CompactionItem
	if item, ok := decodeCompactionSummary(existingSummary); ok {
		prev = &item
	}

	msgs := make([]llm.Message, 0, len(chunk)+1)
	if prev == nil && strings.TrimSpace(existingSummary) != "" {
		msgs = append(msgs, llm.Message{Role: "assistant", Content: strings.TrimSpace(existingSummary)})
	}
	for _, msg := range chunk {
		msgs = append(msgs, decodePersistedChatMessage(msg))
	}

	// Preflight: the compaction request itself must fit in the model context
	// window. Treat compaction items as opaque; we only trim inputs we send.
	// This is best-effort because we may not have an exact tokenizer here.
	ctxSize := m.contextWindowTokens
	if ctxSize <= 0 && m.summaryModel != "" {
		if size, _ := llm.ContextSize(m.summaryModel); size > 0 {
			ctxSize = size
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000
	}
	// Keep a smaller reserve for the compact request; it doesn't need a huge
	// reasoning/output budget, but we still want headroom.
	reserve := m.reserveBufferTokens
	if reserve <= 0 {
		reserve = defaultReserveBuffer
	}
	if reserve > ctxSize/2 {
		reserve = ctxSize / 2
	}
	inputBudget := ctxSize - reserve
	if inputBudget <= 0 {
		inputBudget = ctxSize / 2
	}

	// First, truncate individual message contents to avoid pathological tool logs.
	// Use the same limit knob as summary chunks (token-ish, via chars backstop).
	perMsgLimit := maxSummarizeChunkSize
	if m.maxSummaryChunkTokens > 0 {
		perMsgLimit = m.maxSummaryChunkTokens
	}
	for i := range msgs {
		if strings.TrimSpace(msgs[i].Content) == "" {
			continue
		}
		msgs[i].Content = truncateForSummary(msgs[i].Content, perMsgLimit)
	}

	// If we still exceed the input budget, drop oldest delta messages (keeping the
	// most recent context) and rely on previous compaction state.
	for estimateMessagesTokens(msgs) > inputBudget && len(msgs) > 1 {
		msgs = msgs[1:]
	}

	item, err := compactor.Compact(ctx, msgs, targetModel, prev)
	if err != nil {
		if isCompactionEndpointUnavailable(err) {
			m.compactionUnavailable.Store(true)
			observability.LoggerWithTrace(ctx).Warn().Err(err).Msg("responses_compaction_disabled")
		}
		return existingSummary, err
	}
	if item == nil || strings.TrimSpace(item.EncryptedContent) == "" {
		return existingSummary, fmt.Errorf("responses compaction returned empty content")
	}
	encoded := encodeCompactionSummary(*item)
	if encoded == "" {
		return existingSummary, fmt.Errorf("responses compaction encode failed")
	}
	return encoded, nil
}

func isCompactionEndpointUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "404") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "not_found") ||
		strings.Contains(msg, "responses api required for compaction")
}

func decodeCompactionSummary(summary string) (llm.CompactionItem, bool) {
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return llm.CompactionItem{}, false
	}
	var payload struct {
		Type             string `json:"type"`
		ID               string `json:"id,omitempty"`
		EncryptedContent string `json:"encrypted_content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return llm.CompactionItem{}, false
	}
	if payload.Type != compactionSummaryType || strings.TrimSpace(payload.EncryptedContent) == "" {
		return llm.CompactionItem{}, false
	}
	return llm.CompactionItem{ID: payload.ID, EncryptedContent: payload.EncryptedContent}, true
}

func encodeCompactionSummary(item llm.CompactionItem) string {
	payload := struct {
		Type             string `json:"type"`
		ID               string `json:"id,omitempty"`
		EncryptedContent string `json:"encrypted_content"`
	}{
		Type:             compactionSummaryType,
		ID:               item.ID,
		EncryptedContent: item.EncryptedContent,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(raw)
}
