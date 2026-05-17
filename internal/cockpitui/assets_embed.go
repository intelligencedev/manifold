//go:build !dev_cockpit

package cockpitui

import (
	"embed"
	"io/fs"
)

const embeddedDistDir = "dist"

//go:embed dist/* dist/assets/*
var embeddedDist embed.FS

func frontendFS() (fs.FS, error) { return fs.Sub(embeddedDist, embeddedDistDir) }
