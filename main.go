// main.go
package main

import (
	"context"
	"embed"
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

const (
	service     = "api-gateway"
	environment = "development"
	id          = 1
	imagePath   = "/Users/art/Documents/code/manifold/frontend/public/mlx_out.png"
)

func main() {
	logger := pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)

	config, err := LoadConfig("config.yaml")
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to load configuration: %v", err))
	}

	// Initialize application (create data directory, etc.).
	if err := InitializeApplication(config); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to initialize application: %v", err))
	}

	// Create Echo instance with middleware.
	e := echo.New()
	// Use pterm for logging
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "${time_rfc3339} ${method} ${uri} ${status}\n",
		Output: pterm.DefaultLogger.Writer,
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{echo.GET, echo.PUT, echo.POST, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-File-Path"},
	}))

	// Register all routes.
	registerRoutes(e, config)

	// Start server.
	go func() {
		port := fmt.Sprintf(":%d", config.Port)
		if err := e.Start(port); err != nil && err != http.ErrServerClosed {
			logger.Fatal(fmt.Sprintf("Error starting server: %v", err))
		}
		logger.Info(fmt.Sprintf("Server started on port: %d", config.Port))
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Warn("Received shutdown signal")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Fatal(fmt.Sprintf("Error shutting down server: %v", err))
	}
	logger.Info("Server gracefully stopped")
}
