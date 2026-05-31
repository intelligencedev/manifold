package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

func (e *Engine) runWithReMem(ctx context.Context, userInput string, history []llm.Message, stream bool) (string, []string, error) {
	log := observability.LoggerWithTrace(ctx)
	if e.DisableEvolvingMemory || e.ReMemController == nil {
		msgs := BuildInitialLLMMessages(e.System, userInput, history)
		e.emitContextMetrics(ctx, msgs, ContextMetricPhaseAssembled, nil, 0)
		if e != nil && e.SummaryEnabled && !e.consumeSkipInitialSummarization() {
			msgs = e.maybeSummarize(ctx, msgs)
		}
		msgs = AddRuntimeContextToCurrentUserMessage(msgs, e.UserPromptContext)
		e.emitContextMetrics(ctx, msgs, ContextMetricPhaseRuntimeAdded, nil, 0)
		msgs = e.augmentWithPolicyContext(ctx, userInput, msgs)
		msgs = e.augmentWithBeliefMemory(ctx, userInput, msgs)
		if e != nil && !e.DisableEvolvingMemory && e.EvolvingMemory != nil {
			msgs = e.augmentWithMemory(ctx, userInput, msgs)
		}
		if stream {
			final, err := e.runStreamLoop(ctx, msgs)
			return final, nil, err
		}
		final, err := e.runLoop(ctx, msgs)
		return final, nil, err
	}

	_, reasoningTrace, err := e.ReMemController.Execute(ctx, userInput, nil)
	if err != nil {
		log.Error().Err(err).Msg("remem_execute_failed")
		// Continue with main loop even if ReMem fails - don't block user response
		log.Warn().Msg("remem_failed_continuing_with_main_loop")
	} else {
		log.Info().Int("reasoning_steps", len(reasoningTrace)).Msg("remem_completed")
	}

	// Now run the main agent loop with (potentially refined) memories
	msgs := BuildInitialLLMMessages(e.System, userInput, history)
	e.emitContextMetrics(ctx, msgs, ContextMetricPhaseAssembled, nil, 0)
	// Possibly summarize older history
	if e.SummaryEnabled && !e.consumeSkipInitialSummarization() {
		msgs = e.maybeSummarize(ctx, msgs)
	}
	msgs = AddRuntimeContextToCurrentUserMessage(msgs, e.UserPromptContext)
	e.emitContextMetrics(ctx, msgs, ContextMetricPhaseRuntimeAdded, nil, 0)
	msgs = e.augmentWithPolicyContext(ctx, userInput, msgs)
	msgs = e.augmentWithBeliefMemory(ctx, userInput, msgs)

	// Augment with evolving memory (which may have been refined by ReMem)
	if !e.DisableEvolvingMemory && e.EvolvingMemory != nil {
		msgs = e.augmentWithMemory(ctx, userInput, msgs)
	}

	if e.HarnessEnabled {
		cfg := e.effectiveHarnessConfig()
		log.Info().Str("mode", string(cfg.Mode)).Bool("stream", stream).Msg("remem_continuing_with_harness_loop")
		var final string
		var err error
		if stream {
			final, err = e.runHarnessStreamLoop(ctx, msgs)
		} else {
			final, err = e.runHarnessLoop(ctx, msgs)
		}
		if err != nil {
			return "", reasoningTrace, err
		}
		return final, reasoningTrace, nil
	}

	// Run the streaming loop to generate actual response (preserves existing ReMem behavior)
	final, err := e.runStreamLoop(ctx, msgs)
	if err != nil {
		return "", reasoningTrace, err
	}

	return final, reasoningTrace, nil
}
