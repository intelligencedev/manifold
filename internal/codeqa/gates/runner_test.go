package gates

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"manifold/internal/codeqa"
	"manifold/internal/codeqa/lang"
	"manifold/internal/codeqa/workspace"
)

type fakeRunner struct {
	run      func(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error)
	lookPath func(file string) (string, error)
}

func (f fakeRunner) Run(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error) {
	return f.run(ctx, dir, req)
}

func (f fakeRunner) LookPath(file string) (string, error) {
	if f.lookPath != nil {
		return f.lookPath(file)
	}
	if file == "missing" {
		return "", errors.New("missing")
	}
	return "/usr/bin/" + file, nil
}

type fakeGate struct {
	name string
	ok   bool
	run  func(dir string)
}

func (g fakeGate) Name() string {
	if g.name != "" {
		return g.name
	}
	return "fake_gate"
}

func (g fakeGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if g.run != nil {
		g.run(dir)
	}
	return codeqa.GateResult{Name: g.Name(), OK: g.ok}, nil
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

func TestGatesForLanguagesDedupesSharedGates(t *testing.T) {
	gates := GatesForLanguages([]lang.Language{lang.TypeScript, lang.CSS, lang.HTML})
	names := make([]string, 0, len(gates))
	for _, gate := range gates {
		names = append(names, gate.Name())
	}
	want := []string{"prettier_check", "eslint", "tsc", "npm_test", "stylelint", "html_validate"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("gate names = %v, want %v", names, want)
	}
}

func TestGatesForLanguagesIncludesRust(t *testing.T) {
	gates := GatesForLanguages([]lang.Language{lang.Rust})
	names := make([]string, 0, len(gates))
	for _, gate := range gates {
		names = append(names, gate.Name())
	}
	want := []string{"cargo_fmt", "clippy", "cargo_build", "cargo_test"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("gate names = %v, want %v", names, want)
	}
}

func TestPythonRuffCheckGateRunsAgainstPythonFiles(t *testing.T) {
	tempDir := t.TempDir()
	writeTestFile(t, tempDir, "app.py", "print('hello')\n")
	var got codeqa.CommandRequest
	runner := fakeRunner{run: func(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error) {
		got = req
		return codeqa.CommandResult{OK: true, DurationMs: 7}, nil
	}}
	result, err := NewPythonRuffCheckGate().Run(t.Context(), tempDir, runner)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.OK || result.Name != "ruff_check" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got.Command != "ruff" || !reflect.DeepEqual(got.Args, []string{"check", "app.py"}) {
		t.Fatalf("unexpected command: %+v", got)
	}
}

func TestTypeScriptCheckGateSkipsWithoutTSConfig(t *testing.T) {
	result, err := NewTypeScriptCheckGate().Run(t.Context(), t.TempDir(), fakeRunner{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.OK || !result.Skipped {
		t.Fatalf("expected skipped pass, got %+v", result)
	}
}

func TestPrettierCheckGateFailsWhenNPXMissing(t *testing.T) {
	tempDir := t.TempDir()
	writeTestFile(t, tempDir, "app.ts", "const x = 1\n")
	runner := fakeRunner{lookPath: func(file string) (string, error) {
		return "", errors.New("missing")
	}}
	result, err := NewPrettierCheckGate().Run(t.Context(), tempDir, runner)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.OK {
		t.Fatalf("expected missing npx to fail the gate, got %+v", result)
	}
}

func TestCargoBuildGateRunsCargoBuild(t *testing.T) {
	var got codeqa.CommandRequest
	runner := fakeRunner{run: func(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error) {
		got = req
		return codeqa.CommandResult{OK: true, DurationMs: 9}, nil
	}}
	result, err := NewCargoBuildGate().Run(t.Context(), t.TempDir(), runner)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.OK || result.Name != "cargo_build" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got.Command != "cargo" || !reflect.DeepEqual(got.Args, []string{"build", "--quiet"}) {
		t.Fatalf("unexpected command: %+v", got)
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

func TestRunnerEvaluateUsesIsolatedWorkspacePerGate(t *testing.T) {
	runner := fakeRunner{run: func(ctx context.Context, dir string, req codeqa.CommandRequest) (codeqa.CommandResult, error) {
		return codeqa.CommandResult{OK: true}, nil
	}}
	repo := initGitRepo(t)
	writeTestFile(t, repo, "a.txt", "one")
	gitCmd(t, repo, "add", ".")
	gitCmd(t, repo, "commit", "-m", "base")
	writeTestFile(t, repo, "a.txt", "two")
	gitCmd(t, repo, "commit", "-am", "head")

	var mu sync.Mutex
	seen := map[string]struct{}{}
	record := func(dir string) {
		mu.Lock()
		defer mu.Unlock()
		if _, ok := seen[dir]; ok {
			t.Errorf("workspace reused by multiple gates: %s", dir)
		}
		seen[dir] = struct{}{}
	}

	factory := workspace.NewFactory(codeqa.Options{ArtifactDir: t.TempDir()})
	results, err := NewRunner(
		runner,
		2,
		factory,
		fakeGate{name: "gate_a", ok: true, run: record},
		fakeGate{name: "gate_b", ok: true, run: record},
	).Evaluate(t.Context(), repo, "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 distinct workspaces, got %d: %#v", len(seen), seen)
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
