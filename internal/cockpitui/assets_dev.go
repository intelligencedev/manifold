//go:build dev_cockpit

package cockpitui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const distDir = "frontend/dist"

func frontendFS() (fs.FS, error) {
	dir := os.Getenv("AGENTD_WEB_DIST")
	if dir == "" { dir = filepath.FromSlash(distDir) }
	info, err := os.Stat(dir)
	if err != nil { return nil, fmt.Errorf("locate dev cockpit dist: %w", err) }
	if !info.IsDir() { return nil, fmt.Errorf("dev cockpit dist %q is not a directory", dir) }
	return os.DirFS(dir), nil
}
