package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"manifold/internal/agent/memory"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/tools"
	"manifold/internal/tools/tts"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

type Engine struct {
	LLM      llm.Provider
	Tools    tools.Registry
	MaxSteps int
	System   string
	Model    string // default model name to pass to provider (used for metrics)
	// MaxToolParallelism controls how many tool calls may run concurrently within a single step.
	// <= 0 means unbounded (default to len(toolCalls)); 1 preserves sequential behavior.
	MaxToolParallelism int
	// Delegator, when set, is used to execute nested agent calls (e.g., specialists)
	// without routing through tool implementations. This makes agent-to-agent
	// collaboration a core engine capability and enables rich tracing.
	Delegator Delegator
	// AgentTracer receives trace events emitted during delegated agent runs.
	AgentTracer AgentTracer
	// AgentDepth tracks nesting depth for trace events (0 for top-level orchestrator).
	AgentDepth int
	// ContextWindowTokens is the approximate context window for Model in tokens.
	// summarization auto-mode will derive it using llm.ContextSize.
	ContextWindowTokens int
	// Rolling summarization configuration
	// In fixed mode, Threshold/KeepLast are honored for backwards compatibility.
	// In auto mode, summarization is driven by token budgets instead.
	SummaryEnabled   bool
	SummaryThreshold int    // fixed mode only
	SummaryKeepLast  int    // fixed mode only
	SummaryMode      string // "fixed" (default) or "auto"
	// TargetUtilizationPct controls how much of the model context window we aim
	// to fill with conversation history before summarizing, e.g. 0.7.
	SummaryTargetUtilizationPct float64
	// MinKeepLastMessages is the minimum number of tail messages to always try to
	// keep in raw form, even if the token budget is small.
	SummaryMinKeepLastMessages int
	// MaxSummaryChunkTokens caps the size of the summary prompt (older
	// conversation) in tokens.
	SummaryMaxSummaryChunkTokens int
	// Evolving memory configuration (Search → Synthesis → Evolve)
	EvolvingMemory  *memory.EvolvingMemory  // nil = disabled
	ReMemEnabled    bool                    // enable Think-Act-Refine mode
	ReMemController *memory.ReMemController // nil unless ReMemEnabled
	// OnAssistant, if set, is called with each assistant message the provider
	// returns (including those containing tool calls and the final answer).
	OnAssistant func(llm.Message)
	// OnDelta, if set, is called for streaming content deltas (for partial responses)
	OnDelta func(string)
	// OnTool, if set, is called after each tool execution with tool name, args, result, and tool ID.
	OnTool func(toolName string, args []byte, result []byte, toolID string)
	// OnToolStart, if set, is invoked immediately after the model emits a tool call
	// but before the tool is executed. This allows UIs to display a pending tool
	// invocation and later append the result when OnTool fires. Args are the raw
	// JSON arguments provided by the model (may still be partial JSON in some
	// provider streaming implementations, but are generally complete here).
	OnToolStart func(toolName string, args []byte, toolID string)
	// OnTurnMessage, if set, is called for every message added to the conversation
	// during this turn (including intermediate assistant messages with tool calls
	// and tool response messages). This enables full conversation history capture.
	OnTurnMessage func(llm.Message)
}

// Run executes the agent loop until the model produces a final answer.
func (e *Engine) Run(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)

	// If ReMem mode is enabled, use Think-Act-Refine controller
	if e.ReMemEnabled && e.ReMemController != nil {
		return e.runWithReMem(ctx, userInput, history)
	}

	msgs := BuildInitialLLMMessages(e.System, userInput, history)

	// Augment with evolving memory (ExpRAG or ExpRecent)
	if e.EvolvingMemory != nil {
		log.Info().Bool("enabled", true).Msg("evolving_memory_enabled")
		msgs = e.augmentWithMemory(ctx, userInput, msgs)
	} else {
		log.Debug().Bool("enabled", false).Msg("evolving_memory_disabled")
	}

	// Possibly summarize older history to avoid unbounded token growth.
	if e.SummaryEnabled {
		msgs = e.maybeSummarize(ctx, msgs)
	}

	final, err := e.runLoop(ctx, msgs)
	if err != nil {
		return "", err
	}

	// Store experience in evolving memory if enabled
	if e.EvolvingMemory != nil {
		log.Info().Str("user_input", userInput).Int("response_len", len(final)).Msg("evolving_memory_store_triggered")
		feedback := "success" // default; could be derived from user feedback or evaluation
		bgCtx := context.Background()
		if span := trace.SpanFromContext(ctx); span != nil {
			bgCtx = trace.ContextWithSpanContext(bgCtx, span.SpanContext())
		}
		go func(ctx context.Context, input, response, fb string) {
			if err := e.EvolvingMemory.Evolve(ctx, input, response, fb); err != nil {
				log.Error().Err(err).Str("feedback", fb).Msg("evolving_memory_store_failed")
				return
			}
			log.Info().Str("feedback", fb).Msg("evolving_memory_stored")
		}(bgCtx, userInput, final, feedback)
	}

	return final, nil
}

