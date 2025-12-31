package memory

import (
	"context"
	"encoding/json"
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
	compactionSummaryType = "compaction"
)

// MemoryMode controls how chat history is summarized.
type MemoryMode string

const (
	MemoryModeFixed MemoryMode = "fixed"
	MemoryModeAuto  MemoryMode = "auto"
)

// Config tunes how chat history should be summarized before being sent to the LLM.
//
// In fixed mode, Threshold/KeepLast behave as before. In auto mode, the manager
// attempts to size the history based on the model's context window.
type Config struct {
	Enabled   bool       `yaml:"enabled" json:"enabled"`
	Mode      MemoryMode `yaml:"mode" json:"mode"`
	Threshold int        `yaml:"threshold" json:"threshold"`
	KeepLast  int        `yaml:"keepLast" json:"keepLast"`

	// Auto‑mode fields
	TargetUtilizationPct  float64 `yaml:"targetUtilizationPct" json:"targetUtilizationPct"`
	MinKeepLastMessages   int     `yaml:"minKeepLastMessages" json:"minKeepLastMessages"`
	MaxSummaryChunkTokens int     `yaml:"maxSummaryChunkTokens" json:"maxSummaryChunkTokens"`
	// ContextWindowTokens can be pre‑computed by the caller; if 0, ContextSize
	// is used as a fallback.
	ContextWindowTokens int `yaml:"contextWindowTokens" json:"contextWindowTokens"`

	SummaryModel string `yaml:"summaryModel" json:"summaryModel"`
	// UseResponsesCompaction enables the Responses API compaction endpoint when supported.
	UseResponsesCompaction bool `yaml:"useResponsesCompaction" json:"useResponsesCompaction"`
}

// Manager coordinates persistence-backed chat memory with rolling summaries so that
// orchestrator runs reuse prior turns without resending the full transcript.
type Manager struct {
	store        persistence.ChatStore
	summary      llm.Provider
	summaryModel string

	enabled bool
	mode    MemoryMode

	threshold int
	keepLast  int

	// auto‑mode fields
	targetUtilizationPct   float64
	minKeepLastMessages    int
	maxSummaryChunkTokens  int
	contextWindowTokens    int
	useResponsesCompaction bool
}

// Introspection helpers used by debug/observability surfaces.

// Mode returns the current memory mode (fixed or auto).
func (m *Manager) Mode() MemoryMode { return m.mode }

// ContextWindowTokens returns the approximate context window size (in tokens)
// used for auto-mode budgeting.
func (m *Manager) ContextWindowTokens() int { return m.contextWindowTokens }

// TargetUtilizationPct returns the target fraction of the context window the
// manager aims to occupy with conversation history.
func (m *Manager) TargetUtilizationPct() float64 { return m.targetUtilizationPct }

// MinKeepLastMessages returns the minimum number of tail messages preserved
// in raw form when summarizing.
func (m *Manager) MinKeepLastMessages() int { return m.minKeepLastMessages }

// MaxSummaryChunkTokens returns the maximum token budget used when building
// summary chunks.
func (m *Manager) MaxSummaryChunkTokens() int { return m.maxSummaryChunkTokens }

// NewManager returns a chat memory manager.
func NewManager(store persistence.ChatStore, provider llm.Provider, cfg Config) *Manager {
	m := &Manager{
		store:                  store,
		summary:                provider,
		summaryModel:           cfg.SummaryModel,
		enabled:                cfg.Enabled && provider != nil,
		mode:                   cfg.Mode,
		threshold:              cfg.Threshold,
		keepLast:               cfg.KeepLast,
		targetUtilizationPct:   cfg.TargetUtilizationPct,
		minKeepLastMessages:    cfg.MinKeepLastMessages,
		maxSummaryChunkTokens:  cfg.MaxSummaryChunkTokens,
		contextWindowTokens:    cfg.ContextWindowTokens,
		useResponsesCompaction: cfg.UseResponsesCompaction,
	}
	if m.threshold <= 0 {
		m.threshold = defaultThreshold
	}
	if m.keepLast <= 0 {
		m.keepLast = defaultKeepLast
	}
	if m.mode == "" {
		m.mode = MemoryModeFixed
	}
	if m.targetUtilizationPct <= 0 || m.targetUtilizationPct > 1 {
		m.targetUtilizationPct = 0.7
	}
	if m.minKeepLastMessages <= 0 {
		m.minKeepLastMessages = 4
	}
	if m.maxSummaryChunkTokens <= 0 {
		m.maxSummaryChunkTokens = maxSummarizeChunkSize
	}
	if m.contextWindowTokens <= 0 && cfg.SummaryModel != "" {
		if size, _ := llm.ContextSize(cfg.SummaryModel); size > 0 {
			m.contextWindowTokens = size
		}
	}
	return m
}

