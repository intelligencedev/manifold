package agent

import (
	"context"
	"errors"
	"manifold/internal/agent/memory"
	"manifold/internal/agent/memory/artifact"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/observability"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

type runEpisodeRecord struct {
	startedAt       time.Time
	userInput       string
	final           string
	runErr          error
	evolvingEntryID string
	reasoningTrace  []string
}

func (e *Engine) recordMemoryEpisode(ctx context.Context, record runEpisodeRecord) string {
	if e == nil {
		return ""
	}
	if e.Memory != nil && !e.DisableMemory {
		runtime := e.Memory.Clone()
		e.Memory = runtime
		runtime.RecordEvolving = func(ctx context.Context, episode memory.EpisodeRecord) (string, error) {
			return e.storeExperience(ctx, episode.UserInput, episode.Final, episode.RunErr, episode.ReasoningTrace), nil
		}
		runtime.RecordBeliefs = func(ctx context.Context, episode memory.EpisodeRecord, evolvingEntryID string) error {
			e.recordRunEpisode(ctx, runEpisodeRecord{
				startedAt:       episode.StartedAt,
				userInput:       episode.UserInput,
				final:           episode.Final,
				runErr:          episode.RunErr,
				evolvingEntryID: evolvingEntryID,
				reasoningTrace:  episode.ReasoningTrace,
			})
			return nil
		}
		entryID, err := runtime.RecordEpisode(ctx, memory.EpisodeRecord{
			StartedAt:      record.startedAt,
			UserInput:      record.userInput,
			Final:          record.final,
			RunErr:         record.runErr,
			ReasoningTrace: append([]string(nil), record.reasoningTrace...),
		})
		if err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Msg("unified_memory_record_episode_failed")
		}
		return entryID
	}
	entryID := e.storeExperience(ctx, record.userInput, record.final, record.runErr, record.reasoningTrace)
	record.evolvingEntryID = entryID
	e.recordRunEpisode(ctx, record)
	return entryID
}

func (e *Engine) recordRunEpisode(ctx context.Context, record runEpisodeRecord) {
	if e == nil || e.DisableBeliefMemory || e.BeliefStore == nil {
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
	metadata := map[string]any{"finalLength": len(record.final)}
	if record.runErr != nil {
		outcome = "error"
		outcomeSignal = "runtime_error"
		metadata["error"] = record.runErr.Error()
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
		StartedAt:       record.startedAt,
		EndedAt:         &now,
		Outcome:         outcome,
		OutcomeSignal:   outcomeSignal,
		EvolvingEntryID: strings.TrimSpace(record.evolvingEntryID),
		Metadata:        metadata,
	})
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Msg("belief_episode_store_failed")
		return
	}
	captured := e.captureArtifacts(ctx, episode)
	e.distillBeliefs(ctx, episode, record.userInput, record.final, record.reasoningTrace)
	e.distillDecisions(ctx, episode, record.userInput, record.final, record.reasoningTrace, captured)
}

func (e *Engine) captureArtifacts(ctx context.Context, episode belief.Episode) []artifact.Artifact {
	if e == nil || e.ArtifactCapture == nil {
		return nil
	}
	hints := map[string]string{
		"sessionID": strings.TrimSpace(episode.SessionID),
		"since":     episode.StartedAt.Format(time.RFC3339),
	}
	if episode.EndedAt != nil && !episode.EndedAt.IsZero() {
		hints["until"] = episode.EndedAt.Format(time.RFC3339)
	}
	if info, err := os.Stat(e.ProjectID); err == nil && info.IsDir() {
		hints["repoPath"] = e.ProjectID
	}
	return e.ArtifactCapture.Capture(ctx, artifact.CaptureRequest{
		TenantID:  episode.TenantID,
		ScopeID:   episode.ScopeID,
		EpisodeID: episode.ID,
		Hints:     hints,
	})
}

