package agent

import (
	"context"
	"encoding/json"
	"strings"

	"manifold/internal/agent/harness"
	"manifold/internal/llm"
	"manifold/internal/llm/budget"
	"manifold/internal/observability"
	"manifold/internal/tools"
	"manifold/internal/tools/utility"

	"github.com/rs/zerolog"
)

func (e *Engine) effectiveHarnessConfig() harness.RunConfig {
	cfg := e.HarnessConfig
	if strings.TrimSpace(string(cfg.Mode)) == "" {
		cfg.Mode = harness.ModeGuardedChat
	}
	if cfg.Compact.ContextWindowTokens <= 0 {
		cfg.Compact.ContextWindowTokens = e.ContextWindowTokens
		if cfg.Compact.ContextWindowTokens <= 0 {
			if size, _ := llm.ContextSize(e.model()); size > 0 {
				cfg.Compact.ContextWindowTokens = size
			}
		}
	}
	if cfg.Compact.ReserveTokens <= 0 {
		cfg.Compact.ReserveTokens = e.SummaryReserveBufferTokens
		if cfg.Compact.ReserveTokens <= 0 {
			cfg.Compact.ReserveTokens = budget.DefaultReserveBuffer
		}
	}
	if cfg.Compact.PerMessageRunes <= 0 {
		cfg.Compact.PerMessageRunes = budget.DefaultPerMsgRunes
		if e.SummaryMaxSummaryChunkTokens > 0 {
			cfg.Compact.PerMessageRunes = e.SummaryMaxSummaryChunkTokens * 4
		}
	}
	return cfg
}

func (e *Engine) runHarnessLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	return e.runHarnessLoopMode(ctx, msgs, false)
}

func (e *Engine) runHarnessStreamLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	return e.runHarnessLoopMode(ctx, msgs, true)
}

func (e *Engine) runHarnessLoopMode(ctx context.Context, msgs []llm.Message, stream bool) (string, error) {
	cfg := e.effectiveHarnessConfig()
	restoreTools := e.withHarnessToolRegistry(cfg)
	defer restoreTools()
	state := newHarnessLoopState(cfg, msgs)
	var final string
	finalSet := false

	for step := 0; e.stepAllowed(step); step++ {
		msg, err := e.runHarnessStep(ctx, state, step, stream)
		if err != nil {
			return "", err
		}
		if len(msg.ToolCalls) == 0 {
			final = msg.Content
			finalSet = true
			break
		}

		var terminal bool
		final, terminal, err = e.handleHarnessTools(ctx, state, msg, step, stream)
		if err != nil {
			return "", err
		}
		if terminal {
			finalSet = true
			break
		}
	}

	if !finalSet {
		return "", MaxStepsExceededError{MaxSteps: e.MaxSteps}
	}
	return final, nil
}

type harnessLoopState struct {
	cfg        harness.RunConfig
	history    []harness.HarnessMessage
	tracker    *harness.StepTracker
	toolErrors *harness.ToolErrorTracker
}

func newHarnessLoopState(cfg harness.RunConfig, msgs []llm.Message) *harnessLoopState {
	return &harnessLoopState{
		cfg:        cfg,
		history:    harness.WrapMessages(msgs),
		tracker:    harness.NewStepTracker(),
		toolErrors: harness.NewToolErrorTracker(cfg.MaxToolErrors),
	}
}

