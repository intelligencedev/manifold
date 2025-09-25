package patchtool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyChunksToContentHandlesMultipleChunks(t *testing.T) {
	patch := "*** Begin Patch\n" +
		"*** Update File: file.txt\n" +
		"@@\n" +
		" a\n" +
		"-b\n" +
		"+B\n" +
		"@@\n" +
		" c\n" +
		"-d\n" +
		"+D\n" +
		"+e\n" +
		"*** End Patch"

	parsed, err := ParsePatch(patch)
	require.NoError(t, err)
	require.Len(t, parsed.Hunks, 1)
	chunks := parsed.Hunks[0].Chunks

	original := "a\nb\nc\nd\n"
	updated, err := applyChunksToContent("file.txt", original, chunks)
	require.NoError(t, err)
	require.Equal(t, "a\nB\nc\nD\ne\n", updated)
}

func TestApplyStateSupportsMoveAndWrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("line\n"), 0o644))

	state := newApplyState(dir)
	chunk := UpdateChunk{
		ChangeContext: nil,
		OldLines:      []string{"line"},
		NewLines:      []string{"line moved"},
	}
	require.NoError(t, state.updateFile("src.txt", "dst.txt", []UpdateChunk{chunk}))

	added, modified, deleted, moves := state.summarize()
	require.Empty(t, added)
	require.Equal(t, []string{"dst.txt"}, modified)
	require.Empty(t, deleted)
	require.Equal(t, []moveSummary{{From: "src.txt", To: "dst.txt"}}, moves)

	require.NoError(t, state.writeToDisk())

	_, err := os.Stat(src)
	require.True(t, os.IsNotExist(err))

	dest := filepath.Join(dir, "dst.txt")
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, "line moved\n", string(data))
}

func TestApplyStateAddAndDelete(t *testing.T) {
	dir := t.TempDir()
	state := newApplyState(dir)

	require.NoError(t, state.addFile("new.txt", "value\n"))

	old := filepath.Join(dir, "old.txt")
	require.NoError(t, os.WriteFile(old, []byte("old\n"), 0o644))
	require.NoError(t, state.deleteFile("old.txt", true))
	require.Error(t, state.deleteFile("missing.txt", true))

	require.NoError(t, state.writeToDisk())

	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	require.NoError(t, err)
	require.Equal(t, "value\n", string(data))

	_, err = os.Stat(old)
	require.True(t, os.IsNotExist(err))
}
