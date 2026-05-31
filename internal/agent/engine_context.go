package agent

import (
	"context"
	"manifold/internal/agent/memory"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/policy"
	"strings"
	"sync"
	"time"
)

const maxEvolvingMemoryContextChars = 12000

func (e *Engine) augmentWithUnifiedMemory(ctx context.Context, userInput string, msgs []llm.Message) []llm.Message {
	if e == nil || e.DisableMemory || e.Memory == nil {
		return msgs
	}
	log := observability.LoggerWithTrace(ctx)
	block, diag, err := e.Memory.PrepareContext(ctx, memory.Request{
		UserInput:   userInput,
		UserID:      e.UserID,
		SessionID:   e.SessionID,
		ProjectID:   e.ProjectID,
		ObjectiveID: e.ObjectiveID,
		Role:        e.AgentRole,
	})
	if err != nil {
		log.Warn().Err(err).Msg("unified_memory_prepare_failed")
		return msgs
	}
	if strings.TrimSpace(block.Text) == "" {
		log.Debug().Bool("enabled", diag.Enabled).Int64("duration_ms", diag.DurationMs).Msg("unified_memory_no_context")
		return msgs
	}
	log.Info().
		Int("tokens", block.TokenEstimate).
		Bool("truncated", block.Truncated).
		Int64("duration_ms", diag.DurationMs).
		Msg("unified_memory_context_added")
	e.emitMemoryContext(block, diag)
	return AddRuntimeContextToCurrentUserMessage(msgs, block.Text)
}

func (e *Engine) emitMemoryContext(block memory.ContextBlock, diag memory.Diagnostics) {
	if e == nil || e.OnMemoryContext == nil || strings.TrimSpace(block.Text) == "" {
		return
	}
	e.OnMemoryContext(block, diag)
}

func (e *Engine) augmentWithMemory(ctx context.Context, userInput string, msgs []llm.Message) []llm.Message {
	if e == nil || e.DisableEvolvingMemory || e.EvolvingMemory == nil {
		return msgs
	}
	log := observability.LoggerWithTrace(ctx)

	log.Info().Str("user_input", userInput).Msg("evolving_memory_augment_triggered")

	var memoryContext string

	if e.EvolvingMemory != nil && e.OnMemoryEvent != nil {
		e.EvolvingMemory.SetCallbacks(&memory.MemoryCallbacks{
			OnSearch: func(evt *memory.MemoryEvent) {
				e.OnMemoryEvent(evt)
			},
			OnSynthesized: func(evt *memory.MemoryEvent) {
				e.OnMemoryEvent(evt)
			},
			OnEvolve: func(evt *memory.MemoryEvent) {
				e.OnMemoryEvent(evt)
			},
		})
	}

	// Try ExpRAG (experience retrieval) and ExpRecent (recent window) in parallel when enabled.
	if e.EvolvingMemory != nil {
		log.Debug().Msg("evolving_memory_search_starting")
		var (
			retrieved     []memory.ScoredMemoryEntry
			recentContext string
			diagnostics   memory.SearchDiagnostics
			wg            sync.WaitGroup
		)

		wg.Go(func() {
			res, diag, err := e.EvolvingMemory.SearchWithDiagnostics(ctx, userInput)
			diagnostics = diag
			if err != nil {
				log.Error().Err(err).Str("query", userInput).Msg("evolving_memory_search_failed")
				return
			}
			retrieved = res
			if len(res) > 0 {
				log.Info().Int("retrieved", len(res)).Str("query", userInput).Msg("evolving_memory_search_success")
				return
			}
			log.Debug().Str("query", userInput).Msg("evolving_memory_search_no_results")
		})

		log.Debug().Msg("evolving_memory_exprecent_starting")
		recentContext = e.EvolvingMemory.BuildExpRecentContext()
		if recentContext != "" {
			log.Info().Int("context_len", len(recentContext)).Msg("evolving_memory_exprecent_used")
		} else {
			log.Debug().Msg("evolving_memory_exprecent_empty")
		}

		wg.Wait()
		if len(retrieved) > 0 {
			memoryContext = e.EvolvingMemory.SynthesizeScored(ctx, userInput, retrieved)
			log.Info().Int("retrieved", len(retrieved)).Int("context_len", len(memoryContext)).Str("mode", diagnostics.Mode).Msg("evolving_memory_exprag_synthesized")
		} else if recentContext != "" {
			memoryContext = recentContext
			diagnostics.Mode = "recent"
		}
	}

	if memoryContext == "" {
		log.Debug().Msg("evolving_memory_no_context_skipping_augmentation")
		return msgs
	}

	log.Info().Int("context_len", len(memoryContext)).Int("orig_msgs", len(msgs)).Msg("evolving_memory_adding_runtime_context")

	memoryContext = capEvolvingMemoryContext(memoryContext)
	msgs = AddRuntimeContextToCurrentUserMessage(msgs, "## Relevant Context from Past Interactions\n\n"+memoryContext)

	log.Info().Int("msgs_count", len(msgs)).Msg("evolving_memory_augmentation_complete")
	return msgs
}

