package decision

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

const (
	defaultAutoActivateMinConfidence = 0.85
	defaultConflictSimilarityFloor   = 0.50
)

// AutoAcceptOutcome reports the result of one auto-activation attempt.
type AutoAcceptOutcome struct {
	CandidateID string   `json:"candidateId"`
	Decision    Decision `json:"decision,omitempty"`
	Accepted    bool     `json:"accepted"`
	Reason      string   `json:"reason"`
}

// AutoAcceptCandidates applies the deterministic auto-activation policy to
// freshly distilled candidates: a candidate is accepted as an active decision
// with reviewState=auto_active only when its confidence clears the configured
// floor and it neither duplicates nor conflicts with an active in-scope
// decision. Conflicting active decisions are flagged needs_review instead of
// being mutated; the candidate stays queued for deliberate operator review.
// No LLM calls are involved.
func (s *Service) AutoAcceptCandidates(ctx context.Context, tenantID int64, candidates []Candidate) ([]AutoAcceptOutcome, error) {
	if s == nil || s.Store == nil {
		return nil, ErrStoreRequired
	}
	if !s.Config.AutoActivateCandidates {
		return nil, nil
	}
	minConfidence := s.Config.AutoActivateMinConfidence
	if minConfidence <= 0 {
		minConfidence = defaultAutoActivateMinConfidence
	}
	similarityFloor := s.Config.ConflictSimilarityFloor
	if similarityFloor <= 0 {
		similarityFloor = defaultConflictSimilarityFloor
	}
	outcomes := make([]AutoAcceptOutcome, 0, len(candidates))
	for _, candidate := range candidates {
		outcome := AutoAcceptOutcome{CandidateID: strings.TrimSpace(candidate.ID)}
		switch {
		case outcome.CandidateID == "":
			outcome.Reason = "candidate missing id"
		case NormalizeCandidateValidationStatus(candidate.ValidationStatus) != CandidateValidationQueued:
			outcome.Reason = "candidate is not queued"
		case clampConfidence(candidate.Confidence) < minConfidence:
			outcome.Reason = fmt.Sprintf("confidence %.2f below auto-activation floor %.2f", clampConfidence(candidate.Confidence), minConfidence)
		default:
			conflict, duplicate, err := s.findAutoAcceptConflict(ctx, tenantID, candidate, similarityFloor)
			if err != nil {
				return outcomes, err
			}
			switch {
			case duplicate != nil:
				outcome.Reason = "statement already recorded as decision " + duplicate.ID
			case conflict != nil:
				reason := "auto-activation conflict with distilled candidate " + outcome.CandidateID + ": " + strings.TrimSpace(candidate.Statement)
				if _, err := s.MarkNeedsReview(ctx, tenantID, conflict.ID, reason, outcome.CandidateID); err != nil {
					return outcomes, err
				}
				outcome.Reason = "conflicts with active decision " + conflict.ID + "; existing decision marked needs_review"
			default:
				created, err := s.AcceptCandidate(ctx, tenantID, outcome.CandidateID, nil)
				if err != nil {
					return outcomes, err
				}
				outcome.Decision = created
				outcome.Accepted = true
				outcome.Reason = "auto-activated"
			}
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// findAutoAcceptConflict scans active in-scope decisions for an exact
// statement-hash duplicate or a lexically similar conflicting statement.
func (s *Service) findAutoAcceptConflict(ctx context.Context, tenantID int64, candidate Candidate, similarityFloor float64) (conflict, duplicate *Decision, err error) {
	scopeID := strings.TrimSpace(candidate.ScopeID)
	if scopeID == "" {
		return nil, nil, nil
	}
	results, err := s.Store.SearchDecisions(ctx, SearchQuery{
		TenantID: tenantID,
		ScopeIDs: []string{scopeID},
		Statuses: []DecisionStatus{DecisionStatusActive},
		Limit:    50,
	})
	if err != nil {
		return nil, nil, err
	}
	hash := strings.TrimSpace(candidate.StatementHash)
	if hash == "" {
		hash = StatementHash(candidate.Statement)
	}
	for _, result := range results {
		item := result.Decision
		if item.StatementHash == hash {
			d := item
			return nil, &d, nil
		}
		if statementSimilarity(candidate.Statement, item.Statement) >= similarityFloor {
			d := item
			return &d, nil, nil
		}
	}
	return nil, nil, nil
}

// statementSimilarity returns the token Jaccard similarity between two
// normalized statements. It is deterministic and requires no model calls.
func statementSimilarity(a, b string) float64 {
	ta := statementTokens(a)
	tb := statementTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	intersection := 0
	for token := range ta {
		if tb[token] {
			intersection++
		}
	}
	union := len(ta) + len(tb) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func statementTokens(statement string) map[string]bool {
	normalized := strings.ToLower(NormalizeStatement(statement))
	out := map[string]bool{}
	for _, token := range strings.FieldsFunc(normalized, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(token) >= 3 && !decisionFillerTokens[token] {
			out[token] = true
		}
	}
	return out
}

// decisionFillerTokens removes boilerplate decision language so similarity
// reflects the substance of a statement rather than its framing.
var decisionFillerTokens = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true,
	"this": true, "are": true, "was": true, "were": true, "been": true,
	"being": true, "will": true, "would": true, "could": true, "should": true,
	"must": true, "shall": true, "into": true, "from": true, "over": true,
	"under": true, "our": true, "not": true, "its": true, "has": true,
	"have": true, "had": true, "when": true, "where": true, "then": true,
	"than": true, "they": true, "them": true,
	"decided": true, "decide": true, "decision": true, "chose": true,
	"choose": true, "adopt": true, "adopted": true,
}
