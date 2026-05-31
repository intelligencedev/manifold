// Package budget provides defensive context-window enforcement helpers used
// just before dispatching a request to an LLM provider.
//
// The helpers here intentionally do not perform LLM-based summarization: they
// only truncate individual oversized messages (typically tool outputs that
// returned a very large blob) and, if necessary, drop the oldest non-essential
// messages so the request fits within the target model's effective context.
//
// Higher-level summarization (chat memory roll-ups, evolving memory) lives in
// internal/agent/memory and runs before the engine loop. This package is the
// last line of defense for callers that don't go through that path
// (specialists, sub-agents, tool-driven follow-ups) or for cases where a
// single tool turn returned more tokens than any prior summarization budget
// anticipated.
package budget

import (
	"strings"

	"manifold/internal/llm"
)

// DefaultReserveBuffer mirrors the engine/memory reserve buffer used to leave
// room for output tokens when no caller-specific value is provided.
const DefaultReserveBuffer = 25_000

// DefaultPerMsgRunes is the per-message rune cap used as a fallback when the
// caller does not provide one. It is intentionally generous; the goal is only
// to clip pathological tool blobs (multi-megabyte dumps), not normal content.
const DefaultPerMsgRunes = 64_000

// Fit enforces a context budget on msgs (returns a new slice; does not mutate
// the input). It performs three passes:
//
//  1. Truncate the content of any individual non-system message whose rune
//     length exceeds perMsgRunes. Truncation keeps the head and tail of normal
//     messages, but trims the head of the final user message so prepended
//     runtime context is sacrificed before the current request.
//  2. If the total estimated token count still exceeds (ctxWindow -
//     reserveBuffer), drop the oldest tool/assistant messages until it fits,
//     preserving the leading system message (if any) and the last user
//     message.
//  3. As a final fallback, hard-truncate the head of the protected last user
//     message so the request always fits. Runtime context is commonly prepended
//     to that message, so trimming from the head preserves the current request
//     at the tail before sacrificing user text.
//
// Token estimation is heuristic (rune-count / 4 + 1 per message). It will
// under-estimate for some tokenizers, so callers that talk to small-context
// servers should pass a conservative reserveBuffer.
//
// If ctxWindow <= 0 the function only applies the per-message truncation and
// returns. If perMsgRunes <= 0 the per-message truncation step is skipped.
func Fit(msgs []llm.Message, ctxWindow, reserveBuffer, perMsgRunes int) []llm.Message {
	protectedPrefixEnd := 0
	if len(msgs) > 0 && msgs[0].Role == "system" {
		protectedPrefixEnd = 1
	}
	return FitWithProtectedPrefix(msgs, protectedPrefixEnd, ctxWindow, reserveBuffer, perMsgRunes)
}

// FitWithProtectedPrefix is Fit with an explicit prefix that must not be
// dropped during the oldest-message eviction pass. Callers use this for
// cache-boundary prompt layouts where static messages must remain before
// conversation/tool history.
func FitWithProtectedPrefix(msgs []llm.Message, protectedPrefixEnd, ctxWindow, reserveBuffer, perMsgRunes int) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}
	if protectedPrefixEnd < 0 {
		protectedPrefixEnd = 0
	}
	if protectedPrefixEnd > len(msgs) {
		protectedPrefixEnd = len(msgs)
	}

	out := make([]llm.Message, len(msgs))
	copy(out, msgs)

	lastUser := -1
	for i := len(out) - 1; i >= protectedPrefixEnd; i-- {
		if out[i].Role == "user" {
			lastUser = i
			break
		}
	}

	if perMsgRunes > 0 {
		for i := range out {
			if out[i].Role == "system" {
				continue
			}
			if i == lastUser {
				out[i].Content = truncateContentPreserveTail(out[i].Content, perMsgRunes)
				continue
			}
			out[i].Content = truncateContent(out[i].Content, perMsgRunes)
		}
	}

	if ctxWindow <= 0 {
		return out
	}

	if reserveBuffer < 0 {
		reserveBuffer = 0
	}
	if reserveBuffer >= ctxWindow {
		reserveBuffer = ctxWindow / 2
	}
	budget := ctxWindow - reserveBuffer
	if budget <= 0 {
		budget = ctxWindow / 2
	}

	if EstimateTokens(out) <= budget {
		return out
	}

	start := protectedPrefixEnd
	for i := len(out) - 1; i >= start; i-- {
		if out[i].Role == "user" {
			lastUser = i
			break
		}
	}

	for EstimateTokens(out) > budget && len(out) > start+1 {
		dropIdx := -1
		for i := start; i < len(out); i++ {
			if i == lastUser {
				continue
			}
			dropIdx = i
			break
		}
		if dropIdx < 0 {
			break
		}
		out = append(out[:dropIdx], out[dropIdx+1:]...)
		if lastUser > dropIdx {
			lastUser--
		}
	}

	if EstimateTokens(out) > budget && lastUser >= 0 && lastUser < len(out) {
		over := EstimateTokens(out) - budget
		trimRunes := (over + 64) * 4
		out[lastUser].Content = trimContentHead(out[lastUser].Content, trimRunes)
	}

	return out
}

// EstimateTokens returns a heuristic token count for a slice of messages. It
// matches the chars/4+1 heuristic used elsewhere in the codebase so behaviour
// is consistent across callers.
func EstimateTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		c := strings.TrimSpace(m.Content)
		if c == "" {
			total++
			continue
		}
		total += len([]rune(c))/4 + 1
	}
	return total
}

func truncateContent(content string, limit int) string {
	if limit <= 0 {
		return content
	}
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	marker := []rune("\n[TRUNCATED]\n")
	if limit <= len(marker)+4 {
		return string(runes[:limit])
	}
	available := limit - len(marker)
	head := max(available*6/10, 1)
	tail := available - head
	if tail < 1 {
		tail = 1
		head = available - tail
	}
	return string(runes[:head]) + string(marker) + string(runes[len(runes)-tail:])
}

func truncateContentPreserveTail(content string, limit int) string {
	if limit <= 0 {
		return content
	}
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return trimContentHead(content, len(runes)-limit)
}

func trimContentHead(content string, runesToRemove int) string {
	runes := []rune(content)
	if runesToRemove <= 0 || len(runes) == 0 {
		return content
	}
	if runesToRemove >= len(runes) {
		return "[TRUNCATED]"
	}
	keepStart := min(runesToRemove, len(runes)-1)
	return "[TRUNCATED]\n" + string(runes[keepStart:])
}
