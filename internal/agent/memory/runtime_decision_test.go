package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/policy"
)

type fakeDecisionRetriever struct {
	results []decision.SearchResult
	request decision.RetrievalRequest
}

func (f *fakeDecisionRetriever) Retrieve(_ context.Context, request decision.RetrievalRequest) ([]decision.SearchResult, error) {
	f.request = request
	return f.results, nil
}

func TestRuntimePrepareContextIncludesDecisionLaneAfterPolicy(t *testing.T) {
	t.Parallel()

	retriever := &fakeDecisionRetriever{results: []decision.SearchResult{{
		Decision: decision.Decision{
			ID:          "dec-1",
			ScopeID:     "project/proj-a",
			Statement:   "We decided to keep activation deliberate.",
			Status:      decision.DecisionStatusActive,
			ReviewState: decision.ReviewStateOperatorApproved,
			Confidence:  0.9,
			DecidedAt:   time.Now().UTC(),
		},
		Score: 1,
	}}}
	runtime := &Runtime{
		Config: RuntimeConfig{Enabled: true, MaxTokensPerPrompt: 2200, Timeout: time.Second},
		PolicyProvider: fakePolicyProvider{records: []policy.Record{{
			ID:            "policy-1",
			Scope:         policy.ScopeProject,
			Severity:      policy.SeveritySoft,
			Statement:     "Prefer deterministic tests.",
			ApprovalState: policy.ApprovalApproved,
		}}},
		Decision: retriever,
		Belief: fakeBeliefRetriever{results: []belief.SearchResult{{
			Belief: belief.Belief{
				ID:          "belief-1",
				Statement:   "The project uses Postgres migrations.",
				Confidence:  0.9,
				EvidenceFor: 2,
				Status:      belief.BeliefStatusActive,
			},
			Scope: belief.Scope{ID: "scope-1", Kind: belief.ScopeKindProject},
			Score: 0.8,
		}}},
	}

	block, diag, err := runtime.PrepareContext(context.Background(), Request{
		UserInput:   "should activation be automatic?",
		UserID:      42,
		ProjectID:   "proj-a",
		ObjectiveID: "obj-1",
		SessionID:   "sess-1",
		Role:        "orchestrator",
	})
	if err != nil {
		t.Fatalf("PrepareContext() error = %v", err)
	}
	assertOrder(t, block.Text,
		"Runtime Policy Context",
		"Recorded Decisions",
		"Shared Belief Memory",
	)
	if !strings.Contains(block.Text, "id=dec-1") {
		t.Fatalf("expected decision id in context:\n%s", block.Text)
	}
	if !diag.Lanes["decision"].Returned {
		t.Fatalf("expected decision lane to return context, diagnostics=%#v", diag.Lanes["decision"])
	}
	if retriever.request.TenantID != 42 || retriever.request.ProjectID != "proj-a" || retriever.request.ObjectiveID != "obj-1" || retriever.request.SessionID != "sess-1" {
		t.Fatalf("expected request context forwarded to decision retriever, got %#v", retriever.request)
	}
}

func TestRuntimeDecisionLaneDisabledWithoutRetriever(t *testing.T) {
	t.Parallel()

	runtime := &Runtime{
		Config: RuntimeConfig{Enabled: true, MaxTokensPerPrompt: 2200, Timeout: time.Second},
		Magma:  fakeMagmaRetriever{ctx: MagmaContext{Text: "node", Items: 1}},
	}
	_, diag, err := runtime.PrepareContext(context.Background(), Request{UserInput: "query", UserID: 1})
	if err != nil {
		t.Fatalf("PrepareContext() error = %v", err)
	}
	if diag.Lanes["decision"].Enabled {
		t.Fatalf("expected decision lane disabled, diagnostics=%#v", diag.Lanes["decision"])
	}
}
