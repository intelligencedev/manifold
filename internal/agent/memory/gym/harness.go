package gym

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"manifold/internal/agent/memory"
	"manifold/internal/agent/memory/archaeology"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/agent/memory/magma"
	"manifold/internal/config"
	"manifold/internal/persistence/databases"
	"manifold/internal/policy"
	"manifold/internal/rag/embedder"
)

// Env is one fully wired, offline memory environment. Every dependency is
// in-memory and deterministic: no LLM, no network, no daemon.
type Env struct {
	Knobs    Knobs
	TenantID int64

	BeliefStore     belief.Store
	DecisionStore   decision.Store
	DecisionService *decision.Service
	Reactor         *decision.Reactor
	Archaeology     *archaeology.Service
	Magma           *magma.Service
	Evolving        *memory.EvolvingMemory
	Runtime         *memory.Runtime

	seededDecisions []decision.Decision
}

type staticPolicyProvider struct {
	records []policy.Record
}

func (p staticPolicyProvider) PromptContext(context.Context, policy.EvaluationRequest) ([]policy.Record, error) {
	return p.records, nil
}

type magmaRuntimeRetriever struct {
	service *magma.Service
}

func (r magmaRuntimeRetriever) RetrieveMagmaContext(ctx context.Context, req memory.MagmaRequest) (memory.MagmaContext, error) {
	out, err := r.service.Query(ctx, req.Query, magma.QueryOptions{
		Tenant:               req.Tenant,
		MaxHops:              req.MaxHops,
		MaxNodes:             req.MaxNodes,
		ContextFormat:        req.ContextFormat,
		IntentClassification: req.IntentClassification,
	})
	if err != nil {
		return memory.MagmaContext{}, err
	}
	return memory.MagmaContext{Text: out.Text, Items: len(out.RawEvents)}, nil
}

// NewEnv builds and seeds an environment for one scenario under one knob set.
func NewEnv(ctx context.Context, sc Scenario, knobs Knobs) (*Env, error) {
	beliefStore := databases.NewMemoryBeliefStore()
	decisionStore := databases.NewMemoryDecisionStore()
	decisionSvc := newDecisionService(decisionStore, beliefStore, knobs)
	env := &Env{
		Knobs:           knobs,
		TenantID:        sc.Seed.TenantID,
		BeliefStore:     beliefStore,
		DecisionStore:   decisionStore,
		DecisionService: decisionSvc,
		Reactor:         newDecisionReactor(decisionSvc, beliefStore, knobs),
		Archaeology:     newArchaeologyService(decisionSvc, beliefStore, knobs),
		Magma:           newMagmaService(knobs),
		Evolving:        newEvolvingMemory(knobs),
	}

	if err := env.seed(ctx, sc.Seed); err != nil {
		return nil, err
	}
	env.Runtime = env.newRuntime(sc.Seed.Policies)
	return env, nil
}

func newEvolvingMemory(knobs Knobs) *memory.EvolvingMemory {
	emb := embedder.NewDeterministic(64, true, 0)
	embedFn := func(ctx context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
		return emb.EmbedBatch(ctx, texts)
	}

	return memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn:                      embedFn,
		MaxSize:                      knobs.EvolvingMaxSize,
		TopK:                         knobs.EvolvingTopK,
		WindowSize:                   knobs.EvolvingWindowSize,
		EnableRAG:                    knobs.EvolvingEnableRAG,
		RetrievalSimilarityThreshold: knobs.EvolvingSimilarityThreshold,
		EnableSmartPrune:             knobs.EvolvingEnableSmartPrune,
		PruneThreshold:               knobs.EvolvingPruneThreshold,
		RelevanceDecay:               knobs.EvolvingRelevanceDecay,
		MinRelevance:                 knobs.EvolvingMinRelevance,
		PruneQualityFloor:            knobs.EvolvingPruneQualityFloor,
		PromotionAccessThreshold:     knobs.EvolvingPromotionThreshold,
		MMRLambda:                    knobs.EvolvingMMRLambda,
	})
}

