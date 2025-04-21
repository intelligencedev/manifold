// handlers.go
//
// MCP handler for proxy to MCP servers configured in config.yaml
//
// The main functionality has been moved to a dedicated MCP server in cmd/mcp-manifold
// This file now only contains code to proxy requests to that server and other external
// MCP servers like GitHub.
//
// ---------------------------------------------------------------------------

package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ---------------------------------------------------------------------------
// Static assets &  basic config endpoint
// ---------------------------------------------------------------------------

// configHandler returns the parsed config.yaml.
func configHandler(c echo.Context) error {
	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("config load: %v", err)})
	}
	return c.JSON(http.StatusOK, cfg)
}

// getFileSystem serves the SPA bundle embedded via //go:embed in frontendDist.
func getFileSystem() http.FileSystem {
	sub, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("embed FS error: %v", err)
	}
	return http.FS(sub)
}
