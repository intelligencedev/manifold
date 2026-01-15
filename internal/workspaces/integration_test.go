//go:build enterprise
// +build enterprise

package workspaces_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"manifold/internal/config"
	"manifold/internal/objectstore"
	"manifold/internal/workspaces"
)

// Integration tests for workspace architecture (Phase 7)
// These tests verify the complete workspace lifecycle across different modes.

// TestBackwardCompatibility_SimpleModeUnchanged verifies that simple/legacy mode
// continues to work without requiring Redis, Kafka, or S3.
func TestBackwardCompatibility_SimpleModeUnchanged(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "simple-mode-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a project directory (simulating existing simple deployment)
	projectDir := filepath.Join(tmpDir, "users", "1", "projects", "my-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("# Hello"), 0644))

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "filesystem", // Simple mode
			Workspace: config.WorkspaceConfig{
				Mode: "legacy", // Direct project directory access
			},
		},
	}

	mgr := workspaces.NewManager(cfg)
	require.NotNil(t, mgr)
	assert.Equal(t, "legacy", mgr.Mode())

	ctx := context.Background()

	// Checkout should return the project directory directly
	ws, err := mgr.Checkout(ctx, 1, "my-project", "session-1")
	require.NoError(t, err)
	assert.Equal(t, "legacy", ws.Mode)
	assert.Equal(t, projectDir, ws.BaseDir)

	// File should be accessible
	content, err := os.ReadFile(filepath.Join(ws.BaseDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Hello", string(content))

	// Modify file
	require.NoError(t, os.WriteFile(filepath.Join(ws.BaseDir, "README.md"), []byte("# Updated"), 0644))

	// Commit should be a no-op in legacy mode (changes already on disk)
	err = mgr.Commit(ctx, ws)
	require.NoError(t, err)

	// Cleanup should be a no-op in legacy mode
	err = mgr.Cleanup(ctx, ws)
	require.NoError(t, err)

	// Verify file still exists (not deleted)
	assert.FileExists(t, filepath.Join(projectDir, "README.md"))
}

// TestSessionStability_ReuseWorkspace verifies that multiple checkouts for the
// same session reuse the workspace without re-fetching from S3.
func TestSessionStability_ReuseWorkspace(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "session-stability-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create memory store with project files
	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	projectFiles := map[string]string{
		"workspaces/users/1/projects/proj1/files/main.go": "package main",
		"workspaces/users/1/projects/proj1/files/go.mod":  "module test",
	}
	for key, content := range projectFiles {
		_, err := store.Put(ctx, key, bytes.NewReader([]byte(content)), objectstore.PutOptions{})
		require.NoError(t, err)
	}

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)
	require.NotNil(t, mgr)

	// First checkout
	ws1, err := mgr.Checkout(ctx, 1, "proj1", "session-abc")
	require.NoError(t, err)
	assert.DirExists(t, ws1.BaseDir)

	// Modify a file in the workspace
	modifiedContent := "package main\n\nfunc main() {}"
	require.NoError(t, os.WriteFile(filepath.Join(ws1.BaseDir, "main.go"), []byte(modifiedContent), 0644))

	// Second checkout with same session should return same workspace
	ws2, err := mgr.Checkout(ctx, 1, "proj1", "session-abc")
	require.NoError(t, err)

	// Same base directory
	assert.Equal(t, ws1.BaseDir, ws2.BaseDir)

	// Modified content should still be there (not re-fetched from S3)
	content, err := os.ReadFile(filepath.Join(ws2.BaseDir, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, modifiedContent, string(content))
}

// TestGenerationDetection_StaleWorkspaceTriggerRefresh verifies that when
// the remote generation is newer, the workspace gets refreshed.
func TestGenerationDetection_StaleWorkspaceTriggerRefresh(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "generation-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	// Initial project files with metadata
	meta := map[string]interface{}{
		"generation":       int64(1),
		"skillsGeneration": int64(1),
	}
	metaBytes, _ := json.Marshal(meta)
	_, err = store.Put(ctx, "workspaces/users/1/projects/proj1/.meta/project.json",
		bytes.NewReader(metaBytes), objectstore.PutOptions{})
	require.NoError(t, err)

	_, err = store.Put(ctx, "workspaces/users/1/projects/proj1/files/data.txt",
		bytes.NewReader([]byte("version 1")), objectstore.PutOptions{})
	require.NoError(t, err)

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)

	// First checkout
	ws1, err := mgr.Checkout(ctx, 1, "proj1", "session-gen")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(ws1.BaseDir, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "version 1", string(content))

	// Simulate remote update (bump generation)
	meta["generation"] = int64(2)
	metaBytes, _ = json.Marshal(meta)
	_, err = store.Put(ctx, "workspaces/users/1/projects/proj1/.meta/project.json",
		bytes.NewReader(metaBytes), objectstore.PutOptions{})
	require.NoError(t, err)

	_, err = store.Put(ctx, "workspaces/users/1/projects/proj1/files/data.txt",
		bytes.NewReader([]byte("version 2")), objectstore.PutOptions{})
	require.NoError(t, err)

	// New session checkout should get updated content
	ws2, err := mgr.Checkout(ctx, 1, "proj1", "session-gen-new")
	require.NoError(t, err)

	content, err = os.ReadFile(filepath.Join(ws2.BaseDir, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "version 2", string(content))
}

// TestConcurrentCheckouts_ThreadSafe verifies that concurrent checkouts
// for different sessions are handled safely.
func TestConcurrentCheckouts_ThreadSafe(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "concurrent-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	// Seed project
	_, err = store.Put(ctx, "workspaces/users/1/projects/proj1/files/main.go",
		bytes.NewReader([]byte("package main")), objectstore.PutOptions{})
	require.NoError(t, err)

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)

	// Run 10 concurrent checkouts for different sessions
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	workspaces := make(chan workspaces.Workspace, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(sessionNum int) {
			defer wg.Done()
			sessionID := "session-" + string(rune('A'+sessionNum))
			ws, err := mgr.Checkout(ctx, 1, "proj1", sessionID)
			if err != nil {
				errors <- err
				return
			}
			workspaces <- ws
		}(i)
	}

	wg.Wait()
	close(errors)
	close(workspaces)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent checkout failed: %v", err)
	}

	// Verify all workspaces are distinct
	dirs := make(map[string]bool)
	for ws := range workspaces {
		assert.NotEmpty(t, ws.BaseDir)
		assert.False(t, dirs[ws.BaseDir], "Duplicate workspace directory: %s", ws.BaseDir)
		dirs[ws.BaseDir] = true
	}

	assert.Len(t, dirs, 10, "Expected 10 distinct workspaces")
}