func newMagmaService(knobs Knobs) *magma.Service {
	return magma.NewServiceWithConfig(
		databases.NewMemoryGraph(),
		databases.NewMemoryVector(),
		embedder.NewDeterministic(32, true, 0),
		magma.ServiceConfig{
			SemanticTopK:        knobs.MagmaSemanticTopK,
			SimilarityThreshold: knobs.MagmaSimilarityThreshold,
			CausalThreshold:     knobs.MagmaCausalThreshold,
			Graphs:              magma.GraphConfig{Semantic: true, Temporal: true, Causal: true, Entity: true, CoReference: true},
		},
	)
}

func newDecisionService(decisionStore decision.Store, beliefStore belief.Store, knobs Knobs) *decision.Service {
	return &decision.Service{
		Store:  decisionStore,
		Belief: beliefStore,
		Config: decision.ServiceConfig{
			AutoActivateCandidates:    knobs.AutoActivateEnabled,
			AutoActivateMinConfidence: knobs.AutoActivateMinConfidence,
			ConflictSimilarityFloor:   knobs.ConflictSimilarityFloor,
		},
	}
}

func newDecisionReactor(decisionSvc *decision.Service, beliefStore belief.Store, knobs Knobs) *decision.Reactor {
	return &decision.Reactor{
		Decisions: decisionSvc,
		Beliefs:   beliefStore,
		Config: decision.ReactorConfig{
			ConfidenceFloor:     knobs.ReactorConfidenceFloor,
			ConfidenceDropDelta: knobs.ReactorConfidenceDropDelta,
		},
	}
}

func newArchaeologyService(decisionSvc *decision.Service, beliefStore belief.Store, knobs Knobs) *archaeology.Service {
	return &archaeology.Service{
		Decisions: decisionSvc,
		Beliefs:   beliefStore,
		Config:    archaeology.ServiceConfig{ConfidenceFloor: knobs.ReactorConfidenceFloor},
	}
}

func (e *Env) newRuntime(policies []policy.Record) *memory.Runtime {
	runtime := &memory.Runtime{
		Config: memory.RuntimeConfig{
			Enabled:            true,
			MaxTokensPerPrompt: e.Knobs.MaxTokensPerPrompt,
			Timeout:            e.Knobs.Timeout(),
			IncludeRecent:      e.Knobs.IncludeRecent,
		},
		Evolving:                  e.Evolving,
		Belief:                    belief.NewStoreRetriever(e.BeliefStore),
		Magma:                     magmaRuntimeRetriever{service: e.Magma},
		Decision:                  decision.NewStoreRetriever(e.DecisionStore, e.BeliefStore),
		BeliefMaxBeliefs:          e.Knobs.BeliefMaxPerPrompt,
		BeliefPromptTokenBudget:   e.Knobs.BeliefMaxTokensPerPrompt,
		BeliefMinConfidence:       e.Knobs.BeliefMinConfidence,
		BeliefContradictions:      e.Knobs.BeliefIncludeContradictions,
		MagmaIntentClassification: e.Knobs.MagmaIntentClassification,
		MagmaContextFormat:        e.Knobs.MagmaContextFormat,
		MagmaMaxHops:              e.Knobs.MagmaDefaultHops,
		MagmaMaxNodes:             e.Knobs.MagmaDefaultMaxNodes,
		DecisionMaxPerPrompt:      e.Knobs.DecisionMaxPerPrompt,
		DecisionPromptTokenBudget: e.Knobs.DecisionTokenBudget,
		DecisionTimeout:           e.Knobs.DecisionTimeout(),
	}
	if len(policies) > 0 {
		runtime.PolicyProvider = staticPolicyProvider{records: policies}
	}
	return runtime
}

