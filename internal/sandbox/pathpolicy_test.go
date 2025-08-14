package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPathTraversal(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"../etc/passwd", true},
		{"foo/../bar", false},
		{"..", true},
		{"safe/path", false},
		{"./ok", false},
	}
	for _, c := range cases {
		if got := isPathTraversal(c.in); got != c.want {
			t.Fatalf("isPathTraversal(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsAbsoluteOrDrive(t *testing.T) {
	// absolute paths test
	if !isAbsoluteOrDrive("/usr/bin") {
		t.Fatalf("expected absolute to be true")
	}
	// windows drive (simulate)
	if !isAbsoluteOrDrive("C:foo") && os.PathSeparator == '\\' {
		t.Skip("skipping windows-specific test on non-windows platform")
	}
}

func TestSanitizeArg(t *testing.T) {
	wd := filepath.Clean("/tmp/workdir")
	// normal file
	r, err := SanitizeArg(wd, "file.txt")
	if err != nil || r != "file.txt" {
		t.Fatalf("expected file.txt, got %q err=%v", r, err)
	}
	// traversal
	if _, err := SanitizeArg(wd, "../escape"); err == nil {
		t.Fatalf("expected traversal to error")
	}
	// absolute
	if _, err := SanitizeArg(wd, "/etc/passwd"); err == nil {
		t.Fatalf("expected absolute to error")
	}
	// a normal subdir should be allowed
	if _, err := SanitizeArg(wd, "otherdir/file"); err != nil {
		t.Fatalf("expected subpath to be allowed, got err=%v", err)
	}
}

func TestIsBinaryBlocked(t *testing.T) {
	block := map[string]struct{}{"rm": {}}
	if !IsBinaryBlocked("/bin/rm", block) {
		t.Fatalf("expected path with slash to be blocked")
	}
	if !IsBinaryBlocked("rm", block) {
		t.Fatalf("expected blocked command to be blocked")
	}
	if IsBinaryBlocked("ls", block) {
		t.Fatalf("expected ls to be allowed")
	}
}