// RunStream executes the agent loop with streaming support
func (e *Engine) RunStream(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	// If ReMem mode is enabled, use Think-Act-Refine controller
	// Note: streaming with ReMem may need special handling for THINK/REFINE steps
	if e.ReMemEnabled && e.ReMemController != nil {
		return e.runWithReMem(ctx, userInput, history)
	}

	msgs := BuildInitialLLMMessages(e.System, userInput, history)

	// Augment with evolving memory (ExpRAG or ExpRecent)
	if e.EvolvingMemory != nil {
		log.Info().Bool("enabled", true).Msg("evolving_memory_enabled_stream")
		msgs = e.augmentWithMemory(ctx, userInput, msgs)
	} else {
		log.Debug().Bool("enabled", false).Msg("evolving_memory_disabled_stream")
	}

	// Possibly summarize older history to avoid unbounded token growth.
	if e.SummaryEnabled {
		msgs = e.maybeSummarize(ctx, msgs)
	}

	return e.runStreamLoop(ctx, msgs)
}

// streamHandler implements llm.StreamHandler
type streamHandler struct {
	onDelta    func(string)
	onToolCall func(llm.ToolCall)
	onImage    func(llm.GeneratedImage)
}

func (h *streamHandler) OnDelta(content string) {
	if h.onDelta != nil {
		h.onDelta(content)
	}
}

func (h *streamHandler) OnToolCall(tc llm.ToolCall) {
	if h.onToolCall != nil {
		h.onToolCall(tc)
	}
}

func (h *streamHandler) OnImage(img llm.GeneratedImage) {
	if h.onImage != nil {
		h.onImage(img)
	}
}

func (e *Engine) model() string { return e.Model }

// runLoop contains the core non-streaming agent step loop shared by Run.
// It returns the final assistant content or an error.
func (e *Engine) runLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
	var final string

	for step := 0; step < e.MaxSteps; step++ {
		log.Debug().Int("step", step).Int("history", len(msgs)).Msg("engine_step_start")

		// Capture tool schemas once per step so we can log what the model sees.
		schemas := e.Tools.Schemas()
		toolNames := make([]string, len(schemas))
		for i, s := range schemas {
			toolNames[i] = s.Name
		}
		log.Info().Strs("tools_sent_to_llm", toolNames).Msg("engine_tools_before_chat")

		msg, err := e.LLM.Chat(ctx, msgs, schemas, e.model())
		if err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_step_error")
			return "", err
		}

		msgs = append(msgs, msg)
		if e.OnAssistant != nil {
			e.OnAssistant(msg)
		}
		if e.OnTurnMessage != nil {
			e.OnTurnMessage(msg)
		}

		if len(msg.ToolCalls) == 0 {
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_final")
			final = msg.Content
			break
		}

		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_tool_calls")
		msgs = e.dispatchTools(ctx, msgs, msg.ToolCalls)
	}

	if final == "" {
		final = "(no final text — increase max steps or check logs)"
	}

	return final, nil
}

