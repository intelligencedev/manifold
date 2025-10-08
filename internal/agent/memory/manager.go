package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/persistence"
)

const (
	defaultThreshold      = 40
	defaultKeepLast       = 12
	maxSummarizeChunkSize = 4096
)

// Config tunes how chat history should be summarized before being sent to the LLM.
type Config struct {
	Enabled      bool
	Threshold    int
	KeepLast     int
	SummaryModel string
}

// Manager coordinates persistence-backed chat memory with rolling summaries so that
// orchestrator runs reuse prior turns without resending the full transcript.
type Manager struct {
	store        persistence.ChatStore
	summary      llm.Provider
	summaryModel string

	enabled   bool
	threshold int
	keepLast  int
}

// NewManager returns a chat memory manager.
func NewManager(store persistence.ChatStore, provider llm.Provider, cfg Config) *Manager {
	m := &Manager{
		store:        store,
		summary:      provider,
		summaryModel: cfg.SummaryModel,
		enabled:      cfg.Enabled && provider != nil,
		threshold:    cfg.Threshold,
		keepLast:     cfg.KeepLast,
	}
	if m.threshold <= 0 {
		m.threshold = defaultThreshold
	}
	if m.keepLast <= 0 {
		m.keepLast = defaultKeepLast
	}
	return m
}

// BuildContext assembles the conversation history that should be sent to the orchestrator
// by combining a persisted summary (if any) with the most recent chat turns.
func (m *Manager) BuildContext(ctx context.Context, userID *int64, sessionID string) ([]llm.Message, error) {
	if sessionID == "" {
		return nil, nil
	}

	messages, err := m.store.ListMessages(ctx, userID, sessionID, 0)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			messages = nil
		} else {
			return nil, err
		}
	}

	session, err := m.store.GetSession(ctx, userID, sessionID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			// Session is guaranteed to exist via ensureChatSession but guard anyway.
			session = persistence.ChatSession{ID: sessionID}
		} else {
			return nil, err
		}
	}

	summary := session.Summary
	summarizedCount := session.SummarizedCount

	if m.enabled {
		updatedSummary, updatedCount := m.ensureSummary(ctx, userID, session, messages)
		if updatedSummary != "" || updatedCount != summarizedCount {
			summary = updatedSummary
			summarizedCount = updatedCount
		}
	}

	total := len(messages)
	tailStart := 0
	if m.enabled {
		minTail := total - m.keepLast
		if minTail < 0 {
			minTail = 0
		}
		tailStart = summarizedCount
		if tailStart < minTail {
			tailStart = minTail
		}
		// If summarization failed we fall back to sending the full history.
		if summary == "" && summarizedCount > 0 {
			tailStart = 0
		}
	}

	if tailStart < 0 {
		tailStart = 0
	}
	if tailStart > total {
		tailStart = total
	}

	history := make([]llm.Message, 0, (total-tailStart)+1)
	if summary != "" {
		history = append(history, llm.Message{
			Role:    "system",
			Content: "Conversation summary (for context only):\n" + summary,
		})
	}
	for _, msg := range messages[tailStart:] {
		history = append(history, llm.Message{Role: msg.Role, Content: msg.Content})
	}
	return history, nil
}

func (m *Manager) ensureSummary(ctx context.Context, userID *int64, session persistence.ChatSession, messages []persistence.ChatMessage) (string, int) {
	if !m.enabled || m.summary == nil {
		return session.Summary, session.SummarizedCount
	}

	total := len(messages)
	if total <= m.threshold {
		return session.Summary, session.SummarizedCount
	}

	target := total - m.keepLast
	if target <= 0 {
		return session.Summary, session.SummarizedCount
	}

	summarizedCount := session.SummarizedCount
	if summarizedCount > target {
		summarizedCount = target
	}

	if summarizedCount == target {
		return session.Summary, summarizedCount
	}

	start := summarizedCount
	if start < 0 || start > target {
		start = 0
	}

	chunk := messages[start:target]
	if len(chunk) == 0 {
		return session.Summary, summarizedCount
	}

	summary, err := m.summarizeChunk(ctx, session.Summary, chunk)
	if err != nil {
		observability.LoggerWithTrace(ctx).Error().Err(err).Str("session", session.ID).Msg("chat_summary_failed")
		return session.Summary, summarizedCount
	}

	if err := m.store.UpdateSummary(ctx, userID, session.ID, summary, target); err != nil {
		observability.LoggerWithTrace(ctx).Error().Err(err).Str("session", session.ID).Msg("chat_summary_persist_failed")
		return session.Summary, summarizedCount
	}

	observability.LoggerWithTrace(ctx).Info().Str("session", session.ID).Int("messages", target).Msg("chat_summary_updated")
	return summary, target
}

func (m *Manager) summarizeChunk(ctx context.Context, existingSummary string, chunk []persistence.ChatMessage) (string, error) {
	if m.summary == nil {
		return existingSummary, fmt.Errorf("llm provider unavailable")
	}

	var userPrompt strings.Builder
	userPrompt.WriteString("Update the running summary of this chat.\n")
	if strings.TrimSpace(existingSummary) != "" {
		userPrompt.WriteString("\nExisting summary:\n")
		userPrompt.WriteString(strings.TrimSpace(existingSummary))
		userPrompt.WriteString("\n\n")
	}
	userPrompt.WriteString("New conversation turns:\n")

	for _, msg := range chunk {
		userPrompt.WriteString("\nRole: ")
		userPrompt.WriteString(msg.Role)
		userPrompt.WriteString("\n")
		content := strings.TrimSpace(msg.Content)
		if len(content) > maxSummarizeChunkSize {
			content = content[:maxSummarizeChunkSize] + "\n[TRUNCATED]"
		}
		userPrompt.WriteString(content)
		userPrompt.WriteString("\n")
	}

	userPrompt.WriteString("\nReturn only the updated concise summary (<= 300 characters).")

	sysPrompt := "You are a concise summarizer. Maintain an accurate running summary of a conversation."

	msgs := []llm.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPrompt.String()},
	}

	resp, err := m.summary.Chat(ctx, msgs, nil, m.summaryModel)
	if err != nil {
		return existingSummary, fmt.Errorf("summarize chat: %w", err)
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return existingSummary, fmt.Errorf("empty summary returned")
	}
	return summary, nil
}
