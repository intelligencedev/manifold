package belief

import "strings"

func NormalizeBeliefKind(kind BeliefKind) BeliefKind {
	switch BeliefKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case BeliefKindPreference:
		return BeliefKindPreference
	case BeliefKindProcedure:
		return BeliefKindProcedure
	case BeliefKindConstraint:
		return BeliefKindConstraint
	case BeliefKindCapability:
		return BeliefKindCapability
	default:
		return BeliefKindFact
	}
}

func NormalizeEnforcement(mode EnforcementMode) EnforcementMode {
	switch EnforcementMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case EnforcementPrompt:
		return EnforcementPrompt
	case EnforcementSoftPolicy:
		return EnforcementSoftPolicy
	case EnforcementHardConstraint:
		return EnforcementHardConstraint
	default:
		return EnforcementNone
	}
}

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
