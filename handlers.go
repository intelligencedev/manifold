package main

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	mcp "github.com/metoro-io/mcp-golang"
)

// toolListHandler returns the list of available tools, including name,
// description, and an example JSON snippet for each.
func toolListHandler(c echo.Context) error {
	tools := GetAllTools()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"tools": tools,
	})
}

// toolExecuteHandler executes a tool by name, passing in the JSON arguments.
func toolExecuteHandler(c echo.Context) error {
	var req struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid JSON payload"})
	}

	// Call the appropriate tool function
	output, err := ExecuteToolByName(req.Tool, req.Arguments)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Return the tool’s result
	return c.JSON(http.StatusOK, map[string]interface{}{
		"tool":   req.Tool,
		"output": output,
	})
}

func WithServerName(name string) func(*mcp.ServerOptions) {
	return func(opts *mcp.ServerOptions) {
		WithServerName(name)(opts)
	}
}

func configHandler(c echo.Context) error {
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load config"})
	}
	return c.JSON(http.StatusOK, config)
}

func getFileSystem() http.FileSystem {
	fsys, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to get file system: %v", err)
	}
	return http.FS(fsys)
}
