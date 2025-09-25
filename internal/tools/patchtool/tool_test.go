package patchtool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolCallAppliesPatchEndToEnd(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "update.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "remove.txt"), []byte("old\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "move_src.txt"), []byte("line\n"), 0o644))

	patch := "*** Begin Patch\n" +
		"*** Add File: added.txt\n" +
		"+hello\n" +
		"+world\n" +
		"*** Update File: update.txt\n" +
		"@@\n" +
		" alpha\n" +
		"-beta\n" +
		"+beta2\n" +
		" gamma\n" +
		"*** Delete File: remove.txt\n" +
		"*** Update File: move_src.txt\n" +
		"*** Move to: move_dst.txt\n" +
		"@@\n" +
		"-line\n" +
		"+line moved\n" +
		"*** End Patch"

	tool := New(dir)

	payload, err := json.Marshal(map[string]any{"patch": patch, "dry_run": true})
	require.NoError(t, err)

	resAny, err := tool.Call(context.Background(), payload)
	require.NoError(t, err)
	dryRes, ok := resAny.(callResult)
	require.True(t, ok)
	require.True(t, dryRes.OK)
	require.True(t, dryRes.DryRun)
	require.ElementsMatch(t, []string{"added.txt", "move_dst.txt", "update.txt", "remove.txt", "move_src.txt"}, dryRes.Files)

	// Files untouched after dry-run.
	data, err := os.ReadFile(filepath.Join(dir, "update.txt"))
	require.NoError(t, err)
	require.Equal(t, "alpha\nbeta\ngamma\n", string(data))

	payload, err = json.Marshal(map[string]any{"patch": patch})
	require.NoError(t, err)

	resAny, err = tool.Call(context.Background(), payload)
	require.NoError(t, err)
	res, ok := resAny.(callResult)
	require.True(t, ok)
	require.True(t, res.OK)
	require.ElementsMatch(t, []string{"added.txt", "move_dst.txt", "update.txt", "remove.txt", "move_src.txt"}, res.Files)
	require.ElementsMatch(t, []string{"added.txt"}, res.Added)
	require.ElementsMatch(t, []string{"update.txt", "move_dst.txt"}, res.Modified)
	require.ElementsMatch(t, []string{"remove.txt"}, res.Deleted)
	require.Equal(t, []moveSummary{{From: "move_src.txt", To: "move_dst.txt"}}, res.Moves)

	data, err = os.ReadFile(filepath.Join(dir, "added.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello\nworld\n", string(data))

	data, err = os.ReadFile(filepath.Join(dir, "update.txt"))
	require.NoError(t, err)
	require.Equal(t, "alpha\nbeta2\ngamma\n", string(data))

	_, err = os.Stat(filepath.Join(dir, "remove.txt"))
	require.True(t, os.IsNotExist(err))

	data, err = os.ReadFile(filepath.Join(dir, "move_dst.txt"))
	require.NoError(t, err)
	require.Equal(t, "line moved\n", string(data))

	_, err = os.Stat(filepath.Join(dir, "move_src.txt"))
	require.True(t, os.IsNotExist(err))
}
