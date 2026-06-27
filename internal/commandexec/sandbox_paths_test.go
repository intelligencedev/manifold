package commandexec

import (
	"path/filepath"
	"testing"
)

func TestSandboxReadPathsCleansDedupesAndResolvesRelativePaths(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	paths := sandboxReadPaths(workdir, []string{
		"",
		"/private/etc/ssl",
		"/private/etc/ssl/",
		"cache/../readonly",
	})

	want := []string{
		"/private/etc/ssl",
		filepath.Join(workdir, "readonly"),
	}
	if len(paths) != len(want) {
		t.Fatalf("sandboxReadPaths() = %#v, want %#v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("sandboxReadPaths()[%d] = %q, want %q; all=%#v", i, paths[i], want[i], paths)
		}
	}
}
