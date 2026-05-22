package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"time"

	"github.com/rs/zerolog/log"
)

func (e *Engine) Run(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
	startedAt := time.Now().UTC()
	var final string
	var err error
	var evolvingEntryID string
	var reasoningTrace []string
	defer func() {
		evolvingEntryID = e.storeExperience(ctx, userInput, final, err, reasoningTrace)
		e.recordRunEpisode(ctx, startedAt, final, err, evolvingEntryID)
	}()

	// If ReMem mode is enabled, use Think-Act-Refine controller
	if e.ReMemEnabled && e.ReMemController != nil {
		final, reasoningTrace, err = e.runWithReMem(ctx, userInput, history, false)
		return final, err
	}

	msgs := BuildInitialLLMMessages(e.System, userInput, history)
	msgs = PrependToCurrentUserMessage(msgs, e.UserPromptContext)
	msgs = e.augmentWithPolicyContext(ctx, userInput, msgs)
	msgs = e.augmentWithBeliefMemory(ctx, userInput, msgs)

	// Augment with evolving memory (ExpRAG or ExpRecent)
	if e.EvolvingMemory != nil {
		log.Info().Bool("enabled", true).Msg("evolving_memory_enabled")
		msgs = e.augmentWithMemory(ctx, userInput, msgs)
	} else {
		log.Debug().Bool("enabled", false).Msg("evolving_memory_disabled")
	}

	// Possibly summarize older history to avoid unbounded token growth.
	if e.SummaryEnabled && !e.consumeSkipInitialSummarization() {
		msgs = e.maybeSummarize(ctx, msgs)
	}

	if e.HarnessEnabled {
		final, err = e.runHarnessLoop(ctx, msgs)
	} else {
		final, err = e.runLoop(ctx, msgs)
	}
	if err != nil {
		return "", err
	}

	return final, nil
}

// RunStream executes the agent loop with streaming support
func (e *Engine) RunStream(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	startedAt := time.Now().UTC()
	var final string
	var err error
	var evolvingEntryID string
	var reasoningTrace []string
	defer func() {
		evolvingEntryID = e.storeExperience(ctx, userInput, final, err, reasoningTrace)
		e.recordRunEpisode(ctx, startedAt, final, err, evolvingEntryID)
	}()

	// If ReMem mode is enabled, use Think-Act-Refine controller
	// Note: streaming with ReMem may need special handling for THINK/REFINE steps
	if e.ReMemEnabled && e.ReMemController != nil {
		final, reasoningTrace, err = e.runWithReMem(ctx, userInput, history, true)
		return final, err
	}

	msgs := BuildInitialLLMMessages(e.System, userInput, history)
	msgs = PrependToCurrentUserMessage(msgs, e.UserPromptContext)
	msgs = e.augmentWithPolicyContext(ctx, userInput, msgs)
	msgs = e.augmentWithBeliefMemory(ctx, userInput, msgs)

	// Augment with evolving memory (ExpRAG or ExpRecent)
	if e.EvolvingMemory != nil {
		log.Info().Bool("enabled", true).Msg("evolving_memory_enabled_stream")
		msgs = e.augmentWithMemory(ctx, userInput, msgs)
	} else {
		log.Debug().Bool("enabled", false).Msg("evolving_memory_disabled_stream")
	}

	// Possibly summarize older history to avoid unbounded token growth.
	if e.SummaryEnabled && !e.consumeSkipInitialSummarization() {
		msgs = e.maybeSummarize(ctx, msgs)
	}

	if e.HarnessEnabled {
		final, err = e.runHarnessStreamLoop(ctx, msgs)
	} else {
		final, err = e.runStreamLoop(ctx, msgs)
	}
	if err != nil {
		return "", err
	}

	return final, nil
}
