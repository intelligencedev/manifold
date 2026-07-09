package gym

import (
	"time"

	"manifold/internal/agent/memory"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/agent/memory/magma"
	"manifold/internal/policy"
)

const gymTenant = int64(7)

func intp(v int) *int    { return &v }
func boolp(v bool) *bool { return &v }

// gymRequest is the canonical probe request: the runtime treats UserID as the
// tenant for every lane, so it must match the seeded tenant.
func gymRequest(input string) memory.Request {
	return memory.Request{
		UserInput:   input,
		UserID:      gymTenant,
		ProjectID:   "proj-a",
		ObjectiveID: "obj-1",
		SessionID:   "sess-1",
		Role:        "orchestrator",
	}
}

func projectScopeSeed() []belief.Scope {
	return []belief.Scope{{ID: "scope-proj-a", Kind: belief.ScopeKindProject, Path: "proj-a", Label: "proj-a"}}
}

// Suite returns every Tier 1 memory-gym scenario. Each scenario is
// deterministic and tagged with the configuration knobs it exercises.
func Suite() []Scenario {
	out := []Scenario{}
	out = append(out, decisionLaneScenarios()...)
	out = append(out, beliefScenarios()...)
	out = append(out, evolvingScenarios()...)
	out = append(out, magmaScenarios()...)
	out = append(out, archaeologyScenarios()...)
	out = append(out, runtimeFusionScenarios()...)
	return out
}

func decisionLaneScenarios() []Scenario {
	objectiveScope := belief.Scope{ID: "objective-scope-uuid", Kind: belief.ScopeKindObjective, Path: "proj-a/obj-1", Label: "obj-1"}
	out := decisionScopeScenarios(objectiveScope)
	return append(out, decisionBudgetScenarios()...)
}

func decisionScopeScenarios(objectiveScope belief.Scope) []Scenario {
	activeDecisions := func() []decision.Decision {
		return []decision.Decision{
			{
				ScopeID:    "objective-scope-uuid",
				Statement:  "We decided to keep the migration runner deterministic.",
				Status:     decision.DecisionStatusActive,
				Confidence: 0.7,
			},
			{
				ScopeID:    "project/proj-a/memory",
				Statement:  "We decided to gate auto-activation by confidence.",
				Status:     decision.DecisionStatusActive,
				Confidence: 0.7,
			},
		}
	}
	return []Scenario{
		{
			ID:          "decision-scope-proximity",
			Subsystem:   SubsystemDecision,
			Description: "Deterministic scope walk ranks objective-scoped decisions above project-scoped ones.",
			Knobs:       []string{KnobDecisionMaxPerPrompt, KnobDecisionTokenBudget},
			Seed: Seed{
				TenantID:  gymTenant,
				Scopes:    append(projectScopeSeed(), objectiveScope),
				Decisions: activeDecisions(),
			},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("migration runner determinism"),
					Expect: PromptExpect{
						MustContain:  []string{"## Recorded Decisions"},
						Ordered:      []string{"migration runner deterministic", "gate auto-activation by confidence"},
						LaneReturned: map[string]bool{"decision": true},
						LaneItems:    map[string]int{"decision": 2},
					},
				},
			}},
		},
		{
			ID:          "decision-lifecycle-exclusion",
			Subsystem:   SubsystemDecision,
			Description: "Stale, superseded, and revoked decisions never enter the prompt lane.",
			Knobs:       []string{KnobDecisionMaxPerPrompt},
			Seed: Seed{
				TenantID: gymTenant,
				Scopes:   projectScopeSeed(),
				Decisions: []decision.Decision{
					{ScopeID: "project/proj-a/memory", Statement: "We decided to keep the active rollout plan.", Status: decision.DecisionStatusActive, Confidence: 0.8},
					{ScopeID: "project/proj-a/memory", Statement: "We decided to use the legacy rollout plan.", Status: decision.DecisionStatusStale, Confidence: 0.9},
					{ScopeID: "project/proj-a/memory", Statement: "We decided to ship the abandoned rollout plan.", Status: decision.DecisionStatusRevoked, Confidence: 0.9},
					{ScopeID: "project/proj-a/memory", Statement: "We decided to follow the replaced rollout plan.", Status: decision.DecisionStatusSuperseded, Confidence: 0.9},
				},
			},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("rollout plan"),
					Expect: PromptExpect{
						MustContain:    []string{"active rollout plan"},
						MustNotContain: []string{"legacy rollout plan", "abandoned rollout plan", "replaced rollout plan"},
						LaneItems:      map[string]int{"decision": 1},
					},
				},
			}},
		},
	}
}

func decisionBudgetScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "decision-budget-default",
			Subsystem:   SubsystemDecision,
			Description: "Default lane budget admits all three active in-scope decisions.",
			Knobs:       []string{KnobDecisionMaxPerPrompt, KnobDecisionTokenBudget},
			Seed: Seed{
				TenantID: gymTenant,
				Scopes:   projectScopeSeed(),
				Decisions: []decision.Decision{
					{ScopeID: "project/proj-a/memory", Statement: "We decided to pin the gym budget rule number one.", Status: decision.DecisionStatusActive, Confidence: 0.8},
					{ScopeID: "project/proj-a/memory", Statement: "We decided to pin the gym budget rule number two.", Status: decision.DecisionStatusActive, Confidence: 0.7},
					{ScopeID: "project/proj-a/memory", Statement: "We decided to pin the gym budget rule number three.", Status: decision.DecisionStatusActive, Confidence: 0.6},
				},
			},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("gym budget rule"),
					Expect:  PromptExpect{LaneItems: map[string]int{"decision": 3}},
				},
			}},
		},
		{
			ID:          "decision-budget-capped",
			Subsystem:   SubsystemDecision,
			Description: "max_decisions_per_prompt=1 caps the rendered lane to a single decision.",
			Knobs:       []string{KnobDecisionMaxPerPrompt},
			Mutate:      func(k *Knobs) { k.DecisionMaxPerPrompt = 1 },
			Seed: Seed{
				TenantID: gymTenant,
				Scopes:   projectScopeSeed(),
				Decisions: []decision.Decision{
					{ScopeID: "project/proj-a/memory", Statement: "We decided to pin the gym budget rule number one.", Status: decision.DecisionStatusActive, Confidence: 0.8},
					{ScopeID: "project/proj-a/memory", Statement: "We decided to pin the gym budget rule number two.", Status: decision.DecisionStatusActive, Confidence: 0.7},
					{ScopeID: "project/proj-a/memory", Statement: "We decided to pin the gym budget rule number three.", Status: decision.DecisionStatusActive, Confidence: 0.6},
				},
			},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("gym budget rule"),
					Expect:  PromptExpect{LaneItems: map[string]int{"decision": 1}},
				},
			}},
		},
	}
}

