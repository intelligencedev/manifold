package agentd

import (
	"manifold/internal/agent"
	persist "manifold/internal/persistence"
)

type chatMemoryRunSettings struct {
	EvolvingMemoryEnabled bool
	BeliefMemoryEnabled   bool
}

func defaultChatMemoryRunSettings() chatMemoryRunSettings {
	return chatMemoryRunSettings{
		EvolvingMemoryEnabled: true,
		BeliefMemoryEnabled:   true,
	}
}

func chatMemorySettingsFromSession(sess persist.ChatSession) chatMemoryRunSettings {
	return chatMemoryRunSettings{
		EvolvingMemoryEnabled: sess.EvolvingMemoryEnabled,
		BeliefMemoryEnabled:   sess.BeliefMemoryEnabled,
	}
}

func chatMemorySettingsFromRunRequest(req chatRunRequest) chatMemoryRunSettings {
	settings := defaultChatMemoryRunSettings()
	if req.EvolvingMemoryEnabled != nil {
		settings.EvolvingMemoryEnabled = *req.EvolvingMemoryEnabled
	}
	if req.BeliefMemoryEnabled != nil {
		settings.BeliefMemoryEnabled = *req.BeliefMemoryEnabled
	}
	return settings
}

func boolPtr(v bool) *bool {
	return &v
}

func withChatMemorySettings(settings []chatMemoryRunSettings) chatMemoryRunSettings {
	if len(settings) > 0 {
		return settings[0]
	}
	return defaultChatMemoryRunSettings()
}

func applyChatMemorySettingsToEngine(eng *agent.Engine, settings chatMemoryRunSettings) {
	if eng == nil {
		return
	}
	eng.DisableEvolvingMemory = !settings.EvolvingMemoryEnabled
	eng.DisableBeliefMemory = !settings.BeliefMemoryEnabled
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
		eng.PolicyEnforcer = nil
	}
}
