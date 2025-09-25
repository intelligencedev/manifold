package patchtool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePatchHandlesAddDeleteUpdate(t *testing.T) {
	patchText := "*** Begin Patch\n" +
		"*** Add File: foo.txt\n" +
		"+hello\n" +
		"*** Delete File: old.txt\n" +
		"*** Update File: mod.txt\n" +
		"@@ context\n" +
		" context\n" +
		"-line1\n" +
		"+line2\n" +
		"*** End Patch"

	parsed, err := ParsePatch(patchText)
	require.NoError(t, err)
	require.Len(t, parsed.Hunks, 3)

	require.Equal(t, hunkAdd, parsed.Hunks[0].Kind)
	require.Equal(t, "foo.txt", parsed.Hunks[0].Path)
	require.Equal(t, "hello\n", parsed.Hunks[0].Contents)

	require.Equal(t, hunkDelete, parsed.Hunks[1].Kind)
	require.Equal(t, "old.txt", parsed.Hunks[1].Path)

	require.Equal(t, hunkUpdate, parsed.Hunks[2].Kind)
	require.Equal(t, "mod.txt", parsed.Hunks[2].Path)
	require.Len(t, parsed.Hunks[2].Chunks, 1)
	chunk := parsed.Hunks[2].Chunks[0]
	require.NotNil(t, chunk.ChangeContext)
	require.Equal(t, "context", *chunk.ChangeContext)
	require.Equal(t, []string{"context", "line1"}, chunk.OldLines)
	require.Equal(t, []string{"context", "line2"}, chunk.NewLines)
}

func TestParsePatchLenientHeredoc(t *testing.T) {
	patchText := "<<'EOF'\n*** Begin Patch\n*** Add File: new.txt\n+value\n*** End Patch\nEOF\n"
	parsed, err := ParsePatch(patchText)
	require.NoError(t, err)
	require.Len(t, parsed.Hunks, 1)
	require.Equal(t, "new.txt", parsed.Hunks[0].Path)
}

func TestParsePatchRejectsInvalidHeader(t *testing.T) {
	_, err := ParsePatch("bad data")
	require.Error(t, err)
}
