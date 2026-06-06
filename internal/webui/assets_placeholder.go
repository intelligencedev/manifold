//go:build !dev && !forge

package webui

import (
	"embed"
	"io/fs"
)

const placeholderDistDir = "placeholder_dist"

//go:embed placeholder_dist/index.html
var placeholderDist embed.FS

func frontendFS() (fs.FS, error) {
	return fs.Sub(placeholderDist, placeholderDistDir)
}