func (e *Env) seed(ctx context.Context, seed Seed) error {
	for _, scope := range seed.Scopes {
		scope.TenantID = seed.TenantID
		if _, err := e.BeliefStore.EnsureScope(ctx, scope); err != nil {
			return fmt.Errorf("seed scope %q: %w", scope.Path, err)
		}
	}
	for _, item := range seed.Beliefs {
		item.TenantID = seed.TenantID
		if strings.TrimSpace(item.StatementHash) == "" {
			item.StatementHash = belief.StatementHash(item.Statement)
		}
		if _, err := e.BeliefStore.UpsertBelief(ctx, item); err != nil {
			return fmt.Errorf("seed belief %q: %w", item.ID, err)
		}
	}
	for _, item := range seed.Decisions {
		item.TenantID = seed.TenantID
		created, err := e.DecisionService.CreateDecision(ctx, item)
		if err != nil {
			return fmt.Errorf("seed decision %q: %w", item.Statement, err)
		}
		e.seededDecisions = append(e.seededDecisions, created)
	}
	for _, link := range seed.Assumptions {
		target, ok := e.findSeededDecision(link.DecisionStatement)
		if !ok {
			return fmt.Errorf("assumption seed: no seeded decision matches %q", link.DecisionStatement)
		}
		if _, err := e.DecisionStore.AddAssumption(ctx, decision.AssumptionLink{
			TenantID:               seed.TenantID,
			DecisionID:             target.ID,
			BeliefID:               link.BeliefID,
			Criticality:            link.Criticality,
			BeliefStatementAtLink:  link.StatementAtLink,
			BeliefConfidenceAtLink: link.ConfidenceAtLink,
		}); err != nil {
			return fmt.Errorf("seed assumption for %q: %w", link.DecisionStatement, err)
		}
	}
	for _, ep := range seed.Episodes {
		if err := e.Evolving.EvolveEnhanced(ctx, ep.Input, ep.Output, ep.Feedback, nil, nil, ""); err != nil {
			return fmt.Errorf("seed evolving episode %q: %w", ep.Input, err)
		}
	}
	for _, ev := range seed.MagmaEvents {
		if _, err := e.Magma.Ingest(ctx, ev); err != nil {
			return fmt.Errorf("seed magma event %q: %w", ev.ID, err)
		}
	}
	if len(seed.MagmaEvents) > 0 {
		if _, err := e.Magma.DrainConsolidation(ctx, len(seed.MagmaEvents)); err != nil {
			return fmt.Errorf("drain magma consolidation: %w", err)
		}
	}
	return nil
}

func (e *Env) findSeededDecision(statementSubstr string) (decision.Decision, bool) {
	for _, item := range e.seededDecisions {
		if strings.Contains(item.Statement, statementSubstr) {
			return item, true
		}
	}
	return decision.Decision{}, false
}

func (e *Env) findDecisionByStatement(ctx context.Context, substr string) (decision.Decision, bool, error) {
	results, err := e.DecisionStore.SearchDecisions(ctx, decision.SearchQuery{
		TenantID: e.TenantID,
		Statuses: []decision.DecisionStatus{
			decision.DecisionStatusProposed,
			decision.DecisionStatusActive,
			decision.DecisionStatusStale,
			decision.DecisionStatusSuperseded,
			decision.DecisionStatusRevoked,
		},
		Limit: 200,
	})
	if err != nil {
		return decision.Decision{}, false, err
	}
	for _, result := range results {
		if strings.Contains(result.Decision.Statement, substr) {
			return result.Decision, true, nil
		}
	}
	return decision.Decision{}, false, nil
}

func (e *Env) findCandidateByStatement(ctx context.Context, substr string) (decision.Candidate, bool, error) {
	candidates, err := e.DecisionStore.ListCandidates(ctx, decision.CandidateQuery{TenantID: e.TenantID, Limit: 200})
	if err != nil {
		return decision.Candidate{}, false, err
	}
	for _, candidate := range candidates {
		if strings.Contains(candidate.Statement, substr) {
			return candidate, true, nil
		}
	}
	return decision.Candidate{}, false, nil
}

// RunScenario executes one scenario under the given base knob set (plus the
// scenario's pinned overrides) and returns deterministic check results.
func RunScenario(ctx context.Context, sc Scenario, base Knobs) ScenarioResult {
	knobs := base
	if sc.Mutate != nil {
		sc.Mutate(&knobs)
	}
	result := ScenarioResult{
		ScenarioID:  sc.ID,
		Subsystem:   sc.Subsystem,
		Description: sc.Description,
		Knobs:       append([]string(nil), sc.Knobs...),
		KnobValues:  knobs.Values(),
	}
	env, err := NewEnv(ctx, sc, knobs)
	if err != nil {
		result.Error = err.Error()
		result.Failed = 1
		return result
	}
	for _, step := range sc.Steps {
		result.Checks = append(result.Checks, env.runStep(ctx, step)...)
	}
	for _, check := range result.Checks {
		if check.Pass {
			result.Passed++
		} else {
			result.Failed++
		}
	}
	return result
}

