package codeevolve

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/tools"
)

// ToolName is the public name exposed to the agent.
const ToolName = "code_evolve"

// Tool implements the AlphaEvolve-style code evolution pipeline as a Manifold tool.
//
// It wraps the evolve.RunAlphaEvolve helper and wires it to the currently
// configured LLM provider, respecting sandbox base directories and provider
// overrides propagated through context.
type Tool struct {
	Cfg      *config.Config
	Provider llm.Provider
}

// New constructs a new code_evolve tool bound to the shared config and LLM provider.
func New(cfg *config.Config, provider llm.Provider) *Tool {
	return &Tool{Cfg: cfg, Provider: provider}
}

func (t *Tool) Name() string { return ToolName }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Run an AlphaEvolve-style code evolution loop on a target file using the current LLM.",
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{"file_path"},
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the source file to evolve, relative to WORKDIR.",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Optional problem context or task description to guide evolution.",
				},
				"generations": map[string]any{
					"type":        "integer",
					"description": "Maximum number of evolutionary generations to run (default 1).",
				},
			},
		},
	}
}

// callArgs mirrors the JSON payload accepted by the tool.
type callArgs struct {
	FilePath    string `json:"file_path"`
	Context     string `json:"context"`
	Generations int    `json:"generations"`
}

// callResult is a compact summary of the evolutionary run.
type callResult struct {
	OK          bool               `json:"ok"`
	Error       string             `json:"error,omitempty"`
	BestScore   float64            `json:"best_score,omitempty"`
	Generations int                `json:"generations_run,omitempty"`
	BestCode    string             `json:"best_code,omitempty"`
	BestID      string             `json:"best_id,omitempty"`
	ParentID    string             `json:"parent_id,omitempty"`
	Meta        map[string]any     `json:"meta,omitempty"`
	Scores      map[string]float64 `json:"scores,omitempty"`
}

// toolLLMClient adapts the shared llm.Provider interface to evolve.LLMClient.
type toolLLMClient struct {
	provider llm.Provider
	model    string
}

func (c *toolLLMClient) Generate(ctx context.Context, prompt string) (string, error) {
	msgs := []llm.Message{{Role: "user", Content: prompt}}
	resp, err := c.provider.Chat(ctx, msgs, nil, c.model)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (c *toolLLMClient) ModelName() string { return c.model }

// Call executes a single AlphaEvolve run over the requested file.
func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args callArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.FilePath == "" {
		return callResult{OK: false, Error: "file_path is required"}, nil
	}
	if args.Generations <= 0 {
		args.Generations = 1
	}

	// Resolve per-call base directory and sanitize the file path so evolution
	// always happens inside the sandboxed WORKDIR.
	base := sandbox.ResolveBaseDir(ctx, t.Cfg.Workdir)
	rel, err := sandbox.SanitizeArg(base, args.FilePath)
	if err != nil {
		return callResult{OK: false, Error: err.Error()}, nil
	}
	fullPath := filepath.Join(base, rel)

	// Prefer provider/model from context when available so the tool matches
	// the invoking agent/specialist configuration.
	p := tools.ProviderFromContext(ctx)
	if p == nil {
		p = t.Provider
	}
	if p == nil {
		return callResult{OK: false, Error: "no LLM provider available"}, nil
	}
	model := t.Cfg.OpenAI.Model
	if model == "" {
		model = t.Cfg.LLMClient.OpenAI.Model
	}
	// An empty model string signals to Provider.Chat to use its own default.
	llmClient := &toolLLMClient{provider: p, model: model}
	db := NewInMemoryDB()

	start := time.Now()

	best, err := RunAlphaEvolve(ctx, fullPath, args.Context, nil, llmClient, db, args.Generations, nil)
	if err != nil {
		return callResult{OK: false, Error: err.Error()}, nil
	}

	elapsed := time.Since(start)

	meta := map[string]any{
		"run_id":      uuid.NewString(),
		"duration_ms": elapsed.Milliseconds(),
		"llm_model":   llmClient.ModelName(),
		"file_path":   args.FilePath,
	}

	res := callResult{
		OK:          true,
		BestScore:   best.Scores["score"],
		Generations: best.Generation,
		BestCode:    best.Code,
		BestID:      best.ID,
		ParentID:    best.ParentID,
		Meta:        meta,
		Scores:      best.Scores,
	}
	return res, nil
}
