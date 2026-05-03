package memory

import (
	"manifold/internal/llm"
	"manifold/internal/persistence"
)

func NewManager(store persistence.ChatStore, provider llm.Provider, cfg Config) *Manager {
	m := &Manager{
		store:                 store,
		summary:               provider,
		summaryModel:          cfg.SummaryModel,
		enabled:               cfg.Enabled && provider != nil,
		reserveBufferTokens:   cfg.ReserveBufferTokens,
		minKeepLastMessages:   cfg.MinKeepLastMessages,
		maxKeepLastMessages:   cfg.MaxKeepLastMessages,
		maxSummaryChunkTokens: cfg.MaxSummaryChunkTokens,
		contextWindowTokens:   cfg.ContextWindowTokens,
	}
	if m.reserveBufferTokens <= 0 {
		m.reserveBufferTokens = defaultReserveBuffer
	}
	if m.minKeepLastMessages <= 0 {
		m.minKeepLastMessages = 4
	}
	if m.maxKeepLastMessages <= 0 {
		m.maxKeepLastMessages = 12
	}
	if m.maxKeepLastMessages < m.minKeepLastMessages {
		m.maxKeepLastMessages = m.minKeepLastMessages
	}
	if m.maxSummaryChunkTokens <= 0 {
		m.maxSummaryChunkTokens = maxSummarizeChunkSize
	}
	if m.contextWindowTokens <= 0 && cfg.SummaryModel != "" {
		if size, _ := llm.ContextSize(cfg.SummaryModel); size > 0 {
			m.contextWindowTokens = size
		}
	}
	return m
}

// BuildContextForProvider assembles the conversation history that should be sent to the
// orchestrator by combining a persisted summary (if any) with the most recent chat turns.
// If targetProvider supports native compaction, the manager stores and reuses
// provider-native compacted state alongside its plain text summary.
