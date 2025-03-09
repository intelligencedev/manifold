// main.go
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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
	config, err := LoadConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Configuration loaded: %+v", config)

	// Initialize application (create data directory, etc.).
	if err := InitializeApplication(config); err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Create Echo instance with middleware.
	e := echo.New()
	e.Use(middleware.Logger())
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
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Error shutting down server: %v", err)
	}
	log.Println("Server gracefully stopped")
}
