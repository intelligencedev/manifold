package sandbox

import (
	"fmt"
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
	if !(strings.Contains(arg, "/") || strings.Contains(arg, `\`) || strings.HasPrefix(arg, ".")) {
		return arg, nil
	}
	if isAbsoluteOrDrive(arg) {
		return "", fmt.Errorf("absolute paths not allowed in args: %q", arg)
	}
	if isPathTraversal(arg) {
		return "", fmt.Errorf("path traversal not allowed in args: %q", arg)
	}
	rel := filepath.Clean(arg)
	target := filepath.Clean(filepath.Join(workdir, rel))
	workdirWithSep := workdir
	if !strings.HasSuffix(workdirWithSep, string(os.PathSeparator)) {
		workdirWithSep += string(os.PathSeparator)
	}
	if !(target == workdir || strings.HasPrefix(target, workdirWithSep)) {
		return "", fmt.Errorf("arg escapes WORKDIR: %q", arg)
	}
	return rel, nil
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
