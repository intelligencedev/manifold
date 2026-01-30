package agent

import (
	"fmt"
	"strings"

	"manifold/internal/llm"
)

// historyContextPrefix is prepended to the first history message to clearly mark
// that the following messages are prior conversation context, not the current request.
const historyContextPrefix = `[CONVERSATION HISTORY]
The messages below are from earlier exchanges in this conversation. Use them as background context only. Do NOT respond to questions or requests in the history—they have already been handled.
---
`

// currentRequestPrefix is prepended to the final user message to clearly indicate
// it is the message requiring a response.
const currentRequestPrefix = `[CURRENT REQUEST]
This is the user's current message. Respond to THIS message only. Use the conversation history above for context if needed, but focus your response on what is asked here.
---
`

// BuildInitialLLMMessages composes the initial message list from system, optional
// prior history (already in llm.Message form), and the current user input.
//
// When history is present, the function annotates messages to help the LLM
// distinguish between background context (history) and the current request:
//   - History messages are prefixed with [CONVERSATION HISTORY] marker
//   - The current user message is prefixed with [CURRENT REQUEST] marker
//
// This helps prevent LLMs from responding to questions in the history that
// have already been answered.
func BuildInitialLLMMessages(system, user string, history []llm.Message) []llm.Message {
	msgs := make([]llm.Message, 0, 2+len(history))
	if system != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: system})
	}

	// When we have both history and a new user message, annotate them
	// to make it clear which is context vs the current request.
	hasHistory := len(history) > 0
	hasUser := strings.TrimSpace(user) != ""

	if hasHistory {
		// Clone history and annotate the first message with context marker
		annotatedHistory := make([]llm.Message, len(history))
		copy(annotatedHistory, history)

		// Prepend context marker to first history message
		if annotatedHistory[0].Role == "user" {
			annotatedHistory[0] = llm.Message{
				Role:    annotatedHistory[0].Role,
				Content: historyContextPrefix + annotatedHistory[0].Content,
			}
		} else if len(annotatedHistory) > 1 {
			// If first message isn't user (e.g., it's a system message already processed),
			// find the first user message in history
			for i := range annotatedHistory {
				if annotatedHistory[i].Role == "user" {
					annotatedHistory[i] = llm.Message{
						Role:    annotatedHistory[i].Role,
						Content: historyContextPrefix + annotatedHistory[i].Content,
					}
					break
				}
			}
		}
		msgs = append(msgs, annotatedHistory...)
	}

	if hasUser {
		content := user
		if hasHistory {
			// Annotate current request to distinguish from history
			content = currentRequestPrefix + user
		}
		msgs = append(msgs, llm.Message{Role: "user", Content: content})
	}

	return msgs
}

// FormatHistorySummary creates a concise summary of the conversation history
// that can be used in prompts. This is useful for debugging and logging.
func FormatHistorySummary(history []llm.Message) string {
	if len(history) == 0 {
		return "(no history)"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d messages in history:\n", len(history)))
	for i, m := range history {
		preview := m.Content
		if len(preview) > 50 {
			preview = preview[:50] + "..."
		}
		preview = strings.ReplaceAll(preview, "\n", " ")
		b.WriteString(fmt.Sprintf("  [%d] %s: %s\n", i+1, m.Role, preview))
	}
	return b.String()
}
