package belief

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRAGEvidenceSourceFiltersAndCaps(t *testing.T) {
	t.Parallel()

	var capturedTenant string
	var capturedFilter map[string]string
	var capturedK int
	source := RAGEvidenceSource{
		K:        4,
		MinScore: 0.3,
		MaxItems: 2,
		Retriever: func(_ context.Context, query, tenant string, filter map[string]string, k int) ([]RAGEvidenceItem, error) {
			capturedTenant = tenant
			capturedFilter = filter
			capturedK = k
			if query == "" {
				t.Fatalf("expected non-empty query")
			}
			return []RAGEvidenceItem{
				{ID: "low", DocID: "doc-1", Score: 0.10, Snippet: "skip me"},
				{ID: "high-1", DocID: "doc-2", Score: 0.91, Snippet: "stable build"},
				{ID: "mid", DocID: "doc-3", Score: 0.55, Snippet: "useful context"},
				{ID: "extra", DocID: "doc-4", Score: 0.85, Snippet: "should not appear due to cap"},
			}, nil
		},
	}

	results, err := source.Retrieve(context.Background(), RetrievalRequest{
		TenantID:    7,
		UserID:      11,
		ProjectID:   "project-x",
		ObjectiveID: "obj-1",
		Query:       "build status",
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if capturedTenant != "7" {
		t.Fatalf("expected tenant=7, got %q", capturedTenant)
	}
	if capturedFilter["tenant_id"] != "7" || capturedFilter["project_id"] != "project-x" || capturedFilter["objective_id"] != "obj-1" || capturedFilter["user_id"] != "11" {
		t.Fatalf("unexpected filter scoping: %+v", capturedFilter)
	}
	if capturedK != 4 {
		t.Fatalf("expected K=4 propagated, got %d", capturedK)
	}
	if len(results) != 2 {
		t.Fatalf("expected MaxItems cap to leave 2 results, got %d", len(results))
	}
	for _, item := range results {
		if item.Belief.TenantID != 7 {
			t.Fatalf("expected results tagged with tenant 7, got %d", item.Belief.TenantID)
		}
		if item.Belief.Metadata["source"].(string) != "rag" {
			t.Fatalf("expected source=rag metadata, got %+v", item.Belief.Metadata)
		}
		if !strings.HasPrefix(item.Belief.ID, "rag:") {
			t.Fatalf("expected synthetic rag id prefix, got %q", item.Belief.ID)
		}
	}
}

func TestRAGEvidenceSourceSkipsWhenUnconfigured(t *testing.T) {
	t.Parallel()

	source := RAGEvidenceSource{}
	results, err := source.Retrieve(context.Background(), RetrievalRequest{TenantID: 1, Query: "anything"})
	if err != nil || len(results) != 0 {
		t.Fatalf("expected empty results for unconfigured source, got err=%v len=%d", err, len(results))
	}

	source.Retriever = func(context.Context, string, string, map[string]string, int) ([]RAGEvidenceItem, error) {
		t.Fatalf("retriever should not be invoked without tenant or query")
		return nil, nil
	}
	if r, _ := source.Retrieve(context.Background(), RetrievalRequest{TenantID: 0, Query: "x"}); len(r) != 0 {
		t.Fatalf("expected zero results when tenant missing")
	}
	if r, _ := source.Retrieve(context.Background(), RetrievalRequest{TenantID: 1, Query: "   "}); len(r) != 0 {
		t.Fatalf("expected zero results when query empty")
	}
}

type stubRetriever struct {
	results []SearchResult
	err     error
}

func (s stubRetriever) Retrieve(context.Context, RetrievalRequest) ([]SearchResult, error) {
	return s.results, s.err
}

type stubEvidence struct {
	items []SearchResult
	err   error
}

func (s stubEvidence) Retrieve(context.Context, RetrievalRequest) ([]SearchResult, error) {
	return s.items, s.err
}

func TestBlendedRetrieverPlacesBeliefsBeforeEvidence(t *testing.T) {
	t.Parallel()

	beliefs := []SearchResult{
		{Belief: Belief{ID: "b1", Statement: "belief one"}, Score: 1.5},
		{Belief: Belief{ID: "b2", Statement: "belief two"}, Score: 1.0},
	}
	evidence := []SearchResult{
		{Belief: Belief{ID: "rag:e1", Statement: "evidence one", Metadata: map[string]any{"source": "rag", "source_id": "e1"}}, Score: 5.0},
		{Belief: Belief{ID: "rag:e2", Statement: "evidence two", Metadata: map[string]any{"source": "rag", "source_id": "e2"}}, Score: 4.0},
		{Belief: Belief{ID: "rag:e3", Statement: "should be capped", Metadata: map[string]any{"source": "rag", "source_id": "e3"}}, Score: 3.0},
	}

	r := NewBlendedRetriever(stubRetriever{results: beliefs}, 2, stubEvidence{items: evidence})
	out, err := r.Retrieve(context.Background(), RetrievalRequest{TenantID: 1, Query: "anything"})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("expected 2 beliefs + 2 evidence (cap=2), got %d", len(out))
	}
	if out[0].Belief.ID != "b1" || out[1].Belief.ID != "b2" {
		t.Fatalf("expected beliefs to come first, got %v", []string{out[0].Belief.ID, out[1].Belief.ID})
	}
	if out[2].Belief.ID != "rag:e1" || out[3].Belief.ID != "rag:e2" {
		t.Fatalf("expected evidence ordering preserved, got %v", []string{out[2].Belief.ID, out[3].Belief.ID})
	}
	if out[0].Belief.Metadata["source"].(string) != "belief" {
		t.Fatalf("expected belief source label, got %+v", out[0].Belief.Metadata)
	}
}

func TestBlendedRetrieverEvidenceErrorsAreSwallowed(t *testing.T) {
	t.Parallel()

	beliefs := []SearchResult{{Belief: Belief{ID: "b1"}, Score: 1.0}}
	r := NewBlendedRetriever(stubRetriever{results: beliefs}, 3, stubEvidence{err: errors.New("rag down")})
	out, err := r.Retrieve(context.Background(), RetrievalRequest{TenantID: 1, Query: "x"})
	if err != nil {
		t.Fatalf("evidence errors must not propagate: %v", err)
	}
	if len(out) != 1 || out[0].Belief.ID != "b1" {
		t.Fatalf("expected belief result preserved, got %+v", out)
	}
}

func TestBlendedRetrieverPropagatesBeliefError(t *testing.T) {
	t.Parallel()

	r := NewBlendedRetriever(stubRetriever{err: errors.New("boom")}, 1, stubEvidence{items: []SearchResult{{Belief: Belief{ID: "rag:e1"}}}})
	_, err := r.Retrieve(context.Background(), RetrievalRequest{TenantID: 1, Query: "x"})
	if err == nil {
		t.Fatalf("expected belief retriever error to surface")
	}
}

func TestBuildPromptSectionDelimitsRAGEvidenceBlock(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{Belief: Belief{Statement: "policy belief", Confidence: 0.9, EvidenceFor: 3}, Scope: Scope{Kind: ScopeKindObjective, Path: "p/o"}},
		{Belief: Belief{ID: "rag:1", Statement: "supporting snippet", Metadata: map[string]any{"source": "rag", "doc_id": "doc-7", "title": "Runbook"}}, Score: 0.7},
	}
	prompt := BuildPromptSection(results, PromptOptions{MaxBeliefs: 5, MaxTokens: 1000})
	if !strings.Contains(prompt.Text, "Shared Belief Memory") {
		t.Fatalf("expected belief header present, got %q", prompt.Text)
	}
	if !strings.Contains(prompt.Text, "Retrieved Evidence (untrusted)") {
		t.Fatalf("expected delimited evidence header, got %q", prompt.Text)
	}
	if !strings.Contains(prompt.Text, "supporting snippet") {
		t.Fatalf("expected evidence snippet rendered, got %q", prompt.Text)
	}
	if len(prompt.Selected) != 2 {
		t.Fatalf("expected both items selected, got %d", len(prompt.Selected))
	}
}
