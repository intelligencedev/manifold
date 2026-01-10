package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/persistence"
)

const (
	defaultReserveBuffer  = 25000
	maxSummarizeChunkSize = 4096
	compactionSummaryType = "compaction"

	// Compaction best-practice knobs. We avoid compacting every single turn unless
	// we must (budget overflow), and we treat tool-heavy phases as milestones.
	compactionMinDeltaMessages     = 6
	compactionToolMilestoneOutputs = 2
)

const compactionContinuationRule = "When continuing from prior context (including compacted context), do not restate prior final answers unless the user asks. Only provide new information, the next steps, or the requested delta."

// dualSummary stores both compaction (OpenAI-specific) and plain text summaries.
// This allows sessions to switch between OpenAI and non-OpenAI models seamlessly.
type dualSummary struct {
	Compaction string `json:"compaction,omitempty"` // OpenAI Responses API compaction (encrypted)
	Plain      string `json:"plain,omitempty"`      // Plain text summary for non-OpenAI models
}

// encodeDualSummary encodes a dual summary to JSON for storage.
func encodeDualSummary(ds dualSummary) string {
	if ds.Compaction == "" && ds.Plain == "" {
		return ""
	}
	raw, err := json.Marshal(ds)
	if err != nil {
		return ds.Plain // Fallback to plain if encoding fails
	}
	return string(raw)
}

// decodeDualSummary decodes a stored summary. It handles both the new dual format
// and legacy single-summary formats (plain text or compaction JSON).
func decodeDualSummary(summary string) dualSummary {
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return dualSummary{}
	}

	// Try to decode as dual summary first
	var ds dualSummary
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), &ds); err == nil {
			// Check if it looks like a dual summary (has "plain" or "compaction" keys)
			if ds.Compaction != "" || ds.Plain != "" {
				return ds
			}
		}
		// Check if it's a legacy compaction summary
		if _, ok := decodeCompactionSummary(trimmed); ok {
			return dualSummary{Compaction: trimmed}
		}
	}

	// Treat as plain text summary (legacy format)
	return dualSummary{Plain: trimmed}
}

// SummaryResult contains metadata about summarization that occurred during BuildContext.
// Callers can use this to notify users (e.g., via SSE events) when summarization happens.
type SummaryResult struct {
	// Triggered is true if summarization occurred during this BuildContext call.
	Triggered bool
	// EstimatedTokens is the estimated token count that triggered summarization.
	EstimatedTokens int
	// TokenBudget is the available token budget.
	TokenBudget int
	// MessageCount is the total number of messages before summarization.
	MessageCount int
	// SummarizedCount is the number of messages that were summarized.
	SummarizedCount int
}

// Config tunes how chat history should be summarized before being sent to the LLM.
// All summarization is now token-based using a reserve buffer pattern.
type Config struct {
	Enabled bool `yaml:"enabled" json:"enabled"`

	// ReserveBufferTokens reserves tokens for output (including reasoning tokens
	// for reasoning models). OpenAI recommends ~25,000 for reasoning models.
	ReserveBufferTokens   int `yaml:"reserveBufferTokens" json:"reserveBufferTokens"`
	MinKeepLastMessages   int `yaml:"minKeepLastMessages" json:"minKeepLastMessages"`
	MaxKeepLastMessages   int `yaml:"maxKeepLastMessages" json:"maxKeepLastMessages"`
	MaxSummaryChunkTokens int `yaml:"maxSummaryChunkTokens" json:"maxSummaryChunkTokens"`
	// ContextWindowTokens can be pre-computed by the caller; if 0, ContextSize
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

	// Token-based summarization fields
	reserveBufferTokens    int
	minKeepLastMessages    int
	maxKeepLastMessages    int
	maxSummaryChunkTokens  int
	contextWindowTokens    int
	useResponsesCompaction bool
}

// Introspection helpers used by debug/observability surfaces.

// ContextWindowTokens returns the approximate context window size (in tokens)
// used for token budgeting.
func (m *Manager) ContextWindowTokens() int { return m.contextWindowTokens }

