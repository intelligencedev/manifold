package databases

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"manifold/internal/persistence"
)

func TestMemoryProjectsStore_CreateAndGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	// Create a project
	p, err := store.Create(ctx, 1, "Test Project")
	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, int64(1), p.UserID)
	assert.Equal(t, "Test Project", p.Name)
	assert.Equal(t, int64(1), p.Revision)
	assert.Equal(t, "filesystem", p.StorageBackend)

	// Get the project
	got, err := store.Get(ctx, 1, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.Name, got.Name)
}

func TestMemoryProjectsStore_GetNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	_, err := store.Get(ctx, 1, "nonexistent")
	assert.ErrorIs(t, err, persistence.ErrNotFound)
}

func TestMemoryProjectsStore_GetForbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	// Create project for user 1
	p, err := store.Create(ctx, 1, "User1 Project")
	require.NoError(t, err)

	// Try to get as user 2
	_, err = store.Get(ctx, 2, p.ID)
	assert.ErrorIs(t, err, persistence.ErrForbidden)
}

func TestMemoryProjectsStore_List(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	// Create projects for different users
	p1, err := store.Create(ctx, 1, "Project A")
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond) // Ensure different timestamps

	p2, err := store.Create(ctx, 1, "Project B")
	require.NoError(t, err)

	_, err = store.Create(ctx, 2, "User2 Project")
	require.NoError(t, err)

	// List user 1's projects
	projects, err := store.List(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, projects, 2)

	// Should be sorted by UpdatedAt desc
	assert.Equal(t, p2.ID, projects[0].ID)
	assert.Equal(t, p1.ID, projects[1].ID)

	// List user 2's projects
	projects, err = store.List(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, projects, 1)

	// List nonexistent user
	projects, err = store.List(ctx, 999)
	require.NoError(t, err)
	assert.Empty(t, projects)
}

func TestMemoryProjectsStore_Update(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	// Create a project
	p, err := store.Create(ctx, 1, "Original Name")
	require.NoError(t, err)
	assert.Equal(t, int64(1), p.Revision)

	// Update name
	p.Name = "Updated Name"
	updated, err := store.Update(ctx, p)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, int64(2), updated.Revision)

	// Verify
	got, err := store.Get(ctx, 1, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", got.Name)
}

func TestMemoryProjectsStore_UpdateRevisionConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	// Create a project
	p, err := store.Create(ctx, 1, "Project")
	require.NoError(t, err)

	// Update with correct revision
	p.Name = "First Update"
	_, err = store.Update(ctx, p)
	require.NoError(t, err)

	// Try to update with stale revision
	p.Name = "Second Update"
	// p.Revision is still 1, but DB has 2
	_, err = store.Update(ctx, p)
	assert.ErrorIs(t, err, persistence.ErrRevisionConflict)
}

func TestMemoryProjectsStore_Delete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	// Create a project
	p, err := store.Create(ctx, 1, "To Delete")
	require.NoError(t, err)

	// Delete it
	err = store.Delete(ctx, 1, p.ID)
	require.NoError(t, err)

	// Verify gone
	_, err = store.Get(ctx, 1, p.ID)
	assert.ErrorIs(t, err, persistence.ErrNotFound)

	// Delete again should not error
	err = store.Delete(ctx, 1, p.ID)
	require.NoError(t, err)
}

func TestMemoryProjectsStore_DeleteForbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	// Create project for user 1
	p, err := store.Create(ctx, 1, "User1 Project")
	require.NoError(t, err)

	// Try to delete as user 2
	err = store.Delete(ctx, 2, p.ID)
	assert.ErrorIs(t, err, persistence.ErrForbidden)

	// Project should still exist
	_, err = store.Get(ctx, 1, p.ID)
	require.NoError(t, err)
}

func TestMemoryProjectsStore_UpdateStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	p, err := store.Create(ctx, 1, "Project")
	require.NoError(t, err)
	assert.Equal(t, int64(0), p.Bytes)
	assert.Equal(t, 0, p.FileCount)

	err = store.UpdateStats(ctx, p.ID, 1024, 5)
	require.NoError(t, err)

	got, err := store.Get(ctx, 1, p.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1024), got.Bytes)
	assert.Equal(t, 5, got.FileCount)
}

