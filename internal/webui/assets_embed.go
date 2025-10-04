//go:build !dev

package webui

import (
	"embed"
	"io/fs"
)

const embeddedDistDir = "dist"

// Embed all top-level files and assets, including underscore-prefixed helper chunks
// Note: files beginning with '_' are excluded unless explicitly matched.
// The explicit dist/assets/_* pattern ensures Vite's _plugin-vue_export-helper-*.js is embedded.
//
//go:embed dist/* dist/assets/* dist/assets/_*
var embeddedDist embed.FS

func frontendFS() (fs.FS, error) {
	return fs.Sub(embeddedDist, embeddedDistDir)
}