// TestConflictHandling_ConcurrentCommits verifies that concurrent commits
// to the same project are handled gracefully.
func TestConflictHandling_ConcurrentCommits(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "conflict-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	// Seed project
	meta := map[string]interface{}{"generation": int64(1), "skillsGeneration": int64(1)}
	metaBytes, _ := json.Marshal(meta)
	_, err = store.Put(ctx, "workspaces/users/1/projects/proj1/.meta/project.json",
		bytes.NewReader(metaBytes), objectstore.PutOptions{})
	require.NoError(t, err)

	_, err = store.Put(ctx, "workspaces/users/1/projects/proj1/files/shared.txt",
		bytes.NewReader([]byte("original")), objectstore.PutOptions{})
	require.NoError(t, err)

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)

	// Checkout two sessions
	ws1, err := mgr.Checkout(ctx, 1, "proj1", "session-A")
	require.NoError(t, err)

	ws2, err := mgr.Checkout(ctx, 1, "proj1", "session-B")
	require.NoError(t, err)

	// Modify file in both sessions
	require.NoError(t, os.WriteFile(filepath.Join(ws1.BaseDir, "shared.txt"), []byte("from A"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(ws2.BaseDir, "shared.txt"), []byte("from B"), 0644))

	// Concurrent commits
	var wg sync.WaitGroup
	var commit1Err, commit2Err error

	wg.Add(2)
	go func() {
		defer wg.Done()
		commit1Err = mgr.Commit(ctx, ws1)
	}()
	go func() {
		defer wg.Done()
		commit2Err = mgr.Commit(ctx, ws2)
	}()
	wg.Wait()

	// Both commits should succeed (last writer wins in current implementation)
	// In production with Redis locks, one might fail and require retry
	assert.NoError(t, commit1Err)
	assert.NoError(t, commit2Err)

	// Verify final state in S3
	reader, _, err := store.Get(ctx, "workspaces/users/1/projects/proj1/files/shared.txt")
	require.NoError(t, err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)

	// Content should be one of the two commits
	content := string(data)
	assert.True(t, content == "from A" || content == "from B",
		"Expected content from A or B, got: %s", content)
}

// TestCleanup_ReleasesResources verifies that cleanup properly releases
// workspace resources.
func TestCleanup_ReleasesResources(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "cleanup-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	_, err = store.Put(ctx, "workspaces/users/1/projects/proj1/files/temp.txt",
		bytes.NewReader([]byte("temporary")), objectstore.PutOptions{})
	require.NoError(t, err)

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := workspaces.NewEphemeralManager(store, cfg)

	// Checkout
	ws, err := mgr.Checkout(ctx, 1, "proj1", "cleanup-session")
	require.NoError(t, err)
	assert.DirExists(t, ws.BaseDir)

	// Cleanup
	err = mgr.Cleanup(ctx, ws)
	require.NoError(t, err)

	// Directory should be removed
	assert.NoDirExists(t, ws.BaseDir)
}

// TestValidateProjectID_PathTraversal verifies that malicious project IDs
// are rejected.
func TestValidateProjectID_PathTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		projectID string
		wantErr   bool
	}{
		{"valid simple", "my-project", false},
		{"valid uuid", "123e4567-e89b-12d3-a456-426614174000", false},
		{"dot", ".", true},
		{"dotdot", "..", true},
		{"path traversal", "../etc", true},
		{"absolute path", "/etc/passwd", true},
		{"backslash traversal", "..\\etc", true},
		{"hidden with traversal", ".hidden/../etc", true},
		{"empty", "", false}, // Empty is allowed (returns empty)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := workspaces.ValidateProjectID(tt.projectID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateSessionID_PathTraversal verifies that malicious session IDs
// are rejected.
func TestValidateSessionID_PathTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
	}{
		{"valid simple", "session-123", false},
		{"valid uuid", "ses-123e4567-e89b", false},
		{"dot", ".", true},
		{"dotdot", "..", true},
		{"path traversal", "../tmp", true},
		{"absolute path", "/tmp/hack", true},
		{"empty", "", false}, // Empty is allowed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := workspaces.ValidateSessionID(tt.sessionID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
