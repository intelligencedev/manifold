package workspaces

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"manifold/internal/config"
	"manifold/internal/objectstore"
)

func TestEphemeralWorkspaceManager_CheckoutAndCommit(t *testing.T) {
	t.Parallel()

	// Create temp directory for ephemeral workspaces
	tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create memory store and seed with some files
	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	// Seed project files
	userID := int64(123)
	projectID := "test-project"
	files := map[string]string{
		"users/123/projects/test-project/files/README.md":        "# Test Project",
		"users/123/projects/test-project/files/src/main.go":      "package main",
		"users/123/projects/test-project/files/src/util/util.go": "package util",
	}
	for key, content := range files {
		_, err := store.Put(ctx, "workspaces/"+key, bytes.NewReader([]byte(content)), objectstore.PutOptions{})
		require.NoError(t, err)
	}

	// Create manager
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
	mgr := NewEphemeralManager(store, cfg)

	// Checkout workspace
	ws, err := mgr.Checkout(ctx, userID, projectID, "session-1")
	require.NoError(t, err)
	assert.Equal(t, userID, ws.UserID)
	assert.Equal(t, projectID, ws.ProjectID)
	assert.Equal(t, "session-1", ws.SessionID)
	assert.Equal(t, "ephemeral", ws.Mode)
	assert.DirExists(t, ws.BaseDir)

	// Verify files were downloaded
	readmePath := filepath.Join(ws.BaseDir, "README.md")
	assert.FileExists(t, readmePath)
	content, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	assert.Equal(t, "# Test Project", string(content))

	mainGoPath := filepath.Join(ws.BaseDir, "src", "main.go")
	assert.FileExists(t, mainGoPath)

	// Modify a file
	err = os.WriteFile(readmePath, []byte("# Updated Project"), 0644)
	require.NoError(t, err)

	// Add a new file
	newFile := filepath.Join(ws.BaseDir, "new-file.txt")
	err = os.WriteFile(newFile, []byte("new content"), 0644)
	require.NoError(t, err)

	// Delete a file
	err = os.Remove(mainGoPath)
	require.NoError(t, err)

	// Commit changes
	err = mgr.Commit(ctx, ws)
	require.NoError(t, err)

	// Verify S3 was updated
	// Modified file
	reader, _, err := store.Get(ctx, "workspaces/users/123/projects/test-project/files/README.md")
	require.NoError(t, err)
	defer reader.Close()
	data := make([]byte, 100)
	n, _ := reader.Read(data)
	assert.Equal(t, "# Updated Project", string(data[:n]))

	// New file
	_, _, err = store.Get(ctx, "workspaces/users/123/projects/test-project/files/new-file.txt")
	require.NoError(t, err)

	// Deleted file should be gone from S3
	_, _, err = store.Get(ctx, "workspaces/users/123/projects/test-project/files/src/main.go")
	assert.ErrorIs(t, err, objectstore.ErrNotFound)

	// Cleanup
	err = mgr.Cleanup(ctx, ws)
	require.NoError(t, err)
	assert.NoDirExists(t, ws.BaseDir)
}

func TestEphemeralWorkspaceManager_EmptyProjectID(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}
	mgr := NewEphemeralManager(store, cfg)

	// Empty project ID should return empty workspace
	ws, err := mgr.Checkout(context.Background(), 123, "", "session")
	require.NoError(t, err)
	assert.Empty(t, ws.BaseDir)
	assert.Equal(t, "ephemeral", ws.Mode)
}

func TestEphemeralWorkspaceManager_InvalidProjectID(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}
	mgr := NewEphemeralManager(store, cfg)

	testCases := []string{
		"../escape",
		"/absolute/path",
		"foo/../bar",
	}
	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			_, err := mgr.Checkout(context.Background(), 123, tc, "session")
			assert.ErrorIs(t, err, ErrInvalidProjectID)
		})
	}
}

func TestEphemeralWorkspaceManager_SessionReuse(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "ephemeral-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store := objectstore.NewMemoryStore()
	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}
	mgr := NewEphemeralManager(store, cfg)
	ctx := context.Background()

	// First checkout
	ws1, err := mgr.Checkout(ctx, 123, "project", "session-1")
	require.NoError(t, err)

	// Second checkout with same session should return same workspace
	ws2, err := mgr.Checkout(ctx, 123, "project", "session-1")
	require.NoError(t, err)
	assert.Equal(t, ws1.BaseDir, ws2.BaseDir)

	// Different session should return different workspace
	ws3, err := mgr.Checkout(ctx, 123, "project", "session-2")
	require.NoError(t, err)
	assert.NotEqual(t, ws1.BaseDir, ws3.BaseDir)
}
