package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"

	"go.opentelemetry.io/otel/trace"
)

func (e *Engine) runWithReMem(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)

	// Execute ReMem controller (internal memory reasoning/refinement)
	_, reasoningTrace, err := e.ReMemController.Execute(ctx, userInput, e.Tools.Schemas())
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
		return "", err
	}

	// Store the experience with reasoning trace AFTER we have the actual response
	feedback := "success" // default; in practice could be derived from evaluation
	log.Info().Str("user_input", userInput).Int("reasoning_steps", len(reasoningTrace)).Msg("remem_store_experience_triggered")
	bgCtx := context.Background()
	if span := trace.SpanFromContext(ctx); span != nil {
		bgCtx = trace.ContextWithSpanContext(bgCtx, span.SpanContext())
	}
	go func(ctx context.Context, input, resp, fb string, traceMsgs []string) {
		if storeErr := e.ReMemController.StoreExperience(ctx, input, resp, fb, traceMsgs); storeErr != nil {
			log.Error().Err(storeErr).Str("feedback", fb).Msg("remem_store_experience_failed")
			return
		}
		log.Info().Str("feedback", fb).Int("reasoning_steps", len(traceMsgs)).Msg("remem_experience_stored")
	}(bgCtx, userInput, final, feedback, reasoningTrace)

	return final, nil
}
