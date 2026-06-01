package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/policy"
)

type fakeBeliefRetriever struct {
	results []belief.SearchResult
}

func (f fakeBeliefRetriever) Retrieve(context.Context, belief.RetrievalRequest) ([]belief.SearchResult, error) {
	return f.results, nil
}

type fakePolicyProvider struct {
	records []policy.Record
}

func (f fakePolicyProvider) PromptContext(context.Context, policy.EvaluationRequest) ([]policy.Record, error) {
	return f.records, nil
}

type fakeMagmaRetriever struct {
	ctx MagmaContext
}

func (f fakeMagmaRetriever) RetrieveMagmaContext(context.Context, MagmaRequest) (MagmaContext, error) {
	return f.ctx, nil
}

func TestRuntimePrepareContextDisabled(t *testing.T) {
	t.Parallel()

	runtime := &Runtime{Config: RuntimeConfig{Enabled: false}}
	block, diag, err := runtime.PrepareContext(context.Background(), Request{UserInput: "remember this"})
	if err != nil {
		t.Fatalf("PrepareContext() error = %v", err)
	}
	if block.Text != "" || diag.Enabled {
		t.Fatalf("expected no context when memory is disabled, block=%#v diag=%#v", block, diag)
	}
}

func TestRuntimePrepareContextFusesMemoryLanesInPromptOrder(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EnableRAG: true, TopK: 2, WindowSize: 2})
	em.entries = []*MemoryEntry{{
		ID:       "exp-1",
		Input:    "fix flaky migration test",
		Output:   "use deterministic clock",
		Feedback: "success",
	}}
	runtime := &Runtime{
		Config: RuntimeConfig{
			Enabled:            true,
			MaxTokensPerPrompt: 2200,
			Timeout:            time.Second,
			IncludeRecent:      true,
		},
		PolicyProvider: fakePolicyProvider{records: []policy.Record{{
			ID:            "policy-1",
			Scope:         policy.ScopeProject,
			Severity:      policy.SeveritySoft,
			Statement:     "Prefer deterministic tests.",
			ApprovalState: policy.ApprovalApproved,
		}}},
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
		Magma:                   fakeMagmaRetriever{ctx: MagmaContext{Text: "causal edge: migration -> flaky test", Items: 1}},
		Evolving:                em,
		BeliefMaxBeliefs:        5,
		BeliefPromptTokenBudget: 700,
	}

	block, diag, err := runtime.PrepareContext(context.Background(), Request{
		UserInput: "fix flaky migration test",
		UserID:    42,
		ProjectID: "project-a",
		Role:      "orchestrator",
	})
	if err != nil {
		t.Fatalf("PrepareContext() error = %v", err)
	}
	if block.Text == "" {
		t.Fatal("expected fused memory context")
	}
	assertOrder(t, block.Text,
		"Runtime Policy Context",
		"Shared Belief Memory",
		"Graph Memory",
		"Past Relevant Experiences",
		"Recent Task History",
	)
	for _, lane := range []string{"policy", "belief", "magma", "evolving", "recent"} {
		if !diag.Lanes[lane].Returned {
			t.Fatalf("expected lane %q to return context, diagnostics=%#v", lane, diag.Lanes[lane])
		}
	}
}

func TestRuntimePrepareContextTruncatesAtBudget(t *testing.T) {
	t.Parallel()

	runtime := &Runtime{
		Config: RuntimeConfig{Enabled: true, MaxTokensPerPrompt: 10, Timeout: time.Second},
		Magma:  fakeMagmaRetriever{ctx: MagmaContext{Text: strings.Repeat("large memory context ", 80), Items: 1}},
	}
	block, diag, err := runtime.PrepareContext(context.Background(), Request{UserInput: "query", UserID: 1})
	if err != nil {
		t.Fatalf("PrepareContext() error = %v", err)
	}
	if !block.Truncated || !diag.Truncated {
		t.Fatalf("expected truncation, block=%#v diag=%#v", block, diag)
	}
}

func TestMemoryQualityEvalFixtures(t *testing.T) {
	t.Parallel()

	type fixture struct {
		Name     string `json:"name"`
		Query    string `json:"query"`
		Belief   string `json:"belief"`
		Evolving string `json:"evolving"`
		Magma    string `json:"magma"`
		Expected string `json:"expected"`
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "evals", "unified_memory_quality.json"))
	if err != nil {
		t.Fatalf("read eval fixtures: %v", err)
	}
	var fixtures []fixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("decode eval fixtures: %v", err)
	}
	if len(fixtures) < 5 {
		t.Fatalf("expected core memory eval coverage, got %d fixtures", len(fixtures))
	}

	for _, fx := range fixtures {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			runtime := &Runtime{Config: RuntimeConfig{Enabled: true, MaxTokensPerPrompt: 2200, Timeout: time.Second, IncludeRecent: true}}
			if fx.Belief != "" {
				runtime.Belief = fakeBeliefRetriever{results: []belief.SearchResult{{
					Belief: belief.Belief{ID: fx.Name, Statement: fx.Belief, Confidence: 0.9, EvidenceFor: 2, Status: belief.BeliefStatusActive},
					Scope:  belief.Scope{ID: "scope", Kind: belief.ScopeKindProject},
					Score:  0.9,
				}}}
			}
			if fx.Evolving != "" {
				em := NewEvolvingMemory(EvolvingMemoryConfig{EnableRAG: true, TopK: 2, WindowSize: 2})
				em.entries = []*MemoryEntry{{ID: fx.Name, Input: fx.Query, Output: fx.Evolving, Feedback: "failure"}}
				runtime.Evolving = em
			}
			if fx.Magma != "" {
				runtime.Magma = fakeMagmaRetriever{ctx: MagmaContext{Text: fx.Magma, Items: 1}}
			}

			block, _, err := runtime.PrepareContext(context.Background(), Request{UserInput: fx.Query, UserID: 1, ProjectID: "project"})
			if err != nil {
				t.Fatalf("PrepareContext() error = %v", err)
			}
			if !strings.Contains(block.Text, fx.Expected) {
				t.Fatalf("expected %q in context:\n%s", fx.Expected, block.Text)
			}
		})
	}
}

func BenchmarkMemoryPrepareContext(b *testing.B) {
	runtime := &Runtime{
		Config: RuntimeConfig{Enabled: true, MaxTokensPerPrompt: 2200, Timeout: time.Second},
		Belief: fakeBeliefRetriever{results: []belief.SearchResult{{
			Belief: belief.Belief{ID: "belief", Statement: "Use bounded queues for background memory work.", Confidence: 0.9, EvidenceFor: 2, Status: belief.BeliefStatusActive},
			Scope:  belief.Scope{ID: "scope", Kind: belief.ScopeKindProject},
			Score:  0.9,
		}}},
		Magma: fakeMagmaRetriever{ctx: MagmaContext{Text: "semantic node: bounded queues", Items: 1}},
	}
	req := Request{UserInput: "how should memory workers run?", UserID: 1, ProjectID: "project"}
	for b.Loop() {
		if _, _, err := runtime.PrepareContext(context.Background(), req); err != nil {
			b.Fatal(err)
		}
	}
}

func assertOrder(t *testing.T, text string, needles ...string) {
	t.Helper()
	last := -1
	for _, needle := range needles {
		idx := strings.Index(text, needle)
		if idx < 0 {
			t.Fatalf("missing %q in context:\n%s", needle, text)
		}
		if idx <= last {
			t.Fatalf("expected %q after previous section in context:\n%s", needle, text)
		}
		last = idx
	}
}
