package experiment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"intelligence.dev/internal/playground/dataset"
	"intelligence.dev/internal/playground/registry"
)

// Variant expresses a specific prompt + model configuration.
type Variant struct {
	ID              string                             `json:"id"`
	PromptVersionID string                             `json:"promptVersionId"`
	Model           string                             `json:"model"`
	Params          map[string]any                     `json:"params"`
	PromptTemplate  string                             `json:"promptTemplate"`
	Variables       map[string]registry.VariableSchema `json:"variables"`
}

// EvaluatorConfig specifies an evaluator by name and parameters.
type EvaluatorConfig struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
	Weight float64        `json:"weight"`
}

// BudgetConfig controls token and cost ceilings.
type BudgetConfig struct {
	MaxTokens int     `json:"maxTokens"`
	MaxCost   float64 `json:"maxCost"`
}

// ConcurrencyConfig influences sharding behaviour.
type ConcurrencyConfig struct {
	MaxWorkers        int `json:"maxWorkers"`
	MaxRowsPerShard   int `json:"maxRowsPerShard"`
	MaxVariantsPerRun int `json:"maxVariantsPerRun"`
}

// ExperimentSpec captures how to execute a run against a dataset.
type ExperimentSpec struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"projectId"`
	Name        string            `json:"name"`
	DatasetID   string            `json:"datasetId"`
	SnapshotID  string            `json:"snapshotId"`
	SliceExpr   string            `json:"sliceExpr"`
	Variants    []Variant         `json:"variants"`
	Evaluators  []EvaluatorConfig `json:"evaluators"`
	Budgets     BudgetConfig      `json:"budgets"`
	Concurrency ConcurrencyConfig `json:"concurrency"`
	CreatedAt   time.Time         `json:"createdAt"`
	CreatedBy   string            `json:"createdBy"`
}

// Shard groups rows for concurrent execution.
type Shard struct {
	ID       string        `json:"id"`
	Rows     []dataset.Row `json:"rows"`
	Variants []Variant     `json:"variants"`
}

// RunPlan is the output of the planner.
type RunPlan struct {
	Shards []Shard `json:"shards"`
}

// PlannerConfig tunes the planner heuristics.
type PlannerConfig struct {
	MaxRowsPerShard     int
	MaxVariantsPerShard int
}

// Planner splits rows into shards for execution.
type Planner struct {
	cfg PlannerConfig
}

// NewPlanner creates a planner with sane defaults if needed.
func NewPlanner(cfg PlannerConfig) *Planner {
	if cfg.MaxRowsPerShard <= 0 {
		cfg.MaxRowsPerShard = 32
	}
	if cfg.MaxVariantsPerShard <= 0 {
		cfg.MaxVariantsPerShard = 4
	}
	return &Planner{cfg: cfg}
}

// Plan builds a run plan for the provided experiment and rows.
func (p *Planner) Plan(ctx context.Context, spec ExperimentSpec, rows []dataset.Row) (RunPlan, error) {
	if len(spec.Variants) == 0 {
		return RunPlan{}, errors.New("playground/experiment: at least one variant required")
	}
	select {
	case <-ctx.Done():
		return RunPlan{}, ctx.Err()
	default:
	}

	shards := make([]Shard, 0)
	chunkSize := p.cfg.MaxRowsPerShard
	if spec.Concurrency.MaxRowsPerShard > 0 && spec.Concurrency.MaxRowsPerShard < chunkSize {
		chunkSize = spec.Concurrency.MaxRowsPerShard
	}

	variantLimit := p.cfg.MaxVariantsPerShard
	if spec.Concurrency.MaxVariantsPerRun > 0 && spec.Concurrency.MaxVariantsPerRun < variantLimit {
		variantLimit = spec.Concurrency.MaxVariantsPerRun
	}

	variants := spec.Variants
	for i := 0; i < len(rows); i += chunkSize {
		end := i + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		shardVariants := variants
		if len(variants) > variantLimit {
			shardVariants = variants[:variantLimit]
		}
		shard := Shard{
			ID:       fmt.Sprintf("shard-%d", len(shards)+1),
			Rows:     append([]dataset.Row(nil), rows[i:end]...),
			Variants: cloneVariants(shardVariants),
		}
		shards = append(shards, shard)
	}

	return RunPlan{Shards: shards}, nil
}

func cloneVariants(in []Variant) []Variant {
	out := make([]Variant, len(in))
	copy(out, in)
	return out
}

// Repository stores experiment specs for quick lookup.
type Repository struct {
	experiments map[string]ExperimentSpec
}

// NewRepository initializes an empty repository.
func NewRepository() *Repository {
	return &Repository{experiments: make(map[string]ExperimentSpec)}
}

// Save stores or replaces an experiment spec.
func (r *Repository) Save(spec ExperimentSpec) {
	r.experiments[spec.ID] = spec
}

// Get fetches a spec by ID.
func (r *Repository) Get(id string) (ExperimentSpec, bool) {
	spec, ok := r.experiments[id]
	return spec, ok
}

// Delete removes an experiment spec from the repository cache.
func (r *Repository) Delete(id string) {
	delete(r.experiments, id)
}