// runStreamLoop contains the core streaming agent step loop shared by RunStream.
// It returns the final assistant content or an error.
func (e *Engine) runStreamLoop(ctx context.Context, msgs []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)
	var final string

	for step := 0; step < e.MaxSteps; step++ {
		// Accumulate streaming content and tool calls for this step
		var (
			accumulatedContent   string
			accumulatedToolCalls []llm.ToolCall
			accumulatedImages    []llm.GeneratedImage
		)

		handler := &streamHandler{
			onDelta: func(content string) {
				accumulatedContent += content
				if e.OnDelta != nil {
					e.OnDelta(content)
				}
			},
			onToolCall: func(tc llm.ToolCall) {
				accumulatedToolCalls = append(accumulatedToolCalls, tc)
			},
			onImage: func(img llm.GeneratedImage) {
				accumulatedImages = append(accumulatedImages, img)
			},
		}

		log.Debug().Int("step", step).Int("history", len(msgs)).Msg("engine_stream_step_start")

		// Capture tool schemas once per step so we can log what the model sees.
		schemas := e.Tools.Schemas()
		toolNames := make([]string, len(schemas))
		for i, s := range schemas {
			toolNames[i] = s.Name
		}
		log.Info().Strs("tools_sent_to_llm_stream", toolNames).Msg("engine_tools_before_stream")

		if err := e.LLM.ChatStream(ctx, msgs, schemas, e.model(), handler); err != nil {
			log.Error().Err(err).Int("step", step).Msg("engine_stream_step_error")
			return "", err
		}

		msg := llm.Message{
			Role:      "assistant",
			Content:   accumulatedContent,
			ToolCalls: accumulatedToolCalls,
			Images:    accumulatedImages,
		}

		msgs = append(msgs, msg)
		if e.OnAssistant != nil {
			e.OnAssistant(msg)
		}
		if e.OnTurnMessage != nil {
			e.OnTurnMessage(msg)
		}

		if len(msg.ToolCalls) == 0 {
			log.Info().Int("step", step).Int("final_len", len(msg.Content)).Msg("engine_stream_final")
			final = msg.Content
			break
		}

		log.Info().Int("step", step).Int("tool_calls", len(msg.ToolCalls)).Msg("engine_stream_tool_calls")
		msgs = e.dispatchTools(ctx, msgs, msg.ToolCalls)
	}

	if final == "" {
		final = "(no final text — increase max steps or check logs)"
	}

	return final, nil
}

// dispatchTools executes a batch of tool calls, appending their tool messages to msgs
// and invoking the appropriate callbacks/logging. It returns the updated msgs slice.
func (e *Engine) dispatchTools(ctx context.Context, msgs []llm.Message, toolCalls []llm.ToolCall) []llm.Message {
	if len(toolCalls) == 0 {
		return msgs
	}

	maxParallel := e.MaxToolParallelism
	if maxParallel <= 0 || maxParallel > len(toolCalls) {
		maxParallel = len(toolCalls)
	}
	if maxParallel <= 0 {
		maxParallel = 1
	}

	results := make([]llm.Message, len(toolCalls))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		i, tc := i, tc

		dispatchCtx := ctx
		if e.LLM != nil {
			dispatchCtx = tools.WithProvider(ctx, e.LLM)
		}

		if tc.Name == "text_to_speech" && e.OnTool != nil {
			var raw map[string]any
			_ = json.Unmarshal(tc.Args, &raw)
			if v, ok := raw["stream"].(bool); ok && v {
				cb := func(chunk []byte) {
					meta := map[string]any{"event": "chunk", "bytes": len(chunk), "b64": base64.StdEncoding.EncodeToString(chunk)}
					b, _ := json.Marshal(meta)
					if e.OnTool != nil {
						e.OnTool("text_to_speech_chunk", tc.Args, b, tc.ID)
					}
				}
				dispatchCtx = tts.WithStreamChunkCallback(dispatchCtx, cb)
			}
		}

		if tc.Name == "multi_tool_use_parallel" && (e.OnToolStart != nil || e.OnTool != nil) {
			sink := func(ev tools.SubtoolEvent) {
				if ev.Phase == "start" && e.OnToolStart != nil {
					e.OnToolStart(ev.Name, ev.Args, ev.ToolCallID)
					return
				}
				if ev.Phase == "end" && e.OnTool != nil {
					e.OnTool(ev.Name, ev.Args, ev.Payload, ev.ToolCallID)
					return
				}
			}
			dispatchCtx = tools.WithSubtoolSink(dispatchCtx, sink)
		}

		if e.OnToolStart != nil {
			e.OnToolStart(tc.Name, tc.Args, tc.ID)
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(idx int, tc llm.ToolCall, dctx context.Context) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = e.executeToolCall(dctx, tc)
		}(i, tc, dispatchCtx)
	}

	wg.Wait()
	// Invoke OnTurnMessage for each tool response message
	if e.OnTurnMessage != nil {
		for _, toolMsg := range results {
			e.OnTurnMessage(toolMsg)
		}
	}
	return append(msgs, results...)
}

