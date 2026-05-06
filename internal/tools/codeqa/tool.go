package codeqa

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	appcodeqa "manifold/internal/codeqa"
	"manifold/internal/codeqa/service"
	"manifold/internal/config"
	"manifold/internal/sandbox"
	"manifold/internal/tools"
)

const (
	ToolNameJudge    = "code_qa_judge"
	ToolNameRun      = "code_qa_run"
	ToolNameOptimize = "code_qa_optimize"
)

type Tool struct {
	service *service.Service
	cfg     *config.Config
	name    string
	mode    appcodeqa.RunMode
}

func NewJudge(cfg *config.Config, svc *service.Service) *Tool {
	return &Tool{service: svc, cfg: cfg, name: ToolNameJudge, mode: appcodeqa.ModeJudge}
}

func NewRun(cfg *config.Config, svc *service.Service) *Tool {
	return &Tool{service: svc, cfg: cfg, name: ToolNameRun, mode: appcodeqa.ModeGate}
}

func NewOptimize(cfg *config.Config, svc *service.Service) *Tool {
	return &Tool{service: svc, cfg: cfg, name: ToolNameOptimize, mode: appcodeqa.ModeOptimize}
}

func (t *Tool) Name() string { return t.name }

func (t *Tool) JSONSchema() map[string]any {
	description := "Evaluate a git diff with deterministic Go gates and an LLM judge, then persist a Markdown report and JSON result."
	if t.mode == appcodeqa.ModeGate {
		description = "Run the code-quality gate in isolated workspace mode and return accept, reject, or human_review with persisted artifacts."
	} else if t.mode == appcodeqa.ModeOptimize {
		description = "Generate and evaluate small candidate edits for explicit target paths, then persist reports, JSON results, and candidate patch artifacts."
	}
	return map[string]any{
		"name":        t.name,
		"description": description,
		"parameters": map[string]any{
			"type":     "object",
			"required": []string{},
			"properties": map[string]any{
				"mode":                 map[string]any{"type": "string", "enum": []string{string(appcodeqa.ModeJudge), string(appcodeqa.ModeGate), string(appcodeqa.ModeOptimize)}},
				"repository_path":      map[string]any{"type": "string", "description": "Optional repo path relative to WORKDIR. Defaults to the current workdir."},
				"base_ref":             map[string]any{"type": "string", "description": "Optional git base ref. Defaults to HEAD~1."},
				"head_ref":             map[string]any{"type": "string", "description": "Optional git head ref. Defaults to HEAD."},
				"objective":            map[string]any{"type": "string", "description": "Goal for optimize mode. Preserve behavior unless this explicitly requests a behavior change."},
				"target_paths":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Explicit file paths optimize mode is allowed to edit."},
				"max_iterations":       map[string]any{"type": "integer", "minimum": 1, "maximum": 3},
				"max_diff_bytes":       map[string]any{"type": "integer"},
				"max_changed_files":    map[string]any{"type": "integer"},
				"include_repo_context": map[string]any{"type": "boolean"},
				"accept_threshold":     map[string]any{"type": "number"},
				"min_confidence":       map[string]any{"type": "number"},
			},
		},
	}
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args appcodeqa.RunRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.Mode == "" {
		args.Mode = t.mode
	}
	base := sandbox.ResolveBaseDir(ctx, t.cfg.Workdir)
	repoPath := base
	if args.RepositoryPath != "" {
		rel, err := sandbox.SanitizeArg(base, args.RepositoryPath)
		if err != nil {
			return nil, err
		}
		repoPath = filepath.Join(base, rel)
	}
	args.RepositoryPath = repoPath
	return t.service.Run(ctx, 0, args, nil)
}

var _ tools.Tool = (*Tool)(nil)
