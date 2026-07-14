package chat

import "manifold/internal/agent"

// NormalizeMemoryRunSettings keeps the coordinated memory lanes consistent.
func NormalizeMemoryRunSettings(settings MemoryRunSettings) MemoryRunSettings {
	if !settings.MemoryEnabled && settings.EvolvingMemoryEnabled && settings.BeliefMemoryEnabled {
		settings.MemoryEnabled = true
	}
	settings.EvolvingMemoryEnabled = settings.MemoryEnabled
	settings.BeliefMemoryEnabled = settings.MemoryEnabled
	return settings
}

// ApplyMemorySettings applies per-run memory policy to an engine.
func ApplyMemorySettings(eng *agent.Engine, settings MemoryRunSettings) {
	if eng == nil {
		return
	}
	settings = NormalizeMemoryRunSettings(settings)
	eng.DisableMemory = !settings.MemoryEnabled
	eng.DisableEvolvingMemory = !settings.EvolvingMemoryEnabled
	eng.DisableBeliefMemory = !settings.BeliefMemoryEnabled
	if !settings.MemoryEnabled {
		eng.Memory = nil
		eng.DisableEvolvingMemory = true
		eng.DisableBeliefMemory = true
	}
	if !settings.EvolvingMemoryEnabled {
		eng.EvolvingMemory = nil
		eng.ReMemEnabled = false
		eng.ReMemController = nil
	}
	if !settings.BeliefMemoryEnabled {
		eng.BeliefStore = nil
		eng.BeliefDistiller = nil
		eng.BeliefRetriever = nil
		eng.BeliefGraph = nil
		eng.BeliefMaxBeliefsPerPrompt = 0
		eng.BeliefPromptTokenBudget = 0
		eng.BeliefRetrievalMinConfidence = 0
		eng.BeliefIncludeContradictions = false
		eng.BeliefPromotionThreshold = 0
		eng.BeliefPolicySink = nil
		eng.BeliefMagmaSink = nil
		eng.PolicyEnforcer = nil
		eng.DecisionStore = nil
		eng.DecisionDistiller = nil
		eng.DecisionService = nil
		eng.ArtifactCapture = nil
	}
}
