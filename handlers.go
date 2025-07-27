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
	"net/http"

	"github.com/labstack/echo/v4"

	configpkg "manifold/internal/config"
)

// ---------------------------------------------------------------------------
// Static assets &  basic config endpoint
// ---------------------------------------------------------------------------

// configHandler returns the parsed config.yaml.
func configHandler(c echo.Context) error {
	cfg, err := configpkg.LoadConfig("config.yaml")
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

// func a2aHandler(c echo.Context) error {
// 	desc := "just returns hello world"
// 	helloSkill := a2a.AgentSkill{
// 		Id:          "hello_world",
// 		Name:        "Returns hello world",
// 		Description: &desc,
// 		Tags:        []string{"hello world"},
// 		Examples:    []string{"hi", "hello world"},
// 	}

// 	card := a2a.AgentCard{
// 		Name:               "Hello World Agent",
// 		Description:        &desc,
// 		Url:                "http://localhost:9999/", // Agent will run here
// 		Version:            "1.0.0",
// 		DefaultInputModes:  []string{"text"},
// 		DefaultOutputModes: []string{"text"},
// 		Capabilities:       a2a.AgentCapabilities{},                               // Basic capabilities
// 		Skills:             []a2a.AgentSkill{helloSkill},                          // Includes the skill defined above
// 		Authentication:     &a2a.AgentAuthentication{Schemes: []string{"public"}}, // No auth needed
// 	}

// }
