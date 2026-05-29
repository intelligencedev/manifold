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

const currentRequestMarker = "[CURRENT REQUEST]"

// currentRequestPrefix is prepended to the final user message to clearly indicate
// it is the message requiring a response.
const currentRequestPrefix = currentRequestMarker + `
This is the user's current message. Respond to THIS message only. Use the conversation history above for context if needed, but focus your response on what is asked here.
---
`

// runtimeContextPrefix marks volatile context that must live with the current
// request, after stable conversation history, so provider/KV caches can reuse
// the static prompt and unchanged history prefix across turns.
const runtimeContextMarker = "[RUNTIME CONTEXT]"

const runtimeContextPrefix = runtimeContextMarker + `
The context below is generated for this request. Use it as background context only; it may include summaries, memories, retrieved facts, policies, or specialist lists.
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

	var runtimeContext []string

	// When we have both history and a new user message, annotate them
	// to make it clear which is context vs the current request.
	hasHistory := len(history) > 0
	hasUser := strings.TrimSpace(user) != ""

	if hasHistory {
		annotatedHistory := make([]llm.Message, 0, len(history))
		for _, msg := range history {
			if msg.Role == "system" {
				if section := strings.TrimSpace(msg.Content); section != "" {
					runtimeContext = append(runtimeContext, section)
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
		msgs = append(msgs, llm.Message{Role: "user", Content: content})
		if len(runtimeContext) > 0 {
			msgs = AddRuntimeContextToCurrentUserMessage(msgs, strings.Join(runtimeContext, "\n\n"))
		}
	} else if len(runtimeContext) > 0 {
		msgs = addStandaloneRuntimeContext(msgs, strings.Join(runtimeContext, "\n\n"))
	}

	return msgs
}

func AddRuntimeContextToCurrentUserMessage(msgs []llm.Message, section string) []llm.Message {
	section = strings.TrimSpace(section)
	if section == "" {
		return msgs
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(msgs[i].Role), "user") {
			continue
		}
		content := strings.TrimSpace(msgs[i].Content)
		if content == "" {
			msgs[i].Content = runtimeContextPrefix + section
			return msgs
		}
		if strings.HasPrefix(content, runtimeContextMarker) {
			msgs[i].Content = appendRuntimeContextSection(content, section)
			return msgs
		}
		msgs[i].Content = runtimeContextPrefix + section + "\n\n" + msgs[i].Content
		return msgs
	}
	return addStandaloneRuntimeContext(msgs, section)
}

func appendRuntimeContextSection(content, section string) string {
	content = strings.TrimSpace(content)
	section = strings.TrimSpace(section)
	if section == "" {
		return content
	}
	if idx := strings.Index(content, currentRequestMarker); idx >= 0 {
		before := strings.TrimSpace(content[:idx])
		after := strings.TrimSpace(content[idx:])
		if before == "" {
			return section + "\n\n" + after
		}
		return before + "\n\n" + section + "\n\n" + after
	}
	return content + "\n\n" + section
}

func addStandaloneRuntimeContext(msgs []llm.Message, section string) []llm.Message {
	section = strings.TrimSpace(section)
	if section == "" {
		return msgs
	}
	return append(msgs, llm.Message{Role: "user", Content: runtimeContextPrefix + section})
}

func IsRuntimeContextMessage(msg llm.Message) bool {
	return strings.EqualFold(strings.TrimSpace(msg.Role), "user") &&
		strings.HasPrefix(strings.TrimSpace(msg.Content), runtimeContextMarker)
}

func cacheBoundaryPrefixEnd(msgs []llm.Message) int {
	return staticPromptPrefixEnd(msgs)
}

func staticPromptPrefixEnd(msgs []llm.Message) int {
	end := 0
	for end < len(msgs) {
		switch strings.ToLower(strings.TrimSpace(msgs[end].Role)) {
		case "system", "developer":
			end++
		default:
			return end
		}
	}
	return end
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
