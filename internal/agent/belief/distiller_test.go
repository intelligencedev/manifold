package belief

import (
	"context"
	"strings"
	"testing"
	"time"
)

type testBeliefStore struct {
	NoopStore
	beliefs  map[string]Belief
	evidence []Evidence
}

func newTestBeliefStore() *testBeliefStore {
	return &testBeliefStore{beliefs: map[string]Belief{}}
}

func (s *testBeliefStore) UpsertBelief(_ context.Context, item Belief) (Belief, error) {
	if item.ID == "" {
		item.ID = item.StatementHash
	}
	item.UpdatedAt = time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = item.UpdatedAt
	}
	s.beliefs[item.StatementHash] = item
	return item, nil
}

func (s *testBeliefStore) SearchBeliefs(_ context.Context, query SearchQuery) ([]SearchResult, error) {
	out := make([]SearchResult, 0, len(s.beliefs))
	for _, item := range s.beliefs {
		if item.TenantID != query.TenantID {
			continue
		}
		if len(query.ScopeIDs) > 0 && item.ScopeID != query.ScopeIDs[0] {
			continue
		}
		if query.Query != "" && !strings.Contains(strings.ToLower(item.Statement), strings.ToLower(strings.TrimSuffix(query.Query, "."))) {
			continue
		}
		out = append(out, SearchResult{Belief: item, Score: item.Confidence})
	}
	return out, nil
}

func (s *testBeliefStore) AddEvidence(_ context.Context, evidence Evidence) (Evidence, error) {
	s.evidence = append(s.evidence, evidence)
	return evidence, nil
}

func TestSimpleDistillerProducesLowConfidenceImplicitSuccess(t *testing.T) {
	t.Parallel()

	episode := Episode{
		ID:            "episode-1",
		TenantID:      7,
		ScopeID:       "scope-1",
		ProjectID:     "project-1",
		ObjectiveID:   "objective-1",
		AgentRole:     "orchestrator",
		Outcome:       "success",
		OutcomeSignal: "implicit_success",
	}
	candidates, err := SimpleDistiller{}.Distill(context.Background(), DistillationInput{
		Episode: episode,
		Summary: "Transit is used for durable shared working memory in this project. It supports scoped keys.",
	})
	if err != nil {
		t.Fatalf("Distill returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(candidates))
	}
	if candidates[0].Confidence >= 0.5 {
		t.Fatalf("implicit success should be low confidence, got %f", candidates[0].Confidence)
	}
	if candidates[0].Polarity != EvidencePolarityFor {
		t.Fatalf("expected supporting evidence, got %q", candidates[0].Polarity)
	}
	if candidates[0].StatementHash == "" {
		t.Fatal("expected statement hash")
	}
}

func TestSimpleDistillerAddsEmbeddingWhenConfigured(t *testing.T) {
	t.Parallel()

	distiller := SimpleDistiller{Embed: func(_ context.Context, texts []string) ([][]float32, error) {
		if len(texts) != 1 {
			t.Fatalf("expected one text to embed, got %d", len(texts))
		}
		return [][]float32{{0.1, 0.2, 0.3}}, nil
	}}
	candidates, err := distiller.Distill(context.Background(), DistillationInput{
		Episode: Episode{ID: "episode-1", TenantID: 7, ScopeID: "scope-1", ProjectID: "project-1", ObjectiveID: "objective-1", Outcome: "success", OutcomeSignal: "tests_passed"},
		Summary: "Transit is used for durable shared working memory in this project.",
	})
	if err != nil {
		t.Fatalf("Distill returned error: %v", err)
	}
	if len(candidates) != 1 || len(candidates[0].Embedding) != 3 {
		t.Fatalf("expected candidate embedding, got %#v", candidates)
	}
}

func TestApplyCandidatesConsolidatesAndContradictionLowersConfidence(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestBeliefStore()
	episode := Episode{ID: "episode-1", TenantID: 7, ScopeID: "scope-1"}
	statement := "Transit stores shared working memory for the project."
	hash := StatementHash(statement)
	positive := Candidate{
		Statement:     statement,
		StatementHash: hash,
		Confidence:    0.25,
		Polarity:      EvidencePolarityFor,
		EvidenceNote:  "implicit success",
	}

	first, err := ApplyCandidates(ctx, store, episode, []Candidate{positive})
	if err != nil {
		t.Fatalf("ApplyCandidates first: %v", err)
	}
	second, err := ApplyCandidates(ctx, store, Episode{ID: "episode-2", TenantID: 7, ScopeID: "scope-1"}, []Candidate{positive})
	if err != nil {
		t.Fatalf("ApplyCandidates second: %v", err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected applied beliefs, got %d and %d", len(first), len(second))
	}
	if first[0].ID != second[0].ID {
		t.Fatalf("expected repeated candidates to consolidate, got %q and %q", first[0].ID, second[0].ID)
	}
	if second[0].EvidenceFor != 2 {
		t.Fatalf("expected two supporting evidence counts, got %d", second[0].EvidenceFor)
	}
	if second[0].Confidence <= first[0].Confidence || second[0].Confidence >= 0.5 {
		t.Fatalf("expected low but increasing confidence, first=%f second=%f", first[0].Confidence, second[0].Confidence)
	}

	contradicted, err := ApplyCandidates(ctx, store, Episode{ID: "episode-3", TenantID: 7, ScopeID: "scope-1"}, []Candidate{{
		Statement:     statement,
		StatementHash: hash,
		Confidence:    0.8,
		Polarity:      EvidencePolarityAgainst,
		EvidenceNote:  "human rejected",
	}})
	if err != nil {
		t.Fatalf("ApplyCandidates contradiction: %v", err)
	}
	if contradicted[0].EvidenceAgainst != 1 {
		t.Fatalf("expected one contradicting evidence count, got %d", contradicted[0].EvidenceAgainst)
	}
	if contradicted[0].Confidence >= second[0].Confidence {
		t.Fatalf("expected contradiction to lower confidence from %f to %f", second[0].Confidence, contradicted[0].Confidence)
	}
	if len(store.evidence) != 3 {
		t.Fatalf("expected evidence rows for every candidate, got %d", len(store.evidence))
	}
}

func TestApplyCandidatesStoresEmbedding(t *testing.T) {
	t.Parallel()

	store := newTestBeliefStore()
	applied, err := ApplyCandidates(context.Background(), store, Episode{ID: "episode-1", TenantID: 7, ScopeID: "scope-1"}, []Candidate{{
		Statement:  "Transit stores shared working memory for the project.",
		Confidence: 0.75,
		Polarity:   EvidencePolarityFor,
		Embedding:  []float32{0.4, 0.5},
	}})
	if err != nil {
		t.Fatalf("ApplyCandidates: %v", err)
	}
	if len(applied) != 1 || len(applied[0].Embedding) != 2 {
		t.Fatalf("expected stored embedding, got %#v", applied)
	}
}