// RunSuite executes every scenario under one base knob set.
func RunSuite(ctx context.Context, name string, scenarios []Scenario, base Knobs) SuiteResult {
	suite := SuiteResult{Name: name, KnobValues: base.Values()}
	for _, sc := range scenarios {
		result := RunScenario(ctx, sc, base)
		suite.Scenarios = append(suite.Scenarios, result)
		suite.Passed += result.Passed
		suite.Failed += result.Failed
		if result.Error != "" && result.Passed+result.Failed == 0 {
			suite.Failed++
		}
	}
	return suite
}

func (e *Env) runStep(ctx context.Context, step Step) []Check {
	switch {
	case step.Prompt != nil:
		return e.runPromptProbe(ctx, step.Name, *step.Prompt)
	case step.EvolvingQuery != nil:
		return e.runEvolvingSearchProbe(ctx, step.Name, *step.EvolvingQuery)
	case step.EvolvingState != nil:
		return e.runEvolvingStateProbe(step.Name, *step.EvolvingState)
	case step.Distill != nil:
		return e.runDistillProbe(ctx, step.Name, *step.Distill)
	case step.BeliefChange != nil:
		return e.runBeliefChangeProbe(ctx, step.Name, *step.BeliefChange)
	case step.Reconstruct != nil:
		return e.runReconstructProbe(ctx, step.Name, *step.Reconstruct)
	default:
		return []Check{{Name: step.Name, Pass: false, Detail: "step has no probe"}}
	}
}

func check(name string, pass bool, detail string) Check {
	c := Check{Name: name, Pass: pass}
	if !pass {
		c.Detail = detail
	}
	return c
}

func (e *Env) runPromptProbe(ctx context.Context, name string, probe PromptProbe) []Check {
	block, diag, err := e.Runtime.PrepareContext(ctx, probe.Request)
	if err != nil {
		return []Check{{Name: name, Pass: false, Detail: "PrepareContext: " + err.Error()}}
	}
	checks := make([]Check, 0, 8)
	for _, want := range probe.Expect.MustContain {
		checks = append(checks, check(
			fmt.Sprintf("%s/contains[%s]", name, snippet(want)),
			strings.Contains(block.Text, want),
			fmt.Sprintf("missing %q in context:\n%s", want, block.Text),
		))
	}
	for _, banned := range probe.Expect.MustNotContain {
		checks = append(checks, check(
			fmt.Sprintf("%s/excludes[%s]", name, snippet(banned)),
			!strings.Contains(block.Text, banned),
			fmt.Sprintf("unexpected %q in context:\n%s", banned, block.Text),
		))
	}
	if len(probe.Expect.Ordered) > 0 {
		pass, detail := orderedInText(block.Text, probe.Expect.Ordered)
		checks = append(checks, check(name+"/ordered", pass, detail))
	}
	for lane, want := range probe.Expect.LaneReturned {
		got := diag.Lanes[lane].Returned
		checks = append(checks, check(
			fmt.Sprintf("%s/lane[%s].returned=%v", name, lane, want),
			got == want,
			fmt.Sprintf("lane %s returned=%v want %v (diag=%+v)", lane, got, want, diag.Lanes[lane]),
		))
	}
	for lane, want := range probe.Expect.LaneItems {
		got := diag.Lanes[lane].Items
		checks = append(checks, check(
			fmt.Sprintf("%s/lane[%s].items=%d", name, lane, want),
			got == want,
			fmt.Sprintf("lane %s items=%d want %d (diag=%+v)", lane, got, want, diag.Lanes[lane]),
		))
	}
	for lane, bound := range probe.Expect.LaneItemsMax {
		got := diag.Lanes[lane].Items
		checks = append(checks, check(
			fmt.Sprintf("%s/lane[%s].items<=%d", name, lane, bound),
			got <= bound,
			fmt.Sprintf("lane %s items=%d want <=%d", lane, got, bound),
		))
	}
	if probe.Expect.Truncated != nil {
		checks = append(checks, check(
			fmt.Sprintf("%s/truncated=%v", name, *probe.Expect.Truncated),
			block.Truncated == *probe.Expect.Truncated,
			fmt.Sprintf("truncated=%v want %v (tokens=%d)", block.Truncated, *probe.Expect.Truncated, block.TokenEstimate),
		))
	}
	if probe.Expect.MaxTokenEstimate > 0 {
		checks = append(checks, check(
			fmt.Sprintf("%s/tokens<=%d", name, probe.Expect.MaxTokenEstimate),
			block.TokenEstimate <= probe.Expect.MaxTokenEstimate,
			fmt.Sprintf("token estimate %d exceeds %d", block.TokenEstimate, probe.Expect.MaxTokenEstimate),
		))
	}
	return checks
}

