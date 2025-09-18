//go:build dev

package webui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func frontendFS() (fs.FS, error) {
	dir := os.Getenv("AGENTD_WEB_DIST")
	if dir == "" {
		dir = filepath.FromSlash(distDir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("locate dev frontend dist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("dev frontend dist %q is not a directory", dir)
	}

	return os.DirFS(dir), nil
}
