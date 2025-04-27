package agent

import (
	"context"
	"fmt"
	"time"
)

type Engine struct {
	Planner  Planner
	Executor Executor
	Memory   Memory
	Critic   Critic
	Tracer   Tracer
	Success  SuccessCriterion

	// Operational knobs
	PlanTimeout   time.Duration
	ExecTimeout   time.Duration
	CriticTimeout time.Duration
	MaxIters      int
}

func (e *Engine) Run(ctx context.Context, goal string) (string, error) {
	ctx, end := e.Tracer.Start(ctx, "engine.run", map[string]any{"goal": goal})
	defer end(nil)

	history := make([]Interaction, 0, 32)

	for iter := 0; iter < e.MaxIters; iter++ {
		// ---------- PLAN ----------
		mem, _ := e.Memory.Recall(ctx, goal, 8)
		pctx, cancel := context.WithTimeout(ctx, e.PlanTimeout)
		steps, err := e.Planner.Plan(pctx, goal, mem)
		cancel()
		if err != nil {
			return "", fmt.Errorf("planner: %w", err)
		}

		// ---------- EXEC ----------
		for _, step := range steps {
			sctx, endStep := e.Tracer.Start(ctx, "step.execute", map[string]any{"id": step.ID, "tool": step.Tool})
			res, err := e.Executor.Execute(sctx, step)
			endStep(err)

			obs := Observation{Step: step, Output: res, Err: err}
			history = append(history, Interaction{Step: step, Observation: obs})
			_ = e.Memory.Store(ctx, MemoryItem{Step: step, Observation: obs})

			if err != nil {
				// break early to critique
				break
			}
		}

		// ---------- CRITIQUE ----------
		cctx, cancelC := context.WithTimeout(ctx, e.CriticTimeout)
		critique, cErr := e.Critic.Critique(cctx, history)
		cancelC()
		if cErr == nil && critique.Action == "revise" && critique.Fix != nil {
			// replace last step in history and re-execute only that
			history[len(history)-1].Step = *critique.Fix
			continue
		}

		// ---------- SUCCESS? ----------
		if e.Success.IsSatisfied(history) {
			return summarise(history), nil
		}
	}
	return "", fmt.Errorf("max iterations reached")
}

func summarise(hist []Interaction) string {
	// TODO: call LLM; for now just list outputs.
	out := ""
	for _, h := range hist {
		out += fmt.Sprintf("- %s → %v\n", h.Step.Description, h.Observation.Output)
	}
	return out
}
