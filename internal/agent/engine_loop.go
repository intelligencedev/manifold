package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

func (e *Engine) runLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
	var final string

	for step := 0; step < e.MaxSteps; step++ {
		log.Debug().Int("step", step).Int("history", len(msgs)).Msg("engine_step_start")

		// Re-summarize if context has grown too large during tool execution
		if e.SummaryEnabled && step > 0 {
			msgs = e.maybeSummarize(ctx, msgs)
		}

		// Capture tool schemas once per step so we can log what the model sees.
		schemas := e.Tools.Schemas()
		toolNames := make([]string, len(schemas))
		for i, s := range schemas {
			toolNames[i] = s.Name
		}
		log.Info().Strs("tools_sent_to_llm", toolNames).Msg("engine_tools_before_chat")

		msgs = e.enforceContextBudget(ctx, msgs)
		msg, err := e.LLM.Chat(ctx, msgs, schemas, e.model())
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_step_error")
			return "", err
		}

		msg.ToolCalls = e.ensureToolCallIDs(msgs, msg.ToolCalls)
		msgs = append(msgs, msg)
		if e.OnAssistant != nil {
			e.OnAssistant(msg)
		}
		if e.OnTurnMessage != nil {
			e.OnTurnMessage(msg)
		}

		if len(msg.ToolCalls) == 0 {
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_final")
			final = msg.Content
			break
		}

		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_tool_calls")
		msgs = e.dispatchTools(ctx, msgs, msg.ToolCalls)
	}

	if final == "" {
		final = "(no final text — increase max steps or check logs)"
	}

	return final, nil
}

// runStreamLoop contains the core streaming agent step loop shared by RunStream.
// It returns the final assistant content or an error.
func (e *Engine) runStreamLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
	var final string

	for step := 0; step < e.MaxSteps; step++ {
		// Re-summarize if context has grown too large during tool execution
		if e.SummaryEnabled && step > 0 {
			msgs = e.maybeSummarize(ctx, msgs)
		}

		// Accumulate streaming content and tool calls for this step
		var (
			accumulatedContent    string
			accumulatedToolCalls  []llm.ToolCall
			accumulatedImages     []llm.GeneratedImage
			accumulatedThoughtSig string
		)

		handler := &streamHandler{
			onDelta: func(content string) {
				accumulatedContent += content
				if e.OnDelta != nil {
					e.OnDelta(content)
				}
			},
			onThoughtSummary: func(summary string) {
				if e.OnThoughtSummary != nil {
					e.OnThoughtSummary(summary)
				}
			},
			onThoughtSignature: func(sig string) {
				accumulatedThoughtSig = sig
			},
			onToolCall: func(tc llm.ToolCall) {
				accumulatedToolCalls = append(accumulatedToolCalls, tc)
			},
			onImage: func(img llm.GeneratedImage) {
				accumulatedImages = append(accumulatedImages, img)
			},
		}

		log.Debug().Int("step", step).Int("history", len(msgs)).Msg("engine_stream_step_start")

		// Capture tool schemas once per step so we can log what the model sees.
		schemas := e.Tools.Schemas()
		toolNames := make([]string, len(schemas))
		for i, s := range schemas {
			toolNames[i] = s.Name
		}
		log.Info().Strs("tools_sent_to_llm_stream", toolNames).Msg("engine_tools_before_stream")

		msgs = e.enforceContextBudget(ctx, msgs)
		if err := e.LLM.ChatStream(ctx, msgs, schemas, e.model(), handler); err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_stream_step_error")
			return "", err
		}

		accumulatedToolCalls = e.ensureToolCallIDs(msgs, accumulatedToolCalls)
		msg := llm.Message{
			Role:             "assistant",
			Content:          accumulatedContent,
			ToolCalls:        accumulatedToolCalls,
			Images:           accumulatedImages,
			ThoughtSignature: accumulatedThoughtSig,
		}

		msgs = append(msgs, msg)
		if e.OnAssistant != nil {
			e.OnAssistant(msg)
		}
		if e.OnTurnMessage != nil {
			e.OnTurnMessage(msg)
		}

		if len(msg.ToolCalls) == 0 {
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_stream_final")
			final = msg.Content
			break
		}

		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_stream_tool_calls")
		msgs = e.dispatchTools(ctx, msgs, msg.ToolCalls)
	}

	if final == "" {
		final = "(no final text — increase max steps or check logs)"
	}

	return final, nil
}
