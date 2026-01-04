package objectstore

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_PutAndGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()

	content := []byte("hello, world!")

	// Put an object
	etag, err := store.Put(ctx, "test/file.txt", bytes.NewReader(content), PutOptions{
		ContentType: "text/plain",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, etag)

	// Get the object back
	reader, attrs, err := store.Get(ctx, "test/file.txt")
	require.NoError(t, err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, data)
	assert.Equal(t, "test/file.txt", attrs.Key)
	assert.Equal(t, int64(len(content)), attrs.Size)
	assert.Equal(t, "text/plain", attrs.ContentType)
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()

	_, _, err := store.Get(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_Delete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()

	// Put then delete
	_, err := store.Put(ctx, "to-delete", bytes.NewReader([]byte("data")), PutOptions{})
	require.NoError(t, err)

	err = store.Delete(ctx, "to-delete")
	require.NoError(t, err)

	// Should not exist anymore
	_, _, err = store.Get(ctx, "to-delete")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_List(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()

	// Create some objects
	files := []string{
		"dir1/file1.txt",
		"dir1/file2.txt",
		"dir1/sub/file3.txt",
		"dir2/file4.txt",
		"root.txt",
	}
	for _, f := range files {
		_, err := store.Put(ctx, f, bytes.NewReader([]byte("content")), PutOptions{})
		require.NoError(t, err)
	}

	// List all
	result, err := store.List(ctx, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, result.Objects, 5)

	// List with prefix
	result, err = store.List(ctx, ListOptions{Prefix: "dir1/"})
	require.NoError(t, err)
	assert.Len(t, result.Objects, 3)

	// List with delimiter (pseudo-directory mode)
	result, err = store.List(ctx, ListOptions{Prefix: "", Delimiter: "/"})
	require.NoError(t, err)
	assert.Len(t, result.Objects, 1) // root.txt
	assert.Contains(t, result.CommonPrefixes, "dir1/")
	assert.Contains(t, result.CommonPrefixes, "dir2/")
}

func TestMemoryStore_Head(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()

	content := []byte("test content")
	_, err := store.Put(ctx, "test.txt", bytes.NewReader(content), PutOptions{
		ContentType: "text/plain",
	})
	require.NoError(t, err)

	attrs, err := store.Head(ctx, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, "test.txt", attrs.Key)
	assert.Equal(t, int64(len(content)), attrs.Size)
	assert.Equal(t, "text/plain", attrs.ContentType)

	// Head nonexistent
	_, err = store.Head(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_Copy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()

	content := []byte("copy me")
	_, err := store.Put(ctx, "original", bytes.NewReader(content), PutOptions{})
	require.NoError(t, err)

	err = store.Copy(ctx, "original", "copy")
	require.NoError(t, err)

	// Both should exist with same content
	reader, _, err := store.Get(ctx, "copy")
	require.NoError(t, err)
	defer reader.Close()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, data)

	// Copy nonexistent
	err = store.Copy(ctx, "nonexistent", "dest")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryStore_Exists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore()

	exists, err := store.Exists(ctx, "test")
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = store.Put(ctx, "test", bytes.NewReader([]byte("data")), PutOptions{})
	require.NoError(t, err)

	exists, err = store.Exists(ctx, "test")
	require.NoError(t, err)
	assert.True(t, exists)
}