func beliefScenarios() []Scenario {
	out := beliefConfidenceScenarios()
	out = append(out, beliefContradictionScenarios()...)
	out = append(out, beliefLimitScenarios()...)
	out = append(out, beliefScopeScenarios()...)
	return out
}

func confidenceBeliefs() []belief.Belief {
	return []belief.Belief{
		{ID: "belief-pg", ScopeID: "scope-proj-a", Statement: "The project database is Postgres 16 for primary storage.", Confidence: 0.9, EvidenceFor: 2, Status: belief.BeliefStatusActive},
		{ID: "belief-mysql", ScopeID: "scope-proj-a", Statement: "The project database may move to MySQL next quarter.", Confidence: 0.30, EvidenceFor: 1, Status: belief.BeliefStatusActive},
	}
}

func contradictionBeliefs() []belief.Belief {
	return []belief.Belief{
		{ID: "belief-staging", ScopeID: "scope-proj-a", Statement: "The staging cluster is reachable from the deploy pipeline.", Confidence: 0.6, EvidenceFor: 1, EvidenceAgainst: 2, Status: belief.BeliefStatusActive},
	}
}

func releaseBeliefs() []belief.Belief {
	return []belief.Belief{
		{ID: "belief-r1", ScopeID: "scope-proj-a", Statement: "The release train departs every Tuesday.", Confidence: 0.9, EvidenceFor: 3, Status: belief.BeliefStatusActive},
		{ID: "belief-r2", ScopeID: "scope-proj-a", Statement: "The release checklist requires a rollback plan.", Confidence: 0.8, EvidenceFor: 2, Status: belief.BeliefStatusActive},
		{ID: "belief-r3", ScopeID: "scope-proj-a", Statement: "The release notes are generated from merged pull requests.", Confidence: 0.7, EvidenceFor: 2, Status: belief.BeliefStatusActive},
	}
}

func beliefConfidenceScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "belief-confidence-floor-default",
			Subsystem:   SubsystemBelief,
			Description: "retrieval.minConfidence=0.35 keeps a 0.30-confidence belief out of the prompt.",
			Knobs:       []string{KnobBeliefMinConfidence},
			Seed:        Seed{TenantID: gymTenant, Scopes: projectScopeSeed(), Beliefs: confidenceBeliefs()},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("database"),
					Expect: PromptExpect{
						MustContain:    []string{"Shared Belief Memory", "Postgres 16"},
						MustNotContain: []string{"MySQL"},
						LaneItems:      map[string]int{"belief": 1},
					},
				},
			}},
		},
		{
			ID:          "belief-confidence-floor-lowered",
			Subsystem:   SubsystemBelief,
			Description: "Lowering retrieval.minConfidence to 0.2 admits the speculative belief.",
			Knobs:       []string{KnobBeliefMinConfidence},
			Mutate:      func(k *Knobs) { k.BeliefMinConfidence = 0.2 },
			Seed:        Seed{TenantID: gymTenant, Scopes: projectScopeSeed(), Beliefs: confidenceBeliefs()},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("database"),
					Expect: PromptExpect{
						MustContain: []string{"Postgres 16", "MySQL"},
						LaneItems:   map[string]int{"belief": 2},
					},
				},
			}},
		},
	}
}

func beliefContradictionScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "belief-contradictions-included",
			Subsystem:   SubsystemBelief,
			Description: "includeContradictions=true surfaces contested beliefs under the contradictions header.",
			Knobs:       []string{KnobBeliefIncludeContradictions},
			Seed:        Seed{TenantID: gymTenant, Scopes: projectScopeSeed(), Beliefs: contradictionBeliefs()},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("staging cluster"),
					Expect: PromptExpect{
						MustContain:  []string{"Contradictions and Superseded Context", "staging cluster"},
						LaneReturned: map[string]bool{"belief": true},
					},
				},
			}},
		},
		{
			ID:          "belief-contradictions-excluded",
			Subsystem:   SubsystemBelief,
			Description: "includeContradictions=false removes contested beliefs entirely.",
			Knobs:       []string{KnobBeliefIncludeContradictions},
			Mutate:      func(k *Knobs) { k.BeliefIncludeContradictions = false },
			Seed:        Seed{TenantID: gymTenant, Scopes: projectScopeSeed(), Beliefs: contradictionBeliefs()},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("staging cluster"),
					Expect: PromptExpect{
						MustNotContain: []string{"staging cluster"},
						LaneReturned:   map[string]bool{"belief": false},
					},
				},
			}},
		},
	}
}

func beliefLimitScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "belief-max-per-prompt-default",
			Subsystem:   SubsystemBelief,
			Description: "Default maxBeliefsPerPrompt=5 admits all three release beliefs.",
			Knobs:       []string{KnobBeliefMaxPerPrompt, KnobBeliefMaxTokensPerPrompt},
			Seed:        Seed{TenantID: gymTenant, Scopes: projectScopeSeed(), Beliefs: releaseBeliefs()},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("release"),
					Expect:  PromptExpect{LaneItems: map[string]int{"belief": 3}},
				},
			}},
		},
		{
			ID:          "belief-max-per-prompt-capped",
			Subsystem:   SubsystemBelief,
			Description: "maxBeliefsPerPrompt=1 caps the lane to the highest-ranked belief.",
			Knobs:       []string{KnobBeliefMaxPerPrompt},
			Mutate:      func(k *Knobs) { k.BeliefMaxPerPrompt = 1 },
			Seed:        Seed{TenantID: gymTenant, Scopes: projectScopeSeed(), Beliefs: releaseBeliefs()},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("release"),
					Expect:  PromptExpect{LaneItems: map[string]int{"belief": 1}},
				},
			}},
		},
	}
}

func beliefScopeScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "belief-scope-proximity",
			Subsystem:   SubsystemBelief,
			Description: "Project-scoped beliefs rank above org-scoped beliefs for the same query.",
			Knobs:       []string{KnobBeliefMaxPerPrompt},
			Seed: Seed{
				TenantID: gymTenant,
				Scopes: append(projectScopeSeed(),
					belief.Scope{ID: "scope-org-7", Kind: belief.ScopeKindOrg, Path: "7", Label: "org"}),
				Beliefs: []belief.Belief{
					{ID: "belief-retry-org", ScopeID: "scope-org-7", Statement: "The retry budget defaults to three attempts.", Confidence: 0.9, EvidenceFor: 2, Status: belief.BeliefStatusActive},
					{ID: "belief-retry-proj", ScopeID: "scope-proj-a", Statement: "The retry budget is five attempts for this project.", Confidence: 0.9, EvidenceFor: 2, Status: belief.BeliefStatusActive},
				},
			},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("retry budget"),
					Expect: PromptExpect{
						Ordered:   []string{"five attempts", "three attempts"},
						LaneItems: map[string]int{"belief": 2},
					},
				},
			}},
		},
	}
}

func evolvingScenarios() []Scenario {
	out := evolvingTopKScenarios()
	out = append(out, evolvingSimilarityScenarios()...)
	out = append(out, evolvingPruneScenarios()...)
	out = append(out, evolvingLifecycleScenarios()...)
	return out
}

func canaryEpisodes() []EvolvingEpisode {
	return []EvolvingEpisode{
		{Input: "canary deploy stage one traffic shift", Output: "shift five percent first", Feedback: "success"},
		{Input: "canary deploy stage two health gates", Output: "watch error budget", Feedback: "success"},
		{Input: "canary deploy stage three rollback drill", Output: "rollback in one command", Feedback: "failure"},
		{Input: "canary deploy stage four metrics review", Output: "compare p99 latency", Feedback: "success"},
		{Input: "canary deploy stage five sign off", Output: "record the sign off", Feedback: "success"},
		{Input: "canary deploy stage six postmortem notes", Output: "file lessons learned", Feedback: "partial"},
	}
}

func evolvingTopKScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "evolving-topk-default",
			Subsystem:   SubsystemEvolving,
			Description: "topK=4 bounds retrieval to four scored experiences.",
			Knobs:       []string{KnobEvolvingTopK, KnobEvolvingEnableRAG},
			Seed:        Seed{TenantID: gymTenant, Episodes: canaryEpisodes()},
			Steps: []Step{{
				Name: "search",
				EvolvingQuery: &EvolvingSearchProbe{
					Query:             "canary deploy rollback",
					ExpectResultCount: intp(4),
				},
			}},
		},
		{
			ID:          "evolving-topk-capped",
			Subsystem:   SubsystemEvolving,
			Description: "topK=2 tightens retrieval to two experiences.",
			Knobs:       []string{KnobEvolvingTopK},
			Mutate:      func(k *Knobs) { k.EvolvingTopK = 2 },
			Seed:        Seed{TenantID: gymTenant, Episodes: canaryEpisodes()},
			Steps: []Step{{
				Name: "search",
				EvolvingQuery: &EvolvingSearchProbe{
					Query:             "canary deploy rollback",
					ExpectResultCount: intp(2),
				},
			}},
		},
	}
}

func evolvingSimilarityScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "evolving-similarity-threshold-off",
			Subsystem:   SubsystemEvolving,
			Description: "retrievalSimilarityThreshold=0 keeps dense retrieval active (hybrid mode).",
			Knobs:       []string{KnobEvolvingSimilarityThreshold},
			Seed: Seed{TenantID: gymTenant, Episodes: []EvolvingEpisode{
				{Input: "canary deploy rollback rehearsal", Output: "rollback ready", Feedback: "success"},
				{Input: "canary deploy traffic shaping", Output: "ramp slowly", Feedback: "success"},
				{Input: "unrelated paperwork filing", Output: "filed forms", Feedback: "success"},
			}},
			Steps: []Step{{
				Name: "search",
				EvolvingQuery: &EvolvingSearchProbe{
					Query:               "canary deploy rollback",
					ExpectMode:          "hybrid",
					ExpectInputsContain: []string{"rollback rehearsal"},
				},
			}},
		},
		{
			ID:          "evolving-similarity-threshold-strict",
			Subsystem:   SubsystemEvolving,
			Description: "retrievalSimilarityThreshold=0.95 filters every dense candidate; no keyword-only degradation.",
			Knobs:       []string{KnobEvolvingSimilarityThreshold},
			Mutate:      func(k *Knobs) { k.EvolvingSimilarityThreshold = 0.95 },
			Seed: Seed{TenantID: gymTenant, Episodes: []EvolvingEpisode{
				{Input: "canary deploy rollback rehearsal", Output: "rollback ready", Feedback: "success"},
				{Input: "canary deploy traffic shaping", Output: "ramp slowly", Feedback: "success"},
				{Input: "unrelated paperwork filing", Output: "filed forms", Feedback: "success"},
			}},
			Steps: []Step{{
				Name: "search",
				EvolvingQuery: &EvolvingSearchProbe{
					Query:                   "canary deploy rollback",
					ExpectMode:              "vector_below_threshold",
					ExpectVectorFilteredMin: 1,
					ExpectInputsContain:     []string{},
					ExpectInputsAbsent:      []string{"rollback", "traffic", "paperwork"},
				},
			}},
		},
	}
}

func evolvingPruneScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "evolving-smart-prune-merges-duplicates",
			Subsystem:   SubsystemEvolving,
			Description: "enableSmartPrune=true with pruneThreshold=0.97 merges an exact duplicate episode.",
			Knobs:       []string{KnobEvolvingEnableSmartPrune, KnobEvolvingPruneThreshold},
			Seed: Seed{TenantID: gymTenant, Episodes: []EvolvingEpisode{
				{Input: "configure sandbox network policy", Output: "deny all by default", Feedback: "success"},
				{Input: "configure sandbox network policy", Output: "deny all by default", Feedback: "success"},
				{Input: "tune embedding cache eviction", Output: "use LRU with TTL", Feedback: "success"},
			}},
			Steps: []Step{{
				Name:          "state",
				EvolvingState: &EvolvingStateProbe{ExpectEntryCount: intp(2)},
			}},
		},
		{
			ID:          "evolving-smart-prune-disabled-keeps-duplicates",
			Subsystem:   SubsystemEvolving,
			Description: "enableSmartPrune=false stores the duplicate verbatim.",
			Knobs:       []string{KnobEvolvingEnableSmartPrune},
			Mutate:      func(k *Knobs) { k.EvolvingEnableSmartPrune = false },
			Seed: Seed{TenantID: gymTenant, Episodes: []EvolvingEpisode{
				{Input: "configure sandbox network policy", Output: "deny all by default", Feedback: "success"},
				{Input: "configure sandbox network policy", Output: "deny all by default", Feedback: "success"},
				{Input: "tune embedding cache eviction", Output: "use LRU with TTL", Feedback: "success"},
			}},
			Steps: []Step{{
				Name:          "state",
				EvolvingState: &EvolvingStateProbe{ExpectEntryCount: intp(3)},
			}},
		},
	}
}

func evolvingLifecycleScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "evolving-maxsize-fifo",
			Subsystem:   SubsystemEvolving,
			Description: "maxSize=3 without smart prune falls back to FIFO eviction of the oldest entries.",
			Knobs:       []string{KnobEvolvingMaxSize},
			Mutate: func(k *Knobs) {
				k.EvolvingMaxSize = 3
				k.EvolvingEnableSmartPrune = false
			},
			Seed: Seed{TenantID: gymTenant, Episodes: []EvolvingEpisode{
				{Input: "gym task alpha intake", Output: "done alpha", Feedback: "success"},
				{Input: "gym task bravo triage", Output: "done bravo", Feedback: "success"},
				{Input: "gym task charlie review", Output: "done charlie", Feedback: "success"},
				{Input: "gym task delta merge", Output: "done delta", Feedback: "success"},
				{Input: "gym task echo release", Output: "done echo", Feedback: "success"},
			}},
			Steps: []Step{{
				Name: "state",
				EvolvingState: &EvolvingStateProbe{
					ExpectEntryCount:    intp(3),
					ExpectInputsPresent: []string{"charlie review", "delta merge", "echo release"},
					ExpectInputsAbsent:  []string{"alpha intake", "bravo triage"},
				},
			}},
		},
		{
			ID:          "evolving-recent-window",
			Subsystem:   SubsystemEvolving,
			Description: "windowSize=2 bounds the recent-task lane to the two newest episodes.",
			Knobs:       []string{KnobEvolvingWindowSize, KnobMemIncludeRecent},
			Mutate:      func(k *Knobs) { k.EvolvingWindowSize = 2 },
			Seed: Seed{TenantID: gymTenant, Episodes: []EvolvingEpisode{
				{Input: "recent window task one", Output: "one done", Feedback: "success"},
				{Input: "recent window task two", Output: "two done", Feedback: "success"},
				{Input: "recent window task three", Output: "three done", Feedback: "success"},
			}},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("recent window task"),
					Expect: PromptExpect{
						MustContain: []string{"Recent Task History"},
						LaneItems:   map[string]int{"recent": 2},
					},
				},
			}},
		},
	}
}

