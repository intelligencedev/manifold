package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"manifold/internal/codeqa"
)

func TestPrepareGateUsesGitWorktree(t *testing.T) {
	t.Parallel()
	repo := initWorkspaceGitRepo(t)
	writeWorkspaceFile(t, repo, "go.mod", "module example.com/repo\n\ngo 1.24.5\n")
	gitWorkspace(t, repo, "add", ".")
	gitWorkspace(t, repo, "commit", "-m", "base")

	factory := NewFactory(codeqa.Options{ArtifactDir: t.TempDir()})
	prepared, err := factory.Prepare(t.Context(), repo, "run-git", codeqa.ModeGate)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	defer prepared.Cleanup()
	if !prepared.IsGit {
		t.Fatalf("expected git workspace")
	}
	if prepared.Path == repo {
		t.Fatalf("expected isolated worktree path")
	}
	if _, err := os.Stat(filepath.Join(prepared.Path, "go.mod")); err != nil {
		t.Fatalf("expected checked out file in worktree: %v", err)
	}
}

func TestPrepareOptimizeRejectsDirtyRepo(t *testing.T) {
	t.Parallel()
	repo := initWorkspaceGitRepo(t)
	writeWorkspaceFile(t, repo, "go.mod", "module example.com/repo\n\ngo 1.24.5\n")
	gitWorkspace(t, repo, "add", ".")
	gitWorkspace(t, repo, "commit", "-m", "base")
	writeWorkspaceFile(t, repo, "dirty.txt", "dirty\n")

	factory := NewFactory(codeqa.Options{ArtifactDir: t.TempDir()})
	if _, err := factory.Prepare(t.Context(), repo, "run-dirty", codeqa.ModeOptimize); err == nil {
		t.Fatalf("expected dirty optimize repo to be rejected")
	}
}

func TestPrepareCopiesNonGitDirectory(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	writeWorkspaceFile(t, src, "keep/file.txt", "ok\n")
	writeWorkspaceFile(t, src, "node_modules/skip.txt", "skip\n")

	factory := NewFactory(codeqa.Options{ArtifactDir: t.TempDir()})
	prepared, err := factory.Prepare(t.Context(), src, "run-copy", codeqa.ModeGate)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	defer prepared.Cleanup()
	if prepared.IsGit {
		t.Fatalf("expected copied non-git workspace")
	}
	if _, err := os.Stat(filepath.Join(prepared.Path, "keep", "file.txt")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(prepared.Path, "node_modules", "skip.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected ignored directory to be skipped, got err=%v", err)
	}
}

func initWorkspaceGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitWorkspace(t, repo, "init")
	gitWorkspace(t, repo, "config", "user.email", "codeqa@example.com")
	gitWorkspace(t, repo, "config", "user.name", "CodeQA")
	return repo
}

func writeWorkspaceFile(t *testing.T, root, rel, content string) {
	t.Helper()
	fullPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func gitWorkspace(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}
