package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

const defaultMinTraceStepsForStrategy = 3

// ReMemAction represents the Think-Act-Refine action types.
type ReMemAction string

const (
	ActionThink  ReMemAction = "THINK"
	ActionRefine ReMemAction = "REFINE_MEMORY"
	ActionAct    ReMemAction = "ACT"
)

// ReMemResponse is the structured output expected from the LLM in ReMem mode.
type ReMemResponse struct {
	Action      ReMemAction    `json:"action"`
	Content     string         `json:"content"`
	MemoryEdits []MemoryEditOp `json:"memory_edits,omitempty"`
}

// ReMemController implements the Think-Act-Refine loop from the paper.
// It cycles through THINK (internal reasoning), REFINE (memory editing),
// and ACT (final output) until ACT is chosen.
type ReMemController struct {
	memory        *EvolvingMemory
	llm           llm.Provider
	model         string
	maxInnerSteps int // max THINK/REFINE iterations before forcing ACT

	minTraceStepsForStrategy int
}

// ReMemConfig configures the ReMem controller.
type ReMemConfig struct {
	Memory        *EvolvingMemory
	LLM           llm.Provider
	Model         string
	MaxInnerSteps int // default 5

	// MinTraceStepsForStrategy gates strategy-card generation for trivial tasks.
	// Default is 3 reasoning/tool trace entries.
	MinTraceStepsForStrategy int
}

// NewReMemController creates a new Think-Act-Refine controller.
func NewReMemController(cfg ReMemConfig) *ReMemController {
	maxInner := cfg.MaxInnerSteps
	if maxInner <= 0 {
		maxInner = 5
	}
	minTraceStepsForStrategy := cfg.MinTraceStepsForStrategy
	if minTraceStepsForStrategy <= 0 {
		minTraceStepsForStrategy = defaultMinTraceStepsForStrategy
	}

	return &ReMemController{
		memory:                   cfg.Memory,
		llm:                      cfg.LLM,
		model:                    cfg.Model,
		maxInnerSteps:            maxInner,
		minTraceStepsForStrategy: minTraceStepsForStrategy,
	}
}

// Execute runs the Think-Act-Refine loop for the given task.
// Returns the final action content and any accumulated reasoning trace.
//
// ReMem is a memory-preparation step. It reasons about retrieved memories and
// edits them; it does not dispatch tools. The agent's tool schemas are
// intentionally not forwarded to the underlying LLM call so the prompt stays
// compact even when the host agent has many tools registered. Sending the full
// schema list here was previously overflowing small-context memory models.
func (rc *ReMemController) Execute(ctx context.Context, task string, _ []llm.ToolSchema) (string, []string, error) {
	log := observability.LoggerWithTrace(ctx)

	// Search for relevant memories
	retrieved, err := rc.memory.Search(ctx, task)
	if err != nil {
		log.Warn().Err(err).Msg("remem_search_failed")
		retrieved = nil
	}

	var reasoningTrace []string
	var finalContent string

	for step := 0; step < rc.maxInnerSteps; step++ {
		log.Debug().Int("inner_step", step).Msg("remem_inner_step")

		// Build prompt with task, memories, and reasoning trace
		prompt := rc.buildPrompt(task, retrieved, reasoningTrace)

		// Call LLM. ReMem responses are pure JSON, never tool calls, so we
		// intentionally pass nil for tool schemas to keep the prompt small.
		msgs := []llm.Message{
			{Role: "system", Content: reMemSystemPrompt()},
			{Role: "user", Content: prompt},
		}

		resp, err := rc.llm.Chat(ctx, msgs, nil, rc.model)
		if err != nil {
			log.Error().Err(err).Msg("remem_llm_call_failed")
			return "", reasoningTrace, fmt.Errorf("LLM call failed: %w", err)
		}

		reMemResp, err := parseReMemResponse(resp.Content)
		if err != nil {
			// If JSON parsing fails, treat as ACT with raw content
			log.Warn().Err(err).Msg("remem_json_parse_failed_fallback_to_act")
			return resp.Content, reasoningTrace, nil
		}

		log.Info().Str("action", string(reMemResp.Action)).Msg("remem_action")

		switch reMemResp.Action {
		case ActionThink:
			// Append reasoning to trace, continue loop
			reasoningTrace = append(reasoningTrace, reMemResp.Content)
			log.Debug().Str("thought", reMemResp.Content).Msg("remem_think")

		case ActionRefine:
			// Apply memory edits
			if len(reMemResp.MemoryEdits) > 0 {
				if err := rc.memory.ApplyEdits(ctx, reMemResp.MemoryEdits); err != nil {
					log.Error().Err(err).Msg("remem_apply_edits_failed")
				}
				// Re-search after refinement
				retrieved, _ = rc.memory.Search(ctx, task)
			}
			reasoningTrace = append(reasoningTrace, fmt.Sprintf("[REFINE] %s", reMemResp.Content))
			log.Debug().Int("edits", len(reMemResp.MemoryEdits)).Msg("remem_refine")

		case ActionAct:
			// Final action, end loop
			finalContent = reMemResp.Content
			log.Info().Msg("remem_act")
			return finalContent, reasoningTrace, nil

		default:
			log.Warn().Str("action", string(reMemResp.Action)).Msg("remem_unknown_action")
			return reMemResp.Content, reasoningTrace, nil
		}
	}

	log.Warn().Msg("remem_max_inner_steps_reached")
	finalContent, err = rc.forceFinalAct(ctx, task, retrieved, reasoningTrace)
	if err != nil {
		return "", reasoningTrace, err
	}
	return finalContent, reasoningTrace, nil
}

