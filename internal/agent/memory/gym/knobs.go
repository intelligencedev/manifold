// Package gym is the Tier 1 offline "memory gym" for the unified memory
// runtime. It runs deterministic scenarios against in-memory stores and a
// deterministic embedder, with config knobs injected per run, so memory.*
// and archaeology.* permutations can be scored without a live daemon and
// without any LLM or network calls.
package gym

import "time"

// Knob tags name the configuration paths a scenario exercises. They mirror
// config.yaml.example so Tier 2 / Phase 3 can map gym results back onto the
// real startup configuration.
const (
	KnobMemMaxTokensPerPrompt = "memory.retrieval.maxTokensPerPrompt"
	KnobMemTimeoutMs          = "memory.retrieval.timeoutMs"
	KnobMemIncludeRecent      = "memory.retrieval.includeRecent"

	KnobEvolvingMaxSize             = "memory.evolving.maxSize"
	KnobEvolvingTopK                = "memory.evolving.topK"
	KnobEvolvingWindowSize          = "memory.evolving.windowSize"
	KnobEvolvingEnableRAG           = "memory.evolving.enableRAG"
	KnobEvolvingSimilarityThreshold = "memory.evolving.retrievalSimilarityThreshold"
	KnobEvolvingEnableSmartPrune    = "memory.evolving.enableSmartPrune"
	KnobEvolvingPruneThreshold      = "memory.evolving.pruneThreshold"
	KnobEvolvingRelevanceDecay      = "memory.evolving.relevanceDecay"
	KnobEvolvingMinRelevance        = "memory.evolving.minRelevance"

	KnobBeliefMaxPerPrompt          = "memory.belief.maxBeliefsPerPrompt"
	KnobBeliefMinConfidence         = "memory.belief.retrieval.minConfidence"
	KnobBeliefMaxTokensPerPrompt    = "memory.belief.retrieval.maxTokensPerPrompt"
	KnobBeliefIncludeContradictions = "memory.belief.retrieval.includeContradictions"

	KnobMagmaSemanticTopK         = "memory.magma.graphs.semantic.topK"
	KnobMagmaSimilarityThreshold  = "memory.magma.graphs.semantic.similarityThreshold"
	KnobMagmaCausalThreshold      = "memory.magma.graphs.causal.llmThreshold"
	KnobMagmaDefaultHops          = "memory.magma.retrieval.defaultHops"
	KnobMagmaDefaultMaxNodes      = "memory.magma.retrieval.defaultMaxNodes"
	KnobMagmaContextFormat        = "memory.magma.retrieval.contextFormat"
	KnobMagmaIntentClassification = "memory.magma.retrieval.intentClassification"

	KnobDecisionMaxPerPrompt       = "archaeology.retrieval.max_decisions_per_prompt"
	KnobDecisionTokenBudget        = "archaeology.retrieval.max_tokens_per_prompt"
	KnobDecisionTimeoutMs          = "archaeology.retrieval.timeout_ms"
	KnobAutoActivateEnabled        = "archaeology.auto_activate.enabled"
	KnobAutoActivateMinConfidence  = "archaeology.auto_activate.min_confidence"
	KnobConflictSimilarityFloor    = "archaeology.auto_activate.conflict_similarity_floor"
	KnobReactorConfidenceFloor     = "archaeology.reactor.confidence_floor"
	KnobReactorConfidenceDropDelta = "archaeology.reactor.confidence_drop_delta"
)

// Knobs is the Tier-1-injectable subset of the memory.* and archaeology.*
// configuration. memory.* and archaeology.* are startup-only on a live
// daemon, so the gym injects these values directly into the in-process
// runtime instead of mutating /api/config/agentd.
type Knobs struct {
	// memory.retrieval.*
	MaxTokensPerPrompt int
	TimeoutMs          int
	IncludeRecent      bool

	// memory.evolving.*
	EvolvingMaxSize             int
	EvolvingTopK                int
	EvolvingWindowSize          int
	EvolvingEnableRAG           bool
	EvolvingSimilarityThreshold float64
	EvolvingEnableSmartPrune    bool
	EvolvingPruneThreshold      float64
	EvolvingRelevanceDecay      float64
	EvolvingMinRelevance        float64
	EvolvingPruneQualityFloor   int
	EvolvingPromotionThreshold  int
	EvolvingMMRLambda           float64

	// memory.belief.*
	BeliefMaxPerPrompt          int
	BeliefMinConfidence         float64
	BeliefMaxTokensPerPrompt    int
	BeliefIncludeContradictions bool

	// memory.magma.*
	MagmaSemanticTopK         int
	MagmaSimilarityThreshold  float64
	MagmaCausalThreshold      float64
	MagmaDefaultHops          int
	MagmaDefaultMaxNodes      int
	MagmaContextFormat        string
	MagmaIntentClassification string

	// archaeology.*
	DecisionMaxPerPrompt       int
	DecisionTokenBudget        int
	DecisionTimeoutMs          int
	AutoActivateEnabled        bool
	AutoActivateMinConfidence  float64
	ConflictSimilarityFloor    float64
	ReactorConfidenceFloor     float64
	ReactorConfidenceDropDelta float64
}

