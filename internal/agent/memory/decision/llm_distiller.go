package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"manifold/internal/llm"
)

// LLMDistillerConfig configures LLM-backed decision extraction.
type LLMDistillerConfig struct {
	LLM                    llm.Provider
	Model                  string
	MaxCandidates          int
	MinCandidateConfidence float64
	Embed                  EmbedFunc
}

// LLMDistiller extracts decision candidates with a strict JSON payload.
type LLMDistiller struct {
	Config LLMDistillerConfig
}

type llmDistillerResponse struct {
	Candidates []llmCandidatePayload `json:"candidates"`
}

type llmCandidatePayload struct {
	Title           string           `json:"title"`
	Statement       string           `json:"statement"`
	Rationale       string           `json:"rationale"`
	Alternatives    []llmAlternative `json:"alternatives"`
	AssumptionHints []string         `json:"assumption_hints"`
	EvidenceHints   []EvidenceHint   `json:"evidence_hints"`
	Confidence      *float64         `json:"confidence"`
}

type llmAlternative struct {
	Statement       string `json:"statement"`
	RejectionReason string `json:"rejection_reason"`
}

// Distill returns accepted candidates from an LLM extraction result.
func (d LLMDistiller) Distill(ctx context.Context, input DistillationInput) ([]Candidate, error) {
	result, err := d.DistillWithAudit(ctx, input)
	if err != nil {
		return nil, err
	}
	return result.Candidates, nil
}

// DistillWithAudit returns accepted candidates plus rejected/queued audit rows.
func (d LLMDistiller) DistillWithAudit(ctx context.Context, input DistillationInput) (DistillationResult, error) {
	if d.Config.LLM == nil {
		return DistillationResult{}, nil
	}
	maxCandidates := d.Config.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 5
	}
	bundle, err := json.Marshal(input)
	if err != nil {
		return DistillationResult{}, err
	}
	msg, err := d.Config.LLM.Chat(ctx, []llm.Message{
		{Role: "system", Content: llmDecisionDistillerSystemPrompt(maxCandidates)},
		{Role: "user", Content: string(bundle)},
	}, nil, d.Config.Model)
	if err != nil {
		return DistillationResult{}, err
	}
	raw := strings.TrimSpace(msg.Content)
	parsed, err := parseLLMDecisionResponse(raw)
	if err != nil {
		return DistillationResult{
			Audit: []Candidate{{
				TenantID:         input.Episode.TenantID,
				EpisodeID:        input.Episode.ID,
				ScopeID:          input.Episode.ScopeID,
				RawPayload:       raw,
				ReviewState:      ReviewStateNeedsReview,
				ValidationStatus: CandidateValidationRejected,
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
				result.Candidates[i].Metadata["embedding"] = "attached"
			}
		}
	}
	return result, nil
}

func (d LLMDistiller) validatePayload(input DistillationInput, payload llmCandidatePayload, raw string) (Candidate, Candidate, bool) {
	record := newCandidate(input, payload, raw, d.Config.Model)
	reject := func(reason string) (Candidate, Candidate, bool) {
		record.ValidationStatus = CandidateValidationRejected
		record.ReviewState = ReviewStateNeedsReview
		record.RejectionReason = reason
		return record, Candidate{}, false
	}
	statement := NormalizeStatement(payload.Statement)
	if statement == "" {
		return reject("statement is required")
	}
	if len([]rune(statement)) < 10 {
		return reject("statement is too short")
	}
	if strings.HasSuffix(strings.TrimSpace(payload.Statement), "?") {
		return reject("statement must not be a question")
	}
	if strings.TrimSpace(payload.Rationale) == "" {
		return reject("rationale is required")
	}
	if payload.Confidence == nil || *payload.Confidence < 0 || *payload.Confidence > 1 {
		return reject("confidence must be between 0 and 1")
	}
	confidence := *payload.Confidence
	if confidence > maxCandidateConfidence {
		confidence = maxCandidateConfidence
	}
	record.Statement = statement
	record.StatementHash = StatementHash(statement)
	record.Title = strings.TrimSpace(payload.Title)
	if record.Title == "" {
		record.Title = titleFromStatement(statement)
	}
	record.Rationale = strings.TrimSpace(payload.Rationale)
	record.Confidence = confidence
	record.ValidationStatus = CandidateValidationQueued
	record.ReviewState = ReviewStateNeedsReview
	minConfidence := d.Config.MinCandidateConfidence
	if minConfidence <= 0 {
		minConfidence = 0.55
	}
	if confidence < minConfidence {
		record.ValidationStatus = CandidateValidationRejected
		record.RejectionReason = "confidence below minCandidateConfidence"
		return record, Candidate{}, false
	}
	return record, record, true
}

func newCandidate(input DistillationInput, payload llmCandidatePayload, raw, model string) Candidate {
	alternatives := make([]Alternative, 0, len(payload.Alternatives))
	for _, alt := range payload.Alternatives {
		statement := NormalizeStatement(alt.Statement)
		if statement == "" {
			continue
		}
		alternatives = append(alternatives, Alternative{
			TenantID:        input.Episode.TenantID,
			Statement:       statement,
			RejectionReason: strings.TrimSpace(alt.RejectionReason),
		})
	}
	return Candidate{
		TenantID:         input.Episode.TenantID,
		EpisodeID:        input.Episode.ID,
		ScopeID:          input.Episode.ScopeID,
		Title:            strings.TrimSpace(payload.Title),
		Statement:        NormalizeStatement(payload.Statement),
		StatementHash:    StatementHash(payload.Statement),
		Rationale:        strings.TrimSpace(payload.Rationale),
		Alternatives:     alternatives,
		AssumptionHints:  trimStrings(payload.AssumptionHints),
		EvidenceHints:    payload.EvidenceHints,
		ReviewState:      ReviewStateNeedsReview,
		ValidationStatus: CandidateValidationQueued,
		Model:            model,
		RawPayload:       raw,
		Metadata: map[string]any{
			"distiller":       "llm",
			"projectId":       input.Episode.ProjectID,
			"objectiveId":     input.Episode.ObjectiveID,
			"agentRole":       input.Episode.AgentRole,
			"evolvingEntryId": input.Episode.EvolvingEntryID,
		},
	}
}

func parseLLMDecisionResponse(raw string) (llmDistillerResponse, error) {
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
		return llmDistillerResponse{}, fmt.Errorf("invalid decision distiller JSON: %w", err)
	}
	return parsed, nil
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func llmDecisionDistillerSystemPrompt(maxCandidates int) string {
	return fmt.Sprintf(`You extract durable project decisions from an agent run.
Return strict JSON only, with this schema:
{"candidates":[{"title":"...","statement":"...","rationale":"...","alternatives":[{"statement":"...","rejection_reason":"..."}],"assumption_hints":["..."],"evidence_hints":[{"sourceKind":"episode|human_feedback|tool_result|rag_doc|rag_chunk|transit|belief|artifact","sourceId":"...","polarity":"for|against","note":"..."}],"confidence":0.0}]}

Rules:
- Emit at most %d candidates.
- A decision is a choice made among alternatives, not a generic observation.
- statement must be declarative, at least 10 characters, and not a question.
- rationale must explain why this choice was made at decision time.
- Include rejected alternatives only when the run provides evidence.
- Include assumption_hints as claims the decision depends on.
- LLM candidates must start in needs_review; do not mark them approved.
- confidence must be a number between 0 and 1.`, maxCandidates)
}