func parseReMemResponse(content string) (ReMemResponse, error) {
	content = strings.TrimSpace(stripJSONFence(content))
	if content == "" {
		return ReMemResponse{}, fmt.Errorf("empty ReMem response")
	}

	var out ReMemResponse
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err == nil {
		return out, nil
	}

	extracted := extractJSONObject(content)
	if extracted == "" || extracted == content {
		return ReMemResponse{}, fmt.Errorf("parse ReMem response JSON")
	}
	decoder = json.NewDecoder(strings.NewReader(extracted))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return ReMemResponse{}, fmt.Errorf("parse repaired ReMem response JSON: %w", err)
	}
	return out, nil
}

func stripJSONFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 {
		return trimmed
	}
	first := strings.TrimSpace(lines[0])
	if first != "```" && !strings.EqualFold(first, "```json") {
		return trimmed
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "```" {
		return trimmed
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

func extractJSONObject(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(content[start : end+1])
}

func (rc *ReMemController) forceFinalAct(ctx context.Context, task string, retrieved []*MemoryEntry, trace []string) (string, error) {
	prompt := rc.buildPrompt(task, retrieved, trace)
	prompt += "\n\nYou have used your memory-preparation budget; hand off to the main agent now. Respond with an ACT JSON object."
	msgs := []llm.Message{
		{Role: "system", Content: reMemSystemPrompt()},
		{Role: "user", Content: prompt},
	}
	// ReMem responses are pure JSON, not tool calls, so no tool schemas are forwarded.
	resp, err := rc.llm.Chat(ctx, msgs, nil, rc.model)
	if err != nil {
		return "", fmt.Errorf("force final ACT: %w", err)
	}
	reMemResp, err := parseReMemResponse(resp.Content)
	if err != nil {
		return resp.Content, nil
	}
	if reMemResp.Content == "" {
		return resp.Content, nil
	}
	return reMemResp.Content, nil
}

// buildPrompt constructs the prompt for each ReMem inner step.
func (rc *ReMemController) buildPrompt(task string, retrieved []*MemoryEntry, trace []string) string {
	var prompt strings.Builder

	// Add retrieved memories
	if len(retrieved) > 0 {
		prompt.WriteString("## Relevant Memories\n\n")
		for i, entry := range retrieved {
			prompt.WriteString(fmt.Sprintf("### Memory %d (ID: %s)\n", i+1, entry.ID))
			prompt.WriteString(formatExperience(entry) + "\n")
		}
		prompt.WriteString("\n")
	}

	// Add reasoning trace so far
	if len(trace) > 0 {
		prompt.WriteString("## Your Reasoning So Far\n\n")
		for i, t := range trace {
			prompt.WriteString(fmt.Sprintf("%d. %s\n", i+1, t))
		}
		prompt.WriteString("\n")
	}

	// Add current task
	prompt.WriteString(fmt.Sprintf("## Current Task\n\n%s\n\n", task))

	prompt.WriteString("Respond with JSON following the schema described in the system prompt. Keep THINK content to concise operational notes, not hidden chain-of-thought.")

	return prompt.String()
}

// reMemSystemPrompt returns the system prompt for ReMem mode.
func reMemSystemPrompt() string {
	return `You are a memory preparation controller for an agent with evolving memory. You operate in a Think-Act-Refine loop before the main agent answers the user:

**THINK**: Write concise operational notes about the current task, relevant memories, and whether memory cleanup is needed. These notes may be logged or stored, so do not include hidden chain-of-thought, secrets, or unnecessary sensitive details.
**REFINE_MEMORY**: Maintain the retrieved memories. Prune obsolete entries, merge redundant entries with the same reusable lesson, or update metadata.
**ACT**: Finish memory preparation and hand off to the main agent. This does not answer the user directly; the main agent will produce the visible response.

You MUST respond with valid JSON in this format:

{
  "action": "THINK" | "REFINE_MEMORY" | "ACT",
	"content": "brief operational note or handoff summary",
  "memory_edits": [ /* optional, only for REFINE_MEMORY */
		{"type": "PRUNE", "ids": ["mem_id_1"], "reason": "obsolete or harmful lesson"},
		{"type": "MERGE", "ids": ["mem_id_2", "mem_id_3"], "new_summary": "combined reusable lesson", "reason": "same pattern and same lesson"},
		{"type": "UPDATE_TAG", "ids": ["mem_id_4"], "tag": "very_useful", "reason": "frequently reusable"}
  ]
}

**Guidelines**:
- Use THINK to decide whether retrieved memories are useful, stale, redundant, or risky.
- Use REFINE_MEMORY only for IDs that appear in the retrieved memories.
- Never prune successful or protected-looking memories unless they are clearly obsolete, misleading, or unsafe.
- Prefer UPDATE_TAG over PRUNE when uncertain.
- Merge only memories that share the same reusable lesson; preserve the strongest success/failure signal in the new summary.
- Include a short reason for every memory edit.
- Use ACT when memory preparation is complete; summarize what the main agent should consider, but do not draft the final user-facing answer.
- You have a limited number of inner steps, so be efficient.
- Memory edits help you maintain a clean, useful knowledge base.

Avoid infinite thinking loops. Move to ACT when you have sufficient reasoning.`
}

// StoreExperience saves the task execution as a memory entry with feedback.
// This should be called after Execute() completes.
func (rc *ReMemController) StoreExperience(ctx context.Context, task, output, feedback string, trace []string) error {
	return rc.StoreExperienceEnhanced(ctx, task, output, feedback, nil, trace)
}

// StoreExperienceEnhanced saves the task execution with full structured feedback support.
// This properly integrates strategy cards as described in the paper.
func (rc *ReMemController) StoreExperienceEnhanced(
	ctx context.Context,
	task, output, feedback string,
	structuredFB *StructuredFeedback,
	trace []string,
) error {
	log := observability.LoggerWithTrace(ctx)

	strategyCard := ""
	if rc.shouldGenerateStrategyCard(trace) {
		var err error
		strategyCard, err = rc.generateStrategyCard(ctx, task, output, feedback, trace)
		if err != nil {
			log.Warn().Err(err).Msg("remem_strategy_card_failed")
			strategyCard = "" // Continue without strategy card
		}
	}

	// Use the enhanced Evolve method that properly stores strategy cards
	return rc.memory.EvolveEnhanced(ctx, task, output, feedback, structuredFB, trace, strategyCard)
}

func (rc *ReMemController) shouldGenerateStrategyCard(trace []string) bool {
	return len(trace) >= rc.minTraceStepsForStrategy
}

// generateStrategyCard asks the LLM to produce a reusable strategy from the experience.
func (rc *ReMemController) generateStrategyCard(ctx context.Context, task, output, feedback string, trace []string) (string, error) {
	sys := `You are a strategy synthesizer. Given a task execution trace, produce one compact strategy card that can be reused for similar tasks.

Rules:
- Extract the task pattern, the outcome, the reusable strategy, and the mistake or limit to avoid.
- For a successful outcome, use: "When confronted with <pattern>, do <strategy> because <reason>. Avoid <mistake or limit>."
- For a failed or partial outcome, use: "When confronted with <pattern>, avoid <failed approach>; instead do <better strategy>. Watch for <risk>."
- Do not include secrets, credentials, private user data, or transient one-off details.
- Return only the strategy card, max 100 words.`

	var traceText strings.Builder
	for i, t := range trace {
		traceText.WriteString(fmt.Sprintf("%d. %s\n", i+1, truncate(t, 150)))
	}

	user := fmt.Sprintf(`Task: %s
Outcome: %s
Output: %s
Reasoning trace:
%s


Produce the strategy card.`,
		truncate(task, 200), feedback, truncate(output, 300), traceText.String())

	msgs := []llm.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: user},
	}

	resp, err := rc.llm.Chat(ctx, msgs, nil, rc.model)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}