// DefaultKnobs mirrors the shipped config.yaml.example defaults so the gym
// baseline matches what a stock deployment would run.
func DefaultKnobs() Knobs {
	return Knobs{
		MaxTokensPerPrompt: 2200,
		TimeoutMs:          700,
		IncludeRecent:      true,

		EvolvingMaxSize:             2000,
		EvolvingTopK:                4,
		EvolvingWindowSize:          20,
		EvolvingEnableRAG:           true,
		EvolvingSimilarityThreshold: 0.0,
		EvolvingEnableSmartPrune:    true,
		EvolvingPruneThreshold:      0.97,
		EvolvingRelevanceDecay:      0.99,
		EvolvingMinRelevance:        0.05,
		EvolvingPruneQualityFloor:   3,
		EvolvingPromotionThreshold:  5,
		EvolvingMMRLambda:           0.7,

		BeliefMaxPerPrompt:          5,
		BeliefMinConfidence:         0.35,
		BeliefMaxTokensPerPrompt:    700,
		BeliefIncludeContradictions: true,

		MagmaSemanticTopK:         20,
		MagmaSimilarityThreshold:  0.7,
		MagmaCausalThreshold:      0.8,
		MagmaDefaultHops:          2,
		MagmaDefaultMaxNodes:      10,
		MagmaContextFormat:        "structured",
		MagmaIntentClassification: "hybrid",

		DecisionMaxPerPrompt:       5,
		DecisionTokenBudget:        600,
		DecisionTimeoutMs:          0,
		AutoActivateEnabled:        false,
		AutoActivateMinConfidence:  0.85,
		ConflictSimilarityFloor:    0.50,
		ReactorConfidenceFloor:     0.35,
		ReactorConfidenceDropDelta: 0.30,
	}
}

// Timeout converts the runtime lane timeout knob to a duration.
func (k Knobs) Timeout() time.Duration {
	return time.Duration(k.TimeoutMs) * time.Millisecond
}

// DecisionTimeout converts the decision lane timeout knob to a duration.
func (k Knobs) DecisionTimeout() time.Duration {
	return time.Duration(k.DecisionTimeoutMs) * time.Millisecond
}

// Values reports every knob keyed by its configuration path. Results embed
// this map so Phase 2 scoring can attribute outcomes to exact config values.
func (k Knobs) Values() map[string]any {
	return map[string]any{
		KnobMemMaxTokensPerPrompt: k.MaxTokensPerPrompt,
		KnobMemTimeoutMs:          k.TimeoutMs,
		KnobMemIncludeRecent:      k.IncludeRecent,

		KnobEvolvingMaxSize:             k.EvolvingMaxSize,
		KnobEvolvingTopK:                k.EvolvingTopK,
		KnobEvolvingWindowSize:          k.EvolvingWindowSize,
		KnobEvolvingEnableRAG:           k.EvolvingEnableRAG,
		KnobEvolvingSimilarityThreshold: k.EvolvingSimilarityThreshold,
		KnobEvolvingEnableSmartPrune:    k.EvolvingEnableSmartPrune,
		KnobEvolvingPruneThreshold:      k.EvolvingPruneThreshold,
		KnobEvolvingRelevanceDecay:      k.EvolvingRelevanceDecay,
		KnobEvolvingMinRelevance:        k.EvolvingMinRelevance,

		KnobBeliefMaxPerPrompt:          k.BeliefMaxPerPrompt,
		KnobBeliefMinConfidence:         k.BeliefMinConfidence,
		KnobBeliefMaxTokensPerPrompt:    k.BeliefMaxTokensPerPrompt,
		KnobBeliefIncludeContradictions: k.BeliefIncludeContradictions,

		KnobMagmaSemanticTopK:         k.MagmaSemanticTopK,
		KnobMagmaSimilarityThreshold:  k.MagmaSimilarityThreshold,
		KnobMagmaCausalThreshold:      k.MagmaCausalThreshold,
		KnobMagmaDefaultHops:          k.MagmaDefaultHops,
		KnobMagmaDefaultMaxNodes:      k.MagmaDefaultMaxNodes,
		KnobMagmaContextFormat:        k.MagmaContextFormat,
		KnobMagmaIntentClassification: k.MagmaIntentClassification,

		KnobDecisionMaxPerPrompt:       k.DecisionMaxPerPrompt,
		KnobDecisionTokenBudget:        k.DecisionTokenBudget,
		KnobDecisionTimeoutMs:          k.DecisionTimeoutMs,
		KnobAutoActivateEnabled:        k.AutoActivateEnabled,
		KnobAutoActivateMinConfidence:  k.AutoActivateMinConfidence,
		KnobConflictSimilarityFloor:    k.ConflictSimilarityFloor,
		KnobReactorConfidenceFloor:     k.ReactorConfidenceFloor,
		KnobReactorConfidenceDropDelta: k.ReactorConfidenceDropDelta,
	}
}
