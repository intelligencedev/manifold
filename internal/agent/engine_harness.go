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
	log := observability.LoggerWithTrace(ctx)
	cfg := e.effectiveHarnessConfig()
	restoreTools := e.withHarnessToolRegistry(cfg)
	defer restoreTools()
	history := harness.WrapMessages(msgs)
	tracker := harness.NewStepTracker()
	toolErrors := harness.NewToolErrorTracker(cfg.MaxToolErrors)
	var final string

	for step := 0; step < e.MaxSteps; step++ {
		log.Debug().Int("step", step).Int("history", len(history)).Msg("engine_harness_step_start")

		schemas := e.Tools.Schemas()
		toolNames := make([]string, len(schemas))
		for i, schema := range schemas {
			toolNames[i] = schema.Name
		}
		log.Info().Strs("tools_sent_to_llm", toolNames).Msg("engine_harness_tools_before_chat")

		history = e.prepareHarnessHistory(ctx, history, cfg, tracker)
		priorHistory := history
		result, err := harness.RunInference(ctx, e.LLM, history, schemas, e.model(), cfg, tracker)
		history = result.History
		if len(history) >= len(priorHistory) {
			markHarnessStep(history[len(priorHistory):], step)
		}
		if len(result.Message.ToolCalls) > 0 {
			result.Message.ToolCalls = e.ensureToolCallIDs(harness.SerializeMessages(priorHistory), result.Message.ToolCalls)
			updateLastAssistantMessage(history, result.Message)
		}
		if len(history) >= len(priorHistory) {
			e.emitHarnessTurnMessages(history[len(priorHistory):])
		}
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_harness_step_error")
			return "", err
		}

		msg := result.Message
		if len(msg.ToolCalls) == 0 {
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_harness_final")
			final = msg.Content
			break
		}

		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_harness_tool_calls")
		providerHistory := harness.SerializeMessages(history)
		beforeTools := len(providerHistory)
		providerHistory = e.dispatchTools(ctx, providerHistory, msg.ToolCalls)
		toolMessages := providerHistory[beforeTools:]
		var toolErrorNudges []harness.HarnessMessage
		for i, toolMessage := range toolMessages {
			if i >= len(msg.ToolCalls) {
				break
			}
			call := msg.ToolCalls[i]
			if toolErr, isToolErr := harness.DetectToolErrorPayload([]byte(toolMessage.Content)); isToolErr {
				count, err := toolErrors.RecordFailure(call.Name, toolErr)
				if err != nil {
					log.Error().Err(err).Str("tool", call.Name).Int("consecutive_tool_errors", count).Msg("engine_harness_tool_error_budget_exhausted")
					return "", err
				}
				toolErrorNudges = append(toolErrorNudges, harness.HarnessMessage{
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
				})
				continue
			}
			toolErrors.RecordSuccess()
			tracker.RecordSuccess(call)
			if cfg.Workflow.IsTerminalTool(call.Name) {
				final = terminalResultText(toolMessage.Content)
			}
		}
		history = appendHarnessToolMessages(history, toolMessages, msg.ToolCalls, step)
		if len(toolErrorNudges) > 0 {
			history = append(history, toolErrorNudges...)
			e.emitHarnessTurnMessages(toolErrorNudges)
		}
		if final != "" {
			log.Info().Int("step", step).Int("final_len", len(final)).Msg("engine_harness_terminal_final")
			break
		}
	}

	if final == "" {
		final = "(no final text — increase max steps or check logs)"
	}
	return final, nil
}

func (e *Engine) runHarnessStreamLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
	cfg := e.effectiveHarnessConfig()
	restoreTools := e.withHarnessToolRegistry(cfg)
	defer restoreTools()
	history := harness.WrapMessages(msgs)
	tracker := harness.NewStepTracker()
	toolErrors := harness.NewToolErrorTracker(cfg.MaxToolErrors)
	var final string

	for step := 0; step < e.MaxSteps; step++ {
		log.Debug().Int("step", step).Int("history", len(history)).Msg("engine_harness_stream_step_start")

		schemas := e.Tools.Schemas()
		toolNames := make([]string, len(schemas))
		for i, schema := range schemas {
			toolNames[i] = schema.Name
		}
		log.Info().Strs("tools_sent_to_llm_stream", toolNames).Msg("engine_harness_tools_before_stream")

		history = e.prepareHarnessHistory(ctx, history, cfg, tracker)
		priorHistory := history
		result, err := harness.RunStreamInference(ctx, e.LLM, history, schemas, e.model(), cfg, tracker)
		history = result.History
		if len(history) >= len(priorHistory) {
			markHarnessStep(history[len(priorHistory):], step)
		}
		if len(result.Message.ToolCalls) > 0 {
			result.Message.ToolCalls = e.ensureToolCallIDs(harness.SerializeMessages(priorHistory), result.Message.ToolCalls)
			updateLastAssistantMessage(history, result.Message)
		}
		if len(history) >= len(priorHistory) {
			e.emitHarnessStreamTurnMessages(history[len(priorHistory):], result)
		}
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_harness_stream_step_error")
			return "", err
		}

		msg := result.Message
		if len(msg.ToolCalls) == 0 {
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_harness_stream_final")
			final = msg.Content
			break
		}

		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_harness_stream_tool_calls")
		providerHistory := harness.SerializeMessages(history)
		beforeTools := len(providerHistory)
		providerHistory = e.dispatchTools(ctx, providerHistory, msg.ToolCalls)
		toolMessages := providerHistory[beforeTools:]
		var toolErrorNudges []harness.HarnessMessage
		for i, toolMessage := range toolMessages {
			if i >= len(msg.ToolCalls) {
				break
			}
			call := msg.ToolCalls[i]
			if toolErr, isToolErr := harness.DetectToolErrorPayload([]byte(toolMessage.Content)); isToolErr {
				count, err := toolErrors.RecordFailure(call.Name, toolErr)
				if err != nil {
					log.Error().Err(err).Str("tool", call.Name).Int("consecutive_tool_errors", count).Msg("engine_harness_stream_tool_error_budget_exhausted")
					return "", err
				}
				toolErrorNudges = append(toolErrorNudges, harness.HarnessMessage{
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
				})
				continue
			}
			toolErrors.RecordSuccess()
			tracker.RecordSuccess(call)
			if cfg.Workflow.IsTerminalTool(call.Name) {
				final = terminalResultText(toolMessage.Content)
			}
		}
		history = appendHarnessToolMessages(history, toolMessages, msg.ToolCalls, step)
		if len(toolErrorNudges) > 0 {
			history = append(history, toolErrorNudges...)
			e.emitHarnessTurnMessages(toolErrorNudges)
		}
		if final != "" {
			log.Info().Int("step", step).Int("final_len", len(final)).Msg("engine_harness_stream_terminal_final")
			break
		}
	}

	if final == "" {
		final = "(no final text — increase max steps or check logs)"
	}
	return final, nil
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
		if i == acceptedAssistant {
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
