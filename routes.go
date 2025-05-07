// routes.go
package main

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// registerRoutes sets up all the routes for the application.
func registerRoutes(e *echo.Echo, config *Config) {
	// Authentication routes - publicly accessible
	e.POST("/api/auth/login", loginHandler)
	e.POST("/api/auth/register", registerHandler)

	// Serve static frontend files.
	e.GET("/*", echo.WrapHandler(http.FileServer(getFileSystem())))
	e.Static("/tmp", config.DataPath+"/tmp")

	// API group for all API endpoints.
	api := e.Group("/api")

	// Authentication protected routes
	restricted := api.Group("/restricted")
	// Apply JWT middleware to protected routes
	restricted.Use(configureJWTMiddleware(config))
	restricted.GET("", restrictedHandler) // Sample protected route
	restricted.GET("/user", getUserInfoHandler)
	restricted.POST("/logout", logoutHandler)
	restricted.POST("/change-password", changePasswordHandler) // New endpoint for changing password

	// Register other API endpoints
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

	// Session management endpoints
	registerSessionEndpoints(api)

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

	// ===== ReAct Agentic System endpoints =====
	registerAgentEndpoints(api, config)
}

// registerSessionEndpoints registers routes for session management.
func registerSessionEndpoints(api *echo.Group) {
	sessionGroup := api.Group("/session")
	sessionGroup.POST("/create", createSessionHandler)
	sessionGroup.GET("/current", getSessionHandler)
	sessionGroup.DELETE("/destroy", destroySessionHandler)
}

// registerMCPEndpoints registers all MCP-related routes.
func registerMCPEndpoints(api *echo.Group, config *Config) {
	// Create an MCP subgroup for all MCP-related endpoints
	mcpGroup := api.Group("/mcp")

	// Set up the internal MCP handler with server management
	internalMCPHandler, err := NewInternalMCPHandler(config)
	if err != nil {
		log.Printf("Error creating internal MCP handler: %v", err)
		return
	}

	// Register routes to interact with the configured MCP servers
	// This gives access to our new mcp-manifold server through the Manager
	mcpGroup.GET("/servers", internalMCPHandler.listServersHandler)
	mcpGroup.GET("/servers/:name/tools", internalMCPHandler.listServerToolsHandler)
	mcpGroup.POST("/servers/:name/tools/:tool", internalMCPHandler.callServerToolHandler)
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

// registerAgentEndpoints registers all routes for the ReAct / advanced agentic system.
func registerAgentEndpoints(api *echo.Group, config *Config) {
	agents := api.Group("/agents")
	agents.POST("/react", runReActAgentHandler(config))              // Kick‑off a new ReAct session and run to completion
	agents.POST("/react/stream", runReActAgentStreamHandler(config)) // Streaming endpoint for real-time thoughts
}
