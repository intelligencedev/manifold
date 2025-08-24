package agent

import (
	"context"
	"fmt"
	"strings"

	"singularityio/internal/llm"
	"singularityio/internal/observability"
	"singularityio/internal/tools"
)

type Engine struct {
	LLM      llm.Provider
	Tools    tools.Registry
	MaxSteps int
	System   string
	// Rolling summarization configuration
	// If SummaryEnabled is true, when the constructed messages exceed
	// SummaryThreshold messages we will ask the LLM to compress the
	// older messages into a single summary and keep only SummaryKeepLast
	// most-recent messages in full.
	SummaryEnabled   bool
	SummaryThreshold int // e.g., 40
	SummaryKeepLast  int // e.g., 12
	// OnAssistant, if set, is called with each assistant message the provider
	// returns (including those containing tool calls and the final answer).
	OnAssistant func(llm.Message)
	// OnDelta, if set, is called for streaming content deltas (for partial responses)
	OnDelta func(string)
}

// Run executes the agent loop until the model produces a final answer.
func (e *Engine) Run(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
	msgs := BuildInitialLLMMessages(e.System, userInput, history)
	// Possibly summarize older history to avoid unbounded token growth.
	if e.SummaryEnabled {
		msgs = e.maybeSummarize(ctx, msgs)
	}

	var final string
	for step := 0; step < e.MaxSteps; step++ {
		log.Debug().Int("step", step).Int("history", len(msgs)).Msg("engine_step_start")
		msg, err := e.LLM.Chat(ctx, msgs, e.Tools.Schemas(), e.model())
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_step_error")
			return "", err
		}
		msgs = append(msgs, msg)
		if e.OnAssistant != nil {
			e.OnAssistant(msg)
		}
		if len(msg.ToolCalls) == 0 {
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_final")
			final = msg.Content
			break
		}
		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_tool_calls")
		for _, tc := range msg.ToolCalls {
			// Propagate the agent's provider to the tool dispatch context so
			// tools that make LLM calls can use the same provider/model/baseURL as the agent.
			dispatchCtx := ctx
			if e.LLM != nil {
				dispatchCtx = tools.WithProvider(ctx, e.LLM)
			}

			// Log the tool being called and its (redacted) args at the agent level.
			observability.LoggerWithTrace(ctx).Info().Str("tool", tc.Name).RawJSON("args", observability.RedactJSON(tc.Args)).Msg("engine_tool_call")
			payload, err := e.Tools.Dispatch(dispatchCtx, tc.Name, tc.Args)
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
	log := observability.LoggerWithTrace(ctx)
	msgs := BuildInitialLLMMessages(e.System, userInput, history)

	// Possibly summarize older history to avoid unbounded token growth.
	if e.SummaryEnabled {
		msgs = e.maybeSummarize(ctx, msgs)
	}

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

		log.Debug().Int("step", step).Int("history", len(msgs)).Msg("engine_stream_step_start")
		err := e.LLM.ChatStream(ctx, msgs, e.Tools.Schemas(), e.model(), handler)
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_stream_step_error")
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
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_stream_final")
			final = msg.Content
			break
		}
		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_stream_tool_calls")
		for _, tc := range msg.ToolCalls {
			// Propagate the agent's provider to the tool dispatch context so
			// tools that make LLM calls can use the same provider/model/baseURL as the agent.
			dispatchCtx := ctx
			if e.LLM != nil {
				dispatchCtx = tools.WithProvider(ctx, e.LLM)
			}

			// Log the tool being called and its (redacted) args at the agent level.
			observability.LoggerWithTrace(ctx).Info().Str("tool", tc.Name).RawJSON("args", observability.RedactJSON(tc.Args)).Msg("engine_tool_call")
			payload, err := e.Tools.Dispatch(dispatchCtx, tc.Name, tc.Args)
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

// maybeSummarize inspects msgs and, if the number of messages exceeds
// e.SummaryThreshold, calls the LLM to produce a short summary of the
// older portion of the conversation and returns a new messages slice
// where the older messages have been replaced by a single summary
// assistant message plus the most recent messages preserved.
func (e *Engine) maybeSummarize(ctx context.Context, msgs []llm.Message) []llm.Message {
	// Ensure sensible defaults
	threshold := e.SummaryThreshold
	keep := e.SummaryKeepLast
	if threshold <= 0 {
		threshold = 40
	}
	if keep <= 0 {
		keep = 12
	}
	if len(msgs) <= threshold {
		return msgs
	}

	// Log that summarization will run and include relevant tuning params.
	observability.LoggerWithTrace(ctx).Info().Int("messages", len(msgs)).Int("threshold", threshold).Int("keep_last", keep).Msg("summarization_triggered")

	// Preserve leading system message if present
	start := 0
	var sysMsg *llm.Message
	if len(msgs) > 0 && msgs[0].Role == "system" {
		sysMsg = &msgs[0]
		start = 1
	}

	// Split messages to summarize and recent messages to keep
	// summarizable = msgs[start:len(msgs)-keep]
	if len(msgs)-start <= keep {
		return msgs
	}
	cutIndex := len(msgs) - keep
	toSummarize := msgs[start:cutIndex]
	recent := msgs[cutIndex:]

	// Build a compact user prompt listing the messages to summarize.
	var b strings.Builder
	for _, m := range toSummarize {
		b.WriteString("Role: ")
		b.WriteString(m.Role)
		b.WriteString("\n")
		// Limit content length in the summarizer prompt to avoid sending huge blobs
		content := m.Content
		if len(content) > 4096 {
			content = content[:4096] + "\n[TRUNCATED]"
		}
		b.WriteString(content)
		b.WriteString("\n\n")
	}

	sys := "You are a concise summarizer. Produce a short, factual summary (<= 300 characters) of the conversation that follows. Keep important facts, omit chit-chat. Return only the summary text."
	user := "Summarize the following conversation:\n\n" + b.String()

	// Call the LLM to summarize the older messages
	summReq := []llm.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}}
	sumMsg, err := e.LLM.Chat(ctx, summReq, nil, e.model())
	if err != nil {
		// On error, don't summarize; just return original msgs
		observability.LoggerWithTrace(ctx).Error().Err(err).Msg("summary_failed")
		return msgs
	}

	// Build a summary assistant message and assemble the new message list
	summaryContent := "[SUMMARY] " + strings.TrimSpace(sumMsg.Content)
	summary := llm.Message{Role: "assistant", Content: summaryContent}

	newMsgs := make([]llm.Message, 0, 1+keep+2)
	if sysMsg != nil {
		newMsgs = append(newMsgs, *sysMsg)
	}
	newMsgs = append(newMsgs, summary)
	newMsgs = append(newMsgs, recent...)

	observability.LoggerWithTrace(ctx).Info().Int("orig_messages", len(msgs)).Int("new_messages", len(newMsgs)).Msg("history_summarized")
	return newMsgs
}

// Message exists for future agent-level message modeling.
// Message type removed in favor of llm.Message throughout the engine API.