func magmaScenarios() []Scenario {
	hikeSeed := func() []magma.IngestRequest {
		return []magma.IngestRequest{{
			ID:        "melanie-hike",
			Tenant:    "user:7",
			SessionID: "sess-1",
			Text:      "Yesterday Melanie hiked because the weather improved.",
			CreatedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		}}
	}
	return []Scenario{
		{
			ID:          "magma-structured-context",
			Subsystem:   SubsystemMagma,
			Description: "Structured context format renders graph views including the causal chain.",
			Knobs:       []string{KnobMagmaContextFormat, KnobMagmaCausalThreshold, KnobMagmaIntentClassification},
			Seed:        Seed{TenantID: gymTenant, MagmaEvents: hikeSeed()},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("Why did Melanie hike?"),
					Expect: PromptExpect{
						MustContain:  []string{"Graph Memory", "weather improved"},
						LaneReturned: map[string]bool{"magma": true},
					},
				},
			}},
		},
		{
			ID:          "magma-plain-context",
			Subsystem:   SubsystemMagma,
			Description: "contextFormat=text drops structured headings but keeps event text.",
			Knobs:       []string{KnobMagmaContextFormat},
			Mutate:      func(k *Knobs) { k.MagmaContextFormat = "text" },
			Seed:        Seed{TenantID: gymTenant, MagmaEvents: hikeSeed()},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("Why did Melanie hike?"),
					Expect: PromptExpect{
						MustContain:    []string{"Melanie hiked"},
						MustNotContain: []string{"Temporal timeline:"},
						LaneReturned:   map[string]bool{"magma": true},
					},
				},
			}},
		},
		{
			ID:          "magma-max-nodes-capped",
			Subsystem:   SubsystemMagma,
			Description: "retrieval.defaultMaxNodes=1 bounds the graph lane to a single event.",
			Knobs:       []string{KnobMagmaDefaultMaxNodes, KnobMagmaDefaultHops},
			Mutate:      func(k *Knobs) { k.MagmaDefaultMaxNodes = 1 },
			Seed: Seed{TenantID: gymTenant, MagmaEvents: []magma.IngestRequest{
				{ID: "rel-1", Tenant: "user:7", SessionID: "sess-1", Text: "The release pipeline deployed build one to staging.", CreatedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)},
				{ID: "rel-2", Tenant: "user:7", SessionID: "sess-1", Text: "The release pipeline deployed build two to production.", CreatedAt: time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)},
			}},
			Steps: []Step{{
				Name: "prompt",
				Prompt: &PromptProbe{
					Request: gymRequest("release pipeline deployments"),
					Expect: PromptExpect{
						LaneReturned: map[string]bool{"magma": true},
						LaneItemsMax: map[string]int{"magma": 1},
					},
				},
			}},
		},
	}
}

func archaeologyScenarios() []Scenario {
	distillEpisode, sqliteInput := archaeologyDistillFixture()
	out := []Scenario{
		archaeologyDistillExplicitScenario(distillEpisode, sqliteInput),
		archaeologyAutoActivateDisabledScenario(sqliteInput),
		archaeologyAutoActivateLowFloorScenario(sqliteInput),
		archaeologyAutoActivateDefaultFloorScenario(sqliteInput),
		archaeologyAutoActivateDuplicateScenario(distillEpisode),
		archaeologyAutoActivateConflictScenario(distillEpisode),
		archaeologyAutoActivateConflictRaisedScenario(distillEpisode),
	}
	return append(out, archaeologyReactorScenarios()...)
}

func archaeologyDistillFixture() (belief.Episode, decision.DistillationInput) {
	distillEpisode := belief.Episode{ID: "ep-1", ScopeID: "project/proj-a", ProjectID: "proj-a"}
	sqliteInput := decision.DistillationInput{
		Episode:     distillEpisode,
		FinalAnswer: "Decision: adopt sqlite-vec for local vector search in the gym harness.",
	}
	return distillEpisode, sqliteInput
}

func archaeologyDistillExplicitScenario(distillEpisode belief.Episode, sqliteInput decision.DistillationInput) Scenario {
	return Scenario{
		ID:          "archaeology-distill-explicit-decision",
		Subsystem:   SubsystemArchaeology,
		Description: "SimpleDistiller extracts exactly one candidate from explicit decision language and none from chatter.",
		Knobs:       []string{KnobAutoActivateEnabled},
		Seed:        Seed{TenantID: gymTenant},
		Steps: []Step{
			{
				Name: "explicit",
				Distill: &DistillProbe{
					Input:                sqliteInput,
					ExpectCandidateCount: 1,
				},
			},
			{
				Name: "chatter",
				Distill: &DistillProbe{
					Input: decision.DistillationInput{
						Episode:     distillEpisode,
						FinalAnswer: "We compared a few options and will revisit the topic next sprint.",
					},
					ExpectCandidateCount: 0,
				},
			},
		},
	}
}

func archaeologyAutoActivateDisabledScenario(sqliteInput decision.DistillationInput) Scenario {
	return Scenario{
		ID:          "archaeology-auto-activate-disabled",
		Subsystem:   SubsystemArchaeology,
		Description: "auto_activate.enabled=false leaves distilled candidates queued for deliberate review.",
		Knobs:       []string{KnobAutoActivateEnabled},
		Seed:        Seed{TenantID: gymTenant},
		Steps: []Step{{
			Name: "distill",
			Distill: &DistillProbe{
				Input:                sqliteInput,
				ExpectCandidateCount: 1,
				RecordCandidates:     true,
				AutoAccept:           true,
				ExpectAcceptedCount:  0,
				ExpectCandidateValidation: map[string]decision.CandidateValidationStatus{
					"sqlite-vec": decision.CandidateValidationQueued,
				},
			},
		}},
	}
}

func archaeologyAutoActivateLowFloorScenario(sqliteInput decision.DistillationInput) Scenario {
	return Scenario{
		ID:          "archaeology-auto-activate-low-floor",
		Subsystem:   SubsystemArchaeology,
		Description: "auto_activate enabled with min_confidence=0.50 activates a 0.55-confidence candidate as auto_active.",
		Knobs:       []string{KnobAutoActivateEnabled, KnobAutoActivateMinConfidence},
		Mutate: func(k *Knobs) {
			k.AutoActivateEnabled = true
			k.AutoActivateMinConfidence = 0.50
		},
		Seed: Seed{TenantID: gymTenant},
		Steps: []Step{{
			Name: "distill",
			Distill: &DistillProbe{
				Input:                sqliteInput,
				ExpectCandidateCount: 1,
				RecordCandidates:     true,
				AutoAccept:           true,
				ExpectAcceptedCount:  1,
				ExpectReasonContains: []string{"auto-activated"},
				ExpectDecisionStatus: map[string]decision.DecisionStatus{
					"sqlite-vec": decision.DecisionStatusActive,
				},
				ExpectDecisionReviewState: map[string]decision.ReviewState{
					"sqlite-vec": decision.ReviewStateAutoActive,
				},
				ExpectCandidateValidation: map[string]decision.CandidateValidationStatus{
					"sqlite-vec": decision.CandidateValidationAccepted,
				},
			},
		}},
	}
}

