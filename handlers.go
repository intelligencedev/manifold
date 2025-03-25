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

type toolRequest struct {
	// Change the json tag to "tool" to match your request
	ToolName string `json:"tool"`

	// Change the json tag to "arguments" to match your request
	Args json.RawMessage `json:"arguments"`
}

// toolHandler executes a tool based on the provided JSON input.
func toolHandler(c echo.Context) error {
	// 1) Parse the incoming JSON body
	var req toolRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON body: " + err.Error(),
		})
	}

	// 2) Call the tool
	result, err := ExecuteToolByName(req.ToolName, req.Args)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	// 3) Return the tool output
	return c.JSON(http.StatusOK, map[string]string{
		"result": result,
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
