package agentd

import (
	"manifold/internal/agent"
	persist "manifold/internal/persistence"
)

type chatMemoryRunSettings struct {
	MemoryEnabled         bool
	EvolvingMemoryEnabled bool
	BeliefMemoryEnabled   bool
}

func defaultChatMemoryRunSettings() chatMemoryRunSettings {
	return chatMemoryRunSettings{
		MemoryEnabled:         true,
		EvolvingMemoryEnabled: true,
		BeliefMemoryEnabled:   true,
	}
}

func chatMemorySettingsFromSession(sess persist.ChatSession) chatMemoryRunSettings {
	enabled := sess.MemoryEnabled
	if !enabled && sess.EvolvingMemoryEnabled && sess.BeliefMemoryEnabled {
		enabled = true
	}
	return chatMemoryRunSettings{
		MemoryEnabled:         enabled,
		EvolvingMemoryEnabled: enabled,
		BeliefMemoryEnabled:   enabled,
	}
}

func chatMemorySettingsFromRunRequest(req chatRunRequest) chatMemoryRunSettings {
	settings := defaultChatMemoryRunSettings()
	memoryProvided := req.MemoryEnabled != nil
	if req.MemoryEnabled != nil {
		settings.MemoryEnabled = *req.MemoryEnabled
	}
	if req.EvolvingMemoryEnabled != nil {
		settings.EvolvingMemoryEnabled = *req.EvolvingMemoryEnabled
	}
	if req.BeliefMemoryEnabled != nil {
		settings.BeliefMemoryEnabled = *req.BeliefMemoryEnabled
	}
	if !memoryProvided {
		settings.MemoryEnabled = settings.EvolvingMemoryEnabled && settings.BeliefMemoryEnabled
	}
	settings.EvolvingMemoryEnabled = settings.MemoryEnabled
	settings.BeliefMemoryEnabled = settings.MemoryEnabled
	return settings
}

func boolPtr(v bool) *bool {
	return &v
}

func withChatMemorySettings(settings []chatMemoryRunSettings) chatMemoryRunSettings {
	if len(settings) > 0 {
		return normalizeChatMemoryRunSettings(settings[0])
	}
	return defaultChatMemoryRunSettings()
}

func applyChatMemorySettingsToEngine(eng *agent.Engine, settings chatMemoryRunSettings) {
	if eng == nil {
		return
	}
	settings = normalizeChatMemoryRunSettings(settings)
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
	}
}

func normalizeChatMemoryRunSettings(settings chatMemoryRunSettings) chatMemoryRunSettings {
	if !settings.MemoryEnabled && settings.EvolvingMemoryEnabled && settings.BeliefMemoryEnabled {
		settings.MemoryEnabled = true
	}
	settings.EvolvingMemoryEnabled = settings.MemoryEnabled
	settings.BeliefMemoryEnabled = settings.MemoryEnabled
	return settings
}
