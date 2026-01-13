package filetool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"manifold/internal/sandbox"
)

func TestReadToolSingleFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	base := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(base, 0o755))

	want := "alpha\nbeta\n"
	require.NoError(t, os.WriteFile(filepath.Join(base, "note.txt"), []byte(want), 0o644))

	tool := NewReadTool([]string{tmp}, 0)
	ctx := sandbox.WithBaseDir(context.Background(), base)

	respAny, err := tool.Call(ctx, json.RawMessage(`{"path":"note.txt"}`))
	require.NoError(t, err)

	resp := respAny.(readResult)
	require.True(t, resp.OK)
	require.Equal(t, "note.txt", resp.Path)
	require.Equal(t, want, resp.Content)
	require.Equal(t, "utf-8", resp.Encoding)
	require.False(t, resp.Truncated)
}

func TestReadToolMultipleFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	base := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "a.txt"), []byte("one"), 0o644))

	tool := NewReadTool([]string{tmp}, 0)
	ctx := sandbox.WithBaseDir(context.Background(), base)

	respAny, err := tool.Call(ctx, json.RawMessage(`{"paths":["a.txt","missing.txt"]}`))
	require.NoError(t, err)

	resp := respAny.(readResult)
	require.True(t, resp.OK)
	require.Len(t, resp.Files, 2)
}

func TestWriteToolCreatesFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	base := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(base, 0o755))

	tool := NewWriteTool([]string{tmp}, 0)
	ctx := sandbox.WithBaseDir(context.Background(), base)

	respAny, err := tool.Call(ctx, json.RawMessage(`{"path":"dir/out.txt","content":"hello"}`))
	require.NoError(t, err)

	resp := respAny.(writeResult)
	require.True(t, resp.OK)
	require.True(t, resp.Created)
	data, err := os.ReadFile(filepath.Join(base, "dir", "out.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}

func TestPatchToolReplacesLine(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	base := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "doc.txt"), []byte("one\ntwo\nthree\n"), 0o644))

	tool := NewPatchTool([]string{tmp}, 0)
	ctx := sandbox.WithBaseDir(context.Background(), base)

	respAny, err := tool.Call(ctx, json.RawMessage(`{"path":"doc.txt","start_line":2,"end_line":2,"content":"TWO"}`))
	require.NoError(t, err)

	resp := respAny.(patchResult)
	require.True(t, resp.OK)
	data, err := os.ReadFile(filepath.Join(base, "doc.txt"))
	require.NoError(t, err)
	require.Equal(t, "one\nTWO\nthree\n", string(data))
}

func TestReadToolRequiresBaseDir(t *testing.T) {
	t.Parallel()

	tool := NewReadTool([]string{t.TempDir()}, 0)
	respAny, err := tool.Call(context.Background(), json.RawMessage(`{"path":"file.txt"}`))
	require.NoError(t, err)

	resp := respAny.(readResult)
	require.False(t, resp.OK)
	require.Contains(t, resp.Error, "base directory")
}

func TestDeleteToolDeletesFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	base := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "old.txt"), []byte("bye"), 0o644))

	tool := NewDeleteTool([]string{tmp})
	ctx := sandbox.WithBaseDir(context.Background(), base)

	respAny, err := tool.Call(ctx, json.RawMessage(`{"path":"old.txt"}`))
	require.NoError(t, err)

	resp := respAny.(deleteResult)
	require.True(t, resp.OK)
	require.Equal(t, "old.txt", resp.Path)
	require.True(t, resp.Deleted)
	require.False(t, resp.WasDir)

	_, err = os.Stat(filepath.Join(base, "old.txt"))
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestDeleteToolRequiresRecursiveForDirectory(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	base := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(base, "dir"), 0o755))

	tool := NewDeleteTool([]string{tmp})
	ctx := sandbox.WithBaseDir(context.Background(), base)

	respAny, err := tool.Call(ctx, json.RawMessage(`{"path":"dir"}`))
	require.NoError(t, err)

	resp := respAny.(deleteResult)
	require.False(t, resp.OK)
	require.Contains(t, resp.Error, "recursive")
}

func TestDeleteToolDeletesDirectoryRecursive(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	base := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(base, "dir", "nest"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "dir", "nest", "file.txt"), []byte("data"), 0o644))

	tool := NewDeleteTool([]string{tmp})
	ctx := sandbox.WithBaseDir(context.Background(), base)

	respAny, err := tool.Call(ctx, json.RawMessage(`{"path":"dir","recursive":true}`))
	require.NoError(t, err)

	resp := respAny.(deleteResult)
	require.True(t, resp.OK)
	require.True(t, resp.WasDir)
	require.True(t, resp.Deleted)

	_, err = os.Stat(filepath.Join(base, "dir"))
	require.True(t, errors.Is(err, os.ErrNotExist))
}
