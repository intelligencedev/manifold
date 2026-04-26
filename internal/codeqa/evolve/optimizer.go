package evolve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"manifold/internal/codeqa"
	"manifold/internal/codeqa/lang"
	"manifold/internal/llm"
	"manifold/internal/sandbox"
)

const (
	defaultMaxIterations = 3
	maxTargetFiles       = 4
	maxTargetFileBytes   = 16 * 1024
)

type Optimizer struct {
	opts     codeqa.Options
	runner   codeqa.CommandRunner
	provider llm.Provider
}

type Candidate struct {
	Summary     string
	EditedFiles []string
	Prompt      string
	Response    string
}

type optimizeResponse struct {
	Summary string         `json:"summary"`
	Edits   []proposedEdit `json:"edits"`
}

type proposedEdit struct {
	Path    string `json:"path"`
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

func NewOptimizer(opts codeqa.Options, runner codeqa.CommandRunner, provider llm.Provider) *Optimizer {
	return &Optimizer{opts: opts, runner: runner, provider: provider}
}

func NormalizeMaxIterations(iterations int) int {
	if iterations <= 0 {
		return 1
	}
	if iterations > defaultMaxIterations {
		return defaultMaxIterations
	}
	return iterations
}

func (o *Optimizer) GenerateCandidate(ctx context.Context, repoPath string, req codeqa.RunRequest, iteration int) (Candidate, error) {
	if o.provider == nil {
		return Candidate{}, errors.New("optimizer requires an llm provider")
	}
	targets, err := o.resolveTargets(repoPath, req.TargetPaths)
	if err != nil {
		return Candidate{}, err
	}
	prompt, err := o.buildPrompt(repoPath, targets, req.Objective, iteration, NormalizeMaxIterations(req.MaxIterations))
	if err != nil {
		return Candidate{}, err
	}
	resp, err := o.provider.Chat(ctx, []llm.Message{{Role: "user", Content: prompt}}, nil, o.opts.ProposerModel)
	if err != nil {
		return Candidate{}, fmt.Errorf("generate candidate edits: %w", err)
	}
	parsed, err := o.parseResponse(ctx, resp.Content, targets)
	if err != nil {
		return Candidate{}, err
	}
	editedFiles, err := o.applyEdits(repoPath, parsed.Edits)
	if err != nil {
		return Candidate{}, err
	}
	if len(editedFiles) == 0 {
		return Candidate{}, errors.New("optimizer produced no applicable edits")
	}
	o.formatEditedFiles(ctx, repoPath, editedFiles)
	if err := o.commitCandidate(ctx, repoPath, iteration, parsed.Summary, editedFiles); err != nil {
		return Candidate{}, err
	}
	return Candidate{Summary: strings.TrimSpace(parsed.Summary), EditedFiles: editedFiles, Prompt: prompt, Response: resp.Content}, nil
}

func (o *Optimizer) resolveTargets(repoPath string, targetPaths []string) ([]string, error) {
	if len(targetPaths) == 0 {
		return nil, errors.New("optimize mode requires target_paths")
	}
	seen := make(map[string]struct{}, len(targetPaths))
	targets := make([]string, 0, len(targetPaths))
	for _, candidate := range targetPaths {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		rel, err := sandbox.SanitizeArg(repoPath, candidate)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
		if rel == "" {
			continue
		}
		if codeqa.MatchAny(rel, o.opts.ForbiddenGlobs) {
			return nil, fmt.Errorf("target path is forbidden: %s", rel)
		}
		if _, exists := seen[rel]; exists {
			continue
		}
		fullPath := filepath.Join(repoPath, rel)
		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("stat target path %s: %w", rel, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("target path must be a file: %s", rel)
		}
		seen[rel] = struct{}{}
		targets = append(targets, rel)
	}
	if len(targets) == 0 {
		return nil, errors.New("optimize mode requires at least one valid target path")
	}
	if len(targets) > maxTargetFiles {
		return nil, fmt.Errorf("optimize mode supports at most %d target files", maxTargetFiles)
	}
	return targets, nil
}

func (o *Optimizer) buildPrompt(repoPath string, targets []string, objective string, iteration int, totalIterations int) (string, error) {
	var b strings.Builder
	b.WriteString("You are improving code quality inside a sandboxed git worktree.\n\n")
	b.WriteString("Return strict JSON only with this schema:\n")
	b.WriteString("{\n  \"summary\": \"short rationale\",\n  \"edits\": [\n    {\n      \"path\": \"relative/path.ext\",\n      \"search\": \"exact existing snippet\",\n      \"replace\": \"replacement snippet\"\n    }\n  ]\n}\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Modify only the provided target files.\n")
	b.WriteString("- Keep behavior stable unless the objective explicitly says otherwise.\n")
	b.WriteString("- Prefer small, surgical edits that improve correctness, maintainability, tests, or readability.\n")
	b.WriteString("- Every search string must match exactly once in the target file.\n")
	b.WriteString("- Do not emit markdown fences or prose.\n\n")
	if strings.TrimSpace(objective) == "" {
		objective = "Improve code quality while preserving behavior."
	}
	b.WriteString(fmt.Sprintf("Objective: %s\n", strings.TrimSpace(objective)))
	b.WriteString(fmt.Sprintf("Iteration: %d/%d\n\n", iteration+1, totalIterations))
	for _, rel := range targets {
		fullPath := filepath.Join(repoPath, rel)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return "", fmt.Errorf("read target file %s: %w", rel, err)
		}
		if len(data) > maxTargetFileBytes {
			data = data[:maxTargetFileBytes]
		}
		b.WriteString("FILE: " + rel + "\n")
		b.WriteString("```\n")
		b.Write(data)
		if len(data) == 0 || data[len(data)-1] != '\n' {
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
	}
	return b.String(), nil
}

func (o *Optimizer) parseResponse(ctx context.Context, raw string, targets []string) (optimizeResponse, error) {
	var parsed optimizeResponse
	if err := parseJSONResponse(raw, &parsed); err != nil {
		if o.provider == nil {
			return optimizeResponse{}, err
		}
		repairResp, repairErr := o.provider.Chat(ctx, []llm.Message{{Role: "user", Content: "Repair the following response into strict JSON only. Do not add commentary.\n\n" + raw}}, nil, o.opts.ProposerModel)
		if repairErr != nil {
			return optimizeResponse{}, err
		}
		if err := parseJSONResponse(repairResp.Content, &parsed); err != nil {
			return optimizeResponse{}, err
		}
	}
	if len(parsed.Edits) == 0 {
		return optimizeResponse{}, errors.New("optimizer response contained no edits")
	}
	allowed := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		allowed[target] = struct{}{}
	}
	for idx := range parsed.Edits {
		parsed.Edits[idx].Path = filepath.ToSlash(strings.TrimSpace(parsed.Edits[idx].Path))
		parsed.Edits[idx].Search = strings.TrimSpace(parsed.Edits[idx].Search)
		if _, ok := allowed[parsed.Edits[idx].Path]; !ok {
			return optimizeResponse{}, fmt.Errorf("optimizer edit references unexpected path: %s", parsed.Edits[idx].Path)
		}
		if parsed.Edits[idx].Search == "" {
			return optimizeResponse{}, fmt.Errorf("optimizer edit for %s is missing search text", parsed.Edits[idx].Path)
		}
	}
	return parsed, nil
}

