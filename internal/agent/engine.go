package agent

import (
	"context"

	"manifold/internal/agent/harness"
	"manifold/internal/agent/memory"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/llm"
	"manifold/internal/policy"
	"manifold/internal/tools"
)

type BeliefMagmaSink interface {
	IngestBelief(ctx context.Context, episode belief.Episode, item belief.Belief) (string, error)
}

type Engine struct {
	LLM      llm.Provider
	Tools    tools.Registry
	MaxSteps int
	System   string
	// UserPromptContext is inserted into the current user request as dynamic
	// runtime context.
	UserPromptContext string
	Model             string // default model name to pass to provider (used for metrics)
	SessionID         string
	ProjectID         string
	ObjectiveID       string
	UserID            int64
	AgentRole         string
	BeliefStore       belief.Store
	BeliefDistiller   belief.Distiller
	BeliefRetriever   belief.Retriever
	BeliefGraph       belief.Graph
	// BeliefMaxBeliefsPerPrompt bounds belief-memory prompt injection.
	BeliefMaxBeliefsPerPrompt int
	// BeliefPromptTokenBudget bounds the prompt section generated from belief memory.
	BeliefPromptTokenBudget      int
	BeliefRetrievalMinConfidence float64
	BeliefIncludeContradictions  bool
	BeliefPromotionThreshold     float64
	BeliefLifecyclePolicy        belief.PromotionPolicy
	BeliefPolicySink             belief.PolicySink
	BeliefMagmaSink              BeliefMagmaSink
	BeliefEnforcementPolicy      belief.EnforcementPolicy
	PolicyEnforcer               policy.Enforcer
	// MaxToolParallelism controls how many tool calls may run concurrently within a single step.
	// <= 0 means unbounded (default to len(toolCalls)); 1 preserves sequential behavior.
	MaxToolParallelism int
	// HarnessEnabled routes non-streaming runs through the Forge-style guarded loop.
	// It is disabled by default so existing Manifold behavior is preserved.
	HarnessEnabled bool
	// HarnessConfig controls guarded chat/workflow validation when HarnessEnabled is true.
	HarnessConfig harness.RunConfig
	// Delegator, when set, is used to execute nested agent calls (e.g., specialists)
	// without routing through tool implementations. This makes agent-to-agent
	// collaboration a core engine capability and enables rich tracing.
	Delegator Delegator
	// TeamDelegator, when set, is used to execute nested team calls without
	// routing through the HTTP delegate_to_team tool.
	TeamDelegator TeamDelegator
	// AgentTracer receives trace events emitted during delegated agent runs.
	AgentTracer AgentTracer
	// AgentDepth tracks nesting depth for trace events (0 for top-level orchestrator).
	AgentDepth int
	// ContextWindowTokens is the approximate context window for Model in tokens.
	// If not set, will be derived using llm.ContextSize.
	ContextWindowTokens int
	// Rolling summarization configuration (token-based only)
	SummaryEnabled bool
	// SkipInitialSummarization suppresses the engine's first pre-run summary pass
	// for a single run when chat memory already summarized the persisted history.
	SkipInitialSummarization bool
	// SummaryReserveBufferTokens is the number of tokens to reserve for model output
	// (including reasoning tokens). OpenAI recommends ~25,000 for reasoning models.
	// Default: 25000.
	SummaryReserveBufferTokens int
	// MinKeepLastMessages is the minimum number of tail messages to always try to
	// keep in raw form, even if the token budget is small.
	SummaryMinKeepLastMessages int
	// MaxSummaryChunkTokens caps the size of the summary prompt (older
	// conversation) in tokens.
	SummaryMaxSummaryChunkTokens int
	// Evolving memory configuration (Search → Synthesis → Evolve)
	Memory                *memory.Runtime         // unified memory coordinator; nil = legacy memory wiring
	DisableMemory         bool                    // per-run override for all coordinated memory lanes
	EvolvingMemory        *memory.EvolvingMemory  // nil = disabled
	ReMemEnabled          bool                    // enable Think-Act-Refine mode
	ReMemController       *memory.ReMemController // nil unless ReMemEnabled
	DisableEvolvingMemory bool                    // per-run override; also disables ReMem
	DisableBeliefMemory   bool                    // per-run override for belief retrieval, distillation, and policy context
	// OnAssistant, if set, is called with each assistant message the provider
	// returns (including those containing tool calls and the final answer).
	OnAssistant func(llm.Message)
	// OnDelta, if set, is called for streaming content deltas (for partial responses)
	OnDelta func(string)
	// OnThoughtSummary, if set, is called for streamed reasoning summaries.
	OnThoughtSummary func(string)
	// OnTool, if set, is called after each tool execution with tool name, args, result, and tool ID.
	OnTool             func(toolName string, args []byte, result []byte, toolID string)
	OnToolStart        func(toolName string, args []byte, toolID string)
	OnTurnMessage      func(llm.Message)
	OnSummaryTriggered func(inputTokens, tokenBudget, messageCount, summarizedCount int)
	// OnContextMetrics, if set, receives token budget snapshots at key prompt
	// assembly and compaction boundaries.
	OnContextMetrics func(ContextMetrics)
	// OnMemoryContext, if set, is called when memory context is added to the
	// current user prompt.
	OnMemoryContext func(memory.ContextBlock, memory.Diagnostics)
	// OnMemoryEvent, if set, is invoked when the evolving memory system emits
	// Search/Synthesis/Evolve events. Useful for debugging and observability.
	OnMemoryEvent func(*memory.MemoryEvent)
	// Tokenizer provides accurate token counting when available. If nil, the engine
	// falls back to heuristic estimation (chars/4).
	Tokenizer llm.Tokenizer
	// TokenizationFallbackToHeuristic allows falling back to heuristic on tokenization errors.
	TokenizationFallbackToHeuristic bool
	toolCallSeq                     uint64
}

// AttachTokenizer wires an accurate tokenizer into the engine when the provider exposes one.
// Providers that support the OpenAI Responses or Anthropic count_tokens endpoints accept an
