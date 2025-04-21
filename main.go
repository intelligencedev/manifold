// main.go
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pterm/pterm"
)

//go:embed frontend/dist
var frontendDist embed.FS

func main() {
	logger := pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// Initialize configuration
	config, err := LoadConfig(*configPath)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to load configuration: %v", err))
	}

	// Initialize application (create data directory, etc.).
	if err := InitializeApplication(config); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to initialize application: %v", err))
	}

	// Create a new Echo instance
	e := echo.New()

	// Configure middleware
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "${time_rfc3339} ${method} ${uri} ${status}\n",
		Output: pterm.DefaultLogger.Writer,
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{echo.GET, echo.PUT, echo.POST, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-File-Path"},
	}))

	// Register routes
	registerRoutes(e, config)

	// Create InternalMCPHandler for proper cleanup
	internalMCPHandler, err := NewInternalMCPHandler(config)
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to initialize MCP handler: %v", err))
		// Continue anyway, as MCP functionality is optional
	}

	// Start server in a goroutine
	go func() {
		port := fmt.Sprintf(":%d", config.Port)
		if err := e.Start(port); err != nil && err != http.ErrServerClosed {
			logger.Fatal(fmt.Sprintf("Error starting server: %v", err))
		}
		logger.Info(fmt.Sprintf("Server started on port: %d", config.Port))
	}()

	// Set up graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Warn("Received shutdown signal")

	// Perform cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Close MCP handler if it was successfully created
	if internalMCPHandler != nil {
		logger.Info("Shutting down MCP handler...")
		internalMCPHandler.Close()
	}

	// First, stop all local services
	logger.Info("Shutting down local services...")
	StopAllServices()

	// Stop PGVector container with explicit confirmation
	logger.Info("Shutting down PGVector container...")
	if err := StopPGVectorContainer(); err != nil {
		logger.Error(fmt.Sprintf("Error stopping PGVector container: %v", err))
	} else {
		logger.Info("PGVector container stopped successfully")
	}

	// Shutdown Echo server
	if err := e.Shutdown(ctx); err != nil {
		logger.Fatal(fmt.Sprintf("Error shutting down server: %v", err))
	}

	logger.Info("Server gracefully stopped")
}
