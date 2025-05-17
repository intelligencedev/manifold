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
	"runtime"
	"syscall"
	"time"

	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pterm/pterm"

	configpkg "manifold/internal/config"
)

//go:embed frontend/dist
var frontendDist embed.FS

func main() {
	logger := pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// Initialize configuration
	config, err := configpkg.LoadConfig(*configPath)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to load configuration: %v", err))
	}

	// Initialize application (create data directory, etc.).
	if err := InitializeApplication(config); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to initialize application: %v", err))
	}

	// Initialize the database connection pool with CPU-based sizing
	ctx := context.Background()
	poolConfig, err := pgxpool.ParseConfig(config.Database.ConnectionString)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to parse database connection string: %v", err))
	}

	// Set max connections to 2x CPU cores for optimal performance
	poolConfig.MaxConns = int32(runtime.NumCPU() * 2)
	logger.Info(fmt.Sprintf("Setting database pool max connections to %d (2 × CPU cores)", poolConfig.MaxConns))

	dbpool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create database connection pool: %v", err))
	}
	defer dbpool.Close()

	// Store the connection pool in the config for use throughout the application
	config.DBPool = dbpool
	logger.Info("Database connection pool initialized successfully")

	// Test the database connection
	if err := dbpool.Ping(ctx); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to connect to database: %v", err))
	}
	logger.Info("Successfully connected to database")

	// Initialize user database
	if err := initUserDB(config); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to initialize user database: %v", err))
	}
	defer userDB.Close()

	// Create default admin user if it doesn't exist
	createDefaultAdmin(config)

	// Create a new Echo instance
	e := echo.New()

	// Make config accessible via context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("config", config)
			return next(c)
		}
	})

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

	// Configure session middleware with cookie store
	store := sessions.NewCookieStore([]byte(config.Auth.SecretKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
	}
	e.Use(session.Middleware(store))

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

// createDefaultAdmin creates a default admin user if it doesn't exist
func createDefaultAdmin(config *Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if admin user exists
	_, err := userDB.GetUserByUsername(ctx, "admin")

	// If admin doesn't exist, create it with a default password
	if err != nil {
		pterm.Info.Println("Creating default admin user...")

		// Default admin password - in production, this should be changed immediately after first login
		defaultPassword := "M@nif0ld@dminStr0ngP@ssw0rd"

		user, err := userDB.CreateUser(ctx, "admin", defaultPassword, "", "Administrator")
		if err != nil {
			pterm.Error.Printf("Failed to create default admin user: %v\n", err)
			return
		}

		// Update user role to admin in database
		_, err = userDB.db.ExecContext(ctx, "UPDATE users SET role = 'admin' WHERE id = $1", user.ID)
		if err != nil {
			pterm.Error.Printf("Failed to set admin role: %v\n", err)
			return
		}

		pterm.Success.Println("Default admin user created successfully.")
		pterm.Warning.Println("Please change the default admin password after first login!")
	}
}