func archaeologyAutoActivateDefaultFloorScenario(sqliteInput decision.DistillationInput) Scenario {
	return Scenario{
		ID:          "archaeology-auto-activate-default-floor",
		Subsystem:   SubsystemArchaeology,
		Description: "Default min_confidence=0.85 keeps a 0.55-confidence candidate queued even when auto-activation is on.",
		Knobs:       []string{KnobAutoActivateEnabled, KnobAutoActivateMinConfidence},
		Mutate:      func(k *Knobs) { k.AutoActivateEnabled = true },
		Seed:        Seed{TenantID: gymTenant},
		Steps: []Step{{
			Name: "distill",
			Distill: &DistillProbe{
				Input:                sqliteInput,
				ExpectCandidateCount: 1,
				RecordCandidates:     true,
				AutoAccept:           true,
				ExpectAcceptedCount:  0,
				ExpectReasonContains: []string{"below auto-activation floor"},
				ExpectCandidateValidation: map[string]decision.CandidateValidationStatus{
					"sqlite-vec": decision.CandidateValidationQueued,
				},
			},
		}},
	}
}

func archaeologyAutoActivateDuplicateScenario(distillEpisode belief.Episode) Scenario {
	return Scenario{
		ID:          "archaeology-auto-activate-duplicate",
		Subsystem:   SubsystemArchaeology,
		Description: "A candidate matching an active decision's statement hash is rejected as a duplicate.",
		Knobs:       []string{KnobAutoActivateEnabled, KnobAutoActivateMinConfidence},
		Mutate:      enableLowFloorAutoActivate,
		Seed: Seed{
			TenantID: gymTenant,
			Decisions: []decision.Decision{{
				ScopeID:     "project/proj-a",
				Statement:   "We decided to keep activation deliberate in the memory gym.",
				Status:      decision.DecisionStatusActive,
				ReviewState: decision.ReviewStateOperatorApproved,
				Confidence:  0.9,
			}},
		},
		Steps: []Step{{
			Name: "distill",
			Distill: &DistillProbe{
				Input: decision.DistillationInput{
					Episode:     distillEpisode,
					FinalAnswer: "Decision: keep activation deliberate in the memory gym.",
				},
				ExpectCandidateCount: 1,
				RecordCandidates:     true,
				AutoAccept:           true,
				ExpectAcceptedCount:  0,
				ExpectReasonContains: []string{"already recorded as decision"},
			},
		}},
	}
}

func archaeologyAutoActivateConflictScenario(distillEpisode belief.Episode) Scenario {
	return Scenario{
		ID:          "archaeology-auto-activate-conflict",
		Subsystem:   SubsystemArchaeology,
		Description: "A similar-but-different candidate flags the active decision needs_review and stays queued.",
		Knobs:       []string{KnobAutoActivateEnabled, KnobConflictSimilarityFloor},
		Mutate:      enableLowFloorAutoActivate,
		Seed:        Seed{TenantID: gymTenant, Decisions: postgresEventStoreDecisions()},
		Steps:       []Step{mysqlEventStoreStep(distillEpisode, false)},
	}
}

func archaeologyAutoActivateConflictRaisedScenario(distillEpisode belief.Episode) Scenario {
	return Scenario{
		ID:          "archaeology-auto-activate-conflict-floor-raised",
		Subsystem:   SubsystemArchaeology,
		Description: "conflict_similarity_floor=0.70 treats the 0.67-similar statement as non-conflicting and activates it.",
		Knobs:       []string{KnobConflictSimilarityFloor},
		Mutate: func(k *Knobs) {
			enableLowFloorAutoActivate(k)
			k.ConflictSimilarityFloor = 0.70
		},
		Seed:  Seed{TenantID: gymTenant, Decisions: postgresEventStoreDecisions()},
		Steps: []Step{mysqlEventStoreStep(distillEpisode, true)},
	}
}

func archaeologyReactorScenarios() []Scenario {
	return []Scenario{
		archaeologyReactorRetractionScenario(),
		archaeologyReactorConfidenceScenario(
			"archaeology-reactor-confidence-drop",
			"A 0.40 confidence drop on a load-bearing assumption flags the decision needs_review.",
			nil,
			decision.ReviewStateNeedsReview,
		),
		archaeologyReactorConfidenceScenario(
			"archaeology-reactor-confidence-drop-tolerant",
			"confidence_drop_delta=0.60 tolerates the same 0.40 drop without flagging.",
			func(k *Knobs) { k.ReactorConfidenceDropDelta = 0.60 },
			decision.ReviewStateOperatorApproved,
		),
	}
}

func enableLowFloorAutoActivate(k *Knobs) {
	k.AutoActivateEnabled = true
	k.AutoActivateMinConfidence = 0.50
}

func postgresEventStoreDecisions() []decision.Decision {
	return []decision.Decision{{
		ScopeID:     "project/proj-a",
		Statement:   "We decided to use Postgres for the gym event store.",
		Status:      decision.DecisionStatusActive,
		ReviewState: decision.ReviewStateOperatorApproved,
		Confidence:  0.9,
	}}
}

