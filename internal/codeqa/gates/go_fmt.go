package gates

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"manifold/internal/codeqa"
)

type goFmtGate struct{}

func NewGoFmtGate() Gate { return goFmtGate{} }

func (goFmtGate) Name() string { return "go_fmt" }

func (goFmtGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("gofmt"); err != nil {
		return codeqa.GateResult{Name: "go_fmt", OK: false, Stderr: err.Error()}, nil
	}
	files := make([]string, 0, 64)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "dist" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	if len(files) == 0 {
		return codeqa.GateResult{Name: "go_fmt", OK: true}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "gofmt", Args: append([]string{"-l"}, files...), Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	outLines := 0
	for line := range strings.SplitSeq(strings.TrimSpace(res.Stdout), "\n") {
		if strings.TrimSpace(line) != "" {
			outLines++
		}
	}
	result := codeqa.GateResult{Name: "go_fmt", OK: outLines == 0, Stdout: res.Stdout, Stderr: res.Stderr, DurationMs: res.DurationMs}
	if outLines > 0 {
		result.Metrics = map[string]float64{"unformatted_files": float64(outLines)}
	}
	return result, nil
}
