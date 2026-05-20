package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"manifold/internal/codeqa"
	"manifold/internal/config"
	"manifold/internal/tools/cli"
)

func TestPackagerBuildFiltersAndTruncates(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, repo, "go.mod", "module example.com/repo\n\ngo 1.24.5\n")
	writeFile(t, repo, "foo/foo.go", "package foo\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFile(t, repo, "foo/foo_test.go", "package foo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"bad\") } }\n")
	writeFile(t, repo, "AGENTS.md", "repo guidance")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")

	writeFile(t, repo, "foo/foo.go", "package foo\n\nfunc Add(a, b int) int {\n\treturn a + b + 1\n}\n")
	writeFile(t, repo, "secrets/dev.pem", "secret")
	if err := os.MkdirAll(filepath.Join(repo, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "assets", "logo.png"), []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "head")

	executor := cli.NewExecutor(config.ExecConfig{MaxCommandSeconds: 30}, repo, 64*1024)
	runner := codeqa.NewCLICommandRunner(executor, nil)
	packager := NewPackager(runner, codeqa.Options{DefaultMaxDiffBytes: 40, DefaultMaxChangedFiles: 1, ForbiddenGlobs: []string{"**/*.pem"}})
	bundle, err := packager.Build(t.Context(), repo, "HEAD~1", "HEAD", true, 40, 1)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(bundle.Files) != 1 {
		t.Fatalf("expected one included changed file, got %d: %+v", len(bundle.Files), bundle.Files)
	}
	if bundle.Files[0].Path != "foo/foo.go" {
		t.Fatalf("expected foo/foo.go, got %s", bundle.Files[0].Path)
	}
	if len(bundle.Files[0].RelatedTests) != 1 || bundle.Files[0].RelatedTests[0] != "foo/foo_test.go" {
		t.Fatalf("expected related test discovery, got %v", bundle.Files[0].RelatedTests)
	}
	if !bundle.Truncated {
		t.Fatalf("expected bundle to be truncated")
	}
	if strings.Contains(bundle.UnifiedDiff, "dev.pem") || strings.Contains(bundle.UnifiedDiff, "logo.png") {
		t.Fatalf("expected forbidden/binary files to be excluded from diff: %s", bundle.UnifiedDiff)
	}
	if !strings.Contains(bundle.RepoContext, "repo guidance") {
		t.Fatalf("expected repo context to include AGENTS.md content")
	}
}

func TestRelatedTestCandidatesSupportsMultipleLanguages(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"foo/foo.go":       {"foo/foo_test.go"},
		"foo/foo_test.go":  {"foo/foo_test.go"},
		"pkg/app.py":       {"pkg/test_app.py", "pkg/app_test.py"},
		"pkg/test_app.py":  {"pkg/test_app.py"},
		"web/app.ts":       {"web/app.test.ts", "web/app.spec.ts"},
		"web/app.test.ts":  {"web/app.test.ts"},
		"web/view.jsx":     {"web/view.test.jsx", "web/view.spec.jsx"},
		"styles/main.css":  nil,
		"templates/a.html": nil,
		"src/lib.rs":       nil,
	}
	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			got := relatedTestCandidates(path)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("relatedTestCandidates(%q) = %v, want %v", path, got, want)
			}
		})
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