func mysqlEventStoreStep(distillEpisode belief.Episode, accepted bool) Step {
	probe := &DistillProbe{
		Input: decision.DistillationInput{
			Episode:     distillEpisode,
			FinalAnswer: "Decision: use MySQL for the gym event store.",
		},
		ExpectCandidateCount: 1,
		RecordCandidates:     true,
		AutoAccept:           true,
	}
	if accepted {
		probe.ExpectAcceptedCount = 1
		probe.ExpectDecisionStatus = map[string]decision.DecisionStatus{
			"MySQL for the gym event store": decision.DecisionStatusActive,
		}
		probe.ExpectDecisionReviewState = map[string]decision.ReviewState{
			"Postgres for the gym event store": decision.ReviewStateOperatorApproved,
		}
	} else {
		probe.ExpectAcceptedCount = 0
		probe.ExpectReasonContains = []string{"conflicts with active decision"}
		probe.ExpectDecisionStatus = map[string]decision.DecisionStatus{
			"Postgres for the gym event store": decision.DecisionStatusActive,
		}
		probe.ExpectDecisionReviewState = map[string]decision.ReviewState{
			"Postgres for the gym event store": decision.ReviewStateNeedsReview,
		}
		probe.ExpectCandidateValidation = map[string]decision.CandidateValidationStatus{
			"MySQL for the gym event store": decision.CandidateValidationQueued,
		}
	}
	return Step{Name: "distill", Distill: probe}
}

func archaeologyReactorRetractionScenario() Scenario {
	beliefStatement := "The platform runs Postgres logical replication across regions."
	activeBelief := replicationBelief(beliefStatement, belief.BeliefStatusActive)
	retracted := activeBelief
	retracted.Status = belief.BeliefStatusRetracted
	return Scenario{
		ID:          "archaeology-reactor-retraction-lifecycle",
		Subsystem:   SubsystemArchaeology,
		Description: "Retracting a load-bearing belief stales the dependent decision, flags supporting dependents, and never auto-reactivates.",
		Knobs:       []string{KnobReactorConfidenceFloor, KnobReactorConfidenceDropDelta},
		Seed:        replicationRetractionSeed(beliefStatement, activeBelief),
		Steps:       replicationRetractionSteps(activeBelief, retracted),
	}
}

func replicationBelief(statement string, status belief.BeliefStatus) belief.Belief {
	return belief.Belief{
		ID:         "belief-replication",
		ScopeID:    "scope-proj-a",
		Statement:  statement,
		Confidence: 0.9,
		Status:     status,
	}
}

func replicationRetractionSeed(statement string, activeBelief belief.Belief) Seed {
	return Seed{
		TenantID: gymTenant,
		Scopes:   projectScopeSeed(),
		Beliefs:  []belief.Belief{activeBelief},
		Decisions: []decision.Decision{
			{
				ScopeID:     "project/proj-a",
				Statement:   "We decided to rely on logical replication for cross-region sync.",
				Status:      decision.DecisionStatusActive,
				ReviewState: decision.ReviewStateOperatorApproved,
				Confidence:  0.8,
			},
			{
				ScopeID:     "project/proj-a",
				Statement:   "We decided to mirror analytics data into the warehouse nightly.",
				Status:      decision.DecisionStatusActive,
				ReviewState: decision.ReviewStateOperatorApproved,
				Confidence:  0.8,
			},
		},
		Assumptions: []AssumptionSeed{
			replicationAssumption("rely on logical replication", decision.CriticalityLoadBearing, statement),
			replicationAssumption("mirror analytics data", decision.CriticalitySupporting, statement),
		},
	}
}

func replicationAssumption(decisionStatement string, criticality decision.AssumptionCriticality, statement string) AssumptionSeed {
	return AssumptionSeed{
		DecisionStatement: decisionStatement,
		BeliefID:          "belief-replication",
		Criticality:       criticality,
		StatementAtLink:   statement,
		ConfidenceAtLink:  0.9,
	}
}

func replicationRetractionSteps(activeBelief, retracted belief.Belief) []Step {
	return []Step{
		replicationRetractStep(activeBelief, retracted),
		replicationReinstateStep(activeBelief, retracted),
		replicationReconstructStep(),
	}
}

func replicationRetractStep(activeBelief, retracted belief.Belief) Step {
	return Step{
		Name: "retract",
		BeliefChange: &BeliefChangeProbe{
			Event:       replicationChangeEvent(activeBelief, retracted),
			UpsertAfter: true,
			ExpectDecisionStatus: map[string]decision.DecisionStatus{
				"rely on logical replication": decision.DecisionStatusStale,
				"mirror analytics data":       decision.DecisionStatusActive,
			},
			ExpectDecisionReviewState: map[string]decision.ReviewState{
				"rely on logical replication": decision.ReviewStateNeedsReview,
				"mirror analytics data":       decision.ReviewStateNeedsReview,
			},
		},
	}
}

func replicationReinstateStep(activeBelief, retracted belief.Belief) Step {
	return Step{
		Name: "reinstate-belief-no-auto-reactivation",
		BeliefChange: &BeliefChangeProbe{
			Event:       replicationChangeEvent(retracted, activeBelief),
			UpsertAfter: true,
			ExpectDecisionStatus: map[string]decision.DecisionStatus{
				"rely on logical replication": decision.DecisionStatusStale,
			},
		},
	}
}

func replicationChangeEvent(before, after belief.Belief) belief.ChangeEvent {
	return belief.ChangeEvent{
		BeliefID: "belief-replication",
		Kind:     belief.ChangeStatus,
		Before:   before,
		After:    after,
	}
}

func replicationReconstructStep() Step {
	return Step{
		Name: "reconstruct",
		Reconstruct: &ReconstructProbe{
			Query:        "logical replication",
			IncludeStale: true,
			ExpectVerdicts: map[string]string{
				"rely on logical replication": "stale",
			},
		},
	}
}

