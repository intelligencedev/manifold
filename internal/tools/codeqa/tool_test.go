package codeqa

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	appcodeqa "manifold/internal/codeqa"
	codeqaservice "manifold/internal/codeqa/service"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools/cli"
)

type fakeProvider struct{}

func (fakeProvider) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
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
	return llm.Message{Role: "assistant", Content: `{"verdict":"tie","confidence":0.9,"scores":{},"evidence":["foo/foo.go"]}`}, nil
}

func (fakeProvider) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	return nil
}

func TestOptimizeToolRunsOptimizerMode(t *testing.T) {
	t.Parallel()

	repo := initGitRepo(t)
	writeRepoFile(t, repo, "go.mod", "module example.com/repo\n\ngo 1.24.5\n")
	writeRepoFile(t, repo, "foo/foo.go", "package foo\n\nfunc Add(a, b int) int { return a + b }\n")
	writeRepoFile(t, repo, "foo/foo_test.go", "package foo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	executor := cli.NewExecutor(testExecConfig(60), repo, 128*1024)
	runner := appcodeqa.NewCLICommandRunner(executor, []string{"git", "go", "gofmt"})
	svc := codeqaservice.New(appcodeqa.Options{
		ArtifactDir:            t.TempDir(),
		DefaultMaxDiffBytes:    128 * 1024,
		DefaultMaxChangedFiles: 12,
		AcceptThreshold:        0.10,
		MinConfidence:          0.70,
		JudgeModel:             "fake",
		ProposerModel:          "fake",
		Workdir:                repo,
	}, runner, fakeProvider{}, nil)

	tool := NewOptimize(&config.Config{Workdir: repo}, svc)
	payload, err := json.Marshal(appcodeqa.RunRequest{TargetPaths: []string{"foo/foo.go"}, Objective: "Add a doc comment.", MaxIterations: 1})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	raw, err := tool.Call(context.Background(), payload)
	if err != nil {
		t.Fatalf("tool call failed: %v", err)
	}
	result, ok := raw.(appcodeqa.RunResult)
	if !ok {
		t.Fatalf("unexpected result type %T", raw)
	}
	if result.Mode != appcodeqa.ModeOptimize {
		t.Fatalf("expected optimize mode, got %s", result.Mode)
	}
	if result.Aggregate.Action != appcodeqa.ActionAccept {
		t.Fatalf("expected accept action, got %s", result.Aggregate.Action)
	}
	if _, ok := result.Artifacts["patches/iteration-000.patch"]; !ok {
		t.Fatalf("expected iteration patch artifact, got %+v", result.Artifacts)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "codeqa@example.com")
	git(t, repo, "config", "user.name", "CodeQA")
	return repo
}

func writeRepoFile(t *testing.T, repo string, rel string, content string) {
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