func (e *Engine) executeToolCall(ctx context.Context, tc llm.ToolCall) llm.Message {
	// Handle agent delegation as a first-class engine feature (not a tool).
	if e.Delegator != nil && isAgentCall(tc.Name) {
		payload := e.runDelegatedAgent(ctx, tc)
		if e.OnTool != nil {
			e.OnTool(tc.Name, tc.Args, payload, tc.ID)
		}
		return llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID}
	}

	observability.LoggerWithTrace(ctx).Info().Str("tool", tc.Name).RawJSON("args", observability.RedactJSON(tc.Args)).Msg("engine_tool_call")
	payload, err := e.Tools.Dispatch(ctx, tc.Name, tc.Args)
	if err != nil {
		payload = []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	if e.OnTool != nil {
		e.OnTool(tc.Name, tc.Args, payload, tc.ID)
	}
	return llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID}
}

func isAgentCall(name string) bool {
	return name == "agent_call" || name == "ask_agent"
}

// runDelegatedAgent executes an agent-to-agent handoff using the configured
// Delegator and wraps the output in the legacy tool payload shape so the
// parent loop can continue unchanged.
func (e *Engine) runDelegatedAgent(ctx context.Context, tc llm.ToolCall) []byte {
	var args struct {
		AgentName      string        `json:"agent_name"`
		To             string        `json:"to"`
		Prompt         string        `json:"prompt"`
		History        []llm.Message `json:"history"`
		EnableTools    *bool         `json:"enable_tools"`
		MaxSteps       int           `json:"max_steps"`
		TimeoutSeconds int           `json:"timeout_seconds"`
		ProjectID      string        `json:"project_id"`
		UserID         int64         `json:"user_id"`
	}
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return []byte(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()))
	}
	// Support both `agent_name` (internal) and `to` (ask_agent tool)
	if strings.TrimSpace(args.AgentName) == "" && strings.TrimSpace(args.To) != "" {
		args.AgentName = strings.TrimSpace(args.To)
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return []byte(`{"ok":false,"error":"prompt is required"}`)
	}
	callID := tc.ID
	if strings.TrimSpace(callID) == "" {
		callID = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	req := DelegateRequest{
		AgentName:      strings.TrimSpace(args.AgentName),
		Prompt:         args.Prompt,
		History:        args.History,
		EnableTools:    args.EnableTools,
		MaxSteps:       args.MaxSteps,
		TimeoutSeconds: args.TimeoutSeconds,
		ProjectID:      strings.TrimSpace(args.ProjectID),
		UserID:         args.UserID,
		CallID:         callID,
		ParentCallID:   tc.ID,
		Depth:          e.AgentDepth + 1,
	}
	result, err := e.Delegator.Run(ctx, req, e.AgentTracer)
	if err != nil {
		return []byte(fmt.Sprintf(`{"ok":false,"agent":%q,"error":%q}`, req.AgentName, err.Error()))
	}
	out := map[string]any{"ok": true, "agent": req.AgentName, "output": result}
	if b, err := json.Marshal(out); err == nil {
		return b
	}
	return []byte(result)
}

// maybeSummarize inspects msgs and, if the number of messages exceeds
// e.SummaryThreshold, calls the LLM to produce a short summary of the
// older portion of the conversation and returns a new messages slice
// where the older messages have been replaced by a single summary
// assistant message plus the most recent messages preserved.
func (e *Engine) maybeSummarize(ctx context.Context, msgs []llm.Message) []llm.Message {
	mode := strings.ToLower(strings.TrimSpace(e.SummaryMode))
	if mode == "auto" {
		return e.maybeSummarizeAuto(ctx, msgs)
	}
	return e.maybeSummarizeFixed(ctx, msgs)
}

