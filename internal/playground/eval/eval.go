package eval

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/playground/experiment"
	"manifold/internal/playground/provider"
	"manifold/internal/playground/worker"
)

// Outcome contains aggregated and per-sample scores.
type Outcome struct {
	Aggregate map[string]float64
	Scores    map[int]map[string]float64
}

// Evaluator scores run results.
type Evaluator interface {
	Name() string
	Evaluate(ctx context.Context, spec experiment.ExperimentSpec, results []worker.Result) (Outcome, error)
}

// Factory produces an evaluator for a config.
type Factory func(cfg experiment.EvaluatorConfig, prov provider.Provider) (Evaluator, error)

// Registry indexes evaluator factories by name.
type Registry struct {
	factories map[string]Factory
}

// NewRegistry constructs a registry with built-in evaluators.
func NewRegistry() *Registry {
	r := &Registry{factories: make(map[string]Factory)}
	r.Register("format", newFormatEvaluator)
	r.Register("llm-judge", newJudgeEvaluator)
	return r
}

// Register adds a factory to the registry.
func (r *Registry) Register(name string, factory Factory) {
	r.factories[strings.ToLower(name)] = factory
}

// Instantiate builds an evaluator for a config.
func (r *Registry) Instantiate(cfg experiment.EvaluatorConfig, prov provider.Provider) (Evaluator, error) {
	factory, ok := r.factories[strings.ToLower(cfg.Name)]
	if !ok {
		return nil, fmt.Errorf("playground/eval: unknown evaluator %q", cfg.Name)
	}
	return factory(cfg, prov)
}

// Runner executes configured evaluators and merges their results.
type Runner struct {
	repo     *Registry
	provider provider.Provider
}

// NewRunner constructs a runner.
func NewRunner(repo *Registry, prov provider.Provider) *Runner {
	return &Runner{repo: repo, provider: prov}
}

// Evaluate runs the configured evaluators and merges aggregate metrics.
func (r *Runner) Evaluate(ctx context.Context, spec experiment.ExperimentSpec, results []worker.Result) (map[string]float64, []worker.Result, error) {
	updated := make([]worker.Result, len(results))
	copy(updated, results)
	aggregates := make(map[string]float64)

	for _, cfg := range spec.Evaluators {
		ev, err := r.repo.Instantiate(cfg, r.provider)
		if err != nil {
			return nil, nil, err
		}
		outcome, err := ev.Evaluate(ctx, spec, updated)
		if err != nil {
			return nil, nil, err
		}
		weight := cfg.Weight
		if weight == 0 {
			weight = 1
		}
		for idx, scores := range outcome.Scores {
			if idx < 0 || idx >= len(updated) {
				continue
			}
			if updated[idx].Scores == nil {
				updated[idx].Scores = make(map[string]float64)
			}
			for metric, val := range scores {
				updated[idx].Scores[metric] = val
			}
		}
		for metric, value := range outcome.Aggregate {
			aggregates[metric] += value * weight
		}
	}

	return aggregates, updated, nil
}
