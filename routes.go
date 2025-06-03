// routes.go
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"manifold/internal/a2a"
	agentspkg "manifold/internal/agents"
	evolvepkg "manifold/internal/evolve"
	gitpkg "manifold/internal/git"
	imggenpkg "manifold/internal/imggen"
	llmpkg "manifold/internal/llm"
	sefiipkg "manifold/internal/sefii"
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

	// AlphaEvolve endpoints
	registerEvolveEndpoints(api, config)

	// A2A protocol endpoints for worker nodes
	if config.A2A.Role == "worker" {
		log.Println("starting a2a server endpoints")
		registerA2AEndpoints(api, config)
	}

	// Git-related endpoints.
	api.GET("/git-files", gitpkg.FilesHandler)
	api.POST("/git-files/ingest", gitpkg.FilesIngestHandler(config))

	// Anthropic endpoints.
	registerAnthropicEndpoints(api, config)

	// Google Gemini endpoints.
	registerGeminiEndpoints(api, config)

	// File upload endpoints
	api.POST("/upload", func(c echo.Context) error {
		return fileUploadHandler(c, config)
	})
	api.POST("/upload-multiple", func(c echo.Context) error {
		return fileUploadMultipleHandler(c, config)
	})

	// Miscellaneous endpoints.
	api.POST("/run-fmlx", func(c echo.Context) error {
		return imggenpkg.RunFMLXHandler(c, config.DataPath)
	})
	api.POST("/run-sd", imggenpkg.RunSDHandler)
	api.POST("/repoconcat", repoconcatHandler)
	api.POST("/split-text", splitTextHandler)
	api.POST("/save-file", saveFileHandler)
	api.POST("/open-file", openFileHandler)
	api.GET("/web-content", webContentHandler)
	api.GET("/web-search", func(c echo.Context) error {
		return webSearchHandler(c, config)
	})
	api.POST("/code/eval", evaluateCodeHandler)
	api.POST("/comfy-proxy", imggenpkg.ComfyProxyHandler)

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
	sefiiGroup.POST("/ingest", sefiipkg.IngestHandler(config))
	sefiiGroup.POST("/search", sefiipkg.SearchHandler(config))
	sefiiGroup.POST("/combined-retrieve", sefiipkg.CombinedRetrieveHandler(config))
}

// registerAnthropicEndpoints registers routes for Anthropic-related functionality.
func registerAnthropicEndpoints(api *echo.Group, config *Config) {
	anthropicGroup := api.Group("/anthropic")
	anthropicGroup.POST("/messages", func(c echo.Context) error {
		return llmpkg.HandleMessages(c, config)
	})
}

// registerGeminiEndpoints registers routes for Google Gemini-related functionality.
func registerGeminiEndpoints(api *echo.Group, config *Config) {
	geminiGroup := api.Group("/gemini")
	geminiGroup.POST("/generate", func(c echo.Context) error {
		return llmpkg.HandleGemini(c, config)
	})
}

// registerAgenticMemoryEndpoints registers routes for Agentic Memory-related functionality.
func registerAgenticMemoryEndpoints(api *echo.Group, config *Config) {
	agenticGroup := api.Group("/agentic-memory")
	agenticGroup.POST("/ingest", agentspkg.AgenticMemoryIngestHandler(config))
	agenticGroup.POST("/search", agentspkg.AgenticMemorySearchHandler(config))

	agenticGroup.POST("/update/:id", agentspkg.AgenticMemoryUpdateHandler(config))
	// New graph-based memory endpoints
	memoryGroup := api.Group("/memory")
	memoryGroup.GET("/path/:sourceId/:targetId", agentspkg.FindMemoryPathHandler(config))
	memoryGroup.GET("/related/:memoryId", agentspkg.FindRelatedMemoriesHandler(config))
	memoryGroup.GET("/clusters/:workflowId", agentspkg.DiscoverMemoryClustersHandler(config))
	memoryGroup.GET("/health/:workflowId", agentspkg.AnalyzeNetworkHealthHandler(config))
	memoryGroup.GET("/contradictions/:workflowId", agentspkg.MemoryContradictionsHandler(config))
	memoryGroup.GET("/knowledge-map/:workflowId", agentspkg.BuildKnowledgeMapHandler(config))
}

// registerAgentEndpoints registers all routes for the ReAct / advanced agentic system.
func registerAgentEndpoints(api *echo.Group, config *Config) {
	agents := api.Group("/agents")
	agents.POST("/react", agentspkg.RunReActAgentHandler(config))              // Kick‑off a new ReAct session and run to completion
	agents.POST("/react/stream", agentspkg.RunReActAgentStreamHandler(config)) // Streaming endpoint for real-time thoughts
}