func (e *Engine) runHarnessStep(ctx context.Context, state *harnessLoopState, step int, stream bool) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	log.Debug().Int("step", step).Int("history", len(state.history)).Msg(harnessStepStartEvent(stream))
	schemas := e.Tools.Schemas()
	log.Info().Strs(harnessToolsLogField(stream), toolSchemaNames(schemas)).Msg(harnessToolsLogEvent(stream))

	state.history = e.prepareHarnessHistory(ctx, state.history, state.cfg, state.tracker)
	priorHistory := state.history
	// Permanent harness history stays uncompressed. Inference / emitLLMRequest
	// see a provider-visible minified content view only.
	providerSerial := e.lexMinifyForProvider(ctx, harness.SerializeMessages(priorHistory))
	minifiedHistory := applyProviderContentToHarness(priorHistory, providerSerial)
	e.emitContextMetrics(ctx, providerSerial, ContextMetricPhasePreModel, nil, 0)
	requestID := e.emitLLMRequest(ctx, providerSerial, schemas, e.model())
	minifiedState := *state
	minifiedState.history = minifiedHistory
	result, err := e.runHarnessInference(ctx, &minifiedState, schemas, stream)
	if requestID != "" && e.AgentTracer != nil {
		e.AgentTracer.Trace(AgentTrace{Type: "llm_request", Agent: e.AgentRole, Model: e.model(), Depth: e.AgentDepth, LLMRequestID: requestID})
	}
	// Restore permanent history with only newly appended attempt messages.
	state.history = appendHarnessAttempts(priorHistory, minifiedHistory, result.History)
	result.History = state.history
	msg := e.acceptHarnessResult(priorHistory, result, step, stream)
	e.emitContextMetrics(ctx, harness.SerializeMessages(state.history), ContextMetricPhaseAssistantAdded, nil, 0)
	if err != nil {
		log.Error().Err(err).Int("step", step).Msg(harnessStepErrorEvent(stream))
		return llm.Message{}, err
	}
	if len(msg.ToolCalls) == 0 {
		log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg(harnessFinalEvent(stream))
	}
	return msg, nil
}

// applyProviderContentToHarness returns a shallow copy of history with Content
// taken from providerMsgs (same length). Meta is preserved.
func applyProviderContentToHarness(history []harness.HarnessMessage, providerMsgs []llm.Message) []harness.HarnessMessage {
	out := make([]harness.HarnessMessage, len(history))
	copy(out, history)
	n := len(providerMsgs)
	if n > len(out) {
		n = len(out)
	}
	for i := 0; i < n; i++ {
		out[i].Message.Content = providerMsgs[i].Content
	}
	return out
}

// appendHarnessAttempts rebuilds permanent history as original prefix + any
// messages inference appended after the minified view of that prefix.
func appendHarnessAttempts(prior, minifiedPrefix, resultHistory []harness.HarnessMessage) []harness.HarnessMessage {
	if len(resultHistory) <= len(minifiedPrefix) {
		out := make([]harness.HarnessMessage, len(prior))
		copy(out, prior)
		return out
	}
	appended := resultHistory[len(minifiedPrefix):]
	out := make([]harness.HarnessMessage, 0, len(prior)+len(appended))
	out = append(out, prior...)
	out = append(out, appended...)
	return out
}

func (e *Engine) runHarnessInference(ctx context.Context, state *harnessLoopState, schemas []llm.ToolSchema, stream bool) (harness.InferenceResult, error) {
	req := harness.InferenceRequest{Provider: e.LLM, History: state.history, Schemas: schemas, Model: e.model(), Config: state.cfg, Tracker: state.tracker}
	if stream {
		req.Live = engineStreamObserver{engine: e}
		return harness.RunStreamInference(ctx, req)
	}
	return harness.RunInference(ctx, req)
}

// engineStreamObserver forwards guarded-harness streaming output to the engine's
// live callbacks so deltas and reasoning summaries reach consumers as the model
// produces them, rather than only after an attempt is accepted.
type engineStreamObserver struct {
	engine *Engine
}

func (o engineStreamObserver) OnDelta(delta string) {
	if o.engine.OnDelta != nil {
		o.engine.OnDelta(delta)
	}
}

func (o engineStreamObserver) OnThoughtSummary(summary string) {
	if o.engine.OnThoughtSummary != nil {
		o.engine.OnThoughtSummary(summary)
	}
}

func (o engineStreamObserver) Reset(chars int) {
	if chars > 0 && o.engine.OnStreamRollback != nil {
		o.engine.OnStreamRollback(chars)
	}
}

func (e *Engine) acceptHarnessResult(priorHistory []harness.HarnessMessage, result harness.InferenceResult, step int, stream bool) llm.Message {
	msg := result.Message
	if len(result.History) >= len(priorHistory) {
		markHarnessStep(result.History[len(priorHistory):], step)
	}
	if len(msg.ToolCalls) > 0 {
		msg.ToolCalls = e.ensureToolCallIDs(harness.SerializeMessages(priorHistory), msg.ToolCalls)
		updateLastAssistantMessage(result.History, msg)
	}
	if len(result.History) < len(priorHistory) {
		return msg
	}
	if stream {
		e.emitHarnessStreamTurnMessages(result.History[len(priorHistory):], result)
		return msg
	}
	e.emitHarnessTurnMessages(result.History[len(priorHistory):])
	return msg
}

