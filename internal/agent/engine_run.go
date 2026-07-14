package agent

import (
	"context"
	"time"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

func (e *Engine) Run(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	startedAt := time.Now().UTC()
	var final string
	var err error
	var reasoningTrace []string
	defer func() {
		e.recordMemoryEpisode(ctx, runEpisodeRecord{
			startedAt:      startedAt,
			userInput:      userInput,
			final:          final,
			runErr:         err,
			reasoningTrace: reasoningTrace,
		})
	}()

	// If ReMem mode is enabled, use Think-Act-Refine controller
	if !e.DisableEvolvingMemory && e.ReMemEnabled && e.ReMemController != nil {
		final, reasoningTrace, err = e.runWithReMem(ctx, userInput, history, false)
		return final, err
	}

	msgs, err := e.prepareRunMessages(ctx, userInput, history, false)
	if err != nil {
		return "", err
	}

	if e.HarnessEnabled {
		final, err = e.runHarnessLoop(ctx, msgs)
	} else {
		final, err = e.runLoop(ctx, msgs)
	}
	if err != nil {
		return "", err
	}
	if err := e.saveCheckpoint(ctx, finalCheckpointKey(), final); err != nil {
		return "", err
	}

	return final, nil
}

// RunStream executes the agent loop with streaming support
func (e *Engine) RunStream(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	startedAt := time.Now().UTC()
	var final string
	var err error
	var reasoningTrace []string
	defer func() {
		e.recordMemoryEpisode(ctx, runEpisodeRecord{
			startedAt:      startedAt,
			userInput:      userInput,
			final:          final,
			runErr:         err,
			reasoningTrace: reasoningTrace,
		})
	}()

	// If ReMem mode is enabled, use Think-Act-Refine controller
	// Note: streaming with ReMem may need special handling for THINK/REFINE steps
	if !e.DisableEvolvingMemory && e.ReMemEnabled && e.ReMemController != nil {
		final, reasoningTrace, err = e.runWithReMem(ctx, userInput, history, true)
		return final, err
	}

	msgs, err := e.prepareRunMessages(ctx, userInput, history, true)
	if err != nil {
		return "", err
	}

	if e.HarnessEnabled {
		final, err = e.runHarnessStreamLoop(ctx, msgs)
	} else {
		final, err = e.runStreamLoop(ctx, msgs)
	}
	if err != nil {
		return "", err
	}
	if err := e.saveCheckpoint(ctx, finalCheckpointKey(), final); err != nil {
		return "", err
	}

	return final, nil
}

func (e *Engine) prepareRunMessages(ctx context.Context, userInput string, history []llm.Message, stream bool) ([]llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	msgs, found, err := e.loadPreparedContext(ctx)
	if err != nil {
		return nil, err
	}
	if found {
		return msgs, nil
	}

	msgs = BuildInitialLLMMessages(e.System, userInput, history)
	e.emitContextMetrics(ctx, msgs, ContextMetricPhaseAssembled, nil, 0)
	if e.SummaryEnabled && !e.consumeSkipInitialSummarization() {
		msgs = e.maybeSummarize(ctx, msgs)
	}
	msgs = AddRuntimeContextToCurrentUserMessage(msgs, e.UserPromptContext)
	e.emitContextMetrics(ctx, msgs, ContextMetricPhaseRuntimeAdded, nil, 0)
	if e.Memory != nil && !e.DisableMemory {
		msgs = e.augmentWithUnifiedMemory(ctx, userInput, msgs)
	} else {
		msgs = e.augmentWithPolicyContext(ctx, userInput, msgs)
		msgs = e.augmentWithBeliefMemory(ctx, userInput, msgs)

		if !e.DisableEvolvingMemory && e.EvolvingMemory != nil {
			log.Info().Bool("enabled", true).Bool("stream", stream).Msg("evolving_memory_enabled")
			msgs = e.augmentWithMemory(ctx, userInput, msgs)
		} else {
			log.Debug().Bool("enabled", false).Bool("stream", stream).Msg("evolving_memory_disabled")
		}
	}
	if err := e.savePreparedContext(ctx, msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}
