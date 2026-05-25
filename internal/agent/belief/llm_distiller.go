package belief

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"manifold/internal/llm"
)

type LLMDistillerConfig struct {
	LLM                    llm.Provider
	Model                  string
	MaxCandidates          int
	MinCandidateConfidence float64
	AutoApplyMinConfidence float64
	DefaultConfidence      float64
	Embed                  EmbedFunc
}

type LLMDistiller struct {
	Config LLMDistillerConfig
}

type llmDistillerResponse struct {
	Candidates []llmCandidatePayload `json:"candidates"`
}

type llmCandidatePayload struct {
	Statement     string   `json:"statement"`
	Kind          string   `json:"kind"`
	Enforcement   string   `json:"enforcement"`
	Polarity      string   `json:"polarity"`
	Confidence    *float64 `json:"confidence"`
	SourceQuality *float64 `json:"source_quality"`
	EvidenceNote  string   `json:"evidence_note"`
}

func (d LLMDistiller) Distill(ctx context.Context, input DistillationInput) ([]Candidate, error) {
	result, err := d.DistillWithAudit(ctx, input)
	if err != nil {
		return nil, err
	}
	return result.Candidates, nil
}

func (d LLMDistiller) DistillWithAudit(ctx context.Context, input DistillationInput) (DistillationResult, error) {
	if d.Config.LLM == nil {
		return DistillationResult{}, nil
	}
	maxCandidates := d.Config.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 5
	}
	bundle, err := json.Marshal(map[string]any{
		"episode": map[string]any{
			"id":              input.Episode.ID,
			"tenantId":        input.Episode.TenantID,
			"scopeId":         input.Episode.ScopeID,
			"projectId":       input.Episode.ProjectID,
			"objectiveId":     input.Episode.ObjectiveID,
			"sessionId":       input.Episode.SessionID,
			"agentRole":       input.Episode.AgentRole,
			"userId":          input.Episode.UserID,
			"outcome":         input.Episode.Outcome,
			"outcomeSignal":   input.Episode.OutcomeSignal,
			"evolvingEntryId": input.Episode.EvolvingEntryID,
			"metadata":        input.Episode.Metadata,
		},
		"userRequest":    input.UserRequest,
		"finalAnswer":    input.FinalAnswer,
		"summary":        input.Summary,
		"lesson":         input.Lesson,
		"toolSummary":    input.ToolSummary,
		"reasoningTrace": input.ReasoningTrace,
		"signals":        input.Signals,
	})
	if err != nil {
		return DistillationResult{}, err
	}
	msg, err := d.Config.LLM.Chat(ctx, []llm.Message{
		{Role: "system", Content: llmDistillerSystemPrompt(maxCandidates)},
		{Role: "user", Content: string(bundle)},
	}, nil, d.Config.Model)
	if err != nil {
		return DistillationResult{}, err
	}
	raw := strings.TrimSpace(msg.Content)
	parsed, err := parseLLMDistillerResponse(raw)
	if err != nil {
		return DistillationResult{
			Audit: []CandidateRecord{{
				TenantID:         input.Episode.TenantID,
				EpisodeID:        input.Episode.ID,
				ScopeID:          input.Episode.ScopeID,
				RawPayload:       raw,
				ValidationStatus: CandidateValidationRejected,
				ReviewState:      ReviewStateNeedsReview,
				RejectionReason:  err.Error(),
				Model:            d.Config.Model,
				Metadata:         map[string]any{"distiller": "llm"},
			}},
			RawPayload: raw,
		}, nil
	}
	result := DistillationResult{RawPayload: raw}
	for _, payload := range parsed.Candidates {
		record, candidate, ok := d.validatePayload(input, payload, raw)
		result.Audit = append(result.Audit, record)
		if ok {
			result.Candidates = append(result.Candidates, candidate)
		}
	}
	if d.Config.Embed != nil && len(result.Candidates) > 0 {
		texts := make([]string, 0, len(result.Candidates))
		for _, candidate := range result.Candidates {
			texts = append(texts, candidate.Statement)
		}
		if embeddings, err := d.Config.Embed(ctx, texts); err == nil && len(embeddings) == len(result.Candidates) {
			for i := range result.Candidates {
				result.Candidates[i].Embedding = embeddings[i]
			}
		}
	}
	return result, nil
}