func (e *Env) runEvolvingSearchProbe(ctx context.Context, name string, probe EvolvingSearchProbe) []Check {
	results, diag, err := e.Evolving.SearchWithDiagnostics(ctx, probe.Query)
	if err != nil {
		return []Check{{Name: name, Pass: false, Detail: "SearchWithDiagnostics: " + err.Error()}}
	}
	checks := make([]Check, 0, 6)
	if probe.ExpectResultCount != nil {
		checks = append(checks, check(
			fmt.Sprintf("%s/results=%d", name, *probe.ExpectResultCount),
			len(results) == *probe.ExpectResultCount,
			fmt.Sprintf("got %d results, want %d (mode=%s)", len(results), *probe.ExpectResultCount, diag.Mode),
		))
	}
	if probe.ExpectMode != "" {
		checks = append(checks, check(
			fmt.Sprintf("%s/mode=%s", name, probe.ExpectMode),
			diag.Mode == probe.ExpectMode,
			fmt.Sprintf("mode=%s want %s (diag=%+v)", diag.Mode, probe.ExpectMode, diag),
		))
	}
	if probe.ExpectVectorFilteredMin > 0 {
		checks = append(checks, check(
			fmt.Sprintf("%s/vectorFiltered>=%d", name, probe.ExpectVectorFilteredMin),
			diag.VectorFiltered >= probe.ExpectVectorFilteredMin,
			fmt.Sprintf("vectorFiltered=%d want >=%d", diag.VectorFiltered, probe.ExpectVectorFilteredMin),
		))
	}
	inputs := make([]string, 0, len(results))
	for _, item := range results {
		if item.Entry != nil {
			inputs = append(inputs, item.Entry.Input)
		}
	}
	for _, want := range probe.ExpectInputsContain {
		checks = append(checks, check(
			fmt.Sprintf("%s/retrieved[%s]", name, snippet(want)),
			containsSubstring(inputs, want),
			fmt.Sprintf("no retrieved entry input contains %q (inputs=%v)", want, inputs),
		))
	}
	for _, banned := range probe.ExpectInputsAbsent {
		checks = append(checks, check(
			fmt.Sprintf("%s/not-retrieved[%s]", name, snippet(banned)),
			!containsSubstring(inputs, banned),
			fmt.Sprintf("retrieved entry unexpectedly contains %q (inputs=%v)", banned, inputs),
		))
	}
	return checks
}

func (e *Env) runEvolvingStateProbe(name string, probe EvolvingStateProbe) []Check {
	entries := e.Evolving.ExportMemories()
	inputs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			inputs = append(inputs, entry.Input)
		}
	}
	checks := make([]Check, 0, 4)
	if probe.ExpectEntryCount != nil {
		checks = append(checks, check(
			fmt.Sprintf("%s/entries=%d", name, *probe.ExpectEntryCount),
			len(entries) == *probe.ExpectEntryCount,
			fmt.Sprintf("got %d stored entries, want %d (inputs=%v)", len(entries), *probe.ExpectEntryCount, inputs),
		))
	}
	for _, want := range probe.ExpectInputsPresent {
		checks = append(checks, check(
			fmt.Sprintf("%s/stored[%s]", name, snippet(want)),
			containsSubstring(inputs, want),
			fmt.Sprintf("no stored entry input contains %q (inputs=%v)", want, inputs),
		))
	}
	for _, banned := range probe.ExpectInputsAbsent {
		checks = append(checks, check(
			fmt.Sprintf("%s/not-stored[%s]", name, snippet(banned)),
			!containsSubstring(inputs, banned),
			fmt.Sprintf("stored entry unexpectedly contains %q (inputs=%v)", banned, inputs),
		))
	}
	return checks
}

