package projects

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"manifold/internal/config"
	"manifold/internal/objectstore"
)

func TestS3Service_CreateProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	proj, err := svc.CreateProject(ctx, 123, "Test Project")
	require.NoError(t, err)
	assert.NotEmpty(t, proj.ID)
	assert.Equal(t, "Test Project", proj.Name)
	assert.False(t, proj.CreatedAt.IsZero())
	assert.False(t, proj.UpdatedAt.IsZero())

	// Verify metadata was stored
	metaKey := "workspaces/users/123/projects/" + proj.ID + "/.meta/project.json"
	exists, err := store.Exists(ctx, metaKey)
	require.NoError(t, err)
	assert.True(t, exists)

	// Verify README was created
	readmeKey := "workspaces/users/123/projects/" + proj.ID + "/files/README.md"
	exists, err = store.Exists(ctx, readmeKey)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestS3Service_ListProjects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	// Create multiple projects
	proj1, err := svc.CreateProject(ctx, 123, "Project 1")
	require.NoError(t, err)

	proj2, err := svc.CreateProject(ctx, 123, "Project 2")
	require.NoError(t, err)

	// List projects
	projects, err := svc.ListProjects(ctx, 123)
	require.NoError(t, err)
	assert.Len(t, projects, 2)

	// Should be sorted by UpdatedAt desc
	ids := make(map[string]bool)
	for _, p := range projects {
		ids[p.ID] = true
	}
	assert.True(t, ids[proj1.ID])
	assert.True(t, ids[proj2.ID])
}

func TestS3Service_DeleteProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	proj, err := svc.CreateProject(ctx, 123, "To Delete")
	require.NoError(t, err)

	// Add some files
	err = svc.UploadFile(ctx, 123, proj.ID, ".", "test.txt", bytes.NewReader([]byte("content")))
	require.NoError(t, err)

	// Delete project
	err = svc.DeleteProject(ctx, 123, proj.ID)
	require.NoError(t, err)

	// Verify project is gone
	projects, err := svc.ListProjects(ctx, 123)
	require.NoError(t, err)
	for _, p := range projects {
		assert.NotEqual(t, proj.ID, p.ID)
	}
}

