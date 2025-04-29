package agent

import (
	"context"
	"fmt"
	"strings"
)

// LLMCritic uses a language model to critique agent steps
type LLMCritic struct {
	Client    interface{} // OpenAI client or similar
	SystemTpl string      // Template for system prompt
}

// Critique analyzes the execution trace and suggests improvements
func (c *LLMCritic) Critique(ctx context.Context, trace []Interaction) (Critique, error) {
	// This is a simplified implementation that doesn't make actual LLM calls
	// A real implementation would call the LLM to analyze the trace

	// Check if there are any errors in the trace
	lastInteraction := trace[len(trace)-1]
	if lastInteraction.Observation.Err != nil {
		// Found an error, suggest a fix
		fixedStep := lastInteraction.Step

		// Simple example: if we got an error in text parameter for upper/lower tools,
		// suggest a modified step with a default text value
		if fixedStep.Tool == "upper" || fixedStep.Tool == "lower" {
			errMsg := lastInteraction.Observation.Err.Error()
			if strings.Contains(errMsg, "text parameter") {
				fixedStep.Args = map[string]any{"text": "Sample text to fix the error"}
				return Critique{
					Action: "revise",
					Fix:    &fixedStep,
					Reason: fmt.Sprintf("Fixed missing or invalid text parameter: %s", errMsg),
				}, nil
			}
		}

		// For web tool, suggest a different URL if the current one failed
		if fixedStep.Tool == "fetchWeb" {
			fixedStep.Args = map[string]any{"url": "https://example.com"}
			return Critique{
				Action: "revise",
				Fix:    &fixedStep,
				Reason: "The URL could not be accessed. Trying an alternative URL.",
			}, nil
		}

		// Generic suggestion for other errors
		return Critique{
			Action: "revise",
			Fix:    &fixedStep,
			Reason: fmt.Sprintf("Step failed with error: %v. Suggesting retry with the same parameters.", lastInteraction.Observation.Err),
		}, nil
	}

	// No errors, approve the execution
	return Critique{
		Action: "approve",
		Reason: "All steps executed successfully",
	}, nil
}

// NewLLMCritic creates a new LLM-based critic
func NewLLMCritic(client interface{}) *LLMCritic {
	return &LLMCritic{
		Client: client,
		SystemTpl: `You are a Critic for an AI agent.
Analyze the following execution trace and determine if any steps should be revised.
If a step failed, suggest a fix. Return your analysis as a JSON object with:
- action: "approve" or "revise"
- fix: a new Step object if action is "revise", otherwise null
- reason: a string explaining your decision`,
	}
}
