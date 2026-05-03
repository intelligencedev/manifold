package agent

import (
	"context"
	"manifold/internal/agent/belief"
	"manifold/internal/agent/memory"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/policy"
	"strings"
	"sync"
	"time"
)

func (e *Engine) augmentWithMemory(ctx context.Context, userInput string, msgs []llm.Message) []llm.Message {
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
			retrieved     []*memory.MemoryEntry
			recentContext string
			wg            sync.WaitGroup
		)

		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := e.EvolvingMemory.Search(ctx, userInput)
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
		}()

		log.Debug().Msg("evolving_memory_exprecent_starting")
		recentContext = e.EvolvingMemory.BuildExpRecentContext()
		if recentContext != "" {
			log.Info().Int("context_len", len(recentContext)).Msg("evolving_memory_exprecent_used")
		} else {
			log.Debug().Msg("evolving_memory_exprecent_empty")
		}

		wg.Wait()
		if len(retrieved) > 0 {
			memoryContext = e.EvolvingMemory.Synthesize(ctx, userInput, retrieved)
			log.Info().Int("retrieved", len(retrieved)).Int("context_len", len(memoryContext)).Msg("evolving_memory_exprag_synthesized")
		} else if recentContext != "" {
			memoryContext = recentContext
		}
	}

	if memoryContext == "" {
		log.Debug().Msg("evolving_memory_no_context_skipping_augmentation")
		return msgs
	}

	log.Info().Int("context_len", len(memoryContext)).Int("orig_msgs", len(msgs)).Msg("evolving_memory_appending_to_system")

	// Append memory context to the system prompt instead of injecting as separate message
	// This ensures it's reconstructed on every request and doesn't interfere with history
	systemIdx := -1
	for i, msg := range msgs {
		if msg.Role == "system" {
			systemIdx = i
			break
		}
	}

	if systemIdx >= 0 {
		// Append memory context to existing system message
		msgs[systemIdx].Content += "\n\n## Relevant Context from Past Interactions\n\n" + memoryContext
		log.Debug().Int("system_idx", systemIdx).Int("new_len", len(msgs[systemIdx].Content)).Msg("evolving_memory_appended_to_system")
	} else {
		// No system message exists, create one with memory context
		msgs = append([]llm.Message{{
			Role:    "system",
			Content: "## Relevant Context from Past Interactions\n\n" + memoryContext,
		}}, msgs...)
		log.Debug().Msg("evolving_memory_created_system_with_context")
	}

	log.Info().Int("msgs_count", len(msgs)).Msg("evolving_memory_augmentation_complete")
	return msgs
}

func (e *Engine) augmentWithBeliefMemory(ctx context.Context, userInput string, msgs []llm.Message) []llm.Message {
	if e == nil || e.BeliefRetriever == nil {
		return msgs
	}
	objectiveID := strings.TrimSpace(e.ObjectiveID)
	if objectiveID == "" {
		return msgs
	}
	log := observability.LoggerWithTrace(ctx)
	startedAt := time.Now()
	results, err := e.BeliefRetriever.Retrieve(ctx, belief.RetrievalRequest{
		TenantID:    e.UserID,
		UserID:      e.UserID,
		ProjectID:   e.ProjectID,
		ObjectiveID: objectiveID,
		SessionID:   e.SessionID,
		Role:        e.AgentRole,
		Query:       userInput,
		Limit:       e.BeliefMaxBeliefsPerPrompt,
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
		Msg("belief_memory_appending_to_system")
	return appendToSystemMessage(msgs, contextBlock.Text)
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
	if e == nil || e.PolicyEnforcer == nil {
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
	observability.LoggerWithTrace(ctx).Info().Int("policy_context_items", len(records)).Msg("policy_prompt_context_appended")
	return appendToSystemMessage(msgs, section)
}

func appendToSystemMessage(msgs []llm.Message, section string) []llm.Message {
	section = strings.TrimSpace(section)
	if section == "" {
		return msgs
	}
	for i, msg := range msgs {
		if msg.Role == "system" {
			msgs[i].Content += "\n\n" + section
			return msgs
		}
	}
	return append([]llm.Message{{Role: "system", Content: section}}, msgs...)
}

// runWithReMem executes the Think-Act-Refine pre-processing, then continues with the main agent loop.
