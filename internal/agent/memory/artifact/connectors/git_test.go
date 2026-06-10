package connectors

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"manifold/internal/agent/memory/artifact"
	"manifold/internal/persistence/databases"
)

func TestGitConnectorCapturesCommitsIdempotently(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "Record memory decision")

	store := databases.NewMemoryArtifactStore()
	manager := artifact.CaptureManager{Store: store, Connectors: []artifact.Connector{GitConnector{}}, Timeout: 5 * time.Second}
	req := artifact.CaptureRequest{TenantID: 7, Hints: map[string]string{"repoPath": repo, "since": "1970-01-01T00:00:00Z"}}
	first := manager.Capture(ctx, req)
	second := manager.Capture(ctx, req)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one artifact each run, got first=%d second=%d", len(first), len(second))
	}
	if first[0].ID != second[0].ID || first[0].Kind != artifact.ArtifactGitCommit || first[0].ContentHash == "" {
		t.Fatalf("expected idempotent commit capture, first=%+v second=%+v", first[0], second[0])
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
