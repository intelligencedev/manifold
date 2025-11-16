package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func isPathTraversal(p string) bool {
	clean := filepath.Clean(p)
	return strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || clean == ".."
}

func isAbsoluteOrDrive(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if runtime.GOOS == "windows" {
		if len(p) >= 2 && p[1] == ':' {
			return true
		}
	}
	return false
}

// SanitizeArg returns a safe, cleaned argument if it looks like a path.
// It rejects absolute paths and traversal, and ensures the final path
// would remain under WORKDIR when joined.
func SanitizeArg(workdir, arg string) (string, error) {
	if !looksPathLike(arg) {
		return arg, nil
	}
	if workdir == "" {
		return "", errors.New("workdir is required")
	}
	if isAbsoluteOrDrive(arg) {
		return "", fmt.Errorf("absolute paths not allowed in args: %q", arg)
	}
	if isPathTraversal(arg) {
		return "", fmt.Errorf("path traversal not allowed in args: %q", arg)
	}

	rel := filepath.Clean(arg)
	if rel == "." {
		return rel, nil
	}
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("argument must stay inside workdir: %q", arg)
	}
	if err := ensureWithinRoot(workdir, rel); err != nil {
		return "", err
	}
	return rel, nil
}

func ensureWithinRoot(workdir, rel string) error {
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return fmt.Errorf("open root %q: %w", workdir, err)
	}
	defer root.Close()

	candidate := rel
	for candidate != "" && candidate != "." {
		f, err := root.Open(candidate)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				candidate = filepath.Dir(candidate)
				continue
			}
			return fmt.Errorf("path %q escapes workdir: %w", rel, err)
		}
		f.Close()
		break
	}
	return nil
}

func looksPathLike(arg string) bool {
	if arg == "" {
		return false
	}
	if strings.HasPrefix(arg, ".") {
		return true
	}
	if strings.ContainsRune(arg, os.PathSeparator) {
		return true
	}
	return strings.ContainsRune(arg, '/') || strings.ContainsRune(arg, '\\')
}

func IsBinaryBlocked(cmd string, block map[string]struct{}) bool {
	if strings.Contains(cmd, "/") || strings.Contains(cmd, "\\") {
		return true
	}
	if len(block) == 0 {
		return false
	}
	_, ok := block[cmd]
	return ok
}
