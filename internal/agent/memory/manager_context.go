package memory

import (
	"context"
	"encoding/json"
	"errors"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/persistence"
	"strings"
)

func (m *Manager) BuildContextForProvider(ctx context.Context, userID *int64, sessionID string, targetProvider llm.Provider, targetModel string) ([]llm.Message, *SummaryResult, error) {
	log := observability.LoggerWithTrace(ctx)
	if sessionID == "" {
		return nil, nil, nil
	}
	targetCompactor, targetSupportsCompaction := llm.ProviderCompactor(targetProvider)

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
		updatedSummary, updatedCount, result := m.ensureSummary(ctx, userID, session, messages, targetCompactor, targetModel)
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
