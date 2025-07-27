package main

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"manifold/internal/mcp"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

// InternalMCPHandler manages the routes for internal MCP tools.
type InternalMCPHandler struct {
	config    *Config
	mcpMgr    *mcp.Manager
	configDir string
}

// NewInternalMCPHandler creates a new handler for internal MCP functionality.
func NewInternalMCPHandler(config *Config) (*InternalMCPHandler, error) {
	configPath := "config.yaml"
	configDir := filepath.Dir(configPath)

	ctx := context.Background()

	// Add better error handling and logging for MCP configuration issues
	logrus.Info("Loading MCP server configurations...")
	mcpMgr, err := mcp.NewManager(ctx, configPath)
	if err != nil {
		logrus.Errorf("Failed to initialize MCP manager: %v", err)
		logrus.Warn("This could be due to:")
		logrus.Warn("  - Invalid MCP server configuration in config.yaml")
		logrus.Warn("  - MCP servers not responding or unavailable")
		logrus.Warn("  - Docker containers not running")
		logrus.Warn("  - Network connectivity issues")
		logrus.Info("MCP functionality will be disabled until configuration issues are resolved")
		return nil, fmt.Errorf("creating MCP manager: %w", err)
	}

	logrus.Info("MCP handler initialized successfully")
	return &InternalMCPHandler{
		config:    config,
		mcpMgr:    mcpMgr,
		configDir: configDir,
	}, nil
}

// listServersHandler returns a list of configured MCP servers.
func (h *InternalMCPHandler) listServersHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string][]string{
		"servers": h.mcpMgr.List(),
	})
}

// listServerToolsHandler lists the available tools for a specific MCP server.
func (h *InternalMCPHandler) listServerToolsHandler(c echo.Context) error {
	ctx := c.Request().Context()
	serverName := c.Param("name")

	tools, err := h.mcpMgr.ListTools(ctx, serverName)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("listing tools: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"server": serverName,
		"tools":  tools,
	})
}

// callServerToolHandler calls a specific tool on a specific MCP server.
func (h *InternalMCPHandler) callServerToolHandler(c echo.Context) error {
	ctx := c.Request().Context()
	serverName := c.Param("name")
	toolName := c.Param("tool")

	// Parse payload for arguments
	var payload map[string]interface{}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid JSON payload",
		})
	}

	// Get tool arguments from the payload
	args, ok := payload["args"]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "missing 'args' field in payload",
		})
	}

	// Call the tool on the specified server
	response, err := h.mcpMgr.CallTool(ctx, serverName, toolName, args)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("calling tool: %v", err),
		})
	}

	return c.JSON(http.StatusOK, response)
}

// Close cleans up resources when the server is shutting down.
func (h *InternalMCPHandler) Close() {
	if h.mcpMgr != nil {
		h.mcpMgr.Close()
	}
}