// BuildContext assembles the conversation history that should be sent to the orchestrator
// by combining a persisted summary (if any) with the most recent chat turns.
func (m *Manager) BuildContext(ctx context.Context, userID *int64, sessionID string) ([]llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
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
	log.Info().Str("session_id", sessionID).Int("messages_count", len(messages)).Msg("build_context_list_messages")

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
		if m.mode == MemoryModeAuto {
			// In auto mode, choose the tail based on an approximate token budget
			// rather than a fixed message count.
			ctxSize := m.contextWindowTokens
			if ctxSize <= 0 {
				ctxSize = 32_000
			}
			budget := int(float64(ctxSize) * m.targetUtilizationPct)
			if budget <= 0 {
				budget = ctxSize / 2
			}

			// Reserve roughly half the budget for the tail; the rest is for
			// system prompts, tools, and the summary itself.
			tailBudget := budget / 2
			if tailBudget <= 0 {
				tailBudget = budget
			}

			minTail := m.minKeepLastMessages
			if minTail <= 0 {
				minTail = 4
			}

			remaining := tailBudget
			kept := 0
			tailStart = total
			for i := total - 1; i >= 0; i-- {
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
			// Never include messages that have already been summarized.
			if tailStart < summarizedCount {
				tailStart = summarizedCount
			}
		} else {
			// Fixed mode: original behavior based on KeepLast.
			minTail := total - m.keepLast
			if minTail < 0 {
				minTail = 0
			}
			tailStart = summarizedCount
			if tailStart < minTail {
				tailStart = minTail
			}
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
		if m.useResponsesCompaction {
			if item, ok := decodeCompactionSummary(summary); ok {
				history = append(history, llm.Message{Role: "assistant", Compaction: &item})
			} else {
				history = append(history, llm.Message{
					Role:    "system",
					Content: "Conversation summary (for context only):\n" + summary,
				})
			}
		} else {
			history = append(history, llm.Message{
				Role:    "system",
				Content: "Conversation summary (for context only):\n" + summary,
			})
		}
	}
	for i, msg := range messages[tailStart:] {
		log.Debug().Int("index", i).Str("role", msg.Role).Int("content_len", len(msg.Content)).Str("content_preview", truncate(msg.Content, 100)).Msg("build_context_message")
		// Deserialize JSON-encoded messages (assistant with tool calls, tool messages)
		if msg.Role == "assistant" && strings.HasPrefix(strings.TrimSpace(msg.Content), "{") {
			var data struct {
				Content   string         `json:"content"`
				ToolCalls []llm.ToolCall `json:"tool_calls"`
			}
			if err := json.Unmarshal([]byte(msg.Content), &data); err == nil && len(data.ToolCalls) > 0 {
				history = append(history, llm.Message{
					Role:      msg.Role,
					Content:   data.Content,
					ToolCalls: data.ToolCalls,
				})
				continue
			}
		} else if msg.Role == "tool" && strings.HasPrefix(strings.TrimSpace(msg.Content), "{") {
			var data struct {
				Content string `json:"content"`
				ToolID  string `json:"tool_id"`
			}
			if err := json.Unmarshal([]byte(msg.Content), &data); err == nil && data.ToolID != "" {
				history = append(history, llm.Message{
					Role:    msg.Role,
					Content: data.Content,
					ToolID:  data.ToolID,
				})
				continue
			}
		}
		// Fallback: plain message
		history = append(history, llm.Message{Role: msg.Role, Content: msg.Content})
	}
	return history, nil
}

func (m *Manager) ensureSummary(ctx context.Context, userID *int64, session persistence.ChatSession, messages []persistence.ChatMessage) (string, int) {
	if !m.enabled || m.summary == nil {
		return session.Summary, session.SummarizedCount
	}

	total := len(messages)
	if total == 0 {
		return session.Summary, session.SummarizedCount
	}

	// Fixed mode preserves historical behavior based on message counts.
	if m.mode == MemoryModeFixed {
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

	// Auto mode: estimate token usage and roll summary incrementally based on
	// token budget rather than message counts.
	ctxSize := m.contextWindowTokens
	if ctxSize <= 0 && m.summaryModel != "" {
		if size, _ := llm.ContextSize(m.summaryModel); size > 0 {
			ctxSize = size
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000
	}

	budget := int(float64(ctxSize) * m.targetUtilizationPct)
	if budget <= 0 {
		budget = ctxSize / 2
	}

	estimated := 0
	for _, msg := range messages {
		estimated += len([]rune(strings.TrimSpace(msg.Content)))/4 + 1
	}
	if estimated <= budget {
		return session.Summary, session.SummarizedCount
	}

	// Decide how many early messages to include in the next summary chunk so we
	// keep at least a small tail in raw form.
	minTail := m.minKeepLastMessages
	if minTail <= 0 {
		minTail = 4
	}
	if total <= minTail {
		return session.Summary, session.SummarizedCount
	}

	target := total - minTail
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
	if m.useResponsesCompaction {
		return m.compactChunk(ctx, existingSummary, chunk)
	}

	var userPrompt strings.Builder
	userPrompt.WriteString("Update the running summary of this chat. Keep it concise but information-dense.\n")
	userPrompt.WriteString("Preserve user goals, preferences, decisions, key facts, identifiers (files, URLs, IDs), tool results/errors, and open questions.\n")
	userPrompt.WriteString("If content includes [TRUNCATED], assume important details may be missing.\n")
	if strings.TrimSpace(existingSummary) != "" {
		userPrompt.WriteString("\nExisting summary:\n")
		userPrompt.WriteString(strings.TrimSpace(existingSummary))
		userPrompt.WriteString("\n\n")
	}
	userPrompt.WriteString("New conversation turns:\n")

	for _, msg := range chunk {
		summaryMsg := buildSummaryPromptMessage(msg)
		userPrompt.WriteString("\nRole: ")
		userPrompt.WriteString(summaryMsg.Role)
		userPrompt.WriteString("\n")
		if len(summaryMsg.ToolCalls) > 0 {
			userPrompt.WriteString("Tool calls: ")
			userPrompt.WriteString(strings.Join(summaryMsg.ToolCalls, ", "))
			userPrompt.WriteString("\n")
		}
		if strings.TrimSpace(summaryMsg.ToolID) != "" {
			userPrompt.WriteString("Tool ID: ")
			userPrompt.WriteString(summaryMsg.ToolID)
			userPrompt.WriteString("\n")
		}
		content := strings.TrimSpace(summaryMsg.Content)
		limit := maxSummarizeChunkSize
		if m.maxSummaryChunkTokens > 0 {
			limit = m.maxSummaryChunkTokens
		}
		content = truncateForSummary(content, limit)
		if content == "" {
			content = "(no content)"
		}
		userPrompt.WriteString(content)
		userPrompt.WriteString("\n")
	}

	userPrompt.WriteString("\nReturn only the updated summary. Aim for <= 1200 characters; use short bullets if helpful.")

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

type summaryPromptMessage struct {
	Role      string
	Content   string
	ToolCalls []string
	ToolID    string
}

func buildSummaryPromptMessage(msg persistence.ChatMessage) summaryPromptMessage {
	out := summaryPromptMessage{
		Role:    msg.Role,
		Content: strings.TrimSpace(msg.Content),
	}
	raw := strings.TrimSpace(msg.Content)
	if raw == "" {
		return out
	}
	if msg.Role == "assistant" && strings.HasPrefix(raw, "{") {
		var data struct {
			Content   string         `json:"content"`
			ToolCalls []llm.ToolCall `json:"tool_calls"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err == nil && len(data.ToolCalls) > 0 {
			out.Content = strings.TrimSpace(data.Content)
			out.ToolCalls = summarizeToolCalls(data.ToolCalls)
			return out
		}
	}
	if msg.Role == "tool" && strings.HasPrefix(raw, "{") {
		var data struct {
			Content string `json:"content"`
			ToolID  string `json:"tool_id"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err == nil {
			if strings.TrimSpace(data.Content) != "" {
				out.Content = strings.TrimSpace(data.Content)
			} else {
				out.Content = ""
			}
			out.ToolID = strings.TrimSpace(data.ToolID)
			return out
		}
	}
	return out
}

func summarizeToolCalls(calls []llm.ToolCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		args := strings.TrimSpace(string(call.Args))
		if isEmptySummaryArgs(args) {
			out = append(out, name)
			continue
		}
		args = strings.Join(strings.Fields(args), " ")
		args = truncateInline(args, 160)
		out = append(out, fmt.Sprintf("%s args=%s", name, args))
	}
	return out
}

func isEmptySummaryArgs(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "", "null", "{}", "[]":
		return true
	default:
		return false
	}
}

func truncateInline(content string, limit int) string {
	trimmed := strings.TrimSpace(content)
	if limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func truncateForSummary(content string, limit int) string {
	trimmed := strings.TrimSpace(content)
	if limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	markerRunes := []rune("\n[TRUNCATED]\n")
	if limit <= len(markerRunes)+4 {
		if limit <= 0 {
			return ""
		}
		return string(runes[:limit]) + string(markerRunes)
	}
	available := limit - len(markerRunes)
	head := int(float64(available) * 0.6)
	if head < 1 {
		head = 1
	}
	tail := available - head
	if tail < 1 {
		tail = 1
		head = available - tail
	}
	if head+tail > len(runes) {
		return trimmed
	}
	return string(runes[:head]) + string(markerRunes) + string(runes[len(runes)-tail:])
}

func decodePersistedChatMessage(msg persistence.ChatMessage) llm.Message {
	raw := strings.TrimSpace(msg.Content)
	if msg.Role == "assistant" && strings.HasPrefix(raw, "{") {
		var data struct {
			Content   string         `json:"content"`
			ToolCalls []llm.ToolCall `json:"tool_calls"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err == nil && len(data.ToolCalls) > 0 {
			return llm.Message{
				Role:      msg.Role,
				Content:   strings.TrimSpace(data.Content),
				ToolCalls: data.ToolCalls,
			}
		}
	}
	if msg.Role == "tool" && strings.HasPrefix(raw, "{") {
		var data struct {
			Content string `json:"content"`
			ToolID  string `json:"tool_id"`
		}
		if err := json.Unmarshal([]byte(raw), &data); err == nil {
			return llm.Message{
				Role:    msg.Role,
				Content: strings.TrimSpace(data.Content),
				ToolID:  strings.TrimSpace(data.ToolID),
			}
		}
	}
	return llm.Message{Role: msg.Role, Content: msg.Content}
}

func (m *Manager) compactChunk(ctx context.Context, existingSummary string, chunk []persistence.ChatMessage) (string, error) {
	compactor, ok := m.summary.(llm.CompactionProvider)
	if !ok {
		observability.LoggerWithTrace(ctx).Warn().Msg("responses_compaction_unavailable")
		return existingSummary, nil
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

	item, err := compactor.Compact(ctx, msgs, m.summaryModel, prev)
	if err != nil {
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
