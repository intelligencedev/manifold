//go:build !dev

package webui

import (
	"embed"
	"io/fs"
)

const embeddedDistDir = "dist"

//go:embed dist
var embeddedDist embed.FS

func frontendFS() (fs.FS, error) {
	return fs.Sub(embeddedDist, embeddedDistDir)
}
