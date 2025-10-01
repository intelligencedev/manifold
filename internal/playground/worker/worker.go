package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/playground/artifacts"
	"manifold/internal/playground/dataset"
	"manifold/internal/playground/experiment"
	"manifold/internal/playground/provider"
	"manifold/internal/playground/registry"
)

// Task represents a unit of work for a worker.
type Task struct {
	RunID          string
	ShardID        string
	Variant        experiment.Variant
	Row            dataset.Row
	PromptTemplate string
	Variables      map[string]registry.VariableSchema
}

// Result contains the output from executing a task.
type Result struct {
	ID              string
	RunID           string
	ShardID         string
	RowID           string
	VariantID       string
	PromptVersionID string
	Model           string
	RenderedPrompt  string
	Output          string
	Tokens          int
	Latency         time.Duration
	ProviderName    string
	Artifacts       map[string]string
	Expected        any
	Scores          map[string]float64
}

// Executor defines the worker behaviour required by the service.
type Executor interface {
	ExecuteTask(ctx context.Context, task Task) (Result, error)
}

// Worker executes prompt rendering against a provider.
type Worker struct {
	provider  provider.Provider
	artifacts artifacts.Store
}

// NewWorker constructs a worker.
func NewWorker(provider provider.Provider, artifacts artifacts.Store) *Worker {
	return &Worker{provider: provider, artifacts: artifacts}
}

// ExecuteTask renders the prompt, invokes the provider, and persists artifacts.
func (w *Worker) ExecuteTask(ctx context.Context, task Task) (Result, error) {
	rendered, err := renderTemplate(task.PromptTemplate, task.Row.Inputs)
	if err != nil {
		return Result{}, fmt.Errorf("render template: %w", err)
	}

	resp, err := w.provider.Complete(ctx, provider.Request{
		Model:  task.Variant.Model,
		Prompt: rendered,
		Inputs: task.Row.Inputs,
		Params: task.Variant.Params,
	})
	if err != nil {
		return Result{}, fmt.Errorf("provider execute: %w", err)
	}

	ares := make(map[string]string)
	if w.artifacts != nil {
		artifactName := fmt.Sprintf("%s-%s.txt", task.ShardID, task.Row.ID)
		path, storeErr := w.artifacts.Save(ctx, task.RunID, artifacts.Artifact{
			Name:        artifactName,
			ContentType: "text/plain",
			Bytes:       []byte(resp.Output),
		})
		if storeErr == nil {
			ares[artifactName] = path
		}
	}

	return Result{
		ID:              uuid.NewString(),
		RunID:           task.RunID,
		ShardID:         task.ShardID,
		RowID:           task.Row.ID,
		VariantID:       task.Variant.ID,
		PromptVersionID: task.Variant.PromptVersionID,
		Model:           task.Variant.Model,
		RenderedPrompt:  rendered,
		Output:          resp.Output,
		Tokens:          resp.Tokens,
		Latency:         resp.Latency,
		ProviderName:    resp.ProviderName,
		Artifacts:       ares,
		Expected:        task.Row.Expected,
	}, nil
}

func renderTemplate(template string, inputs map[string]any) (string, error) {
	rendered := template
	for key, value := range inputs {
		placeholder := fmt.Sprintf("{{%s}}", key)
		rendered = strings.ReplaceAll(rendered, placeholder, fmt.Sprintf("%v", value))
	}
	if strings.Contains(rendered, "{{") {
		return "", fmt.Errorf("unbound placeholders remain in template")
	}
	return rendered, nil
}

// NewRunID generates an identifier for runs.
func NewRunID() string {
	return uuid.NewString()
}

// TasksFromShard expands a shard into concrete tasks.
func TasksFromShard(runID string, spec experiment.ExperimentSpec, shard experiment.Shard) []Task {
	tasks := make([]Task, 0, len(shard.Rows)*len(shard.Variants))
	for _, row := range shard.Rows {
		for _, variant := range shard.Variants {
			tasks = append(tasks, Task{
				RunID:          runID,
				ShardID:        shard.ID,
				Variant:        variant,
				Row:            row,
				PromptTemplate: variant.PromptTemplate,
				Variables:      variant.Variables,
			})
		}
	}
	return tasks
}
