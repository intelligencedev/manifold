package agent

import (
	"context"
	"fmt"

	"gptagent/internal/llm"
	"gptagent/internal/tools"
)

type Engine struct {
	LLM      llm.Provider
	Tools    tools.Registry
	MaxSteps int
	System   string
	// OnAssistant, if set, is called with each assistant message the provider
	// returns (including those containing tool calls and the final answer).
	OnAssistant func(llm.Message)
	// OnDelta, if set, is called for streaming content deltas (for partial responses)
	OnDelta func(string)
}

// Run executes the agent loop until the model produces a final answer.
func (e *Engine) Run(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	msgs := BuildInitialLLMMessages(e.System, userInput, history)

	var final string
	for step := 0; step < e.MaxSteps; step++ {
		msg, err := e.LLM.Chat(ctx, msgs, e.Tools.Schemas(), e.model())
		if err != nil {
			return "", err
		}
		msgs = append(msgs, msg)
		if e.OnAssistant != nil {
			e.OnAssistant(msg)
		}
		if len(msg.ToolCalls) == 0 {
			final = msg.Content
			break
		}
		for _, tc := range msg.ToolCalls {
			payload, err := e.Tools.Dispatch(ctx, tc.Name, tc.Args)
			if err != nil {
				payload = []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
			}
			msgs = append(msgs, llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID})
		}
	}
	if final == "" {
		final = "(no final text — increase max steps or check logs)"
	}
	return final, nil
}

// RunStream executes the agent loop with streaming support
func (e *Engine) RunStream(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	msgs := BuildInitialLLMMessages(e.System, userInput, history)

	var final string
	for step := 0; step < e.MaxSteps; step++ {
		// Accumulate streaming content for this step
		var accumulatedContent string
		var accumulatedToolCalls []llm.ToolCall

		handler := &streamHandler{
			onDelta: func(content string) {
				accumulatedContent += content
				if e.OnDelta != nil {
					e.OnDelta(content)
				}
			},
			onToolCall: func(tc llm.ToolCall) {
				accumulatedToolCalls = append(accumulatedToolCalls, tc)
			},
		}

		err := e.LLM.ChatStream(ctx, msgs, e.Tools.Schemas(), e.model(), handler)
		if err != nil {
			return "", err
		}

		// Create the complete message from accumulated content and tool calls
		msg := llm.Message{
			Role:      "assistant",
			Content:   accumulatedContent,
			ToolCalls: accumulatedToolCalls,
		}

		msgs = append(msgs, msg)
		if e.OnAssistant != nil {
			e.OnAssistant(msg)
		}

		if len(msg.ToolCalls) == 0 {
			final = msg.Content
			break
		}

		for _, tc := range msg.ToolCalls {
			payload, err := e.Tools.Dispatch(ctx, tc.Name, tc.Args)
			if err != nil {
				payload = []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
			}
			msgs = append(msgs, llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID})
		}
	}
	if final == "" {
		final = "(no final text — increase max steps or check logs)"
	}
	return final, nil
}

// streamHandler implements llm.StreamHandler
type streamHandler struct {
	onDelta    func(string)
	onToolCall func(llm.ToolCall)
}

func (h *streamHandler) OnDelta(content string) {
	if h.onDelta != nil {
		h.onDelta(content)
	}
}

func (h *streamHandler) OnToolCall(tc llm.ToolCall) {
	if h.onToolCall != nil {
		h.onToolCall(tc)
	}
}

func (e *Engine) model() string { return "" }

// Message exists for future agent-level message modeling.
// Message type removed in favor of llm.Message throughout the engine API.