func archaeologyReactorConfidenceScenario(id, description string, mutate func(*Knobs), wantReview decision.ReviewState) Scenario {
	beliefStatement := "The cache hit rate stays above ninety percent in production."
	cacheBelief := belief.Belief{
		ID:         "belief-cache",
		ScopeID:    "scope-proj-a",
		Statement:  beliefStatement,
		Confidence: 0.9,
		Status:     belief.BeliefStatusActive,
	}
	dropped := cacheBelief
	dropped.Confidence = 0.5
	return Scenario{
		ID:          id,
		Subsystem:   SubsystemArchaeology,
		Description: description,
		Knobs:       []string{KnobReactorConfidenceFloor, KnobReactorConfidenceDropDelta},
		Mutate:      mutate,
		Seed: Seed{
			TenantID: gymTenant,
			Scopes:   projectScopeSeed(),
			Beliefs:  []belief.Belief{cacheBelief},
			Decisions: []decision.Decision{{
				ScopeID:     "project/proj-a",
				Statement:   "We decided to size the cache cluster for a ninety percent hit rate.",
				Status:      decision.DecisionStatusActive,
				ReviewState: decision.ReviewStateOperatorApproved,
				Confidence:  0.8,
			}},
			Assumptions: []AssumptionSeed{{
				DecisionStatement: "size the cache cluster",
				BeliefID:          "belief-cache",
				Criticality:       decision.CriticalityLoadBearing,
				StatementAtLink:   beliefStatement,
				ConfidenceAtLink:  0.9,
			}},
		},
		Steps: []Step{{
			Name: "confidence-drop",
			BeliefChange: &BeliefChangeProbe{
				Event: belief.ChangeEvent{
					BeliefID: "belief-cache",
					Kind:     belief.ChangeConfidence,
					Before:   cacheBelief,
					After:    dropped,
				},
				UpsertAfter: true,
				ExpectDecisionStatus: map[string]decision.DecisionStatus{
					"size the cache cluster": decision.DecisionStatusActive,
				},
				ExpectDecisionReviewState: map[string]decision.ReviewState{
					"size the cache cluster": wantReview,
				},
			},
		}},
	}
}

func runtimeFusionScenarios() []Scenario {
	return []Scenario{
		runtimeLaneOrderScenario(),
		runtimeTokenBudgetScenario(),
	}
}

func runtimeLaneOrderScenario() Scenario {
	return Scenario{
		ID:          "runtime-lane-order",
		Subsystem:   SubsystemRuntime,
		Description: "Fused context renders policy -> decision -> belief -> magma -> evolving -> recent, in order.",
		Knobs:       []string{KnobMemMaxTokensPerPrompt, KnobMemTimeoutMs, KnobMemIncludeRecent},
		Seed:        runtimeLaneOrderSeed(),
		Steps: []Step{{
			Name: "prompt",
			Prompt: &PromptProbe{
				Request: gymRequest("memory fusion"),
				Expect:  runtimeLaneOrderExpect(),
			},
		}},
	}
}

func runtimeLaneOrderSeed() Seed {
	return Seed{
		TenantID: gymTenant,
		Scopes:   projectScopeSeed(),
		Policies: []policy.Record{{
			ID:            "policy-1",
			Scope:         policy.ScopeProject,
			Severity:      policy.SeveritySoft,
			Statement:     "Prefer deterministic memory fusion checks.",
			ApprovalState: policy.ApprovalApproved,
		}},
		Decisions: []decision.Decision{{
			ScopeID:     "project/proj-a",
			Statement:   "We decided to fuse memory lanes in a fixed order.",
			Status:      decision.DecisionStatusActive,
			ReviewState: decision.ReviewStateOperatorApproved,
			Confidence:  0.9,
		}},
		Beliefs: []belief.Belief{{
			ID:         "belief-fusion",
			ScopeID:    "scope-proj-a",
			Statement:  "The memory fusion order is fixed by the runtime.",
			Confidence: 0.9,
			Status:     belief.BeliefStatusActive,
		}},
		MagmaEvents: []magma.IngestRequest{{
			ID:        "fusion-event",
			Tenant:    "user:7",
			SessionID: "sess-1",
			Text:      "The memory fusion order was retested because the runtime changed.",
			CreatedAt: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
		}},
		Episodes: []EvolvingEpisode{{
			Input:    "memory fusion ordering check",
			Output:   "lanes rendered in fixed order",
			Feedback: "success",
		}},
	}
}

func runtimeLaneOrderExpect() PromptExpect {
	return PromptExpect{
		Ordered: []string{
			"Runtime Policy Context",
			"Recorded Decisions",
			"Shared Belief Memory",
			"Graph Memory",
			"Past Relevant Experiences",
			"Recent Task History",
		},
		LaneReturned: map[string]bool{
			"policy":   true,
			"decision": true,
			"belief":   true,
			"magma":    true,
			"evolving": true,
			"recent":   true,
		},
		Truncated: boolp(false),
	}
}

func runtimeTokenBudgetScenario() Scenario {
	return Scenario{
		ID:          "runtime-token-budget-truncation",
		Subsystem:   SubsystemRuntime,
		Description: "maxTokensPerPrompt=10 forces fused-context truncation.",
		Knobs:       []string{KnobMemMaxTokensPerPrompt},
		Mutate:      func(k *Knobs) { k.MaxTokensPerPrompt = 10 },
		Seed: Seed{
			TenantID: gymTenant,
			Episodes: []EvolvingEpisode{{
				Input:    "truncation budget exercise with a long descriptive task input",
				Output:   "a deliberately verbose solution description that easily exceeds ten tokens of budget",
				Feedback: "success",
			}},
		},
		Steps: []Step{{
			Name: "prompt",
			Prompt: &PromptProbe{
				Request: gymRequest("truncation budget exercise"),
				Expect: PromptExpect{
					Truncated: boolp(true),
				},
			},
		}},
	}
}
