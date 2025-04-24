package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

func main() {
	log.Println("Starting SecurityTrails API MCP Server...")

	// Create a context that will be canceled when receiving termination signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, initiating shutdown...", sig)
		cancel()
	}()

	if err := run(ctx); err != nil {
		log.Fatalf("Fatal error: %v", err)
	}

	log.Println("SecurityTrails MCP server stopped gracefully")
}

// run is the main entry point for running the SecurityTrails API MCP server.
func run(ctx context.Context) error {
	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransport()

	// Create a new server with the transport
	server := mcp.NewServer(serverTransport)

	// Get the SecurityTrails client for dependency injection
	client, err := getSecurityTrailsClient()
	if err != nil {
		return fmt.Errorf("failed to initialize SecurityTrails client: %w", err)
	}

	// Create tool dependencies
	deps := ToolDependencies{
		Client: client,
	}

	// Register all SecurityTrails MCP tools
	if err := registerSecurityTrailsTools(server, deps); err != nil {
		return fmt.Errorf("failed to register tools: %w", err)
	}

	// Start the server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := server.Serve(); err != nil {
			errChan <- fmt.Errorf("MCP server error: %w", err)
		}
	}()

	// Wait for termination signal or error
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		// Context was canceled by signal handler
		return nil
	}
}

// registerSecurityTrailsTools wires every logical tool-group into the MCP server.
// It delegates to smaller “registerXTools” helpers so each group can live in its
// own file, keeping compile units small and testable.
func registerSecurityTrailsTools(server *mcp.Server, deps ToolDependencies) error {
	regs := []struct {
		name string
		fn   func(*mcp.Server, ToolDependencies) error
	}{
		{"basic tools", registerBasicTools},
		{"project tools", registerProjectTools},
		{"asset tools", registerAssetTools},
		{"tag tools", registerTagTools},
		{"exposure tools", registerExposureTools},
	}

	for _, r := range regs {
		if err := r.fn(server, deps); err != nil {
			return fmt.Errorf("failed to register %s: %w", r.name, err)
		}
	}

	log.Println("All SecurityTrails MCP tools registered successfully")
	return nil
}
