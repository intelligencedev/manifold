// routes.go
package main

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// registerRoutes sets up all the routes for the application.
func registerRoutes(e *echo.Echo, config *Config) {
	// Serve static frontend files.
	e.GET("/*", echo.WrapHandler(http.FileServer(getFileSystem())))
	e.Static("/tmp", config.DataPath+"/tmp")

	// API group for all API endpoints.
	api := e.Group("/api")
	registerAPIEndpoints(api, config)
}

// registerAPIEndpoints registers all API-related routes.
func registerAPIEndpoints(api *echo.Group, config *Config) {
	api.GET("/config", configHandler)

	// Completions proxy endpoints.
	registerCompletionsEndpoints(api, config)

	// SEFII endpoints.
	registerSEFIIEndpoints(api, config)

	// MCP endpoints
	registerMCPEndpoints(api, config)

	// Git-related endpoints.
	api.GET("/git-files", gitFilesHandler)
	api.POST("/git-files/ingest", gitFilesIngestHandler(config))

	// Anthropic endpoints.
	registerAnthropicEndpoints(api, config)

	// File upload endpoints
	api.POST("/upload", func(c echo.Context) error {
		return fileUploadHandler(c, config)
	})
	api.POST("/upload-multiple", func(c echo.Context) error {
		return fileUploadMultipleHandler(c, config)
	})

	// Miscellaneous endpoints.
	api.POST("/run-fmlx", func(c echo.Context) error {
		return runFMLXHandler(c, config.DataPath)
	})
	api.POST("/run-sd", runSDHandler)
	api.POST("/repoconcat", repoconcatHandler)
	api.POST("/split-text", splitTextHandler)
	api.POST("/save-file", saveFileHandler)
	api.POST("/open-file", openFileHandler)
	api.GET("/web-content", webContentHandler)
	api.GET("/web-search", webSearchHandler)
	api.POST("/code/eval", evaluateCodeHandler)
	api.POST("/datadog", datadogHandler)
	api.POST("/comfy-proxy", comfyProxyHandler)

	// Agentic Memory endpoints.
	registerAgenticMemoryEndpoints(api, config)
}

// registerMCPEndpoints registers all MCP-related routes.
func registerMCPEndpoints(api *echo.Group, config *Config) {
	// Main combined MCP endpoint (internal + external GitHub tools)
	api.POST("/mcp", executeMCPCombinedHandler)

	// Create an MCP subgroup for more specific endpoints
	mcpGroup := api.Group("/mcp")

	// Internal MCP tools only
	mcpGroup.POST("/internal", executeMCPInternalHandler)

	// External GitHub MCP tools only
	mcpGroup.POST("/github", executeMCPGitHubHandler)

	// List available tools endpoints
	mcpGroup.GET("/tools", listMCPToolsHandler)
	mcpGroup.GET("/tools/internal", listInternalMCPToolsHandler)
	mcpGroup.GET("/tools/github", listGitHubMCPToolsHandler)

	// Tool-specific endpoints for direct execution
	mcpGroup.POST("/tools/:toolName", executeMCPToolHandler)

	// Set up the internal MCP handler with server management
	internalMCPHandler, err := NewInternalMCPHandler(config)
	if err != nil {
		log.Printf("Error creating internal MCP handler: %v", err)
	} else {
		// Register internal MCP server routes under /api/mcp/internal/
		internalMCPHandler.RegisterRoutes(mcpGroup.Group("/internal"))
	}
}

// registerCompletionsEndpoints registers routes for completions-related functionality.
func registerCompletionsEndpoints(api *echo.Group, config *Config) {
	completionsGroup := api.Group("/v1")
	completionsGroup.POST("/chat/completions", func(c echo.Context) error {
		return completionsHandler(c, config)
	})
}

// registerSEFIIEndpoints registers routes for SEFII-related functionality.
func registerSEFIIEndpoints(api *echo.Group, config *Config) {
	sefiiGroup := api.Group("/sefii")
	sefiiGroup.POST("/ingest", sefiiIngestHandler(config))
	sefiiGroup.POST("/search", sefiiSearchHandler(config))
	sefiiGroup.POST("/combined-retrieve", sefiiCombinedRetrieveHandler(config))
}

// registerAnthropicEndpoints registers routes for Anthropic-related functionality.
func registerAnthropicEndpoints(api *echo.Group, config *Config) {
	anthropicGroup := api.Group("/anthropic")
	anthropicGroup.POST("/messages", func(c echo.Context) error {
		return handleAnthropicMessages(c, config)
	})
}

// registerAgenticMemoryEndpoints registers routes for Agentic Memory-related functionality.
func registerAgenticMemoryEndpoints(api *echo.Group, config *Config) {
	agenticGroup := api.Group("/agentic-memory")
	agenticGroup.POST("/ingest", agenticMemoryIngestHandler(config))
	agenticGroup.POST("/search", agenticMemorySearchHandler(config))
}
