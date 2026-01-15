//go:build enterprise
// +build enterprise

package orchestrator

import (
	"context"
	"fmt"

	"manifold/internal/warpp"
)

// NewWarppAdapter returns an orchestrator.Runner that delegates to a warpp.Runner.
func NewWarppAdapter(r *warpp.Runner) Runner {
	return &warppAdapter{runner: r}
}

type warppAdapter struct {
	runner *warpp.Runner
}

func (a *warppAdapter) Execute(ctx context.Context, workflow string, attrs map[string]any, publish func(ctx context.Context, stepID string, payload []byte) error) (map[string]any, error) {
	if a.runner == nil {
		return nil, fmt.Errorf("nil warpp runner")
	}
	// Lookup workflow by intent/name
	w, err := a.runner.Workflows.Get(workflow)
	if err != nil {
		return nil, err
	}
	// Ensure attrs is non-nil when passed to Personalize
	if attrs == nil {
		attrs = map[string]any{}
	}
	// Personalize the workflow (applies guards, infers attrs)
	personalized, _, A, err := a.runner.Personalize(ctx, w, attrs)
	if err != nil {
		return nil, err
	}
	// Build allowed tool map from personalized steps
	allowed := map[string]bool{}
	for _, s := range personalized.Steps {
		if s.Tool != nil {
			allowed[s.Tool.Name] = true
		}
	}
	// Execute the personalized workflow
	// Map the orchestrator-level publish func to the warpp.StepPublisher signature.
	var wp warpp.StepPublisher
	if publish != nil {
		wp = func(pctx context.Context, stepID string, payload []byte) error {
			return publish(pctx, stepID, payload)
		}
	}

	summary, err := a.runner.Execute(ctx, personalized, allowed, A, wp)
	if err != nil {
		return nil, err
	}
	return map[string]any{"summary": summary}, nil
}
