package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

func (e *Engine) runWithReMem(ctx context.Context, userInput string, history []llm.Message) (string, []string, error) {
	log := observability.LoggerWithTrace(ctx)

	// Execute ReMem controller (internal memory reasoning/refinement).
	// ReMem does not dispatch tools; the tool schema argument is preserved on
	// the signature for compatibility but is ignored by the controller.
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
	msgs = PrependToCurrentUserMessage(msgs, e.UserPromptContext)
	msgs = e.augmentWithPolicyContext(ctx, userInput, msgs)
	msgs = e.augmentWithBeliefMemory(ctx, userInput, msgs)

	// Augment with evolving memory (which may have been refined by ReMem)
	if e.EvolvingMemory != nil {
		msgs = e.augmentWithMemory(ctx, userInput, msgs)
	}

	// Possibly summarize older history
	if e.SummaryEnabled && !e.consumeSkipInitialSummarization() {
		msgs = e.maybeSummarize(ctx, msgs)
	}

	// Run the streaming loop to generate actual response (preserves streaming behavior)
	final, err := e.runStreamLoop(ctx, msgs)
	if err != nil {
		return "", reasoningTrace, err
	}

	return final, reasoningTrace, nil
}