func (d LLMDistiller) validatePayload(input DistillationInput, payload llmCandidatePayload, raw string) (CandidateRecord, Candidate, bool) {
	normalized := map[string]any{
		"statement":      payload.Statement,
		"kind":           payload.Kind,
		"enforcement":    payload.Enforcement,
		"polarity":       payload.Polarity,
		"evidence_note":  payload.EvidenceNote,
		"source_quality": payload.SourceQuality,
	}
	if payload.Confidence != nil {
		normalized["confidence"] = *payload.Confidence
	}
	record := CandidateRecord{
		TenantID:          input.Episode.TenantID,
		EpisodeID:         input.Episode.ID,
		ScopeID:           input.Episode.ScopeID,
		RawPayload:        raw,
		NormalizedPayload: normalized,
		Model:             d.Config.Model,
		Metadata: map[string]any{
			"distiller":       "llm",
			"projectId":       input.Episode.ProjectID,
			"objectiveId":     input.Episode.ObjectiveID,
			"agentRole":       input.Episode.AgentRole,
			"evolvingEntryId": input.Episode.EvolvingEntryID,
		},
	}
	reject := func(reason string) (CandidateRecord, Candidate, bool) {
		record.ValidationStatus = CandidateValidationRejected
		record.ReviewState = ReviewStateNeedsReview
		record.RejectionReason = reason
		return record, Candidate{}, false
	}

	statement := NormalizeStatement(payload.Statement)
	if statement == "" {
		return reject("statement is required")
	}
	kind, ok := parseBeliefKind(payload.Kind)
	if !ok {
		return reject("invalid kind")
	}
	enforcement, ok := parseEnforcementMode(payload.Enforcement)
	if !ok {
		return reject("invalid enforcement")
	}
	polarity, ok := parseEvidencePolarity(payload.Polarity)
	if !ok {
		return reject("invalid polarity")
	}
	if strings.TrimSpace(payload.EvidenceNote) == "" {
		return reject("evidence_note is required")
	}
	if payload.Confidence == nil || *payload.Confidence < 0 || *payload.Confidence > 1 {
		return reject("confidence must be between 0 and 1")
	}
	sourceQuality := *payload.Confidence
	if payload.SourceQuality != nil {
		if *payload.SourceQuality < 0 || *payload.SourceQuality > 1 {
			return reject("source_quality must be between 0 and 1")
		}
		sourceQuality = *payload.SourceQuality
	}
	minConfidence := d.Config.MinCandidateConfidence
	if minConfidence <= 0 {
		minConfidence = 0.55
	}
	autoApplyConfidence := d.Config.AutoApplyMinConfidence
	if autoApplyConfidence <= 0 {
		autoApplyConfidence = 0.65
	}
	record.Statement = statement
	record.StatementHash = StatementHash(statement)
	record.Kind = kind
	record.Enforcement = enforcement
	record.Polarity = polarity
	record.Confidence = *payload.Confidence
	record.SourceQuality = sourceQuality
	record.EvidenceNote = strings.TrimSpace(payload.EvidenceNote)
	if *payload.Confidence < minConfidence {
		record.ValidationStatus = CandidateValidationRejected
		record.ReviewState = ReviewStateNeedsReview
		record.RejectionReason = "confidence below minCandidateConfidence"
		return record, Candidate{}, false
	}
	if *payload.Confidence < autoApplyConfidence {
		record.ValidationStatus = CandidateValidationQueued
		record.ReviewState = ReviewStateNeedsReview
		return record, Candidate{}, false
	}
	record.ValidationStatus = CandidateValidationAccepted
	record.ReviewState = ReviewStateAutoActive
	return record, Candidate{
		Statement:     statement,
		StatementHash: record.StatementHash,
		Kind:          kind,
		Enforcement:   enforcement,
		SourceQuality: sourceQuality,
		ReviewState:   ReviewStateAutoActive,
		Confidence:    *payload.Confidence,
		Polarity:      polarity,
		EvidenceNote:  strings.TrimSpace(payload.EvidenceNote),
		Metadata:      cloneCandidateMetadata(record.Metadata),
	}, true
}

func parseLLMDistillerResponse(raw string) (llmDistillerResponse, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	if start := strings.Index(raw, "{"); start > 0 {
		raw = raw[start:]
	}
	if end := strings.LastIndex(raw, "}"); end >= 0 && end < len(raw)-1 {
		raw = raw[:end+1]
	}
	var parsed llmDistillerResponse
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parsed); err != nil {
		return llmDistillerResponse{}, fmt.Errorf("invalid distiller JSON: %w", err)
	}
	return parsed, nil
}

func parseBeliefKind(raw string) (BeliefKind, bool) {
	switch BeliefKind(strings.ToLower(strings.TrimSpace(raw))) {
	case BeliefKindFact:
		return BeliefKindFact, true
	case BeliefKindPreference:
		return BeliefKindPreference, true
	case BeliefKindProcedure:
		return BeliefKindProcedure, true
	case BeliefKindConstraint:
		return BeliefKindConstraint, true
	case BeliefKindCapability:
		return BeliefKindCapability, true
	default:
		return "", false
	}
}

func parseEnforcementMode(raw string) (EnforcementMode, bool) {
	switch EnforcementMode(strings.ToLower(strings.TrimSpace(raw))) {
	case EnforcementNone:
		return EnforcementNone, true
	case EnforcementPrompt:
		return EnforcementPrompt, true
	case EnforcementSoftPolicy:
		return EnforcementSoftPolicy, true
	case EnforcementHardConstraint:
		return EnforcementHardConstraint, true
	default:
		return "", false
	}
}

func parseEvidencePolarity(raw string) (EvidencePolarity, bool) {
	switch EvidencePolarity(strings.ToLower(strings.TrimSpace(raw))) {
	case EvidencePolarityFor:
		return EvidencePolarityFor, true
	case EvidencePolarityAgainst:
		return EvidencePolarityAgainst, true
	default:
		return "", false
	}
}

func llmDistillerSystemPrompt(maxCandidates int) string {
	return fmt.Sprintf(`You extract durable project beliefs from an agent run.
Return strict JSON only, with this schema:
{"candidates":[{"statement":"...","kind":"fact|preference|procedure|constraint|capability","enforcement":"none|prompt|soft_policy|hard_constraint","polarity":"for|against","confidence":0.0,"source_quality":0.0,"evidence_note":"..."}]}

Rules:
- Emit at most %d candidates.
- A belief is a scoped claim likely to matter in future runs.
- Use kind=constraint only for enforceable operational rules.
- Use enforcement=soft_policy or hard_constraint only when the statement is a concrete constraint.
- Do not copy user instructions as facts unless the run outcome/evidence supports them.
- Prefer fewer, higher-quality candidates over broad summaries.
- confidence and source_quality must be numbers between 0 and 1.
- evidence_note must explain the observed evidence in one sentence.`, maxCandidates)
}