func (e *Engine) distillDecisions(ctx context.Context, episode belief.Episode, userInput, final string, reasoningTrace []string, artifacts []artifact.Artifact) {
	if e == nil || e.DisableBeliefMemory || e.DecisionStore == nil || e.DecisionDistiller == nil {
		return
	}
	refs := make([]decision.ArtifactRef, 0, len(artifacts))
	for _, item := range artifacts {
		refs = append(refs, decision.ArtifactRef{
			ID:         item.ID,
			Kind:       string(item.Kind),
			ExternalID: item.ExternalID,
			URI:        item.URI,
			Title:      item.Title,
			Excerpt:    item.Excerpt,
		})
	}
	input := decision.DistillationInput{
		Episode:        episode,
		UserRequest:    userInput,
		FinalAnswer:    final,
		Summary:        final,
		ReasoningTrace: append([]string(nil), reasoningTrace...),
		Artifacts:      refs,
	}
	var candidates []decision.Candidate
	var audit []decision.Candidate
	if distiller, ok := e.DecisionDistiller.(decision.AuditDistiller); ok {
		result, err := distiller.DistillWithAudit(ctx, input)
		if err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("decision_distillation_failed")
			return
		}
		candidates = result.Candidates
		audit = result.Audit
	} else {
		var err error
		candidates, err = e.DecisionDistiller.Distill(ctx, input)
		if err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("decision_distillation_failed")
			return
		}
		audit = candidates
	}
	recorded := make([]decision.Candidate, 0, len(audit))
	for _, record := range audit {
		if record.TenantID == 0 {
			record.TenantID = episode.TenantID
		}
		if record.EpisodeID == "" {
			record.EpisodeID = episode.ID
		}
		if record.ScopeID == "" {
			record.ScopeID = episode.ScopeID
		}
		stored, err := e.DecisionStore.RecordCandidate(ctx, record)
		if err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("decision_candidate_audit_failed")
			continue
		}
		recorded = append(recorded, stored)
	}
	observability.LoggerWithTrace(ctx).Info().Int("candidate_count", len(candidates)).Str("episode_id", episode.ID).Msg("decision_distillation_queued")
	e.autoActivateDecisionCandidates(ctx, episode, recorded)
}

// autoActivateDecisionCandidates applies the deterministic confidence-gated
// auto-activation policy to freshly recorded candidates. Conflicting active
// decisions are flagged needs_review; lifecycle reviews (reaffirm, revoke,
// supersede, stale->active) always stay deliberate via decision_review.
func (e *Engine) autoActivateDecisionCandidates(ctx context.Context, episode belief.Episode, candidates []decision.Candidate) {
	if e == nil || e.DisableBeliefMemory || e.DecisionService == nil || len(candidates) == 0 {
		return
	}
	outcomes, err := e.DecisionService.AutoAcceptCandidates(ctx, episode.TenantID, candidates)
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("decision_auto_activation_failed")
		return
	}
	for _, outcome := range outcomes {
		event := observability.LoggerWithTrace(ctx).Info()
		if !outcome.Accepted {
			event = observability.LoggerWithTrace(ctx).Debug()
		}
		event.Str("episode_id", episode.ID).
			Str("candidate_id", outcome.CandidateID).
			Bool("accepted", outcome.Accepted).
			Str("decision_id", outcome.Decision.ID).
			Str("reason", outcome.Reason).
			Msg("decision_auto_activation")
	}
}

func (e *Engine) distillBeliefs(ctx context.Context, episode belief.Episode, userInput, final string, reasoningTrace []string) {
	if e == nil || e.DisableBeliefMemory || e.BeliefStore == nil || e.BeliefDistiller == nil {
		return
	}
	input := belief.DistillationInput{
		Episode:        episode,
		UserRequest:    userInput,
		FinalAnswer:    final,
		Summary:        final,
		ReasoningTrace: append([]string(nil), reasoningTrace...),
		Signals: map[string]any{
			"outcome":       episode.Outcome,
			"outcomeSignal": episode.OutcomeSignal,
		},
	}
	var candidates []belief.Candidate
	var audit []belief.CandidateRecord
	if distiller, ok := e.BeliefDistiller.(belief.AuditDistiller); ok {
		result, err := distiller.DistillWithAudit(ctx, input)
		if err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("belief_distillation_failed")
			return
		}
		candidates = result.Candidates
		audit = result.Audit
	} else {
		var err error
		candidates, err = e.BeliefDistiller.Distill(ctx, input)
		if err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("belief_distillation_failed")
			return
		}
		audit = auditCandidates(episode, candidates)
	}
	if len(audit) > 0 {
		e.recordCandidateAudit(ctx, audit)
	}
	if len(candidates) == 0 {
		return
	}
	applied := make([]belief.Belief, 0, len(candidates))
	for _, candidate := range candidates {
		item, err := belief.ApplyCandidate(ctx, e.BeliefStore, episode, candidate)
		if err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", episode.ID).Msg("belief_candidate_apply_failed")
			return
		}
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		applied = append(applied, item)
		e.linkCandidateAudit(ctx, episode, candidate, item)
	}
	e.promoteEligibleBeliefs(ctx, applied)
	e.compileEnforceableBeliefs(ctx, applied, belief.Promotion{})
	e.ingestBeliefsMagma(ctx, episode, applied)
	observability.LoggerWithTrace(ctx).Info().Int("candidate_count", len(candidates)).Str("episode_id", episode.ID).Msg("belief_distillation_applied")
}