// registerWorkflowEndpoints registers routes for workflow templates.
func registerWorkflowEndpoints(api *echo.Group, config *Config) {
	workflowGroup := api.Group("/workflows")
	workflowGroup.GET("/templates", listWorkflowTemplatesHandler(config))
	workflowGroup.GET("/templates/:id", getWorkflowTemplateHandler(config))
}

// registerEvolveEndpoints registers routes for the AlphaEvolve system.
func registerEvolveEndpoints(api *echo.Group, config *Config) {
	evolveGroup := api.Group("/evolve")
	evolveGroup.POST("/run", evolvepkg.RunHandler(config))
	evolveGroup.GET("/status/:id", evolvepkg.StatusHandler)
	evolveGroup.GET("/results/:id", evolvepkg.ResultHandler)
}

// registerA2AEndpoints registers all A2A protocol-related routes.
func registerA2AEndpoints(api *echo.Group, config *Config) {
	// Create an A2A subgroup for all A2A protocol endpoints
	a2aGroup := api.Group("/a2a")

	// Create a TaskStore implementation (we can use the existing InMemoryStore for now)
	taskStore := a2a.NewTaskStore(config)

	// Create an Authenticator (we can use the NoopAuthenticator for now or integrate with Manifold's auth)
	authenticator := a2a.NewAuthenticator(config)

	// Main A2A endpoint that handles JSON-RPC requests
	a2aGroup.POST("", echo.WrapHandler(a2a.NewEchoHandler(taskStore, authenticator)))
	a2aGroup.GET("/hello", func(c echo.Context) error {
		return c.String(http.StatusOK, "hello world")
	})

	// Well-known Agent Card endpoint (required by A2A specification)
	a2aGroup.GET("/.well-known/agent.json", a2a.AgentCardHandler(config))
}

// WorkflowTemplate represents a workflow template with its metadata
type WorkflowTemplate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// listWorkflowTemplatesHandler returns a list of available workflow templates
func listWorkflowTemplatesHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Get templates directory from config
		templatesDir := filepath.Join(config.DataPath, "workflows")

		// Read the directory for template files
		files, err := os.ReadDir(templatesDir)
		if err != nil {
			log.Printf("Error reading workflow templates directory: %v", err)
			// Return empty array if directory doesn't exist instead of error
			if os.IsNotExist(err) {
				return c.JSON(http.StatusOK, []WorkflowTemplate{})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to read templates directory",
			})
		}

		// Build the list of templates
		templates := []WorkflowTemplate{}
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
				// Extract base filename without extension
				name := strings.TrimSuffix(file.Name(), ".json")

				// Format the name for display (remove numbers and underscores, capitalize words)
				displayName := formatTemplateName(name)

				templates = append(templates, WorkflowTemplate{
					ID:   file.Name(),
					Name: displayName,
				})
			}
		}

		return c.JSON(http.StatusOK, templates)
	}
}

// formatTemplateName formats the template filename for display by removing
// leading numbers and formatting underscores as spaces with capitalization
func formatTemplateName(filename string) string {
	// Remove any leading numbers and underscores (like "1_" in "1_chat_completion")
	nameWithoutPrefix := strings.Replace(filename, ".json", "", 1)
	nameWithoutPrefix = strings.TrimPrefix(nameWithoutPrefix, "1_")
	nameWithoutPrefix = strings.TrimPrefix(nameWithoutPrefix, "2_")
	nameWithoutPrefix = strings.TrimPrefix(nameWithoutPrefix, "3_")

	// Replace underscores with spaces
	nameWithSpaces := strings.ReplaceAll(nameWithoutPrefix, "_", " ")

	// Capitalize each word
	words := strings.Fields(nameWithSpaces)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[0:1]) + word[1:]
		}
	}

	return strings.Join(words, " ")
}

// getWorkflowTemplateHandler returns the content of a specific workflow template
func getWorkflowTemplateHandler(config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		templateID := c.Param("id")
		if templateID == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Template ID is required",
			})
		}

		// Validate the template ID to prevent directory traversal
		if strings.Contains(templateID, "..") || strings.Contains(templateID, "/") {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid template ID",
			})
		}

		// Get the full path to the template
		templatePath := filepath.Join(config.DataPath, "workflows", templateID)

		// Read the template file
		data, err := os.ReadFile(templatePath)
		if err != nil {
			log.Printf("Error reading workflow template: %v", err)
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Template not found",
			})
		}

		// Parse the JSON data to verify it's valid
		var templateData map[string]interface{}
		if err := json.Unmarshal(data, &templateData); err != nil {
			log.Printf("Error parsing workflow template JSON: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to parse template data",
			})
		}

		return c.JSON(http.StatusOK, templateData)
	}
}
