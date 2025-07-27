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

	configpkg "manifold/internal/config"
	servicespkg "manifold/internal/services"
)

//go:embed frontend/dist
var frontendDist embed.FS

func main() {
	logger := log.WithField("component", "server")
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// Initialize configuration
	config, err := configpkg.LoadConfig(*configPath)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to load configuration: %v", err))
	}

	// Initialize the database connection pool with CPU-based sizing first
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

	// Initialize application (create data directory, etc.) after DB pool is ready
	if err := InitializeApplication(config); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to initialize application: %v", err))
	}

	// Test the database connection
	if err := dbpool.Ping(ctx); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to connect to database: %v", err))
	}
	logger.Info("Successfully connected to database")

	// Ensure web tables exist
	conn, err := dbpool.Acquire(ctx)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to acquire db connection: %v", err))
	}
	if err := CreateWebTables(ctx, conn.Conn()); err != nil {
		conn.Release()
		logger.Fatal(fmt.Sprintf("Failed to create web tables: %v", err))
	}
	conn.Release()

	// Initialize user database
	if err := initUserDB(config); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to initialize user database: %v", err))
	}
	defer userDB.Close()

	// Create default admin user if it doesn't exist
	logger.Info("Starting admin user creation...")
	createDefaultAdmin(config)
	logger.Info("Admin user creation completed")

	// Create a new Echo instance
	logger.Info("Creating Echo web server instance...")
	e := echo.New()

	// Remove banner and version info from Echo logs
	e.HideBanner = true
	e.HidePort = true

	logger.Info("Configuring Echo middleware...")
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
		Output: log.Writer(),
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
	logger.Info("Registering routes...")
	registerRoutes(e, config)

	// Start server in a goroutine
	go func() {
		port := fmt.Sprintf(":%d", config.Port)
		logger.Info(fmt.Sprintf("Manifold server listening on port: %d", config.Port))
		if err := e.Start(port); err != nil && err != http.ErrServerClosed {
			logger.Fatal(fmt.Sprintf("Error starting server: %v", err))
		}
	}()

	// Set up graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Warn("Received shutdown signal")

	// Perform cleanup

	// First, stop all local services if they were started
	if config.SingleNodeInstance {
		logger.Info("Shutting down local services...")
		servicespkg.StopAllServices()
	}

	// Stop PGVector container with explicit confirmation
	logger.Info("Shutting down PGVector container...")
	if err := servicespkg.StopPGVectorContainer(); err != nil {
		logger.Error(fmt.Sprintf("Error stopping PGVector container: %v", err))
	} else {
		logger.Info("PGVector container stopped successfully")
	}

	// Shutdown Echo server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		logger.Fatal(fmt.Sprintf("Error shutting down server: %v", err))
	}

	logger.Info("Server gracefully stopped")
}

// createDefaultAdmin creates a default admin user if it doesn't exist
func createDefaultAdmin(config *Config) {
	logger := log.WithField("component", "admin")
	logger.Info("Starting createDefaultAdmin function")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if admin user exists
	logger.Info("Checking if admin user exists")
	_, err := userDB.GetUserByUsername(ctx, "admin")

	// If admin doesn't exist, create it without a password (password_hash will be empty)
	if err != nil {
		logger.Warn("Admin user not found, creating admin user without password")

		// Create admin user with empty password hash - user must set password on first access
		user, err := userDB.CreateUserWithoutPassword(ctx, "admin", "", "Administrator")
		if err != nil {
			logger.WithError(err).Error("Failed to create default admin user")
			return
		}

		// Update user role to admin in database
		logger.Info("Updating user role to admin")
		_, err = userDB.db.ExecContext(ctx, "UPDATE users SET role = 'admin' WHERE id = $1", user.ID)
		if err != nil {
			logger.WithError(err).Error("Failed to set admin role")
			return
		}

		// Set force password change flag
		logger.Info("Setting force password change flag")
		err = userDB.SetForcePasswordChange(ctx, user.ID, true)
		if err != nil {
			logger.WithError(err).Error("Failed to set force password change flag")
			return
		}

		logger.Info("Default admin user created successfully")
		logger.Warn("Admin user must set their password on first access to the application")
	} else {
		logger.Info("Admin user already exists")
	}
}
