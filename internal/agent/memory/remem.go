package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

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
}

// ReMemConfig configures the ReMem controller.
type ReMemConfig struct {
	Memory        *EvolvingMemory
	LLM           llm.Provider
	Model         string
	MaxInnerSteps int // default 5
}

// NewReMemController creates a new Think-Act-Refine controller.
func NewReMemController(cfg ReMemConfig) *ReMemController {
	maxInner := cfg.MaxInnerSteps
	if maxInner <= 0 {
		maxInner = 5
	}

	return &ReMemController{
		memory:        cfg.Memory,
		llm:           cfg.LLM,
		model:         cfg.Model,
		maxInnerSteps: maxInner,
	}
}

// Execute runs the Think-Act-Refine loop for the given task.
// Returns the final action content and any accumulated reasoning trace.
func (rc *ReMemController) Execute(ctx context.Context, task string, tools []llm.ToolSchema) (string, []string, error) {
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

		// Call LLM
		msgs := []llm.Message{
			{Role: "system", Content: reMemSystemPrompt()},
			{Role: "user", Content: prompt},
		}

		resp, err := rc.llm.Chat(ctx, msgs, tools, rc.model)
		if err != nil {
			log.Error().Err(err).Msg("remem_llm_call_failed")
			return "", reasoningTrace, fmt.Errorf("LLM call failed: %w", err)
		}

		// Parse response as JSON
		var reMemResp ReMemResponse
		if err := json.Unmarshal([]byte(resp.Content), &reMemResp); err != nil {
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

	// If we exhausted inner steps without ACT, return last reasoning
	if len(reasoningTrace) > 0 {
		finalContent = reasoningTrace[len(reasoningTrace)-1]
	} else {
		finalContent = "(no final answer produced)"
	}

	log.Warn().Msg("remem_max_inner_steps_reached")
	return finalContent, reasoningTrace, nil
}

// buildPrompt constructs the prompt for each ReMem inner step.
func (rc *ReMemController) buildPrompt(task string, retrieved []*MemoryEntry, trace []string) string {
	var prompt string

	// Add retrieved memories
	if len(retrieved) > 0 {
		prompt += "## Relevant Memories\n\n"
		for i, entry := range retrieved {
			prompt += fmt.Sprintf("### Memory %d (ID: %s)\n", i+1, entry.ID)
			prompt += formatExperience(entry) + "\n"
		}
		prompt += "\n"
	}

	// Add reasoning trace so far
	if len(trace) > 0 {
		prompt += "## Your Reasoning So Far\n\n"
		for i, t := range trace {
			prompt += fmt.Sprintf("%d. %s\n", i+1, t)
		}
		prompt += "\n"
	}

	// Add current task
	prompt += fmt.Sprintf("## Current Task\n\n%s\n\n", task)

	prompt += "Respond with JSON following the schema described in the system prompt."

	return prompt
}

// reMemSystemPrompt returns the system prompt for ReMem mode.
func reMemSystemPrompt() string {
	return `You are an agent with evolving memory. You operate in a Think-Act-Refine loop:

**THINK**: Decompose the task, reason about it internally. This is private reasoning.
**REFINE_MEMORY**: Reason about your memories. Prune irrelevant ones, merge similar ones, or update metadata.
**ACT**: Execute the final action or provide the final answer. This ends your turn.

You MUST respond with valid JSON in this format:

{
  "action": "THINK" | "REFINE_MEMORY" | "ACT",
  "content": "your reasoning or final answer",
  "memory_edits": [ /* optional, only for REFINE_MEMORY */
    {"type": "PRUNE", "ids": ["mem_id_1"]},
    {"type": "MERGE", "ids": ["mem_id_2", "mem_id_3"], "new_summary": "combined lesson"},
    {"type": "UPDATE_TAG", "ids": ["mem_id_4"], "tag": "very_useful"}
  ]
}

**Guidelines**:
- Use THINK to break down the problem or plan your approach.
- Use REFINE_MEMORY when you notice redundant or low-quality memories. Be selective.
- Use ACT when you're ready to provide the final answer or action.
- You have a limited number of inner steps, so be efficient.
- Memory edits help you maintain a clean, useful knowledge base.

Avoid infinite thinking loops. Move to ACT when you have sufficient reasoning.`
}

// StoreExperience saves the task execution as a memory entry with feedback.
// This should be called after Execute() completes.
func (rc *ReMemController) StoreExperience(ctx context.Context, task, output, feedback string, trace []string) error {
	log := observability.LoggerWithTrace(ctx)

	// Combine reasoning trace into raw trace
	rawTrace := ""
	if len(trace) > 0 {
		for i, t := range trace {
			rawTrace += fmt.Sprintf("Step %d: %s\n", i+1, t)
		}
	}

	// Generate a strategy card - for now, just use basic feedback
	// In a full implementation, could enhance Evolve to accept optional strategy card
	_, err := rc.generateStrategyCard(ctx, task, output, feedback, trace)
	if err != nil {
		log.Warn().Err(err).Msg("remem_strategy_card_failed")
	}

	// Store in evolving memory via Evolve method which handles embedding and storage
	return rc.memory.Evolve(ctx, task, output, feedback)
}

// generateStrategyCard asks the LLM to produce a reusable strategy from the experience.
func (rc *ReMemController) generateStrategyCard(ctx context.Context, task, output, feedback string, trace []string) (string, error) {
	sys := `You are a strategy synthesizer. Given a task execution trace, produce a compact "strategy card" 
that can be reused for similar tasks. Format: "When confronted with <pattern>, do <strategy>. Avoid <mistakes>."`

	traceText := ""
	for i, t := range trace {
		traceText += fmt.Sprintf("%d. %s\n", i+1, truncate(t, 150))
	}

	user := fmt.Sprintf(`Task: %s
Outcome: %s
Reasoning trace:
%s

Produce a strategy card (max 100 words).`,
		truncate(task, 200), feedback, traceText)

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