func (e *Engine) handleHarnessTools(ctx context.Context, state *harnessLoopState, msg llm.Message, step int, stream bool) (string, bool, error) {
	log := observability.LoggerWithTrace(ctx)
	log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg(harnessToolCallsEvent(stream))
	providerHistory := harness.SerializeMessages(state.history)
	beforeTools := len(providerHistory)
	providerHistory = e.dispatchTools(ctx, providerHistory, msg.ToolCalls)
	toolMessages := providerHistory[beforeTools:]
	final, terminal, nudges, err := processHarnessToolMessages(state, toolMessages, msg.ToolCalls, step, stream, log)
	if err != nil {
		return "", false, err
	}
	state.history = appendHarnessToolMessages(state.history, toolMessages, msg.ToolCalls, step)
	if len(nudges) > 0 {
		state.history = append(state.history, nudges...)
		e.emitHarnessTurnMessages(nudges)
	}
	e.emitContextMetrics(ctx, harness.SerializeMessages(state.history), ContextMetricPhaseToolAdded, nil, 0)
	if terminal {
		log.Info().Int("step", step).Int("final_len", len(final)).Msg(harnessTerminalFinalEvent(stream))
	}
	return final, terminal, nil
}

func processHarnessToolMessages(state *harnessLoopState, toolMessages []llm.Message, calls []llm.ToolCall, step int, stream bool, log *zerolog.Logger) (string, bool, []harness.HarnessMessage, error) {
	var final string
	var terminal bool
	var nudges []harness.HarnessMessage
	for i, toolMessage := range toolMessages {
		if i >= len(calls) {
			break
		}
		call := calls[i]
		if toolErr, isToolErr := harness.DetectToolErrorPayload([]byte(toolMessage.Content)); isToolErr {
			nudge, err := recordHarnessToolError(state, call, toolErr, step, stream, log)
			if err != nil {
				return "", false, nil, err
			}
			nudges = append(nudges, nudge)
			continue
		}
		state.toolErrors.RecordSuccess()
		state.tracker.RecordSuccess(call)
		if state.cfg.Workflow.IsTerminalTool(call.Name) {
			final = terminalResultText(toolMessage.Content)
			terminal = true
		}
	}
	return final, terminal, nudges, nil
}

func recordHarnessToolError(state *harnessLoopState, call llm.ToolCall, toolErr harness.ToolError, step int, stream bool, log *zerolog.Logger) (harness.HarnessMessage, error) {
	count, err := state.toolErrors.RecordFailure(call.Name, toolErr)
	if err != nil {
		log.Error().Err(err).Str("tool", call.Name).Int("consecutive_tool_errors", count).Msg(harnessToolBudgetEvent(stream))
		return harness.HarnessMessage{}, err
	}
	return harness.HarnessMessage{
		Message: llm.Message{
			Role:    "user",
			Content: harness.ToolErrorNudge(call.Name, toolErr),
		},
		Meta: harness.MessageMeta{
			Type:       harness.MessageTypeNudge,
			StepIndex:  step,
			ToolName:   call.Name,
			ToolCallID: call.ID,
		},
	}, nil
}

func harnessStepStartEvent(stream bool) string {
	if stream {
		return "engine_harness_stream_step_start"
	}
	return "engine_harness_step_start"
}

func harnessToolsLogField(stream bool) string {
	if stream {
		return "tools_sent_to_llm_stream"
	}
	return "tools_sent_to_llm"
}

func harnessToolsLogEvent(stream bool) string {
	if stream {
		return "engine_harness_tools_before_stream"
	}
	return "engine_harness_tools_before_chat"
}

func harnessStepErrorEvent(stream bool) string {
	if stream {
		return "engine_harness_stream_step_error"
	}
	return "engine_harness_step_error"
}

func harnessFinalEvent(stream bool) string {
	if stream {
		return "engine_harness_stream_final"
	}
	return "engine_harness_final"
}

func harnessToolCallsEvent(stream bool) string {
	if stream {
		return "engine_harness_stream_tool_calls"
	}
	return "engine_harness_tool_calls"
}

func harnessTerminalFinalEvent(stream bool) string {
	if stream {
		return "engine_harness_stream_terminal_final"
	}
	return "engine_harness_terminal_final"
}

func harnessToolBudgetEvent(stream bool) string {
	if stream {
		return "engine_harness_stream_tool_error_budget_exhausted"
	}
	return "engine_harness_tool_error_budget_exhausted"
}

