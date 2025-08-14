package agent

import "singularityio/internal/llm"

// BuildInitialLLMMessages composes the initial message list from system, optional
// prior history (already in llm.Message form), and the current user input.
func BuildInitialLLMMessages(system, user string, history []llm.Message) []llm.Message {
	msgs := make([]llm.Message, 0, 2+len(history))
	if system != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: system})
	}
	if len(history) > 0 {
		msgs = append(msgs, history...)
	}
	if user != "" {
		msgs = append(msgs, llm.Message{Role: "user", Content: user})
	}
	return msgs
}
