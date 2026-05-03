package agent

import (
	"context"
	"manifold/internal/agent/belief"
	"manifold/internal/agent/memory"
	"manifold/internal/observability"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func (e *Engine) recordRunEpisode(ctx context.Context, startedAt time.Time, final string, runErr error, evolvingEntryID string) {
	if e == nil || e.BeliefStore == nil {
		return
	}
	projectID := belief.NormalizeProjectID(e.ProjectID)
	objectiveID := strings.TrimSpace(e.ObjectiveID)
	if objectiveID == "" {
		return
	}
	now := time.Now().UTC()
	outcome := "success"
	outcomeSignal := "implicit_success"
	metadata := map[string]any{"finalLength": len(final)}
	if runErr != nil {
		outcome = "error"
		outcomeSignal = "runtime_error"
		metadata["error"] = runErr.Error()
	}
	agentRole := strings.TrimSpace(e.AgentRole)
	if agentRole == "" {
		agentRole = "orchestrator"
	}
	scope, err := e.BeliefStore.EnsureScope(ctx, belief.Scope{
		TenantID: e.UserID,
		Kind:     belief.ScopeKindObjective,
		Path:     projectID + "/" + objectiveID,
		Label:    objectiveID,
		Metadata: map[string]any{"projectId": projectID, "objectiveId": objectiveID},
	})
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Msg("belief_episode_scope_failed")
		return
	}
	episode, err := e.BeliefStore.UpsertEpisode(ctx, belief.Episode{
		TenantID:        e.UserID,
		ScopeID:         scope.ID,
		ProjectID:       projectID,
		ObjectiveID:     objectiveID,
		SessionID:       strings.TrimSpace(e.SessionID),
		AgentRole:       agentRole,
		UserID:          e.UserID,
		StartedAt:       startedAt,
		EndedAt:         &now,
		Outcome:         outcome,
		OutcomeSignal:   outcomeSignal,
		EvolvingEntryID: strings.TrimSpace(evolvingEntryID),
		Metadata:        metadata,
	})
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Msg("belief_episode_store_failed")
		return
	}
	e.distillBeliefs(ctx, episode, final)
}

func (e *Engine) distillBeliefs(ctx context.Context, episode belief.Episode, final string) {
	if e == nil || e.BeliefStore == nil || e.BeliefDistiller == nil {
		return
	}
	candidates, err := e.BeliefDistiller.Distill(ctx, belief.DistillationInput{
		Episode: episode,
		Summary: final,
		Signals: map[string]any{
			"outcome":       episode.Outcome,
			"outcomeSignal": episode.OutcomeSignal,
		},
	})
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("belief_distillation_failed")
		return
	}
	if len(candidates) == 0 {
		return
	}
	applied, err := belief.ApplyCandidates(ctx, e.BeliefStore, episode, candidates)
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("belief_candidate_apply_failed")
		return
	}
	e.promoteEligibleBeliefs(ctx, applied)
	observability.LoggerWithTrace(ctx).Info().Int("candidate_count", len(candidates)).Str("episode_id", episode.ID).Msg("belief_distillation_applied")
}

func (e *Engine) promoteEligibleBeliefs(ctx context.Context, items []belief.Belief) {
	if e == nil || e.BeliefStore == nil || len(items) == 0 {
		return
	}
	projectID := belief.NormalizeProjectID(e.ProjectID)
	if projectID == "" {
		return
	}
	projectScope, err := e.BeliefStore.EnsureScope(ctx, belief.Scope{
		TenantID: e.UserID,
		Kind:     belief.ScopeKindProject,
		Path:     projectID,
		Label:    projectID,
		Metadata: map[string]any{"projectId": projectID},
	})
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Msg("belief_project_scope_failed")
		return
	}
	threshold := e.BeliefPromotionThreshold
	if threshold <= 0 {
		threshold = 0.80
	}
	service := belief.LifecycleService{Store: e.BeliefStore, Graph: e.BeliefGraph, Policy: belief.PromotionPolicy{ConfidenceThreshold: threshold, MinEvidenceFor: 2, ScopeWideningDecay: 0.85}}
	promoted := 0
	for _, item := range items {
		if result, err := service.Promote(ctx, belief.PromotionRequest{TenantID: item.TenantID, BeliefID: item.ID, ToScope: projectScope, Reason: "automatic objective corroboration"}); err != nil {
			observability.LoggerWithTrace(ctx).Debug().Err(err).Str("belief_id", item.ID).Msg("belief_promotion_skipped")
		} else {
			promoted++
			observability.LoggerWithTrace(ctx).Info().Str("belief_id", item.ID).Str("promoted_belief_id", result.Belief.ID).Float64("confidence_after", result.Belief.Confidence).Msg("belief_promoted")
		}
	}
	if promoted > 0 {
		observability.LoggerWithTrace(ctx).Info().Int("promoted", promoted).Int("candidates", len(items)).Msg("belief_promotion_complete")
	}
}

func (e *Engine) storeSuccessfulExperience(ctx context.Context, userInput, final string) string {
	if e.EvolvingMemory == nil {
		return ""
	}

	log := observability.LoggerWithTrace(ctx)
	log.Info().Str("user_input", userInput).Int("response_len", len(final)).Msg("evolving_memory_store_triggered")

	feedback := "success" // default; could be derived from user feedback or evaluation
	structuredFB := &memory.StructuredFeedback{
		Type:         memory.FeedbackSuccess,
		Correct:      true,
		ProgressRate: 1.0,
		Message:      "Task completed successfully",
	}

	bgCtx := context.Background()
	if span := trace.SpanFromContext(ctx); span != nil {
		bgCtx = trace.ContextWithSpanContext(bgCtx, span.SpanContext())
	}
	entryID := uuid.NewString()
	bgCtx = memory.WithEntryID(bgCtx, entryID)

	go func(ctx context.Context, input, response, fb string, sfb *memory.StructuredFeedback) {
		if err := e.EvolvingMemory.EvolveEnhanced(ctx, input, response, fb, sfb, nil, ""); err != nil {
			log.Error().Err(err).Str("feedback", fb).Msg("evolving_memory_store_failed")
			return
		}
		log.Info().Str("feedback", fb).Bool("has_structured_feedback", sfb != nil).Msg("evolving_memory_stored")
	}(bgCtx, userInput, final, feedback, structuredFB)
	return entryID
}