// maybeSummarizeFixed preserves the original message-count based behavior for
// backwards compatibility.
func (e *Engine) maybeSummarizeFixed(ctx context.Context, msgs []llm.Message) []llm.Message {
	// Ensure sensible defaults
	threshold := e.SummaryThreshold
	keep := e.SummaryKeepLast
	if threshold <= 0 {
		threshold = 40
	}
	if keep <= 0 {
		keep = 12
	}
	if len(msgs) <= threshold {
		return msgs
	}

	// Log that summarization will run and include relevant tuning params.
	observability.LoggerWithTrace(ctx).Info().
		Int("messages", len(msgs)).
		Int("threshold", threshold).
		Int("keep_last", keep).
		Str("mode", "fixed").
		Msg("summarization_triggered")

	// Preserve leading system message if present
	start := 0
	var sysMsg *llm.Message
	if len(msgs) > 0 && msgs[0].Role == "system" {
		sysMsg = &msgs[0]
		start = 1
	}

	if len(msgs)-start <= keep {
		return msgs
	}
	cutIndex := len(msgs) - keep
	toSummarize := msgs[start:cutIndex]
	recent := msgs[cutIndex:]

	return e.buildSummarizedMessages(ctx, sysMsg, toSummarize, recent, keep)
}

// maybeSummarizeAuto uses an estimated token budget based on the model's
// context window and a target utilization percentage.
func (e *Engine) maybeSummarizeAuto(ctx context.Context, msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}

	ctxSize := e.ContextWindowTokens
	if ctxSize <= 0 {
		if sz, ok := llm.ContextSize(e.model()); sz > 0 {
			ctxSize = sz
			_ = ok // we don't care if it's guessed here
		}
	}
	if ctxSize <= 0 {
		ctxSize = 32_000
	}

	util := e.SummaryTargetUtilizationPct
	if util <= 0 || util > 1 {
		util = 0.7
	}

	minTail := e.SummaryMinKeepLastMessages
	if minTail <= 0 {
		minTail = 4
	}

	// Estimate token usage for the full history.
	estTotal := estimateTokensForMessages(msgs)
	tokenBudget := int(float64(ctxSize) * util)
	if estTotal <= tokenBudget {
		// No summarization needed; fits comfortably.
		return msgs
	}

	log := observability.LoggerWithTrace(ctx)
	log.Info().
		Int("messages", len(msgs)).
		Int("estimated_tokens", estTotal).
		Int("token_budget", tokenBudget).
		Int("context_window", ctxSize).
		Float64("target_utilization", util).
		Str("mode", "auto").
		Msg("summarization_triggered")

	// Preserve leading system message if present.
	start := 0
	var sysMsg *llm.Message
	if msgs[0].Role == "system" {
		sysMsg = &msgs[0]
		start = 1
	}

	// Work backwards from the end of the conversation, keeping as many recent
	// messages as will fit into ~half the budget (so we leave room for system
	// prompt, tools, and summary itself). This is intentionally simple.
	recent := make([]llm.Message, 0, len(msgs))
	remaining := tokenBudget / 2
	for i := len(msgs) - 1; i >= start; i-- {
		msgTokens := estimateTokens(msgs[i].Content)
		if len(recent) >= minTail && remaining-msgTokens <= 0 {
			break
		}
		recent = append(recent, msgs[i])
		remaining -= msgTokens
		if remaining <= 0 {
			break
		}
	}
	// We appended in reverse order; restore chronological order.
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}

	// Everything between start and the beginning of the recent slice becomes the
	// summary input.
	cutIndex := len(msgs) - len(recent)
	if cutIndex < start {
		cutIndex = start
	}
	toSummarize := msgs[start:cutIndex]
	if len(toSummarize) == 0 {
		return msgs
	}

	return e.buildSummarizedMessages(ctx, sysMsg, toSummarize, recent, len(recent))
}

