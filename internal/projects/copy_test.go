package projects

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyDir_RecursiveAndOverwrite(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	if err := os.MkdirAll(filepath.Join(src, "a", "references"), 0o755); err != nil {
		t.Fatalf("MkdirAll src error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "a", "SKILL.md"), []byte("skill"), 0o644); err != nil {
		t.Fatalf("WriteFile src skill error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "a", "references", "ref.txt"), []byte("ref"), 0o644); err != nil {
		t.Fatalf("WriteFile src ref error: %v", err)
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("MkdirAll dst error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile dst old error: %v", err)
	}

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected overwrite to remove old dst contents, stat err=%v", err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "a", "references", "ref.txt"))
	if err != nil {
		t.Fatalf("ReadFile copied ref error: %v", err)
	}
	if string(b) != "ref" {
		t.Fatalf("unexpected copied ref contents: %q", string(b))
	}
}

func TestCopyDir_SkipsSymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on windows")
	}
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("MkdirAll src error: %v", err)
	}
	realFile := filepath.Join(src, "real.txt")
	if err := os.WriteFile(realFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile real error: %v", err)
	}
	if err := os.Symlink(realFile, filepath.Join(src, "link.txt")); err != nil {
		t.Fatalf("Symlink error: %v", err)
	}

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "real.txt")); err != nil {
		t.Fatalf("expected real file copied: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "link.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected symlink skipped, lstat err=%v", err)
	}
}
