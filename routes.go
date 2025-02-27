// routes.go
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func registerRoutes(e *echo.Echo, config *Config) {
	// Serve static frontend files.
	e.GET("/*", echo.WrapHandler(http.FileServer(getFileSystem())))

	api := e.Group("/api")
	api.GET("/config", configHandler)

	// SEFII endpoints.
	sefiiGroup := api.Group("/sefii")
	sefiiGroup.POST("/ingest", sefiiIngestHandler(config))
	sefiiGroup.POST("/search", sefiiSearchHandler(config))
	sefiiGroup.POST("/combined-retrieve", sefiiCombinedRetrieveHandler(config))

	// Git-related endpoints.
	api.GET("/git-files", gitFilesHandler)
	api.POST("/git-files/ingest", gitFilesIngestHandler(config))

	// Anthropic endpoints.
	anthropicGroup := api.Group("/anthropic")
	anthropicGroup.POST("/messages", func(c echo.Context) error {
		return handleAnthropicMessages(c, config)
	})

	api.POST("/run-fmlx", runFMLXHandler)
	e.GET("/mlx_out.png", imageHandler)
	api.POST("/run-sd", runSDHandler)

	api.POST("/repoconcat", repoconcatHandler)
	api.POST("/split-text", splitTextHandler)
	api.POST("/save-file", saveFileHandler)
	api.POST("/open-file", openFileHandler)

	api.GET("/web-content", webContentHandler)
	api.GET("/web-search", webSearchHandler)
	api.POST("/executePython", executePythonHandler)
	api.POST("/datadog", datadogHandler)
	api.POST("/download-llama", downloadLlamaHandler)
	api.POST("/comfy-proxy", comfyProxyHandler)
}
