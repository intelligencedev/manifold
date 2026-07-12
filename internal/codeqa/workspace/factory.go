package workspace

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"manifold/internal/codeqa"
)

type Prepared struct {
	Path     string
	RepoRoot string
	IsGit    bool
	cleanup  func() error
}

func (p Prepared) Cleanup() error {
	if p.cleanup == nil {
		return nil
	}
	return p.cleanup()
}

type Factory struct {
	opts codeqa.Options
}

func NewFactory(opts codeqa.Options) *Factory {
	return &Factory{opts: opts}
}

func (f *Factory) Prepare(ctx context.Context, repoPath, runID string, mode codeqa.RunMode) (Prepared, error) {
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return Prepared{}, fmt.Errorf("resolve repository path: %w", err)
	}
	gitRoot, isGit := f.gitRoot(ctx, absRepo)
	if mode == codeqa.ModeJudge {
		if isGit {
			return Prepared{Path: absRepo, RepoRoot: gitRoot, IsGit: true}, nil
		}
		return Prepared{Path: absRepo, RepoRoot: absRepo, IsGit: false}, nil
	}
	if isGit {
		if mode == codeqa.ModeOptimize {
			dirty, err := f.isDirty(ctx, gitRoot)
			if err != nil {
				return Prepared{}, err
			}
			if dirty {
				return Prepared{}, fmt.Errorf("optimize mode requires a clean git worktree")
			}
		}
		return f.prepareGitCheckout(ctx, gitRoot, runID, "HEAD")
	}
	return f.prepareCopy(absRepo, runID)
}

func (f *Factory) CheckoutRef(ctx context.Context, repoPath, runID, ref string) (Prepared, error) {
	repoRoot, isGit := f.gitRoot(ctx, repoPath)
	if !isGit {
		return Prepared{}, fmt.Errorf("reference checkout requires a git repository")
	}
	if strings.TrimSpace(ref) == "" {
		ref = "HEAD"
	}
	return f.prepareGitCheckout(ctx, repoRoot, filepath.Join(runID, "refs"), ref)
}

func (f *Factory) prepareGitCheckout(ctx context.Context, repoRoot, runID, ref string) (Prepared, error) {
	target := filepath.Join(f.workspaceRoot(runID), sanitizeRef(ref))
	if err := os.RemoveAll(target); err != nil {
		return Prepared{}, fmt.Errorf("clear workspace path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Prepared{}, fmt.Errorf("create workspace root: %w", err)
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "--no-local", repoRoot, target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Prepared{}, fmt.Errorf("clone git workspace: %w: %s", err, strings.TrimSpace(string(output)))
	}
	checkoutCmd := exec.CommandContext(ctx, "git", "-C", target, "checkout", "--quiet", "--detach", ref)
	if output, err := checkoutCmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(target)
		return Prepared{}, fmt.Errorf("checkout git workspace ref %s: %w: %s", ref, err, strings.TrimSpace(string(output)))
	}
	cleanup := func() error {
		return os.RemoveAll(target)
	}
	return Prepared{Path: target, RepoRoot: repoRoot, IsGit: true, cleanup: cleanup}, nil
}

func (f *Factory) prepareCopy(repoPath, runID string) (Prepared, error) {
	target := filepath.Join(f.workspaceRoot(runID), "copy")
	if err := copyDirFiltered(repoPath, target, f.opts.ForbiddenGlobs); err != nil {
		return Prepared{}, err
	}
	cleanup := func() error { return os.RemoveAll(target) }
	return Prepared{Path: target, RepoRoot: repoPath, IsGit: false, cleanup: cleanup}, nil
}

func (f *Factory) workspaceRoot(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = "adhoc"
	}
	return filepath.Join(f.opts.ArtifactDir, "workspaces", runID)
}

func (f *Factory) gitRoot(ctx context.Context, repoPath string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", false
	}
	return root, true
}

func (f *Factory) isDirty(ctx context.Context, repoRoot string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "status", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("check dirty worktree: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) != "", nil
}

func copyDirFiltered(src, dst string, forbidden []string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory: %s", src)
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		rel = filepath.ToSlash(rel)
		if shouldSkip(rel, d, forbidden) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		entryInfo, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, entryInfo.Mode().Perm())
		}
		if !entryInfo.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entryInfo.Mode().Perm())
		if err != nil {
			return err
		}
		defer dstFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}
		return nil
	})
}

func shouldSkip(rel string, d fs.DirEntry, forbidden []string) bool {
	base := filepath.Base(rel)
	if d.IsDir() {
		switch base {
		case ".git", "node_modules", "dist", ".next", "tmp", "coverage", ".nyc_output":
			return true
		}
	}
	if strings.HasPrefix(base, ".coverage") {
		return true
	}
	return codeqa.MatchAny(rel, forbidden)
}

func sanitizeRef(ref string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", "~", "_", "^", "_")
	cleaned := replacer.Replace(strings.TrimSpace(ref))
	if cleaned == "" {
		return "head"
	}
	return cleaned
}
