package webui

import (
	"fmt"
	"io/fs"
)

func DistFS() (fs.FS, error) {
	fsys, err := frontendFS()
	if err != nil {
		return nil, fmt.Errorf("webui: resolve frontend dist: %w", err)
	}
	return fsys, nil
}