func capEvolvingMemoryContext(memoryContext string) string {
	if len(memoryContext) <= maxEvolvingMemoryContextChars {
		return memoryContext
	}
	return memoryContext[:maxEvolvingMemoryContextChars] + "\n\n[Additional memory context omitted due to prompt budget.]\n"
}

func (e *Engine) augmentWithBeliefMemory(ctx context.Context, userInput string, msgs []llm.Message) []llm.Message {
	if e == nil || e.DisableBeliefMemory || e.BeliefRetriever == nil {
		return msgs
	}
	objectiveID := strings.TrimSpace(e.ObjectiveID)
	if objectiveID == "" {
		return msgs
	}
	log := observability.LoggerWithTrace(ctx)
	startedAt := time.Now()
	results, err := e.BeliefRetriever.Retrieve(ctx, belief.RetrievalRequest{
		TenantID:              e.UserID,
		UserID:                e.UserID,
		ProjectID:             e.ProjectID,
		ObjectiveID:           objectiveID,
		SessionID:             e.SessionID,
		Role:                  e.AgentRole,
		Query:                 userInput,
		Limit:                 e.BeliefMaxBeliefsPerPrompt,
		MinConfidence:         e.BeliefRetrievalMinConfidence,
		IncludeContradictions: e.BeliefIncludeContradictions,
	})
	latency := time.Since(startedAt)
	if err != nil {
		log.Warn().Err(err).Dur("latency", latency).Msg("belief_memory_retrieval_failed")
		return msgs
	}
	log.Info().Int("results", len(results)).Dur("latency", latency).Msg("belief_memory_retrieved")
	contextBlock := belief.BuildPromptSection(results, belief.PromptOptions{
		MaxBeliefs: e.BeliefMaxBeliefsPerPrompt,
		MaxTokens:  e.BeliefPromptTokenBudget,
	})
	if strings.TrimSpace(contextBlock.Text) == "" {
		log.Debug().Msg("belief_memory_no_context_skipping_augmentation")
		return msgs
	}
	beliefSelected, ragSelected := countBySource(contextBlock.Selected)
	log.Info().
		Int("selected", len(contextBlock.Selected)).
		Int("selected_belief", beliefSelected).
		Int("selected_rag", ragSelected).
		Int("overflow", len(contextBlock.Overflow)).
		Int("tokens", contextBlock.TokenEstimate).
		Msg("belief_memory_adding_runtime_context")
	return AddRuntimeContextToCurrentUserMessage(msgs, contextBlock.Text)
}

func countBySource(results []belief.SearchResult) (beliefCount, ragCount int) {
	for _, result := range results {
		if result.Belief.Metadata == nil {
			beliefCount++
			continue
		}
		switch src, _ := result.Belief.Metadata["source"].(string); src {
		case "rag":
			ragCount++
		default:
			beliefCount++
		}
	}
	return beliefCount, ragCount
}

func (e *Engine) augmentWithPolicyContext(ctx context.Context, _ string, msgs []llm.Message) []llm.Message {
	if e == nil || e.DisableBeliefMemory || e.PolicyEnforcer == nil {
		return msgs
	}
	provider, ok := e.PolicyEnforcer.(policy.ContextProvider)
	if !ok {
		return msgs
	}
	records, err := provider.PromptContext(ctx, policy.EvaluationRequest{
		TenantID:    e.UserID,
		UserID:      e.UserID,
		ProjectID:   e.ProjectID,
		ObjectiveID: e.ObjectiveID,
		Role:        e.AgentRole,
	})
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Msg("policy_prompt_context_failed")
		return msgs
	}
	section := policy.BuildPromptSection(records)
	if strings.TrimSpace(section) == "" {
		return msgs
	}
	observability.LoggerWithTrace(ctx).Info().Int("policy_context_items", len(records)).Msg("policy_prompt_context_added_runtime_context")
	return AddRuntimeContextToCurrentUserMessage(msgs, section)
}

// runWithReMem executes the Think-Act-Refine pre-processing, then continues with the main agent loop.