// ReserveBufferTokens returns the reserve buffer for output tokens.
func (m *Manager) ReserveBufferTokens() int { return m.reserveBufferTokens }

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
		reserveBufferTokens:    cfg.ReserveBufferTokens,
		minKeepLastMessages:    cfg.MinKeepLastMessages,
		maxKeepLastMessages:    cfg.MaxKeepLastMessages,
		maxSummaryChunkTokens:  cfg.MaxSummaryChunkTokens,
		contextWindowTokens:    cfg.ContextWindowTokens,
		useResponsesCompaction: cfg.UseResponsesCompaction,
	}
	if m.reserveBufferTokens <= 0 {
		m.reserveBufferTokens = defaultReserveBuffer
	}
	if m.minKeepLastMessages <= 0 {
		m.minKeepLastMessages = 4
	}
	if m.maxKeepLastMessages <= 0 {
		m.maxKeepLastMessages = 12
	}
	if m.maxKeepLastMessages < m.minKeepLastMessages {
		m.maxKeepLastMessages = m.minKeepLastMessages
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
// Returns the messages and a SummaryResult indicating if summarization was triggered.
//
// Deprecated: Use BuildContextForProvider instead, which properly handles compaction
// compatibility with the target LLM provider.
func (m *Manager) BuildContext(ctx context.Context, userID *int64, sessionID string) ([]llm.Message, *SummaryResult, error) {
	// Default to compaction support based on the manager's global setting.
	// This maintains backward compatibility but callers should migrate to
	// BuildContextForProvider for proper cross-provider support.
	return m.BuildContextForProvider(ctx, userID, sessionID, m.useResponsesCompaction)
}

// BuildContextForProvider assembles the conversation history that should be sent to the
// orchestrator by combining a persisted summary (if any) with the most recent chat turns.
// The targetSupportsCompaction parameter indicates whether the target LLM provider supports
// OpenAI Responses API compaction. If false and the stored summary uses compaction format
// (encrypted_content), the summary will be treated as unavailable for this request.
// Returns the messages and a SummaryResult indicating if summarization was triggered.
func (m *Manager) BuildContextForProvider(ctx context.Context, userID *int64, sessionID string, targetSupportsCompaction bool) ([]llm.Message, *SummaryResult, error) {
	log := observability.LoggerWithTrace(ctx)
	if sessionID == "" {
		return nil, nil, nil
	}

	messages, err := m.store.ListMessages(ctx, userID, sessionID, 0)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			messages = nil
		} else {
			return nil, nil, err
		}
	}
	log.Info().Str("session_id", sessionID).Int("messages_count", len(messages)).Msg("build_context_list_messages")

	session, err := m.store.GetSession(ctx, userID, sessionID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			// Session is guaranteed to exist via ensureChatSession but guard anyway.
			session = persistence.ChatSession{ID: sessionID}
		} else {
			return nil, nil, err
		}
	}

	summary := session.Summary
	summarizedCount := session.SummarizedCount
	var summaryResult *SummaryResult

	if m.enabled {
		updatedSummary, updatedCount, result := m.ensureSummary(ctx, userID, session, messages)
		if updatedSummary != "" || updatedCount != summarizedCount {
			summary = updatedSummary
			summarizedCount = updatedCount
		}
		if result != nil && result.Triggered {
			summaryResult = result
		}
	}

	total := len(messages)
	tailStart := 0
	if m.enabled {
		// Token-based approach: choose the tail based on available token budget
		// (context window minus reserve buffer).
		ctxSize := m.contextWindowTokens
		if ctxSize <= 0 {
			ctxSize = 32_000 // Conservative default for memory budgeting
		}
		reserveBuffer := m.reserveBufferTokens
		if reserveBuffer <= 0 {
			reserveBuffer = defaultReserveBuffer
		}
		budget := ctxSize - reserveBuffer
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
		// Cap the raw tail to avoid sending an excessively long transcript even
		// when it fits within the model context budget.
		maxTail := m.maxKeepLastMessages
		if maxTail > 0 && total-tailStart > maxTail {
			tailStart = total - maxTail
			if tailStart < summarizedCount {
				tailStart = summarizedCount
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

	// IMPORTANT: Never start the returned history on a tool response message.
	// Many providers (notably Anthropic) require a tool_result block to have a
	// corresponding tool_use block in a previous assistant message.
	// Our tail selection is token/count based and can cut between these, so we
	// adjust the tail start to include any required assistant tool call messages.
	if total > 0 {
		minIdx := summarizedCount
		if minIdx < 0 {
			minIdx = 0
		}
		adjusted := adjustIndexForToolDeps(messages, 0, tailStart)
		if adjusted < tailStart {
			if adjusted < minIdx {
				log.Warn().
					Str("session_id", sessionID).
					Int("summarized_count", summarizedCount).
					Int("tail_start", tailStart).
					Int("adjusted_tail_start", adjusted).
					Msg("tail_start_crosses_tool_chain_including_pre_summarized_tool_calls")
			}
			tailStart = adjusted
		}
	}

	history := make([]llm.Message, 0, (total-tailStart)+1)
	// Only add compaction continuation rule if target supports compaction
	if targetSupportsCompaction {
		history = append(history, llm.Message{Role: "system", Content: compactionContinuationRule})
	}
	if summary != "" {
		// Decode the stored summary (handles both dual format and legacy formats)
		ds := decodeDualSummary(summary)

		if targetSupportsCompaction && ds.Compaction != "" {
			// Target supports compaction and we have compaction data
			if item, ok := decodeCompactionSummary(ds.Compaction); ok {
				history = append(history, llm.Message{Role: "assistant", Compaction: &item})
			} else {
				// Compaction decode failed, fall back to plain if available
				if ds.Plain != "" {
					history = append(history, llm.Message{
						Role:    "system",
						Content: "Conversation summary (for context only):\n" + ds.Plain,
					})
				}
			}
		} else if ds.Plain != "" {
			// Target doesn't support compaction or no compaction data - use plain text
			history = append(history, llm.Message{
				Role:    "system",
				Content: "Conversation summary (for context only):\n" + ds.Plain,
			})
		} else if ds.Compaction != "" && !targetSupportsCompaction {
			// We only have compaction data but target doesn't support it
			// Log warning and reset tailStart to include more raw messages
			log.Warn().
				Str("session_id", sessionID).
				Msg("compaction_summary_incompatible_with_target_provider_no_plain_fallback")
			tailStart = 0
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
	return history, summaryResult, nil
}

// adjustIndexForToolDeps ensures that if the kept tail includes any tool response
// messages, it also includes the preceding assistant message(s) that contain the
// corresponding ToolCalls.
//
// This prevents provider-specific request validation failures (e.g., Anthropic
// requires tool_result to reference a prior tool_use) and avoids losing metadata
// chains required by some providers (e.g., Gemini thought signatures).
//
// The returned index is always <= cutIndex.
func adjustIndexForToolDeps(msgs []persistence.ChatMessage, start, cutIndex int) int {
	if cutIndex <= start || cutIndex >= len(msgs) {
		return cutIndex
	}

	extractToolID := func(m persistence.ChatMessage) string {
		if id := strings.TrimSpace(m.ToolID); id != "" {
			return id
		}
		trimmed := strings.TrimSpace(m.Content)
		if strings.HasPrefix(trimmed, "{") {
			var data struct {
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
				return strings.TrimSpace(data.ToolID)
			}
		}
		return ""
	}

	required := make(map[string]struct{})
	for i := cutIndex; i < len(msgs); i++ {
		if msgs[i].Role != "tool" {
			continue
		}
		if id := extractToolID(msgs[i]); id != "" {
			required[id] = struct{}{}
		}
	}
	if len(required) == 0 {
		return cutIndex
	}

	containsToolCallID := func(m persistence.ChatMessage, toolID string) bool {
		trimmed := strings.TrimSpace(m.Content)
		if !strings.HasPrefix(trimmed, "{") {
			return false
		}
		var data struct {
			ToolCalls []llm.ToolCall `json:"tool_calls"`
		}
		if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
			return false
		}
		for _, tc := range data.ToolCalls {
			if strings.TrimSpace(tc.ID) == toolID {
				return true
			}
		}
		return false
	}

	earliestNeeded := cutIndex
	for toolID := range required {
		foundIdx := -1
		for i := cutIndex - 1; i >= start; i-- {
			if msgs[i].Role != "assistant" {
				continue
			}
			if containsToolCallID(msgs[i], toolID) {
				foundIdx = i
				break
			}
		}
		if foundIdx != -1 && foundIdx < earliestNeeded {
			earliestNeeded = foundIdx
		}
	}

	return earliestNeeded
}

func (m *Manager) ensureSummary(ctx context.Context, userID *int64, session persistence.ChatSession, messages []persistence.ChatMessage) (string, int, *SummaryResult) {
	if !m.enabled || m.summary == nil {
		return session.Summary, session.SummarizedCount, nil
	}

	total := len(messages)
	if total == 0 {
		return session.Summary, session.SummarizedCount, nil
	}

	// Token-based: estimate token usage and roll summary incrementally based on
	// token budget (context window minus reserve buffer).
	ctxSize := m.contextWindowTokens
	if ctxSize <= 0 && m.summaryModel != "" {
		if size, _ := llm.ContextSize(m.summaryModel); size > 0 {
			ctxSize = size
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000 // Conservative default for memory budgeting
	}

	reserveBuffer := m.reserveBufferTokens
	if reserveBuffer <= 0 {
		reserveBuffer = defaultReserveBuffer
	}

	budget := ctxSize - reserveBuffer
	if budget <= 0 {
		budget = ctxSize / 2
	}

	// Force summarization once the chat exceeds the configured max tail size.
	// This keeps the raw transcript short even for very large-context models.
	maxTail := m.maxKeepLastMessages
	if maxTail < m.minKeepLastMessages {
		maxTail = m.minKeepLastMessages
	}
	forceByCount := !m.useResponsesCompaction && maxTail > 0 && total > maxTail

	estimated := 0
	for _, msg := range messages {
		estimated += len([]rune(strings.TrimSpace(msg.Content)))/4 + 1
	}
	if !forceByCount && estimated <= budget {
		// Compaction mode: compact after milestones, not every turn.
		if m.useResponsesCompaction {
			delta := total - session.SummarizedCount
			if delta <= 0 {
				return session.Summary, session.SummarizedCount, nil
			}
			toolOutputs := 0
			if session.SummarizedCount >= 0 && session.SummarizedCount < total {
				for _, msg := range messages[session.SummarizedCount:] {
					if msg.Role == "tool" {
						toolOutputs++
					}
				}
			}
			if delta < compactionMinDeltaMessages && toolOutputs < compactionToolMilestoneOutputs {
				return session.Summary, session.SummarizedCount, nil
			}
		}
		return session.Summary, session.SummarizedCount, nil
	}

	// Decide how many early messages to include in the next summary chunk.
	// For classic summarization we keep a small raw tail; for Responses compaction
	// we prefer to compact the full eligible delta so the compaction blob fully
	// represents prior state.
	minTail := m.minKeepLastMessages
	if m.useResponsesCompaction {
		minTail = 0
	} else {
		if minTail <= 0 {
			minTail = 4
		}
		if forceByCount {
			minTail = maxTail
		}
	}
	if total <= minTail {
		return session.Summary, session.SummarizedCount, nil
	}

	target := total - minTail
	if target <= 0 {
		return session.Summary, session.SummarizedCount, nil
	}

	summarizedCount := session.SummarizedCount
	if summarizedCount > target {
		summarizedCount = target
	}

	if summarizedCount == target {
		return session.Summary, summarizedCount, nil
	}

	start := summarizedCount
	if start < 0 || start > target {
		start = 0
	}

	// Never cut between an assistant tool call and its tool response.
	// If the tail includes tool responses, ensure the boundary keeps the
	// corresponding assistant ToolCalls in the raw tail too.
	if target > start {
		adjustedTarget := adjustIndexForToolDeps(messages, start, target)
		if adjustedTarget < target {
			target = adjustedTarget
			if target <= start {
				return session.Summary, summarizedCount, nil
			}
		}
	}

	chunk := messages[start:target]
	if len(chunk) == 0 {
		return session.Summary, summarizedCount, nil
	}

	// Log summarization trigger with consistent format as Engine.maybeSummarize
	log := observability.LoggerWithTrace(ctx)
	log.Info().
		Str("session", session.ID).
		Int("messages", total).
		Int("estimated_tokens", estimated).
		Int("token_budget", budget).
		Int("context_window", ctxSize).
		Int("reserve_buffer", reserveBuffer).
		Int("summarizing_count", len(chunk)).
		Msg("summarization_triggered")

	// Build result metadata for the caller to notify users
	result := &SummaryResult{
		Triggered:       true,
		EstimatedTokens: estimated,
		TokenBudget:     budget,
		MessageCount:    total,
		SummarizedCount: len(chunk),
	}

	summary, err := m.summarizeChunk(ctx, session.Summary, chunk)
	if err != nil {
		log.Error().Err(err).Str("session", session.ID).Msg("chat_summary_failed")
		return session.Summary, summarizedCount, result
	}

	if err := m.store.UpdateSummary(ctx, userID, session.ID, summary, target); err != nil {
		log.Error().Err(err).Str("session", session.ID).Msg("chat_summary_persist_failed")
		return session.Summary, summarizedCount, result
	}

	log.Info().Str("session", session.ID).Int("messages", target).Msg("chat_summary_updated")
	return summary, target, result
}

func (m *Manager) summarizeChunk(ctx context.Context, existingSummary string, chunk []persistence.ChatMessage) (string, error) {
	if m.summary == nil {
		return existingSummary, fmt.Errorf("llm provider unavailable")
	}

	// Decode existing summary to get both compaction and plain components
	existing := decodeDualSummary(existingSummary)
	log := observability.LoggerWithTrace(ctx)

	if m.useResponsesCompaction {
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

		// Goroutine 1: Compaction summarization (OpenAI Responses API)
		go func() {
			defer wg.Done()
			res, err := m.compactChunk(ctx, existing.Compaction, chunk)
			if err != nil {
				compactionErr = err
				log.Warn().Err(err).Msg("compaction_summarization_failed")
				return
			}
			compactionRes = res
		}()

		// Goroutine 2: Plain text summarization (for non-OpenAI model fallback)
		go func() {
			defer wg.Done()
			res, err := m.plainSummarize(ctx, existing.Plain, chunk)
			if err != nil {
				plainErr = err
				log.Warn().Err(err).Msg("plain_summarization_failed")
				return
			}
			plainRes = res
		}()

		wg.Wait()

		// Build dual summary with whatever succeeded
		ds := dualSummary{
			Compaction: compactionRes,
			Plain:      plainRes,
		}

		// If compaction failed but plain succeeded, use plain only
		if compactionErr != nil && plainErr == nil {
			return ds.Plain, nil
		}
		// If both failed, return error
		if compactionErr != nil && plainErr != nil {
			return existingSummary, fmt.Errorf("both summarization methods failed: compaction: %v, plain: %v", compactionErr, plainErr)
		}
		// If plain failed but compaction succeeded, still return dual (plain will be empty)
		// Future requests to non-OpenAI models will get no summary, which is safer than garbage

		return encodeDualSummary(ds), nil
	}

	// Non-compaction mode: just do plain text summarization
	plainRes, err := m.plainSummarize(ctx, existing.Plain, chunk)
	if err != nil {
		return existingSummary, err
	}
	return plainRes, nil
}

// plainSummarize generates a plain text summary of the conversation chunk.
// This is used for non-OpenAI models and as a fallback when compaction is enabled.
func (m *Manager) plainSummarize(ctx context.Context, existingPlainSummary string, chunk []persistence.ChatMessage) (string, error) {
	var userPrompt strings.Builder
	userPrompt.WriteString("Update the running summary of this chat. Keep it concise but information-dense.\n")
	userPrompt.WriteString("Preserve user goals, preferences, decisions, key facts, identifiers (files, URLs, IDs), tool results/errors, and open questions.\n")
	userPrompt.WriteString("If content includes [TRUNCATED], assume important details may be missing.\n")
	if strings.TrimSpace(existingPlainSummary) != "" {
		userPrompt.WriteString("\nExisting summary:\n")
		userPrompt.WriteString(strings.TrimSpace(existingPlainSummary))
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
		return existingPlainSummary, fmt.Errorf("summarize chat: %w", err)
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return existingPlainSummary, fmt.Errorf("empty summary returned")
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