func (e *Env) runDistillProbe(ctx context.Context, name string, probe DistillProbe) []Check {
	input := probe.Input
	input.Episode.TenantID = e.TenantID
	candidates, err := decision.SimpleDistiller{}.Distill(ctx, input)
	if err != nil {
		return []Check{{Name: name, Pass: false, Detail: "Distill: " + err.Error()}}
	}
	checks := []Check{check(
		fmt.Sprintf("%s/candidates=%d", name, probe.ExpectCandidateCount),
		len(candidates) == probe.ExpectCandidateCount,
		fmt.Sprintf("got %d candidates, want %d (%+v)", len(candidates), probe.ExpectCandidateCount, candidates),
	)}
	recorded := make([]decision.Candidate, 0, len(candidates))
	if probe.RecordCandidates {
		for _, candidate := range candidates {
			stored, err := e.DecisionStore.RecordCandidate(ctx, candidate)
			if err != nil {
				checks = append(checks, Check{Name: name + "/record", Pass: false, Detail: err.Error()})
				return checks
			}
			recorded = append(recorded, stored)
		}
	}
	if probe.AutoAccept {
		outcomes, err := e.DecisionService.AutoAcceptCandidates(ctx, e.TenantID, recorded)
		if err != nil {
			checks = append(checks, Check{Name: name + "/auto-accept", Pass: false, Detail: err.Error()})
			return checks
		}
		accepted := 0
		reasons := make([]string, 0, len(outcomes))
		for _, outcome := range outcomes {
			if outcome.Accepted {
				accepted++
			}
			reasons = append(reasons, outcome.Reason)
		}
		joined := strings.Join(reasons, " | ")
		checks = append(checks, check(
			fmt.Sprintf("%s/accepted=%d", name, probe.ExpectAcceptedCount),
			accepted == probe.ExpectAcceptedCount,
			fmt.Sprintf("accepted=%d want %d (reasons=%s)", accepted, probe.ExpectAcceptedCount, joined),
		))
		for _, want := range probe.ExpectReasonContains {
			checks = append(checks, check(
				fmt.Sprintf("%s/reason[%s]", name, snippet(want)),
				strings.Contains(joined, want),
				fmt.Sprintf("outcome reasons %q missing %q", joined, want),
			))
		}
	}
	checks = append(checks, e.checkDecisionStates(ctx, name, probe.ExpectDecisionStatus, probe.ExpectDecisionReviewState)...)
	for substr, want := range sortedMap(probe.ExpectCandidateValidation) {
		candidate, ok, err := e.findCandidateByStatement(ctx, substr)
		if err != nil {
			checks = append(checks, Check{Name: name + "/candidate-lookup", Pass: false, Detail: err.Error()})
			continue
		}
		if !ok {
			checks = append(checks, Check{Name: fmt.Sprintf("%s/candidate[%s]", name, snippet(substr)), Pass: false, Detail: "candidate not found"})
			continue
		}
		got := decision.NormalizeCandidateValidationStatus(candidate.ValidationStatus)
		checks = append(checks, check(
			fmt.Sprintf("%s/candidate[%s].validation=%s", name, snippet(substr), want),
			got == want,
			fmt.Sprintf("validation=%s want %s", got, want),
		))
	}
	return checks
}

func (e *Env) runBeliefChangeProbe(ctx context.Context, name string, probe BeliefChangeProbe) []Check {
	event := probe.Event
	event.TenantID = e.TenantID
	event.Before.TenantID = e.TenantID
	event.After.TenantID = e.TenantID
	if probe.UpsertAfter {
		if _, err := e.BeliefStore.UpsertBelief(ctx, event.After); err != nil {
			return []Check{{Name: name + "/upsert-after", Pass: false, Detail: err.Error()}}
		}
	}
	e.Reactor.OnBeliefChanged(ctx, event)
	return e.checkDecisionStates(ctx, name, probe.ExpectDecisionStatus, probe.ExpectDecisionReviewState)
}

