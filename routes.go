package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	a2aAuth "manifold/internal/a2a/auth"
	a2aServer "manifold/internal/a2a/server"
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

	// Workflow templates endpoints
	registerWorkflowEndpoints(api, config)

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

	// ===== A2A server integration =====
	// Create the a2a server instance with in-memory store and noop authenticator
	 a2aStore := a2aServer.NewInMemory()
	 a2aAuth := a2aAuth.NewNoop()
	 a2aSrv := a2aServer.NewServer(a2aStore, a2aAuth)

	// Mount the a2a server to handle all requests under /api/a2a/*
	 api.Any("/a2a/*", echo.WrapHandler(http.StripPrefix("/api/a2a", a2aSrv)))
}
