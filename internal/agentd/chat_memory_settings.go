package agentd

import (
	"manifold/internal/agent"
	chatpkg "manifold/internal/agentd/chat"
	persist "manifold/internal/persistence"
)

type chatMemoryRunSettings = chatpkg.MemoryRunSettings

func defaultChatMemoryRunSettings() chatMemoryRunSettings {
	return chatMemoryRunSettings{
		MemoryEnabled:         false,
		EvolvingMemoryEnabled: false,
		BeliefMemoryEnabled:   false,
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
	chatpkg.ApplyMemorySettings(eng, settings)
}

func normalizeChatMemoryRunSettings(settings chatMemoryRunSettings) chatMemoryRunSettings {
	return chatpkg.NormalizeMemoryRunSettings(settings)
}
