package webui

import (
	"fmt"
	"io/fs"
	"path"
)

const distDir = "web/agentd-ui/dist"

func DistFS() (fs.FS, error) {
	fsys, err := frontendFS()
	if err != nil {
		return nil, fmt.Errorf("webui: resolve frontend dist: %w", err)
	}
	return fsys, nil
}

func ReadAsset(name string) ([]byte, error) {
	fsys, err := DistFS()
	if err != nil {
		return nil, err
	}
	clean := path.Clean(name)
	if clean == "." {
		clean = "index.html"
	}
	data, err := fs.ReadFile(fsys, clean)
	if err != nil {
		return nil, fmt.Errorf("webui: read asset %q: %w", clean, err)
	}
	return data, nil
}

func IndexHTML() ([]byte, error) {
	return ReadAsset("index.html")
}
