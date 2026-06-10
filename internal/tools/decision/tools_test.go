package decisiontools

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/agent/memory/archaeology"
	decisionmem "manifold/internal/agent/memory/decision"
	"manifold/internal/auth"
	"manifold/internal/persistence/databases"
)

func TestRecordSearchAndReconstructTools(t *testing.T) {
	t.Parallel()
	ctx := auth.WithUser(context.Background(), &auth.User{ID: 7})
	store := databases.NewMemoryDecisionStore()
	service := &decisionmem.Service{Store: store}

	recorded, err := NewRecordTool(service).Call(ctx, json.RawMessage(`{
		"scopeId":"scope-1",
		"statement":"Use PostgreSQL JSONB payloads for decision archaeology stores.",
		"rationale":"The typed columns support filters while payloads preserve forward-compatible records.",
		"confidence":0.86,
		"evidence":[{"sourceKind":"artifact","sourceId":"artifact-1","polarity":"for","note":"implementation patch"}],
		"assumptions":[{"beliefId":"belief-1","criticality":"load_bearing","beliefStatementAtLink":"PostgreSQL is configured","beliefConfidenceAtLink":0.9}],
		"alternatives":[{"statement":"Use memory-only stores","rejectionReason":"not durable"}]
	}`))
	if err != nil {
		t.Fatalf("decision_record error = %v", err)
	}
	payload := recorded.(map[string]any)
	decision := payload["decision"].(decisionmem.Decision)
	if decision.TenantID != 7 || decision.Status != decisionmem.DecisionStatusActive || decision.DecidedBy != "human:7" {
		t.Fatalf("unexpected recorded decision: %#v", decision)
	}

	results, err := NewSearchTool(store).Call(ctx, json.RawMessage(`{"query":"JSONB","scopeIds":["scope-1"]}`))
	if err != nil {
		t.Fatalf("decision_search error = %v", err)
	}
	searchResults := results.([]decisionmem.SearchResult)
	if len(searchResults) != 1 || searchResults[0].Decision.ID != decision.ID {
		t.Fatalf("unexpected search results: %#v", searchResults)
	}

	report, err := NewReconstructTool(service, nil, nil).Call(ctx, json.RawMessage(`{"query":"JSONB","scopeIds":["scope-1"],"includeStale":true}`))
	if err != nil {
		t.Fatalf("decision_reconstruct error = %v", err)
	}
	reconstructed := report.(archaeology.Report)
	if len(reconstructed.Decisions) != 1 || reconstructed.Decisions[0].Decision.ID != decision.ID {
		t.Fatalf("unexpected reconstruction report: %#v", reconstructed)
	}
	lineage, ok, err := service.LoadLineage(ctx, 7, decision.ID)
	if err != nil || !ok {
		t.Fatalf("LoadLineage() ok=%v error=%v", ok, err)
	}
	if len(lineage.Evidence) != 1 || len(lineage.Assumptions) != 1 || len(lineage.Alternatives) != 1 {
		t.Fatalf("expected attached context, got %#v", lineage)
	}
}

func TestReviewToolAcceptsAndRejectsCandidates(t *testing.T) {
	t.Parallel()
	ctx := auth.WithUser(context.Background(), &auth.User{ID: 7})
	store := databases.NewMemoryDecisionStore()
	service := &decisionmem.Service{Store: store}
	candidate, err := store.RecordCandidate(ctx, decisionmem.Candidate{
		TenantID:         7,
		ScopeID:          "scope-1",
		Title:            "Use durable decisions",
		Statement:        "Persist decisions in the configured database.",
		Rationale:        "Recovered context must survive process restarts.",
		Confidence:       0.8,
		ReviewState:      decisionmem.ReviewStateNeedsReview,
		ValidationStatus: decisionmem.CandidateValidationQueued,
	})
	if err != nil {
		t.Fatalf("RecordCandidate() error = %v", err)
	}
	accepted, err := NewReviewTool(service).Call(ctx, json.RawMessage(`{"action":"accept_candidate","candidateId":"`+candidate.ID+`"}`))
	if err != nil {
		t.Fatalf("accept candidate error = %v", err)
	}
	decision := accepted.(decisionmem.Decision)
	if decision.Status != decisionmem.DecisionStatusActive || decision.ReviewState != decisionmem.ReviewStateOperatorApproved {
		t.Fatalf("unexpected accepted decision: %#v", decision)
	}

	rejectedCandidate, err := store.RecordCandidate(ctx, decisionmem.Candidate{
		TenantID:         7,
		ScopeID:          "scope-1",
		Title:            "Use transient decisions",
		Statement:        "Keep decisions only in process memory.",
		Confidence:       0.7,
		ReviewState:      decisionmem.ReviewStateNeedsReview,
		ValidationStatus: decisionmem.CandidateValidationQueued,
	})
	if err != nil {
		t.Fatalf("RecordCandidate() error = %v", err)
	}
	rejected, err := NewReviewTool(service).Call(ctx, json.RawMessage(`{"action":"reject_candidate","candidateId":"`+rejectedCandidate.ID+`","reason":"not durable"}`))
	if err != nil {
		t.Fatalf("reject candidate error = %v", err)
	}
	rejectedRecord := rejected.(decisionmem.Candidate)
	if rejectedRecord.ValidationStatus != decisionmem.CandidateValidationRejected || rejectedRecord.ReviewState != decisionmem.ReviewStateOperatorRejected {
		t.Fatalf("unexpected rejected candidate: %#v", rejectedRecord)
	}
}