func (e *Env) runReconstructProbe(ctx context.Context, name string, probe ReconstructProbe) []Check {
	report, err := e.Archaeology.Reconstruct(ctx, e.TenantID, probe.Query, archaeology.ReconstructOptions{
		ScopeIDs:     probe.ScopeIDs,
		IncludeStale: probe.IncludeStale,
		MaxDecisions: 10,
	})
	if err != nil {
		return []Check{{Name: name, Pass: false, Detail: "Reconstruct: " + err.Error()}}
	}
	checks := make([]Check, 0, len(probe.ExpectVerdicts))
	for substr, want := range sortedMap(probe.ExpectVerdicts) {
		found := false
		got := ""
		for _, dossier := range report.Decisions {
			if strings.Contains(dossier.Decision.Statement, substr) {
				found = true
				got = dossier.Verdict
				break
			}
		}
		checks = append(checks, check(
			fmt.Sprintf("%s/verdict[%s]=%s", name, snippet(substr), want),
			found && got == want,
			fmt.Sprintf("found=%v verdict=%q want %q (decisions=%d)", found, got, want, len(report.Decisions)),
		))
	}
	return checks
}

func (e *Env) checkDecisionStates(ctx context.Context, name string, statuses map[string]decision.DecisionStatus, reviews map[string]decision.ReviewState) []Check {
	checks := make([]Check, 0, len(statuses)+len(reviews))
	for substr, want := range sortedMap(statuses) {
		item, ok, err := e.findDecisionByStatement(ctx, substr)
		switch {
		case err != nil:
			checks = append(checks, Check{Name: name + "/decision-lookup", Pass: false, Detail: err.Error()})
		case !ok:
			checks = append(checks, Check{Name: fmt.Sprintf("%s/decision[%s]", name, snippet(substr)), Pass: false, Detail: "decision not found"})
		default:
			got := decision.NormalizeDecisionStatus(item.Status)
			checks = append(checks, check(
				fmt.Sprintf("%s/decision[%s].status=%s", name, snippet(substr), want),
				got == want,
				fmt.Sprintf("status=%s want %s", got, want),
			))
		}
	}
	for substr, want := range sortedMap(reviews) {
		item, ok, err := e.findDecisionByStatement(ctx, substr)
		switch {
		case err != nil:
			checks = append(checks, Check{Name: name + "/decision-lookup", Pass: false, Detail: err.Error()})
		case !ok:
			checks = append(checks, Check{Name: fmt.Sprintf("%s/decision[%s]", name, snippet(substr)), Pass: false, Detail: "decision not found"})
		default:
			got := decision.NormalizeReviewState(item.ReviewState)
			checks = append(checks, check(
				fmt.Sprintf("%s/decision[%s].review=%s", name, snippet(substr), want),
				got == want,
				fmt.Sprintf("reviewState=%s want %s", got, want),
			))
		}
	}
	return checks
}

func orderedInText(text string, needles []string) (bool, string) {
	last := -1
	for _, needle := range needles {
		idx := strings.Index(text, needle)
		if idx < 0 {
			return false, fmt.Sprintf("missing %q in context:\n%s", needle, text)
		}
		if idx <= last {
			return false, fmt.Sprintf("%q appears out of order in context:\n%s", needle, text)
		}
		last = idx
	}
	return true, ""
}

func containsSubstring(haystacks []string, needle string) bool {
	for _, h := range haystacks {
		if strings.Contains(h, needle) {
			return true
		}
	}
	return false
}

func snippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 32 {
		return s[:32] + "…"
	}
	return s
}

// sortedMap yields map entries in deterministic key order so check output is
// stable across runs.
func sortedMap[V any](in map[string]V) func(func(string, V) bool) {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return func(yield func(string, V) bool) {
		for _, key := range keys {
			if !yield(key, in[key]) {
				return
			}
		}
	}
}
