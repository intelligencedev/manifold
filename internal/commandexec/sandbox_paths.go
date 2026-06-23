package commandexec

import (
	"path/filepath"
	"strings"
)

func sandboxReadPaths(workdir string, configured []string) []string {
	paths := make([]string, 0, len(configured)*2)
	for _, path := range configured {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(workdir, path)
		}
		paths = append(paths, path)
		if realPath, err := filepath.EvalSymlinks(path); err == nil {
			paths = append(paths, realPath)
		}
	}
	return compactPaths(paths)
}

func compactPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}
