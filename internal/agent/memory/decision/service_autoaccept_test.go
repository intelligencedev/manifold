package decision_test

import (
	"context"
	"strings"
	"testing"

	. "manifold/internal/agent/memory/decision"
	"manifold/internal/persistence/databases"
)

func recordQueuedCandidate(t *testing.T, store Store, c Candidate) Candidate {
	t.Helper()
	if c.ReviewState == "" {
		c.ReviewState = ReviewStateNeedsReview
	}
	if c.ValidationStatus == "" {
		c.ValidationStatus = CandidateValidationQueued
	}
	if c.StatementHash == "" {
		c.StatementHash = StatementHash(c.Statement)
	}
	stored, err := store.RecordCandidate(context.Background(), c)
	if err != nil {
		t.Fatalf("RecordCandidate: %v", err)
	}
	return stored
}

func TestAutoAcceptCandidatesActivatesHighConfidenceCandidate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store, Config: ServiceConfig{AutoActivateCandidates: true}}
	candidate := recordQueuedCandidate(t, store, Candidate{
		TenantID:   7,
		EpisodeID:  "ep-1",
		ScopeID:    "scope-1",
		Statement:  "We decided to inject a deterministic decision lane into prompt context.",
		Confidence: 0.88,
	})

	outcomes, err := service.AutoAcceptCandidates(ctx, 7, []Candidate{candidate})
	if err != nil {
		t.Fatalf("AutoAcceptCandidates: %v", err)
	}
	if len(outcomes) != 1 || !outcomes[0].Accepted {
		t.Fatalf("expected accepted outcome, got %#v", outcomes)
	}
	created := outcomes[0].Decision
	if created.Status != DecisionStatusActive {
		t.Fatalf("expected active decision, got %s", created.Status)
	}
	if created.ReviewState != ReviewStateAutoActive {
		t.Fatalf("expected auto_active review state, got %s", created.ReviewState)
	}
	updated, ok, err := store.GetCandidate(ctx, 7, candidate.ID)
	if err != nil || !ok {
		t.Fatalf("GetCandidate: ok=%v err=%v", ok, err)
	}
	if updated.ValidationStatus != CandidateValidationAccepted || updated.AcceptedDecisionID != created.ID {
		t.Fatalf("expected candidate accepted with decision link, got %#v", updated)
	}
}

func TestAutoAcceptCandidatesSkipsBelowConfidenceFloor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store, Config: ServiceConfig{AutoActivateCandidates: true}}
	candidate := recordQueuedCandidate(t, store, Candidate{
		TenantID:   7,
		ScopeID:    "scope-1",
		Statement:  "We decided to try a speculative approach.",
		Confidence: 0.70,
	})

	outcomes, err := service.AutoAcceptCandidates(ctx, 7, []Candidate{candidate})
	if err != nil {
		t.Fatalf("AutoAcceptCandidates: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Accepted {
		t.Fatalf("expected skipped outcome, got %#v", outcomes)
	}
	if !strings.Contains(outcomes[0].Reason, "below auto-activation floor") {
		t.Fatalf("unexpected reason: %q", outcomes[0].Reason)
	}
	updated, ok, err := store.GetCandidate(ctx, 7, candidate.ID)
	if err != nil || !ok {
		t.Fatalf("GetCandidate: ok=%v err=%v", ok, err)
	}
	if updated.ValidationStatus != CandidateValidationQueued {
		t.Fatalf("expected candidate to stay queued, got %s", updated.ValidationStatus)
	}
}

