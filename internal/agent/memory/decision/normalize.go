package decision

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"manifold/internal/agent/memory/belief"
)

// NormalizeStatement normalizes decision statements with the belief normalizer.
func NormalizeStatement(statement string) string {
	return belief.NormalizeStatement(statement)
}

// StatementHash returns the stable hash used for per-scope decision dedupe.
func StatementHash(statement string) string {
	normalized := strings.ToLower(strings.TrimSpace(NormalizeStatement(statement)))
	digest := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", digest)
}

// NormalizeDecisionStatus returns a safe default for blank or invalid statuses.
func NormalizeDecisionStatus(status DecisionStatus) DecisionStatus {
	switch DecisionStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case DecisionStatusProposed:
		return DecisionStatusProposed
	case DecisionStatusStale:
		return DecisionStatusStale
	case DecisionStatusSuperseded:
		return DecisionStatusSuperseded
	case DecisionStatusRevoked:
		return DecisionStatusRevoked
	default:
		return DecisionStatusActive
	}
}

// NormalizeReviewState returns a safe default for blank or invalid review states.
func NormalizeReviewState(state ReviewState) ReviewState {
	switch ReviewState(strings.ToLower(strings.TrimSpace(string(state)))) {
	case ReviewStateNeedsReview:
		return ReviewStateNeedsReview
	case ReviewStateOperatorApproved:
		return ReviewStateOperatorApproved
	case ReviewStateOperatorRejected:
		return ReviewStateOperatorRejected
	default:
		return ReviewStateAutoActive
	}
}

// NormalizeCandidateValidationStatus returns a safe default for candidate validation status.
func NormalizeCandidateValidationStatus(status CandidateValidationStatus) CandidateValidationStatus {
	switch CandidateValidationStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case CandidateValidationQueued:
		return CandidateValidationQueued
	case CandidateValidationRejected:
		return CandidateValidationRejected
	default:
		return CandidateValidationAccepted
	}
}

// NormalizeEvidencePolarity returns "for" unless an explicit opposing polarity is present.
func NormalizeEvidencePolarity(polarity EvidencePolarity) EvidencePolarity {
	switch EvidencePolarity(strings.ToLower(strings.TrimSpace(string(polarity)))) {
	case EvidencePolarityAgainst:
		return EvidencePolarityAgainst
	default:
		return EvidencePolarityFor
	}
}

// NormalizeAssumptionCriticality returns contextual for blank or invalid criticality.
func NormalizeAssumptionCriticality(criticality AssumptionCriticality) AssumptionCriticality {
	switch AssumptionCriticality(strings.ToLower(strings.TrimSpace(string(criticality)))) {
	case CriticalityLoadBearing:
		return CriticalityLoadBearing
	case CriticalitySupporting:
		return CriticalitySupporting
	default:
		return CriticalityContextual
	}
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
