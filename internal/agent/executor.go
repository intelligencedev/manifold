package agent

import (
	"context"
	"fmt"
)

type ConcurrentExecutor struct {
	Registry *Registry
}

func (e *ConcurrentExecutor) Execute(ctx context.Context, step Step) (any, error) {
	if step.Tool == "" {
		// TODO: call a lightweight LLM. Placeholder:
		return fmt.Sprintf("LLM-answer for %q", step.Description), nil
	}
	return e.Registry.Execute(ctx, step.Tool, step.Args)
}