func TestMemoryProjectsStore_FileIndex(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	p, err := store.Create(ctx, 1, "Project")
	require.NoError(t, err)

	// Index some files
	now := time.Now().UTC()
	files := []persistence.ProjectFile{
		{ProjectID: p.ID, Path: "README.md", Name: "README.md", IsDir: false, Size: 100, ModTime: now},
		{ProjectID: p.ID, Path: "src", Name: "src", IsDir: true, Size: 0, ModTime: now},
		{ProjectID: p.ID, Path: "src/main.go", Name: "main.go", IsDir: false, Size: 500, ModTime: now},
		{ProjectID: p.ID, Path: "src/utils", Name: "utils", IsDir: true, Size: 0, ModTime: now},
		{ProjectID: p.ID, Path: "src/utils/helper.go", Name: "helper.go", IsDir: false, Size: 200, ModTime: now},
	}
	for _, f := range files {
		require.NoError(t, store.IndexFile(ctx, f))
	}

	// List root
	rootFiles, err := store.ListFiles(ctx, p.ID, ".")
	require.NoError(t, err)
	assert.Len(t, rootFiles, 2) // src (dir) and README.md
	assert.True(t, rootFiles[0].IsDir)
	assert.Equal(t, "src", rootFiles[0].Name)
	assert.Equal(t, "README.md", rootFiles[1].Name)

	// List src
	srcFiles, err := store.ListFiles(ctx, p.ID, "src")
	require.NoError(t, err)
	assert.Len(t, srcFiles, 2) // utils (dir) and main.go
	assert.True(t, srcFiles[0].IsDir)
	assert.Equal(t, "utils", srcFiles[0].Name)
	assert.Equal(t, "main.go", srcFiles[1].Name)

	// List src/utils
	utilsFiles, err := store.ListFiles(ctx, p.ID, "src/utils")
	require.NoError(t, err)
	assert.Len(t, utilsFiles, 1)
	assert.Equal(t, "helper.go", utilsFiles[0].Name)

	// Get specific file
	file, err := store.GetFile(ctx, p.ID, "src/main.go")
	require.NoError(t, err)
	assert.Equal(t, "main.go", file.Name)
	assert.Equal(t, int64(500), file.Size)

	// Remove single file
	require.NoError(t, store.RemoveFileIndex(ctx, p.ID, "README.md"))
	rootFiles, err = store.ListFiles(ctx, p.ID, ".")
	require.NoError(t, err)
	assert.Len(t, rootFiles, 1)

	// Remove directory prefix
	require.NoError(t, store.RemoveFileIndexPrefix(ctx, p.ID, "src"))
	rootFiles, err = store.ListFiles(ctx, p.ID, ".")
	require.NoError(t, err)
	assert.Empty(t, rootFiles)
}

func TestMemoryProjectsStore_DefaultName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemoryProjectsStore()
	require.NoError(t, store.Init(ctx))

	p, err := store.Create(ctx, 1, "")
	require.NoError(t, err)
	assert.Equal(t, "Untitled", p.Name)

	p2, err := store.Create(ctx, 1, "   ")
	require.NoError(t, err)
	assert.Equal(t, "Untitled", p2.Name)
}

func TestNormalizePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected string
	}{
		{"", "."},
		{".", "."},
		{"/", "."},
		{"//", "."},
		{"src", "src"},
		{"/src", "src"},
		{"src/", "src"},
		{"/src/", "src"},
		{"src/main.go", "src/main.go"},
		{"/src/main.go", "src/main.go"},
		{"  src  ", "src"},
		{"src/../foo", "foo"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, normalizePath(tc.input))
		})
	}
}

func TestParentDir(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected string
	}{
		{".", "."},
		{"file.txt", "."},
		{"src", "."},
		{"src/main.go", "src"},
		{"src/utils/helper.go", "src/utils"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, parentDir(tc.input))
		})
	}
}
