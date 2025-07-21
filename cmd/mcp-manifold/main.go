package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RunMCPServer is the main entry point for running the MCP server with all registered tools.
func main() {
	log.Println("Starting Manifold MCP Server...")

	// Handle termination signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, initiating shutdown...", sig)
	}()

	if err := run(); err != nil {
		log.Fatalf("Fatal error: %v", err)
	}

	log.Println("MCP server stopped gracefully")
}

// run starts the MCP server and blocks until the context is canceled or an error occurs.
func run() error {
	// Create a new MCP server
	mcpServer := newMCPServer()

	// Start the server with stdio transport
	if err := server.ServeStdio(mcpServer); err != nil {
		return fmt.Errorf("MCP server error: %w", err)
	}

	return nil
}

// newMCPServer creates and configures a new MCP server with all tools
func newMCPServer() *server.MCPServer {
	// Create a new server
	mcpServer := server.NewMCPServer(
		"mcp-manifold",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)

	// Register all MCP tools
	registerAllTools(mcpServer)

	return mcpServer
}

// registerAllTools registers all the tools that our MCP server will provide.
func registerAllTools(mcpServer *server.MCPServer) {
	// Basic tools
	registerBasicTools(mcpServer)

	// Git tools
	registerGitTools(mcpServer)

	// Additional tools (incl. file_tool)
	registerAdditionalTools(mcpServer)

	log.Println("All MCP tools registered successfully")
}

// registerBasicTools registers the simple utility tools
func registerBasicTools(mcpServer *server.MCPServer) {
	// Calculate Tool
	mcpServer.AddTool(mcp.NewTool("calculate",
		mcp.WithDescription("Performs basic mathematical operations"),
		mcp.WithString("operation",
			mcp.Description("The mathematical operation to perform"),
			mcp.Enum("add", "subtract", "multiply", "divide"),
			mcp.Required(),
		),
		mcp.WithNumber("a",
			mcp.Description("First number"),
			mcp.Required(),
		),
		mcp.WithNumber("b",
			mcp.Description("Second number"),
			mcp.Required(),
		),
	), handleCalculateTool)

	// Time Tool
	mcpServer.AddTool(mcp.NewTool("time",
		mcp.WithDescription("Returns the current time"),
		mcp.WithString("format",
			mcp.Description("Optional time format (default: RFC3339)"),
		),
	), handleTimeTool)

	// Weather Tool
	mcpServer.AddTool(mcp.NewTool("get_weather",
		mcp.WithDescription("Get the weather forecast"),
		mcp.WithNumber("longitude",
			mcp.Description("Longitude in decimal degrees"),
			mcp.Required(),
		),
		mcp.WithNumber("latitude",
			mcp.Description("Latitude in decimal degrees"),
			mcp.Required(),
		),
	), handleWeatherTool)
}

// registerGitTools registers tools related to git operations
// These tools are missing in other git MCP servers tested
func registerGitTools(mcpServer *server.MCPServer) {
	// Git Pull Tool
	mcpServer.AddTool(mcp.NewTool("git_pull",
		mcp.WithDescription("Pulls changes"),
		mcp.WithString("path",
			mcp.Description("Local path to an existing Git repo"),
			mcp.Required(),
		),
	), handleGitPullTool)

	// Git Push Tool
	mcpServer.AddTool(mcp.NewTool("git_push",
		mcp.WithDescription("Pushes commits"),
		mcp.WithString("path",
			mcp.Description("Local path to an existing Git repo"),
			mcp.Required(),
		),
	), handleGitPushTool)

	// Git Clone Tool
	mcpServer.AddTool(mcp.NewTool("git_clone",
		mcp.WithDescription("Clones a remote Git repository"),
		mcp.WithString("repoUrl",
			mcp.Description("URL of the Git repository to clone"),
			mcp.Required(),
		),
		mcp.WithString("path",
			mcp.Description("Local path where to clone the repository"),
			mcp.Required(),
		),
	), handleGitCloneTool)
}

// registerAdditionalTools registers various other tools, including the new file_tool
func registerAdditionalTools(mcpServer *server.MCPServer) {
	// Run Shell Command Tool
	// mcpServer.AddTool(mcp.NewTool("run_shell_command",
	// 	mcp.WithDescription("Executes an arbitrary shell command"),
	// 	mcp.WithArray("command",
	// 		mcp.Description("Command to execute and its arguments"),
	// 		mcp.Required(),
	// 		mcp.Items(map[string]interface{}{"type": "string"}),
	// 	),
	// 	mcp.WithString("dir",
	// 		mcp.Description("Directory in which to run the command"),
	// 		mcp.Required(),
	// 	),
	// ), handleShellCommandTool)

	// CLI Tool
	mcpServer.AddTool(mcp.NewTool("cli",
		mcp.WithDescription("Execute a raw CLI command"),
		mcp.WithString("command",
			mcp.Description("Command string to execute"),
			mcp.Required(),
		),
		mcp.WithString("dir",
			mcp.Description("Optional working directory"),
		),
	), handleCLITool)

	// Go Build Tool
	mcpServer.AddTool(mcp.NewTool("go_build",
		mcp.WithDescription("Builds a Go module"),
		mcp.WithString("path",
			mcp.Description("Directory of Go module"),
			mcp.Required(),
		),
	), handleGoBuildTool)

	// Go Test Tool
	mcpServer.AddTool(mcp.NewTool("go_test",
		mcp.WithDescription("Runs Go tests"),
		mcp.WithString("path",
			mcp.Description("Directory of Go tests"),
			mcp.Required(),
		),
	), handleGoTestTool)

	// Format Go Code Tool
	mcpServer.AddTool(mcp.NewTool("format_go_code",
		mcp.WithDescription("Formats Go code using go fmt"),
		mcp.WithString("path",
			mcp.Description("Directory of Go code to format"),
			mcp.Required(),
		),
	), handleFormatGoCodeTool)

	// Lint Code Tool
	mcpServer.AddTool(mcp.NewTool("lint_code",
		mcp.WithDescription("Runs a code linter"),
		mcp.WithString("path",
			mcp.Description("Dir or file to lint"),
			mcp.Required(),
		),
		mcp.WithString("linterName",
			mcp.Description("Optional linter name"),
		),
	), handleLintCodeTool)

	// Enhanced File Editor Tool
	mcpServer.AddTool(mcp.NewTool("edit_file",
		mcp.WithDescription("High-precision, atomic edits to text files"),
		mcp.WithString("operation",
			mcp.Description("Type of operation to perform"),
			mcp.Enum("read", "read_range", "search", "replace_line",
				"replace_range", "insert_after", "delete_range",
				"apply_patch", "preview_patch"),
			mcp.Required(),
		),
		mcp.WithString("path",
			mcp.Description("Path to the file to edit (relative to workspace)"),
			mcp.Required(),
		),
		mcp.WithNumber("start",
			mcp.Description("1-based line number for range operations"),
		),
		mcp.WithNumber("end",
			mcp.Description("1-based end line number (inclusive) for range operations"),
		),
		mcp.WithString("pattern",
			mcp.Description("Regex or literal pattern for search operation"),
		),
		mcp.WithString("replacement",
			mcp.Description("Replacement or insertion text; may contain \\n"),
		),
		mcp.WithString("patch",
			mcp.Description("Unified-diff content for apply/preview_patch operations"),
		),
	), handleEditFileTool)
}
