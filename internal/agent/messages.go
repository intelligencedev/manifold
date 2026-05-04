package agent

import (
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

	var userPromptContext []string

	// When we have both history and a new user message, annotate them
	// to make it clear which is context vs the current request.
	hasHistory := len(history) > 0
	hasUser := strings.TrimSpace(user) != ""

	if hasHistory {
		// Clone history and annotate the first message with context marker.
		// Synthetic system messages in history (conversation summaries, provider
		// continuation rules, etc.) are moved into the current user prompt so the
		// runtime system prompt remains cache-stable across turns.
		annotatedHistory := make([]llm.Message, 0, len(history))
		for _, msg := range history {
			if msg.Role == "system" {
				if section := strings.TrimSpace(msg.Content); section != "" {
					userPromptContext = append(userPromptContext, section)
				}
				continue
			}
			annotatedHistory = append(annotatedHistory, msg)
		}

		// Prepend context marker to first history message
		if len(annotatedHistory) > 0 && annotatedHistory[0].Role == "user" {
			annotatedHistory[0].Content = historyContextPrefix + annotatedHistory[0].Content
		} else if len(annotatedHistory) > 1 {
			// If first message isn't user (e.g., it's a system message already processed),
			// find the first user message in history
			for i := range annotatedHistory {
				if annotatedHistory[i].Role == "user" {
					annotatedHistory[i].Content = historyContextPrefix + annotatedHistory[i].Content
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
		if len(userPromptContext) > 0 {
			content = strings.Join(userPromptContext, "\n\n") + "\n\n" + content
		}
		msgs = append(msgs, llm.Message{Role: "user", Content: content})
	} else if len(userPromptContext) > 0 {
		msgs = append(msgs, llm.Message{Role: "user", Content: strings.Join(userPromptContext, "\n\n")})
	}

	return msgs
}

func PrependToCurrentUserMessage(msgs []llm.Message, section string) []llm.Message {
	section = strings.TrimSpace(section)
	if section == "" {
		return msgs
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		content := strings.TrimSpace(msgs[i].Content)
		if content == "" {
			msgs[i].Content = section
		} else {
			msgs[i].Content = section + "\n\n" + msgs[i].Content
		}
		return msgs
	}
	return append(msgs, llm.Message{Role: "user", Content: section})
}