// buildSummarizedMessages constructs a summary prompt, calls the LLM, and
// returns the new message list (system + [summary] + recent).
func (e *Engine) buildSummarizedMessages(
	ctx context.Context,
	sysMsg *llm.Message,
	toSummarize []llm.Message,
	recent []llm.Message,
	keep int,
) []llm.Message {
	maxChunkTokens := e.SummaryMaxSummaryChunkTokens
	if maxChunkTokens <= 0 {
		maxChunkTokens = 4096
	}

	var b strings.Builder
	currentTokens := 0
	for _, m := range toSummarize {
		// Approximate token cost per message and cap at maxChunkTokens.
		msgTokens := estimateTokens(m.Content) + 8 // overhead for role/formatting
		if currentTokens+msgTokens > maxChunkTokens {
			break
		}
		b.WriteString("Role: ")
		b.WriteString(m.Role)
		b.WriteString("\n")
		content := m.Content
		// Hard safety cap in characters as a backstop.
		if len(content) > maxChunkTokens*4 {
			content = content[:maxChunkTokens*4] + "\n[TRUNCATED]"
		}
		b.WriteString(content)
		b.WriteString("\n\n")
		currentTokens += msgTokens
	}

	sys := "You are a concise summarizer. Produce a short, factual summary (<= 300 characters) of the conversation that follows. Keep important facts, omit chit-chat. Return only the summary text."
	user := "Summarize the following conversation:\n\n" + b.String()

	summReq := []llm.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}}
	sumMsg, err := e.LLM.Chat(ctx, summReq, nil, e.model())
	if err != nil {
		observability.LoggerWithTrace(ctx).Error().Err(err).Msg("summary_failed")
		return append([]llm.Message{}, append(toSummarize, recent...)...)
	}

	summaryContent := "[SUMMARY] " + strings.TrimSpace(sumMsg.Content)
	summary := llm.Message{Role: "assistant", Content: summaryContent}

	newMsgs := make([]llm.Message, 0, 1+keep+2)
	if sysMsg != nil {
		newMsgs = append(newMsgs, *sysMsg)
	}
	newMsgs = append(newMsgs, summary)
	newMsgs = append(newMsgs, recent...)

	observability.LoggerWithTrace(ctx).Info().
		Int("orig_messages", len(toSummarize)+len(recent)).
		Int("new_messages", len(newMsgs)).
		Msg("history_summarized")
	return newMsgs
}

// estimateTokensForMessages provides a very rough token estimate for a slice
// of messages by summing estimateTokens over their content.
func estimateTokensForMessages(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateTokens(m.Content)
	}
	return total
}

// estimateTokens is a chars/4 heuristic fallback; providers with proper
// tokenizers can later plug in a more accurate implementation.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	// Simple heuristic: 4 characters per token on average.
	return len([]rune(s))/4 + 1
}

// augmentWithMemory appends evolving memory context to the system prompt (ExpRAG or ExpRecent).
// This ensures memory context is reconstructed on every request without interfering with conversation history.
func (e *Engine) augmentWithMemory(ctx context.Context, userInput string, msgs []llm.Message) []llm.Message {
	log := observability.LoggerWithTrace(ctx)

	log.Info().Str("user_input", userInput).Msg("evolving_memory_augment_triggered")

	var memoryContext string

	// Try ExpRAG (experience retrieval) and ExpRecent (recent window) in parallel when enabled.
	if e.EvolvingMemory != nil {
		log.Debug().Msg("evolving_memory_search_starting")
		var (
			retrieved     []*memory.MemoryEntry
			recentContext string
			wg            sync.WaitGroup
		)

		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := e.EvolvingMemory.Search(ctx, userInput)
			if err != nil {
				log.Error().Err(err).Str("query", userInput).Msg("evolving_memory_search_failed")
				return
			}
			retrieved = res
			if len(res) > 0 {
				log.Info().Int("retrieved", len(res)).Str("query", userInput).Msg("evolving_memory_search_success")
				return
			}
			log.Debug().Str("query", userInput).Msg("evolving_memory_search_no_results")
		}()

		log.Debug().Msg("evolving_memory_exprecent_starting")
		recentContext = e.EvolvingMemory.BuildExpRecentContext()
		if recentContext != "" {
			log.Info().Int("context_len", len(recentContext)).Msg("evolving_memory_exprecent_used")
		} else {
			log.Debug().Msg("evolving_memory_exprecent_empty")
		}

		wg.Wait()
		if len(retrieved) > 0 {
			memoryContext = e.EvolvingMemory.Synthesize(ctx, userInput, retrieved)
			log.Info().Int("retrieved", len(retrieved)).Int("context_len", len(memoryContext)).Msg("evolving_memory_exprag_synthesized")
		} else if recentContext != "" {
			memoryContext = recentContext
		}
	}

	if memoryContext == "" {
		log.Debug().Msg("evolving_memory_no_context_skipping_augmentation")
		return msgs
	}

	log.Info().Int("context_len", len(memoryContext)).Int("orig_msgs", len(msgs)).Msg("evolving_memory_appending_to_system")

	// Append memory context to the system prompt instead of injecting as separate message
	// This ensures it's reconstructed on every request and doesn't interfere with history
	systemIdx := -1
	for i, msg := range msgs {
		if msg.Role == "system" {
			systemIdx = i
			break
		}
	}

	if systemIdx >= 0 {
		// Append memory context to existing system message
		msgs[systemIdx].Content += "\n\n## Relevant Context from Past Interactions\n\n" + memoryContext
		log.Debug().Int("system_idx", systemIdx).Int("new_len", len(msgs[systemIdx].Content)).Msg("evolving_memory_appended_to_system")
	} else {
		// No system message exists, create one with memory context
		msgs = append([]llm.Message{{
			Role:    "system",
			Content: "## Relevant Context from Past Interactions\n\n" + memoryContext,
		}}, msgs...)
		log.Debug().Msg("evolving_memory_created_system_with_context")
	}

	log.Info().Int("msgs_count", len(msgs)).Msg("evolving_memory_augmentation_complete")
	return msgs
}