func TestAutoAcceptCandidatesSkipsDuplicateStatement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store, Config: ServiceConfig{AutoActivateCandidates: true}}
	statement := "We decided to keep stale-to-active transitions deliberate."
	existing, err := service.CreateDecision(ctx, Decision{
		TenantID:    7,
		ScopeID:     "scope-1",
		Statement:   statement,
		Status:      DecisionStatusActive,
		ReviewState: ReviewStateOperatorApproved,
		Confidence:  0.95,
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	candidate := recordQueuedCandidate(t, store, Candidate{
		TenantID:   7,
		ScopeID:    "scope-1",
		Statement:  statement,
		Confidence: 0.90,
	})

	outcomes, err := service.AutoAcceptCandidates(ctx, 7, []Candidate{candidate})
	if err != nil {
		t.Fatalf("AutoAcceptCandidates: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Accepted {
		t.Fatalf("expected duplicate skip, got %#v", outcomes)
	}
	if !strings.Contains(outcomes[0].Reason, existing.ID) {
		t.Fatalf("expected duplicate reason to reference %s, got %q", existing.ID, outcomes[0].Reason)
	}
	// The operator decision must not be mutated by the duplicate candidate.
	current, ok, err := store.GetDecision(ctx, 7, existing.ID)
	if err != nil || !ok {
		t.Fatalf("GetDecision: ok=%v err=%v", ok, err)
	}
	if current.ReviewState != ReviewStateOperatorApproved {
		t.Fatalf("expected operator decision untouched, got %#v", current)
	}
}

func TestAutoAcceptCandidatesFlagsConflictingDecisionForReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store, Config: ServiceConfig{AutoActivateCandidates: true}}
	existing, err := service.CreateDecision(ctx, Decision{
		TenantID:    7,
		ScopeID:     "scope-1",
		Statement:   "We decided to use Postgres for persistence storage.",
		Status:      DecisionStatusActive,
		ReviewState: ReviewStateOperatorApproved,
		Confidence:  0.9,
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	candidate := recordQueuedCandidate(t, store, Candidate{
		TenantID:   7,
		ScopeID:    "scope-1",
		Statement:  "We decided to use MySQL for persistence storage.",
		Confidence: 0.90,
	})

	outcomes, err := service.AutoAcceptCandidates(ctx, 7, []Candidate{candidate})
	if err != nil {
		t.Fatalf("AutoAcceptCandidates: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Accepted {
		t.Fatalf("expected conflict skip, got %#v", outcomes)
	}
	if !strings.Contains(outcomes[0].Reason, "needs_review") {
		t.Fatalf("unexpected reason: %q", outcomes[0].Reason)
	}
	current, ok, err := store.GetDecision(ctx, 7, existing.ID)
	if err != nil || !ok {
		t.Fatalf("GetDecision: ok=%v err=%v", ok, err)
	}
	if current.Status != DecisionStatusActive {
		t.Fatalf("conflict must not change status, got %s", current.Status)
	}
	if current.ReviewState != ReviewStateNeedsReview {
		t.Fatalf("expected existing decision flagged needs_review, got %s", current.ReviewState)
	}
	updated, ok, err := store.GetCandidate(ctx, 7, candidate.ID)
	if err != nil || !ok {
		t.Fatalf("GetCandidate: ok=%v err=%v", ok, err)
	}
	if updated.ValidationStatus != CandidateValidationQueued {
		t.Fatalf("expected conflicting candidate to stay queued, got %s", updated.ValidationStatus)
	}
}

func TestAutoAcceptCandidatesDisabledIsNoop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store}
	candidate := recordQueuedCandidate(t, store, Candidate{
		TenantID:   7,
		ScopeID:    "scope-1",
		Statement:  "We decided to do something with very high confidence.",
		Confidence: 0.90,
	})

	outcomes, err := service.AutoAcceptCandidates(ctx, 7, []Candidate{candidate})
	if err != nil {
		t.Fatalf("AutoAcceptCandidates: %v", err)
	}
	if outcomes != nil {
		t.Fatalf("expected nil outcomes when disabled, got %#v", outcomes)
	}
	updated, ok, err := store.GetCandidate(ctx, 7, candidate.ID)
	if err != nil || !ok {
		t.Fatalf("GetCandidate: ok=%v err=%v", ok, err)
	}
	if updated.ValidationStatus != CandidateValidationQueued {
		t.Fatalf("expected candidate untouched, got %#v", updated)
	}
}
