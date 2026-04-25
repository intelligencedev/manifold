package gates

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"manifold/internal/codeqa"
	"manifold/internal/codeqa/workspace"
)

type fakeRunner struct {
	run func(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error)
}

func (f fakeRunner) Run(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error) {
	return f.run(ctx, dir, req)
}

func (f fakeRunner) LookPath(file string) (string, error) {
	if file == "missing" {
		return "", errors.New("missing")
	}
	return "/usr/bin/" + file, nil
}

type fakeGate struct{ ok bool }

func (g fakeGate) Name() string { return "fake_gate" }

func (g fakeGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	return codeqa.GateResult{Name: "fake_gate", OK: g.ok}, nil
}

func TestGoFmtGateDetectsUnformattedFiles(t *testing.T) {
	gate := NewGoFmtGate()
	runner := fakeRunner{run: func(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error) {
		if req.Command != "gofmt" {
			t.Fatalf("expected gofmt command, got %s", req.Command)
		}
		return codeqa.CommandResult{OK: true, Stdout: "a.go\nb.go\n", DurationMs: 5}, nil
	}}
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "a.go"), []byte("package gates\n"), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	result, err := gate.Run(t.Context(), tempDir, runner)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.OK {
		t.Fatalf("expected formatting gate to fail")
	}
	if result.Metrics["unformatted_files"] != 2 {
		t.Fatalf("expected 2 unformatted files, got %v", result.Metrics)
	}
}

func TestRunnerEvaluateMarksHeadFailureHardFail(t *testing.T) {
	runner := fakeRunner{run: func(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error) {
		return codeqa.CommandResult{OK: true}, nil
	}}
	repo := initGitRepo(t)
	writeTestFile(t, repo, "a.txt", "one")
	gitCmd(t, repo, "add", ".")
	gitCmd(t, repo, "commit", "-m", "base")
	writeTestFile(t, repo, "a.txt", "two")
	gitCmd(t, repo, "commit", "-am", "head")
	factory := workspace.NewFactory(codeqa.Options{ArtifactDir: t.TempDir()})
	results, err := NewRunner(runner, 1, factory, fakeGate{ok: false}).Evaluate(t.Context(), repo, "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].HardFail {
		t.Fatalf("did not expect base ref to hard fail")
	}
	if !results[1].HardFail {
		t.Fatalf("expected head ref to hard fail")
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitCmd(t, repo, "init")
	gitCmd(t, repo, "config", "user.email", "codeqa@example.com")
	gitCmd(t, repo, "config", "user.name", "CodeQA")
	return repo
}

func writeTestFile(t *testing.T, repo string, rel string, content string) {
	t.Helper()
	fullPath := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}
