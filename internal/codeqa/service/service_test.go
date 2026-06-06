package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"manifold/internal/codeqa"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools/cli"
)

type fakeProvider struct{ response string }

func (f fakeProvider) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	if f.response == "__AUTO_POSITIVE__" {
		prompt := ""
		if len(msgs) > 0 {
			prompt = msgs[len(msgs)-1].Content
		}
		if strings.Contains(prompt, "You are improving code quality inside a sandboxed git worktree.") {
			return llm.Message{Role: "assistant", Content: `{"summary":"add documentation","edits":[{"path":"foo/foo.go","search":"package foo\n\nfunc Add(a, b int) int { return a + b }","replace":"package foo\n\n// Add returns a stable sum.\nfunc Add(a, b int) int { return a + b }"}]}`}, nil
		}
		if strings.Contains(prompt, "OPTION_A = CANDIDATE") {
			return llm.Message{Role: "assistant", Content: `{"verdict":"option_a_better","confidence":0.9,"scores":{"correctness":0.4,"maintainability":0.4,"test_quality":0.2},"evidence":["foo/foo.go"]}`}, nil
		}
		if strings.Contains(prompt, "OPTION_B = CANDIDATE") {
			return llm.Message{Role: "assistant", Content: `{"verdict":"option_b_better","confidence":0.9,"scores":{"correctness":-0.4,"maintainability":-0.4,"test_quality":-0.2},"evidence":["foo/foo.go"]}`}, nil
		}
		return llm.Message{Role: "assistant", Content: `{"verdict":"tie","confidence":0.9,"scores":{"correctness":0.0,"maintainability":0.0,"test_quality":0.0},"evidence":["foo/foo.go"]}`}, nil
	}
	return llm.Message{Role: "assistant", Content: f.response}, nil
}

func (f fakeProvider) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	return nil
}

func TestServiceRunAcceptsCandidate(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)
	setupGoodRepo(t, repo)
	git(t, repo, "commit", "-am", "base")
	writeFile(t, repo, "foo/foo.go", "package foo\n\n// Add returns a stable sum.\nfunc Add(a, b int) int { return a + b }\n")
	git(t, repo, "commit", "-am", "head")

	service := newTestService(repo, t.TempDir(), "__AUTO_POSITIVE__")
	result, err := service.Run(t.Context(), 0, codeqa.RunRequest{RepositoryPath: repo, BaseRef: "HEAD~1", HeadRef: "HEAD", IncludeRepoContext: true}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Aggregate.Action != codeqa.ActionAccept {
		t.Fatalf("expected accept, got %s aggregate=%+v judges=%+v gates=%+v", result.Aggregate.Action, result.Aggregate, result.Judges, result.Gates)
	}
	if _, err := os.Stat(result.Artifacts["report.md"]); err != nil {
		t.Fatalf("expected report artifact to exist: %v", err)
	}
}

func TestServiceRunRejectsHardFailingCandidate(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)
	setupGoodRepo(t, repo)
	git(t, repo, "commit", "-am", "base")
	writeFile(t, repo, "foo/foo.go", "package foo\n\nfunc Add(a, b int) int {\n\treturn a +\n}\n")
	git(t, repo, "commit", "-am", "head")

	service := newTestService(repo, t.TempDir(), `{"verdict":"option_b_better","confidence":0.9,"scores":{"correctness":0.4},"evidence":["foo/foo.go"]}`)
	result, err := service.Run(t.Context(), 0, codeqa.RunRequest{RepositoryPath: repo, BaseRef: "HEAD~1", HeadRef: "HEAD"}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Aggregate.Action != codeqa.ActionReject {
		t.Fatalf("expected reject, got %s", result.Aggregate.Action)
	}
	if len(result.Aggregate.HardFailures) == 0 {
		t.Fatalf("expected hard failures to be reported")
	}
}

func TestServiceRunSupportsGateModeIsolation(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)
	setupGoodRepo(t, repo)
	git(t, repo, "commit", "-am", "base")
	writeFile(t, repo, "foo/foo.go", "package foo\n\n// Add returns a stable sum.\nfunc Add(a, b int) int { return a + b }\n")
	git(t, repo, "commit", "-am", "head")

	service := newTestService(repo, t.TempDir(), "__AUTO_POSITIVE__")
	result, err := service.Run(t.Context(), 0, codeqa.RunRequest{Mode: codeqa.ModeGate, RepositoryPath: repo, BaseRef: "HEAD~1", HeadRef: "HEAD"}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Mode != codeqa.ModeGate {
		t.Fatalf("expected gate mode, got %s", result.Mode)
	}
	if result.Status != codeqa.StatusCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if result.Aggregate.Action != codeqa.ActionAccept {
		t.Fatalf("expected accept, got %s", result.Aggregate.Action)
	}
}

func TestServiceRunOptimizeCreatesCandidateArtifacts(t *testing.T) {
	t.Parallel()
	repo := initGitRepo(t)
	setupGoodRepo(t, repo)
	git(t, repo, "commit", "-am", "base")

	service := newTestService(repo, t.TempDir(), "__AUTO_POSITIVE__")
	result, err := service.Run(t.Context(), 0, codeqa.RunRequest{
		Mode:           codeqa.ModeOptimize,
		RepositoryPath: repo,
		Objective:      "Add a doc comment.",
		TargetPaths:    []string{"foo/foo.go"},
		MaxIterations:  1,
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Mode != codeqa.ModeOptimize {
		t.Fatalf("expected optimize mode, got %s", result.Mode)
	}
	if result.Aggregate.Action != codeqa.ActionAccept {
		t.Fatalf("expected accept, got %s", result.Aggregate.Action)
	}
	if _, ok := result.Artifacts["patches/iteration-000.patch"]; !ok {
		t.Fatalf("expected patch artifact, got %+v", result.Artifacts)
	}
	if _, err := os.Stat(result.Artifacts["patches/iteration-000.patch"]); err != nil {
		t.Fatalf("expected saved patch artifact: %v", err)
	}
}

func newTestService(repo string, artifactDir string, providerResponse string) *Service {
	executor := cli.NewExecutor(testExecConfig(60), repo, 128*1024)
	runner := codeqa.NewCLICommandRunner(executor, []string{"go", "gofmt"})
	return New(codeqa.Options{
		ArtifactDir:            artifactDir,
		DefaultMaxDiffBytes:    128 * 1024,
		DefaultMaxChangedFiles: 12,
		AcceptThreshold:        0.10,
		MinConfidence:          0.70,
		JudgeModel:             "fake",
		ProposerModel:          "fake",
		HighRiskGlobs:          []string{"**/auth/**"},
		Workdir:                repo,
	}, runner, fakeProvider{response: providerResponse}, nil)
}

func setupGoodRepo(t *testing.T, repo string) {
	t.Helper()
	writeFile(t, repo, "go.mod", "module example.com/repo\n\ngo 1.24.5\n")
	writeFile(t, repo, "foo/foo.go", "package foo\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFile(t, repo, "foo/foo_test.go", "package foo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	git(t, repo, "add", ".")
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "codeqa@example.com")
	git(t, repo, "config", "user.name", "CodeQA")
	return repo
}

func writeFile(t *testing.T, repo string, rel string, content string) {
	t.Helper()
	fullPath := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}

func testExecConfig(maxCommandSeconds int) config.ExecConfig {
	disabled := false
	return config.ExecConfig{
		MaxCommandSeconds: maxCommandSeconds,
		Sandbox: config.ExecSandboxConfig{
			Enabled: &disabled,
		},
	}
}