func (e *Engine) withHarnessToolRegistry(cfg harness.RunConfig) func() {
	original := e.Tools
	if !cfg.Workflow.IsTerminalTool("agent_response") {
		return func() {}
	}
	if original == nil {
		e.Tools = tools.NewRegistry()
	}
	e.Tools = tools.NewOverlayRegistry(e.Tools, utility.NewAgentResponseTool())
	return func() {
		e.Tools = original
	}
}

func (e *Engine) prepareHarnessHistory(ctx context.Context, history []harness.HarnessMessage, cfg harness.RunConfig, tracker *harness.StepTracker) []harness.HarnessMessage {
	if cfg.Compact.Enabled {
		result := harness.CompactMessages(history, cfg.Compact, tracker)
		if result.Changed {
			observability.LoggerWithTrace(ctx).Info().
				Int("phase", result.Phase).
				Int("messages_before", len(history)).
				Int("messages_after", len(result.Messages)).
				Int("tokens_before", result.TokensBefore).
				Int("tokens_after", result.TokensAfter).
				Int("token_budget", result.BudgetTokens).
				Int("dropped_nudges", result.DroppedNudges).
				Int("truncated_tool_results", result.TruncatedToolResults).
				Int("compacted_tool_results", result.CompactedToolResults).
				Int("compacted_assistant_messages", result.CompactedAssistantMessages).
				Msg("engine_harness_context_compacted")
		}
		return result.Messages
	}
	return harness.WrapMessages(e.enforceContextBudget(ctx, harness.SerializeMessages(history)))
}

func markHarnessStep(messages []harness.HarnessMessage, step int) {
	for i := range messages {
		switch messages[i].Meta.Type {
		case harness.MessageTypeAssistant, harness.MessageTypeTool, harness.MessageTypeNudge:
			messages[i].Meta.StepIndex = step
		}
	}
}

func appendHarnessToolMessages(history []harness.HarnessMessage, toolMessages []llm.Message, calls []llm.ToolCall, step int) []harness.HarnessMessage {
	for i, toolMessage := range toolMessages {
		meta := harness.MessageMeta{
			Type:       harness.MessageTypeTool,
			StepIndex:  step,
			ToolCallID: toolMessage.ToolID,
		}
		if i < len(calls) {
			meta.ToolName = calls[i].Name
			if meta.ToolCallID == "" {
				meta.ToolCallID = calls[i].ID
			}
		}
		history = append(history, harness.HarnessMessage{
			Message: toolMessage,
			Meta:    meta,
		})
	}
	return history
}

func updateLastAssistantMessage(history []harness.HarnessMessage, msg llm.Message) {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Message.Role != "assistant" {
			continue
		}
		history[i].Message = msg
		return
	}
}

func (e *Engine) emitHarnessStreamTurnMessages(messages []harness.HarnessMessage, result harness.InferenceResult) {
	acceptedAssistant := -1
	if result.Validation.Valid {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Message.Role == "assistant" {
				acceptedAssistant = i
				break
			}
		}
	}
	for i, message := range messages {
		// When output streamed live during inference, deltas and thought
		// summaries were already forwarded; replaying them here would duplicate.
		if i == acceptedAssistant && !result.Streamed {
			for _, summary := range result.ThoughtSummaries {
				if e.OnThoughtSummary != nil {
					e.OnThoughtSummary(summary)
				}
			}
			for _, delta := range result.Deltas {
				if e.OnDelta != nil {
					e.OnDelta(delta)
				}
			}
		}
		if message.Message.Role == "assistant" && e.OnAssistant != nil {
			e.OnAssistant(message.Message)
		}
		if e.OnTurnMessage != nil {
			e.OnTurnMessage(message.Message)
		}
	}
}

func (e *Engine) emitHarnessTurnMessages(messages []harness.HarnessMessage) {
	for _, message := range messages {
		if message.Message.Role == "assistant" && e.OnAssistant != nil {
			e.OnAssistant(message.Message)
		}
		if e.OnTurnMessage != nil {
			e.OnTurnMessage(message.Message)
		}
	}
}

func terminalResultText(content string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return content
	}
	if text, ok := payload["text"].(string); ok {
		return text
	}
	if output, ok := payload["output"].(string); ok {
		return output
	}
	return content
}
