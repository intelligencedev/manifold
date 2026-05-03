package memory

import (
	"encoding/json"
	"strings"
	"sync/atomic"

	"manifold/internal/llm"
	"manifold/internal/persistence"
)

const (
	defaultReserveBuffer  = 25000
	maxSummarizeChunkSize = 4096
	compactionSummaryType = "compaction"

	// Compaction best-practice knobs. We avoid compacting every single turn unless
	// we must (budget overflow), and we treat tool-heavy phases as milestones.
	compactionMinDeltaMessages     = 6
	compactionToolMilestoneOutputs = 2
)

const compactionContinuationRule = "When continuing from prior context (including compacted context), do not restate prior final answers unless the user asks. Only provide new information, the next steps, or the requested delta."

// dualSummary stores both compaction (OpenAI-specific) and plain text summaries.
// This allows sessions to switch between OpenAI and non-OpenAI models seamlessly.
type dualSummary struct {
	Compaction string `json:"compaction,omitempty"` // OpenAI Responses API compaction (encrypted)
	Plain      string `json:"plain,omitempty"`      // Plain text summary for non-OpenAI models
}

// encodeDualSummary encodes a dual summary to JSON for storage.
func encodeDualSummary(ds dualSummary) string {
	if ds.Compaction == "" && ds.Plain == "" {
		return ""
	}
	raw, err := json.Marshal(ds)
	if err != nil {
		return ds.Plain // Fallback to plain if encoding fails
	}
	return string(raw)
}

// decodeDualSummary decodes a stored summary. It handles both the new dual format
// and legacy single-summary formats (plain text or compaction JSON).
func decodeDualSummary(summary string) dualSummary {
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return dualSummary{}
	}

	// Try to decode as dual summary first
	var ds dualSummary
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), &ds); err == nil {
			// Check if it looks like a dual summary (has "plain" or "compaction" keys)
			if ds.Compaction != "" || ds.Plain != "" {
				return ds
			}
		}
		// Check if it's a legacy compaction summary
		if _, ok := decodeCompactionSummary(trimmed); ok {
			return dualSummary{Compaction: trimmed}
		}
	}

	// Treat as plain text summary (legacy format)
	return dualSummary{Plain: trimmed}
}

// SummaryResult contains metadata about summarization that occurred during BuildContext.
// Callers can use this to notify users (e.g., via SSE events) when summarization happens.
type SummaryResult struct {
	// Triggered is true if summarization occurred during this BuildContext call.
	Triggered bool
	// EstimatedTokens is the estimated token count that triggered summarization.
	EstimatedTokens int
	// TokenBudget is the available token budget.
	TokenBudget int
	// MessageCount is the total number of messages before summarization.
	MessageCount int
	// SummarizedCount is the number of messages that were summarized.
	SummarizedCount int
}

// Config tunes how chat history should be summarized before being sent to the LLM.
// All summarization is now token-based using a reserve buffer pattern.
type Config struct {
	Enabled bool `yaml:"enabled" json:"enabled"`

	// ReserveBufferTokens reserves tokens for output (including reasoning tokens
	// for reasoning models). OpenAI recommends ~25,000 for reasoning models.
	ReserveBufferTokens   int `yaml:"reserveBufferTokens" json:"reserveBufferTokens"`
	MinKeepLastMessages   int `yaml:"minKeepLastMessages" json:"minKeepLastMessages"`
	MaxKeepLastMessages   int `yaml:"maxKeepLastMessages" json:"maxKeepLastMessages"`
	MaxSummaryChunkTokens int `yaml:"maxSummaryChunkTokens" json:"maxSummaryChunkTokens"`
	// ContextWindowTokens can be pre-computed by the caller; if 0, ContextSize
	// is used as a fallback.
	ContextWindowTokens int `yaml:"contextWindowTokens" json:"contextWindowTokens"`

	SummaryModel string `yaml:"summaryModel" json:"summaryModel"`
}

// Manager coordinates persistence-backed chat memory with rolling summaries so that
// orchestrator runs reuse prior turns without resending the full transcript.
type Manager struct {
	store        persistence.ChatStore
	summary      llm.Provider
	summaryModel string

	enabled bool

	// Token-based summarization fields
	reserveBufferTokens   int
	minKeepLastMessages   int
	maxKeepLastMessages   int
	maxSummaryChunkTokens int
	contextWindowTokens   int
	compactionUnavailable atomic.Bool
}

// Introspection helpers used by debug/observability surfaces.

// ContextWindowTokens returns the approximate context window size (in tokens)
// used for token budgeting.
func (m *Manager) ContextWindowTokens() int { return m.contextWindowTokens }

// ReserveBufferTokens returns the reserve buffer for output tokens.
func (m *Manager) ReserveBufferTokens() int { return m.reserveBufferTokens }

// MinKeepLastMessages returns the minimum number of tail messages preserved
// in raw form when summarizing.
func (m *Manager) MinKeepLastMessages() int { return m.minKeepLastMessages }

// MaxSummaryChunkTokens returns the maximum token budget used when building
// summary chunks.
func (m *Manager) MaxSummaryChunkTokens() int { return m.maxSummaryChunkTokens }

// NewManager returns a chat memory manager.