func auditCandidates(episode belief.Episode, candidates []belief.Candidate) []belief.CandidateRecord {
	out := make([]belief.CandidateRecord, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, belief.CandidateRecord{
			TenantID:         episode.TenantID,
			EpisodeID:        episode.ID,
			ScopeID:          episode.ScopeID,
			Statement:        belief.NormalizeStatement(candidate.Statement),
			StatementHash:    candidate.StatementHash,
			Kind:             belief.NormalizeBeliefKind(candidate.Kind),
			Enforcement:      belief.NormalizeEnforcement(candidate.Enforcement),
			Polarity:         candidate.Polarity,
			Confidence:       candidate.Confidence,
			SourceQuality:    candidate.SourceQuality,
			ReviewState:      belief.ReviewStateAutoActive,
			EvidenceNote:     candidate.EvidenceNote,
			ValidationStatus: belief.CandidateValidationAccepted,
			Model:            "simple",
			Metadata:         map[string]any{"distiller": "simple"},
		})
	}
	return out
}

func (e *Engine) recordCandidateAudit(ctx context.Context, records []belief.CandidateRecord) {
	if e == nil || e.DisableBeliefMemory || e.BeliefStore == nil {
		return
	}
	for _, record := range records {
		if _, err := e.BeliefStore.RecordCandidate(ctx, record); err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("episode_id", record.EpisodeID).Msg("belief_candidate_audit_failed")
		}
	}
}

func (e *Engine) linkCandidateAudit(ctx context.Context, episode belief.Episode, candidate belief.Candidate, item belief.Belief) {
	if e == nil || e.DisableBeliefMemory || e.BeliefStore == nil {
		return
	}
	records, err := e.BeliefStore.ListCandidates(ctx, belief.CandidateQuery{
		TenantID:         episode.TenantID,
		EpisodeID:        episode.ID,
		ValidationStatus: belief.CandidateValidationAccepted,
		Limit:            50,
	})
	if err != nil {
		return
	}
	hash := strings.TrimSpace(candidate.StatementHash)
	if hash == "" {
		hash = belief.StatementHash(candidate.Statement)
	}
	for _, record := range records {
		if record.StatementHash != hash || record.AcceptedBeliefID != "" {
			continue
		}
		record.AcceptedBeliefID = item.ID
		_, _ = e.BeliefStore.RecordCandidate(ctx, record)
		return
	}
}

func (e *Engine) promoteEligibleBeliefs(ctx context.Context, items []belief.Belief) {
	if e == nil || e.DisableBeliefMemory || e.BeliefStore == nil || len(items) == 0 {
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
	policy := e.BeliefLifecyclePolicy
	if policy.ConfidenceThreshold <= 0 {
		policy.ConfidenceThreshold = threshold
	}
	service := belief.LifecycleService{Store: e.BeliefStore, Graph: e.BeliefGraph, Policy: policy}
	promoted := 0
	for _, item := range items {
		if result, err := service.Promote(ctx, belief.PromotionRequest{TenantID: item.TenantID, BeliefID: item.ID, ToScope: projectScope, Reason: "automatic objective corroboration"}); err != nil {
			observability.LoggerWithTrace(ctx).Debug().Err(err).Str("belief_id", item.ID).Msg("belief_promotion_skipped")
		} else {
			promoted++
			e.compileEnforceableBeliefs(ctx, []belief.Belief{result.Belief}, result.Promotion)
			observability.LoggerWithTrace(ctx).Info().Str("belief_id", item.ID).Str("promoted_belief_id", result.Belief.ID).Float64("confidence_after", result.Belief.Confidence).Msg("belief_promoted")
		}
	}
	if promoted > 0 {
		observability.LoggerWithTrace(ctx).Info().Int("promoted", promoted).Int("candidates", len(items)).Msg("belief_promotion_complete")
	}
}

func (e *Engine) compileEnforceableBeliefs(ctx context.Context, items []belief.Belief, promotion belief.Promotion) {
	if e == nil || e.DisableBeliefMemory || e.BeliefPolicySink == nil || len(items) == 0 {
		return
	}
	policy := e.BeliefEnforcementPolicy
	if !policy.AutoEnable {
		return
	}
	if policy.SoftPolicyThreshold <= 0 {
		policy.SoftPolicyThreshold = 0.85
	}
	if policy.HardConstraintThreshold <= 0 {
		policy.HardConstraintThreshold = 0.95
	}
	if policy.HardConstraintMinEvidenceFor <= 0 {
		policy.HardConstraintMinEvidenceFor = 3
	}
	for _, item := range items {
		if item.Kind != belief.BeliefKindConstraint {
			continue
		}
		if item.Enforcement == belief.EnforcementSoftPolicy && item.Confidence < policy.SoftPolicyThreshold {
			continue
		}
		if item.Enforcement == belief.EnforcementHardConstraint && (item.Confidence < policy.HardConstraintThreshold || item.EvidenceFor < policy.HardConstraintMinEvidenceFor) {
			continue
		}
		if item.Enforcement != belief.EnforcementSoftPolicy && item.Enforcement != belief.EnforcementHardConstraint {
			continue
		}
		if err := e.BeliefPolicySink.UpsertPolicyForBelief(ctx, item, promotion); err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("belief_id", item.ID).Msg("belief_policy_compile_failed")
			continue
		}
		observability.LoggerWithTrace(ctx).Info().Str("belief_id", item.ID).Str("enforcement", string(item.Enforcement)).Msg("belief_policy_compiled")
	}
}

