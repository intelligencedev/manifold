package diff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"manifold/internal/codeqa"
)

const defaultCommandTimeout = 30 * time.Second

type Packager struct {
	runner codeqa.CommandRunner
	opts   codeqa.Options
}

func NewPackager(runner codeqa.CommandRunner, opts codeqa.Options) *Packager {
	return &Packager{runner: runner, opts: opts}
}

func (p *Packager) Build(ctx context.Context, repoPath string, baseRef string, headRef string, includeRepoContext bool, maxDiffBytes int, maxChangedFiles int) (codeqa.DiffBundle, error) {
	if strings.TrimSpace(baseRef) == "" {
		baseRef = "HEAD~1"
	}
	if strings.TrimSpace(headRef) == "" {
		headRef = "HEAD"
	}
	if maxDiffBytes <= 0 {
		maxDiffBytes = p.opts.DefaultMaxDiffBytes
	}
	if maxChangedFiles <= 0 {
		maxChangedFiles = p.opts.DefaultMaxChangedFiles
	}
	repoRoot, err := p.repoRoot(ctx, repoPath)
	if err != nil {
		return codeqa.DiffBundle{}, err
	}
	changedFiles, err := p.changedFiles(ctx, repoRoot, baseRef, headRef)
	if err != nil {
		return codeqa.DiffBundle{}, err
	}
	bundle := codeqa.DiffBundle{
		BaseRef:     baseRef,
		HeadRef:     headRef,
		Files:       make([]codeqa.ChangedFile, 0, len(changedFiles)),
		SourceTrees: map[string]string{},
	}
	for _, file := range changedFiles {
		if codeqa.MatchAny(file.Path, p.opts.ForbiddenGlobs) || isLikelyBinaryPath(file.Path) {
			continue
		}
		file.RelatedTests = p.relatedTests(ctx, repoRoot, headRef, file.Path)
		bundle.Files = append(bundle.Files, file)
		bundle.SourceTrees[file.Path] = filepath.ToSlash(filepath.Dir(file.Path))
	}
	sort.Slice(bundle.Files, func(i int, j int) bool { return bundle.Files[i].Path < bundle.Files[j].Path })
	if len(bundle.Files) > maxChangedFiles {
		bundle.Files = append([]codeqa.ChangedFile(nil), bundle.Files[:maxChangedFiles]...)
		bundle.Truncated = true
	}
	paths := make([]string, 0, len(bundle.Files))
	for _, file := range bundle.Files {
		paths = append(paths, file.Path)
	}
	if len(paths) > 0 {
		res, err := p.runner.Run(ctx, repoRoot, codeqa.CommandRequest{
			Command: "git",
			Args:    append([]string{"diff", "--unified=3", baseRef, headRef, "--"}, paths...),
			Timeout: defaultCommandTimeout,
		})
		if err != nil {
			return codeqa.DiffBundle{}, fmt.Errorf("git diff: %w", err)
		}
		bundle.UnifiedDiff = res.Stdout
	}
	if maxDiffBytes > 0 && len(bundle.UnifiedDiff) > maxDiffBytes {
		bundle.UnifiedDiff = bundle.UnifiedDiff[:maxDiffBytes] + "\n[TRUNCATED]"
		bundle.Truncated = true
	}
	if includeRepoContext {
		bundle.RepoContext = p.repoContext(repoRoot)
	}
	return bundle, nil
}

func (p *Packager) repoRoot(ctx context.Context, repoPath string) (string, error) {
	res, err := p.runner.Run(ctx, repoPath, codeqa.CommandRequest{
		Command: "git",
		Args:    []string{"rev-parse", "--show-toplevel"},
		Timeout: defaultCommandTimeout,
	})
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	if !res.OK {
		return "", fmt.Errorf("resolve repo root: %s", strings.TrimSpace(res.Stderr))
	}
	return strings.TrimSpace(res.Stdout), nil
}

func (p *Packager) changedFiles(ctx context.Context, repoRoot string, baseRef string, headRef string) ([]codeqa.ChangedFile, error) {
	res, err := p.runner.Run(ctx, repoRoot, codeqa.CommandRequest{
		Command: "git",
		Args:    []string{"diff", "--name-status", "--find-renames", baseRef, headRef},
		Timeout: defaultCommandTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("list changed files: %w", err)
	}
	if !res.OK {
		return nil, fmt.Errorf("list changed files: %s", strings.TrimSpace(res.Stderr))
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	out := make([]codeqa.ChangedFile, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		path := parts[len(parts)-1]
		out = append(out, codeqa.ChangedFile{Path: filepath.ToSlash(path), Status: status})
	}
	return out, nil
}

func (p *Packager) relatedTests(ctx context.Context, repoRoot string, headRef string, changedPath string) []string {
	if !strings.HasSuffix(changedPath, ".go") {
		return nil
	}
	if strings.HasSuffix(changedPath, "_test.go") {
		return []string{changedPath}
	}
	testPath := strings.TrimSuffix(changedPath, ".go") + "_test.go"
	res, err := p.runner.Run(ctx, repoRoot, codeqa.CommandRequest{
		Command: "git",
		Args:    []string{"cat-file", "-e", headRef + ":" + testPath},
		Timeout: 10 * time.Second,
	})
	if err != nil || !res.OK {
		return nil
	}
	return []string{testPath}
}

func (p *Packager) repoContext(repoRoot string) string {
	files := []string{"AGENTS.md", "README.md"}
	parts := make([]string, 0, len(files))
	for _, name := range files {
		fullPath := filepath.Join(repoRoot, name)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		if len(content) > 4096 {
			content = content[:4096]
		}
		parts = append(parts, "## "+name+"\n"+string(content))
	}
	return strings.Join(parts, "\n\n")
}

func isLikelyBinaryPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf", ".zip", ".gz", ".tar", ".sqlite", ".sqlite3", ".mp3", ".mp4", ".mov", ".wasm":
		return true
	default:
		return false
	}
}
