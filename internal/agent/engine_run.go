package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"time"
)

func (e *Engine) Run(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
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

	msgs, found, checkpointErr := e.loadPreparedContext(ctx)
	if checkpointErr != nil {
		return "", checkpointErr
	}
	if !found {
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

			// Augment with evolving memory (ExpRAG or ExpRecent)
			if !e.DisableEvolvingMemory && e.EvolvingMemory != nil {
				log.Info().Bool("enabled", true).Msg("evolving_memory_enabled")
				msgs = e.augmentWithMemory(ctx, userInput, msgs)
			} else {
				log.Debug().Bool("enabled", false).Msg("evolving_memory_disabled")
			}
		}
		if err := e.savePreparedContext(ctx, msgs); err != nil {
			return "", err
		}
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
	log := observability.LoggerWithTrace(ctx)
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

	msgs, found, checkpointErr := e.loadPreparedContext(ctx)
	if checkpointErr != nil {
		return "", checkpointErr
	}
	if !found {
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

			// Augment with evolving memory (ExpRAG or ExpRecent)
			if !e.DisableEvolvingMemory && e.EvolvingMemory != nil {
				log.Info().Bool("enabled", true).Msg("evolving_memory_enabled_stream")
				msgs = e.augmentWithMemory(ctx, userInput, msgs)
			} else {
				log.Debug().Bool("enabled", false).Msg("evolving_memory_disabled_stream")
			}
		}
		if err := e.savePreparedContext(ctx, msgs); err != nil {
			return "", err
		}
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