func (e *Engine) ingestBeliefsMagma(ctx context.Context, episode belief.Episode, items []belief.Belief) {
	if e == nil || e.DisableBeliefMemory || e.BeliefMagmaSink == nil || len(items) == 0 {
		return
	}
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		if _, err := e.BeliefMagmaSink.IngestBelief(ctx, episode, item); err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("belief_id", item.ID).Msg("belief_magma_ingest_failed")
		}
	}
}

func (e *Engine) storeExperience(ctx context.Context, userInput, final string, runErr error, reasoningTrace []string) string {
	if e == nil || e.DisableEvolvingMemory || e.EvolvingMemory == nil {
		return ""
	}
	if strings.TrimSpace(final) == "" && runErr == nil {
		return ""
	}

	log := observability.LoggerWithTrace(ctx)
	log.Info().Str("user_input", userInput).Int("response_len", len(final)).Msg("evolving_memory_store_triggered")

	feedback, structuredFB := deriveMemoryFeedback(final, runErr)
	storedOutput := final
	if runErr != nil {
		if strings.TrimSpace(storedOutput) != "" {
			storedOutput += "\n\n"
		}
		storedOutput += "Error: " + runErr.Error()
	}

	bgCtx := context.Background()
	if span := trace.SpanFromContext(ctx); span != nil {
		bgCtx = trace.ContextWithSpanContext(bgCtx, span.SpanContext())
	}
	entryID := uuid.NewString()
	bgCtx = memory.WithEntryID(bgCtx, entryID)

	go func(ctx context.Context, input, response, fb string, sfb *memory.StructuredFeedback, traceMsgs []string) {
		var err error
		if e.ReMemController != nil && len(traceMsgs) > 0 {
			err = e.ReMemController.StoreExperienceEnhanced(ctx, input, response, fb, sfb, traceMsgs)
		} else {
			err = e.EvolvingMemory.EvolveEnhanced(ctx, input, response, fb, sfb, traceMsgs, "")
		}
		if err != nil {
			log.Error().Err(err).Str("feedback", fb).Msg("evolving_memory_store_failed")
			return
		}
		log.Info().Str("feedback", fb).Bool("has_structured_feedback", sfb != nil).Int("reasoning_steps", len(traceMsgs)).Msg("evolving_memory_stored")
	}(bgCtx, userInput, storedOutput, feedback, structuredFB, append([]string(nil), reasoningTrace...))
	return entryID
}

func deriveMemoryFeedback(final string, runErr error) (string, *memory.StructuredFeedback) {
	if runErr != nil {
		if errors.Is(runErr, ErrMaxStepsExceeded) {
			return string(memory.FeedbackPartial), &memory.StructuredFeedback{
				Type:         memory.FeedbackPartial,
				Correct:      false,
				ProgressRate: 0.5,
				Message:      "Task ended without a complete final response: " + runErr.Error(),
			}
		}
		return string(memory.FeedbackFailure), &memory.StructuredFeedback{
			Type:         memory.FeedbackFailure,
			Correct:      false,
			ProgressRate: 0,
			Message:      "Task failed before completion: " + runErr.Error(),
		}
	}
	if strings.TrimSpace(final) == "" {
		return string(memory.FeedbackPartial), &memory.StructuredFeedback{
			Type:         memory.FeedbackPartial,
			Correct:      false,
			ProgressRate: 0.5,
			Message:      "Task ended without a complete final response",
		}
	}
	return string(memory.FeedbackSuccess), &memory.StructuredFeedback{
		Type:         memory.FeedbackSuccess,
		Correct:      true,
		ProgressRate: 1.0,
		Message:      "Task completed successfully",
	}
}