func TestS3Service_UploadAndReadFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	proj, err := svc.CreateProject(ctx, 123, "Test")
	require.NoError(t, err)

	// Upload file
	content := []byte("hello, world!")
	err = svc.UploadFile(ctx, 123, proj.ID, "src", "main.go", bytes.NewReader(content))
	require.NoError(t, err)

	// Read file back
	reader, err := svc.ReadFile(ctx, 123, proj.ID, "src/main.go")
	require.NoError(t, err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestS3Service_EncryptProjectFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	ciphertext, encrypted, err := svc.EncryptProjectFile(ctx, 123, "proj1", []byte("plaintext"))
	require.NoError(t, err)
	assert.False(t, encrypted)
	assert.Equal(t, []byte("plaintext"), ciphertext)

	tmpDir := t.TempDir()
	provider, err := NewFileKeyProvider(tmpDir, "")
	require.NoError(t, err)
	defer provider.Close()

	svc.SetKeyProvider(provider)
	require.NoError(t, svc.EnableEncryption(true))

	plaintext := []byte("secret-data")
	ciphertext, encrypted, err = svc.EncryptProjectFile(ctx, 123, "proj1", plaintext)
	require.NoError(t, err)
	assert.True(t, encrypted)
	assert.NotEqual(t, plaintext, ciphertext)
	assert.True(t, bytes.HasPrefix(ciphertext, fileMagic[:]))

	decrypted, err := svc.DecryptProjectFile(ctx, 123, "proj1", ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestS3Service_ListTree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	proj, err := svc.CreateProject(ctx, 123, "Test")
	require.NoError(t, err)

	// Create directory structure
	files := []struct {
		path string
		name string
	}{
		{".", ".metadata"},
		{".", "file1.txt"},
		{".", "file2.txt"},
		{"src", "main.go"},
		{"src", "util.go"},
		{"src/pkg", "pkg.go"},
	}
	for _, f := range files {
		err = svc.UploadFile(ctx, 123, proj.ID, f.path, f.name, bytes.NewReader([]byte("content")))
		require.NoError(t, err)
	}

	// List root
	entries, err := svc.ListTree(ctx, 123, proj.ID, ".")
	require.NoError(t, err)
	// Should have: README.md, file1.txt, file2.txt, src/
	// Note: dirs first, then files alphabetically
	assert.True(t, len(entries) >= 3)

	// Hidden dotfiles should be present (S3 implementation previously filtered ".meta*" at root).
	hasHidden := false
	for _, e := range entries {
		if e.Type == "file" && e.Name == ".metadata" {
			hasHidden = true
			break
		}
	}
	assert.True(t, hasHidden, "should include hidden dotfiles at root")

	// List src directory
	entries, err = svc.ListTree(ctx, 123, proj.ID, "src")
	require.NoError(t, err)
	// Should have: pkg/, main.go, util.go
	hasDir := false
	hasFile := false
	for _, e := range entries {
		if e.Type == "dir" && e.Name == "pkg" {
			hasDir = true
		}
		if e.Type == "file" && e.Name == "main.go" {
			hasFile = true
		}
	}
	assert.True(t, hasDir, "should have pkg directory")
	assert.True(t, hasFile, "should have main.go file")
}

func TestS3Service_DeleteFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	proj, err := svc.CreateProject(ctx, 123, "Test")
	require.NoError(t, err)

	// Upload file
	err = svc.UploadFile(ctx, 123, proj.ID, ".", "test.txt", bytes.NewReader([]byte("content")))
	require.NoError(t, err)

	// Delete file
	err = svc.DeleteFile(ctx, 123, proj.ID, "test.txt")
	require.NoError(t, err)

	// File should be gone
	_, err = svc.ReadFile(ctx, 123, proj.ID, "test.txt")
	assert.Error(t, err)
}

func TestS3Service_MovePath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	proj, err := svc.CreateProject(ctx, 123, "Test")
	require.NoError(t, err)

	// Upload file
	content := []byte("move me")
	err = svc.UploadFile(ctx, 123, proj.ID, ".", "original.txt", bytes.NewReader(content))
	require.NoError(t, err)

	// Move file
	err = svc.MovePath(ctx, 123, proj.ID, "original.txt", "moved.txt")
	require.NoError(t, err)

	// Original should be gone
	_, err = svc.ReadFile(ctx, 123, proj.ID, "original.txt")
	assert.Error(t, err)

	// New location should have content
	reader, err := svc.ReadFile(ctx, 123, proj.ID, "moved.txt")
	require.NoError(t, err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestS3Service_CreateDir(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	proj, err := svc.CreateProject(ctx, 123, "Test")
	require.NoError(t, err)

	// Create directory
	err = svc.CreateDir(ctx, 123, proj.ID, "new-dir/sub-dir")
	require.NoError(t, err)

	// Directory should be listable
	entries, err := svc.ListTree(ctx, 123, proj.ID, "new-dir")
	require.NoError(t, err)
	// Should have sub-dir/
	found := false
	for _, e := range entries {
		if e.Type == "dir" && e.Name == "sub-dir" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have sub-dir")
}

func TestS3Service_InvalidFileNames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := objectstore.NewMemoryStore()
	svc := NewS3Service(store, config.S3Config{Prefix: "workspaces"})

	proj, err := svc.CreateProject(ctx, 123, "Test")
	require.NoError(t, err)

	// Empty name
	err = svc.UploadFile(ctx, 123, proj.ID, ".", "", bytes.NewReader([]byte("content")))
	assert.Error(t, err)

	// Name with slash
	err = svc.UploadFile(ctx, 123, proj.ID, ".", "bad/name", bytes.NewReader([]byte("content")))
	assert.Error(t, err)
}