// runWithReMem executes the Think-Act-Refine pre-processing, then continues with the main agent loop.
// ReMem is a memory-management phase that refines memories before answering, NOT a replacement for the main loop.
func (e *Engine) runWithReMem(ctx context.Context, userInput string, history []llm.Message) (string, error) {
	log := observability.LoggerWithTrace(ctx)

	// Execute ReMem controller (internal memory reasoning/refinement)
	_, reasoningTrace, err := e.ReMemController.Execute(ctx, userInput, e.Tools.Schemas())
	if err != nil {
		log.Error().Err(err).Msg("remem_execute_failed")
		// Continue with main loop even if ReMem fails - don't block user response
		log.Warn().Msg("remem_failed_continuing_with_main_loop")
	} else {
		log.Info().Int("reasoning_steps", len(reasoningTrace)).Msg("remem_completed")
	}

	// Now run the main agent loop with (potentially refined) memories
	msgs := BuildInitialLLMMessages(e.System, userInput, history)

	// Augment with evolving memory (which may have been refined by ReMem)
	if e.EvolvingMemory != nil {
		msgs = e.augmentWithMemory(ctx, userInput, msgs)
	}

	// Possibly summarize older history
	if e.SummaryEnabled {
		msgs = e.maybeSummarize(ctx, msgs)
	}

	// Run the streaming loop to generate actual response (preserves streaming behavior)
	final, err := e.runStreamLoop(ctx, msgs)
	if err != nil {
		return "", err
	}

	// Store the experience with reasoning trace AFTER we have the actual response
	feedback := "success" // default; in practice could be derived from evaluation
	log.Info().Str("user_input", userInput).Int("reasoning_steps", len(reasoningTrace)).Msg("remem_store_experience_triggered")
	bgCtx := context.Background()
	if span := trace.SpanFromContext(ctx); span != nil {
		bgCtx = trace.ContextWithSpanContext(bgCtx, span.SpanContext())
	}
	go func(ctx context.Context, input, resp, fb string, traceMsgs []string) {
		if storeErr := e.ReMemController.StoreExperience(ctx, input, resp, fb, traceMsgs); storeErr != nil {
			log.Error().Err(storeErr).Str("feedback", fb).Msg("remem_store_experience_failed")
			return
		}
		log.Info().Str("feedback", fb).Int("reasoning_steps", len(traceMsgs)).Msg("remem_experience_stored")
	}(bgCtx, userInput, final, feedback, reasoningTrace)

	return final, nil
}

// Message exists for future agent-level message modeling.
// Message type removed in favor of llm.Message throughout the engine API.
