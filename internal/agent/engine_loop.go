package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

func (e *Engine) runLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
	var final string
	finalSet := false

	for step := 0; e.stepAllowed(step); step++ {
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
		e.emitContextMetrics(ctx, msgs, ContextMetricPhasePreModel, nil, 0)
		var (
			msg llm.Message
			err error
		)
		if found, err := e.loadCheckpoint(ctx, assistantCheckpointKey(step), &msg); err != nil {
			return "", err
		} else if found {
			log.Info().Int("step", step).Msg("engine_step_replay_assistant_checkpoint")
		} else {
			msg, err = e.LLM.Chat(ctx, msgs, schemas, e.model())
			if err != nil {
				log.Error().Err(err).Int("step", step).Msg("engine_step_error")
				return "", err
			}
			msg.ToolCalls = llm.NormalizeToolCalls(msg.ToolCalls)
			msg.ToolCalls = e.ensureToolCallIDs(msgs, msg.ToolCalls)
			if err := e.saveCheckpoint(ctx, assistantCheckpointKey(step), msg); err != nil {
				return "", err
			}
		}

		msgs = append(msgs, msg)
		if e.OnAssistant != nil {
			e.OnAssistant(msg)
		}
		if e.OnTurnMessage != nil {
			e.OnTurnMessage(msg)
		}
		e.emitContextMetrics(ctx, msgs, ContextMetricPhaseAssistantAdded, nil, 0)

		if len(msg.ToolCalls) == 0 {
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_final")
			final = msg.Content
			finalSet = true
			break
		}

		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_tool_calls")
		msgs, err = e.dispatchToolsAtStep(ctx, msgs, msg.ToolCalls, step)
		if err != nil {
			return "", err
		}
		e.emitContextMetrics(ctx, msgs, ContextMetricPhaseToolAdded, nil, 0)
	}

	if !finalSet {
		return "", MaxStepsExceededError{MaxSteps: e.MaxSteps}
	}

	return final, nil
}

// runStreamLoop contains the core streaming agent step loop shared by RunStream.
// It returns the final assistant content or an error.
func (e *Engine) runStreamLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	var final string
	finalSet := false

	for step := 0; e.stepAllowed(step); step++ {
		if e.SummaryEnabled && step > 0 {
			msgs = e.maybeSummarize(ctx, msgs)
		}
		nextMsgs, content, done, err := e.runStreamStep(ctx, msgs, step)
		if err != nil {
			return "", err
		}
		msgs = nextMsgs
		if done {
			final = content
			finalSet = true
			break
		}
	}

	if !finalSet {
		return "", MaxStepsExceededError{MaxSteps: e.MaxSteps}
	}

	return final, nil
}

func (e *Engine) runStreamStep(ctx context.Context, msgs []llm.Message, step int) ([]llm.Message, string, bool, error) {
	log := observability.LoggerWithTrace(ctx)
	acc := newStreamAccumulator(e)

	log.Debug().Int("step", step).Int("history", len(msgs)).Msg("engine_stream_step_start")
	schemas := e.Tools.Schemas()
	log.Info().Strs("tools_sent_to_llm_stream", toolSchemaNames(schemas)).Msg("engine_tools_before_stream")

	msgs = e.enforceContextBudget(ctx, msgs)
	e.emitContextMetrics(ctx, msgs, ContextMetricPhasePreModel, nil, 0)
	var msg llm.Message
	if found, err := e.loadCheckpoint(ctx, assistantCheckpointKey(step), &msg); err != nil {
		return nil, "", false, err
	} else if found {
		log.Info().Int("step", step).Msg("engine_stream_step_replay_assistant_checkpoint")
	} else {
		if err := e.LLM.ChatStream(ctx, msgs, schemas, e.model(), acc.handler()); err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_stream_step_error")
			return nil, "", false, err
		}
		msg = acc.message(e, msgs)
		if err := e.saveCheckpoint(ctx, assistantCheckpointKey(step), msg); err != nil {
			return nil, "", false, err
		}
	}

	msgs = append(msgs, msg)
	e.emitAssistantMessage(msg)
	e.emitContextMetrics(ctx, msgs, ContextMetricPhaseAssistantAdded, nil, 0)
	if len(msg.ToolCalls) == 0 {
		log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_stream_final")
		return msgs, msg.Content, true, nil
	}

	log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_stream_tool_calls")
	msgs, err := e.dispatchToolsAtStep(ctx, msgs, msg.ToolCalls, step)
	if err != nil {
		return nil, "", false, err
	}
	e.emitContextMetrics(ctx, msgs, ContextMetricPhaseToolAdded, nil, 0)
	return msgs, "", false, nil
}

func toolSchemaNames(schemas []llm.ToolSchema) []string {
	toolNames := make([]string, len(schemas))
	for i, s := range schemas {
		toolNames[i] = s.Name
	}
	return toolNames
}

type streamAccumulator struct {
	engine           *Engine
	content          string
	toolCalls        []llm.ToolCall
	images           []llm.GeneratedImage
	thoughtSignature string
}

func newStreamAccumulator(e *Engine) *streamAccumulator {
	return &streamAccumulator{engine: e}
}

func (a *streamAccumulator) handler() *streamHandler {
	return &streamHandler{
		onDelta:            a.onDelta,
		onThoughtSummary:   a.onThoughtSummary,
		onThoughtSignature: a.onThoughtSignature,
		onToolCall:         a.onToolCall,
		onImage:            a.onImage,
	}
}

func (a *streamAccumulator) onDelta(content string) {
	a.content += content
	if a.engine.OnDelta != nil {
		a.engine.OnDelta(content)
	}
}

func (a *streamAccumulator) onThoughtSummary(summary string) {
	if a.engine.OnThoughtSummary != nil {
		a.engine.OnThoughtSummary(summary)
	}
}

func (a *streamAccumulator) onThoughtSignature(sig string) {
	a.thoughtSignature = sig
}

func (a *streamAccumulator) onToolCall(tc llm.ToolCall) {
	a.toolCalls = append(a.toolCalls, tc)
}

func (a *streamAccumulator) onImage(img llm.GeneratedImage) {
	a.images = append(a.images, img)
}

func (a *streamAccumulator) message(e *Engine, msgs []llm.Message) llm.Message {
	toolCalls := llm.NormalizeToolCalls(a.toolCalls)
	toolCalls = e.ensureToolCallIDs(msgs, toolCalls)
	return llm.Message{
		Role:             "assistant",
		Content:          a.content,
		ToolCalls:        toolCalls,
		Images:           a.images,
		ThoughtSignature: a.thoughtSignature,
	}
}

func (e *Engine) emitAssistantMessage(msg llm.Message) {
	if e.OnAssistant != nil {
		e.OnAssistant(msg)
	}
	if e.OnTurnMessage != nil {
		e.OnTurnMessage(msg)
	}
}