func parseJSONResponse(raw string, target *optimizeResponse) error {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end < start {
		return errors.New("response did not contain JSON object")
	}
	return json.Unmarshal([]byte(raw[start:end+1]), target)
}

func (o *Optimizer) applyEdits(repoPath string, edits []proposedEdit) ([]string, error) {
	changed := make([]string, 0, len(edits))
	seen := make(map[string]struct{}, len(edits))
	for _, edit := range edits {
		fullPath := filepath.Join(repoPath, filepath.FromSlash(edit.Path))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read edit target %s: %w", edit.Path, err)
		}
		content := string(data)
		occurrences := strings.Count(content, edit.Search)
		if occurrences != 1 {
			return nil, fmt.Errorf("optimizer search snippet for %s matched %d times", edit.Path, occurrences)
		}
		updated := strings.Replace(content, edit.Search, edit.Replace, 1)
		if updated == content {
			continue
		}
		if err := os.WriteFile(fullPath, []byte(updated), 0o644); err != nil {
			return nil, fmt.Errorf("write edit target %s: %w", edit.Path, err)
		}
		if _, ok := seen[edit.Path]; !ok {
			seen[edit.Path] = struct{}{}
			changed = append(changed, edit.Path)
		}
	}
	return changed, nil
}

func (o *Optimizer) formatEditedFiles(ctx context.Context, repoPath string, editedFiles []string) {
	byLanguage := map[lang.Language][]string{}
	for _, rel := range editedFiles {
		language := lang.FromPath(rel)
		if language == "" {
			continue
		}
		byLanguage[language] = append(byLanguage[language], rel)
	}
	o.runFormatter(ctx, repoPath, "gofmt", []string{"-w"}, byLanguage[lang.Go])
	o.runFormatter(ctx, repoPath, "ruff", []string{"format"}, byLanguage[lang.Python])
	prettierFiles := append([]string{}, byLanguage[lang.JavaScript]...)
	prettierFiles = append(prettierFiles, byLanguage[lang.TypeScript]...)
	prettierFiles = append(prettierFiles, byLanguage[lang.CSS]...)
	prettierFiles = append(prettierFiles, byLanguage[lang.HTML]...)
	o.runFormatter(ctx, repoPath, "npx", append([]string{"--no-install", "prettier", "--write"}, prettierFiles...), nil)
	o.runFormatter(ctx, repoPath, "rustfmt", nil, byLanguage[lang.Rust])
}

func (o *Optimizer) runFormatter(ctx context.Context, repoPath string, command string, args []string, files []string) {
	if len(files) == 0 && command != "npx" {
		return
	}
	if command == "npx" {
		if len(args) <= 3 {
			return
		}
	} else {
		args = append(args, files...)
	}
	if _, err := o.runner.LookPath(command); err != nil {
		return
	}
	_, _ = o.runner.Run(ctx, repoPath, codeqa.CommandRequest{Command: command, Args: args})
}

func (o *Optimizer) commitCandidate(ctx context.Context, repoPath string, iteration int, summary string, editedFiles []string) error {
	args := append([]string{"add", "--"}, editedFiles...)
	if _, err := o.runner.Run(ctx, repoPath, codeqa.CommandRequest{Command: "git", Args: args}); err != nil {
		return fmt.Errorf("stage candidate edits: %w", err)
	}
	status, err := o.runner.Run(ctx, repoPath, codeqa.CommandRequest{Command: "git", Args: []string{"diff", "--cached", "--name-only"}})
	if err != nil {
		return fmt.Errorf("inspect staged candidate diff: %w", err)
	}
	if strings.TrimSpace(status.Stdout) == "" {
		return errors.New("optimizer produced no staged diff")
	}
	message := strings.TrimSpace(summary)
	if message == "" {
		message = fmt.Sprintf("codeqa optimizer candidate %d", iteration+1)
	}
	if _, err := o.runner.Run(ctx, repoPath, codeqa.CommandRequest{Command: "git", Args: []string{"commit", "--no-verify", "-m", message}}); err != nil {
		return fmt.Errorf("commit candidate diff: %w", err)
	}
	return nil
}
