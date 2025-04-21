package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

// RunMCPServer is the main entry point for running the SecurityTrails API MCP server.
func main() {
	log.Println("Starting SecurityTrails API MCP Server...")

	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransport()

	// Create a new server with the transport
	server := mcp.NewServer(serverTransport)

	// Register all SecurityTrails MCP tools
	registerSecurityTrailsTools(server)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

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
		log.Fatalf("Server error: %v", err)
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
	}

	log.Println("SecurityTrails MCP server stopped")
}

// registerSecurityTrailsTools registers all the tools that our SecurityTrails MCP server will provide
func registerSecurityTrailsTools(server *mcp.Server) {
	// Register API ping tool
	if err := server.RegisterTool("ping", "Check API status for SecurityTrails", func(args PingArgs) (*mcp.ToolResponse, error) {
		res, err := pingTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering ping tool: %v", err)
	}

	log.Println("All SecurityTrails MCP tools registered successfully")
}
