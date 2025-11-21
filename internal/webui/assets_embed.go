//go:build !dev

package webui

import (
	"embed"
	"io/fs"
)

const embeddedDistDir = "dist"

// Embed all top-level files and assets
// Note: files beginning with '_' are excluded by default in embed patterns.
// If Vite generates underscore-prefixed files, add dist/assets/_* pattern.
//
//go:embed dist/* dist/assets/*
var embeddedDist embed.FS

func frontendFS() (fs.FS, error) {
	return fs.Sub(embeddedDist, embeddedDistDir)
}
