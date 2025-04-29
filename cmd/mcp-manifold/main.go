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

// RunMCPServer is the main entry point for running the MCP server with all registered tools.
func main() {
	log.Println("Starting Manifold MCP Server...")

	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransport()

	// Create a new server with the transport
	server := mcp.NewServer(serverTransport)

	// Register all MCP tools
	registerAllTools(server)

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

	log.Println("MCP server stopped")
}

// registerAllTools registers all the tools that our MCP server will provide.
func registerAllTools(server *mcp.Server) {
	// Basic tools
	registerBasicTools(server)

	// File system tools
	registerFileSystemTools(server)

	// Git tools
	registerGitTools(server)

	// Additional tools
	registerAdditionalTools(server)

	log.Println("All MCP tools registered successfully")
}

// registerBasicTools registers the simple utility tools
func registerBasicTools(server *mcp.Server) {
	tools := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{"hello", "Says hello to the provided name", func(args HelloArgs) (*mcp.ToolResponse, error) {
			res, err := helloTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"calculate", "Performs basic mathematical operations", func(args CalculateArgs) (*mcp.ToolResponse, error) {
			res, err := calculateTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"time", "Returns the current time", func(args TimeArgs) (*mcp.ToolResponse, error) {
			res, err := timeTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"get_weather", "Get the weather forecast", func(args WeatherArgs) (*mcp.ToolResponse, error) {
			res, err := getWeatherTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"web_search", "Performs a web search using selected backend", func(args WebSearchArgs) (*mcp.ToolResponse, error) {
			res, err := webSearchTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"web_content", "Fetches and extracts content from web URLs", func(args WebContentArgs) (*mcp.ToolResponse, error) {
			res, err := webContentTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
	}

	for _, tool := range tools {
		if err := server.RegisterTool(tool.name, tool.description, tool.handler); err != nil {
			log.Printf("Error registering %s tool: %v", tool.name, err)
		}
	}
}

// registerFileSystemTools registers tools related to file system operations
func registerFileSystemTools(server *mcp.Server) {
	tools := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{"read_file", "Reads the entire contents of a text file", func(args ReadFileArgs) (*mcp.ToolResponse, error) {
			res, err := readFileTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"write_file", "Writes text content to a file", func(args WriteFileArgs) (*mcp.ToolResponse, error) {
			res, err := writeFileTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"list_directory", "Lists files and directories", func(args ListDirectoryArgs) (*mcp.ToolResponse, error) {
			res, err := listDirectoryTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"create_directory", "Creates a directory", func(args CreateDirectoryArgs) (*mcp.ToolResponse, error) {
			res, err := createDirectoryTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"move_file", "Moves or renames a file/directory", func(args MoveFileArgs) (*mcp.ToolResponse, error) {
			res, err := moveFileTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"read_multiple_files", "Reads the contents of multiple files", func(args ReadMultipleFilesArgs) (*mcp.ToolResponse, error) {
			res, err := readMultipleFilesTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"edit_file", "Edits a file via search and replace", func(args EditFileArgs) (*mcp.ToolResponse, error) {
			res, err := editFileTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"directory_tree", "Recursively lists the directory structure", func(args DirectoryTreeArgs) (*mcp.ToolResponse, error) {
			res, err := directoryTreeTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"search_files", "Searches for a text pattern in files", func(args SearchFilesArgs) (*mcp.ToolResponse, error) {
			res, err := searchFilesTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"get_file_info", "Returns metadata for a file or directory", func(args GetFileInfoArgs) (*mcp.ToolResponse, error) {
			res, err := getFileInfoTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"list_allowed_directories", "Lists directories allowed for access", func(args ListAllowedDirectoriesArgs) (*mcp.ToolResponse, error) {
			res, err := listAllowedDirectoriesTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"delete_file", "Deletes a file or directory", func(args DeleteFileArgs) (*mcp.ToolResponse, error) {
			res, err := deleteFileTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"copy_file", "Copies a file or directory", func(args CopyFileArgs) (*mcp.ToolResponse, error) {
			res, err := copyFileTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
	}

	for _, tool := range tools {
		if err := server.RegisterTool(tool.name, tool.description, tool.handler); err != nil {
			log.Printf("Error registering %s tool: %v", tool.name, err)
		}
	}
}

// registerGitTools registers tools related to git operations
func registerGitTools(server *mcp.Server) {
	tools := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{"git_init", "Initializes a new Git repository", func(args GitInitArgs) (*mcp.ToolResponse, error) {
			res, err := gitInitTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"git_status", "Shows Git status", func(args GitRepoArgs) (*mcp.ToolResponse, error) {
			res, err := gitStatusTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"git_add", "Stages file changes", func(args GitAddArgs) (*mcp.ToolResponse, error) {
			res, err := gitAddTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"git_commit", "Commits staged changes", func(args GitCommitArgs) (*mcp.ToolResponse, error) {
			res, err := gitCommitTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"git_pull", "Pulls changes", func(args GitRepoArgs) (*mcp.ToolResponse, error) {
			res, err := gitPullTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"git_push", "Pushes commits", func(args GitRepoArgs) (*mcp.ToolResponse, error) {
			res, err := gitPushTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"git_clone", "Clones a remote Git repository", func(args GitCloneArgs) (*mcp.ToolResponse, error) {
			res, err := gitCloneTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"git_checkout", "Switches or creates a new Git branch", func(args GitCheckoutArgs) (*mcp.ToolResponse, error) {
			res, err := gitCheckoutTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"git_diff", "Shows Git diff between references", func(args GitDiffArgs) (*mcp.ToolResponse, error) {
			res, err := gitDiffTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
	}

	for _, tool := range tools {
		if err := server.RegisterTool(tool.name, tool.description, tool.handler); err != nil {
			log.Printf("Error registering %s tool: %v", tool.name, err)
		}
	}
}

// registerAdditionalTools registers various other tools
func registerAdditionalTools(server *mcp.Server) {
	tools := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{"run_shell_command", "Executes an arbitrary shell command", func(args ShellCommandArgs) (*mcp.ToolResponse, error) {
			res, err := runShellCommandTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"go_build", "Builds a Go module", func(args GoBuildArgs) (*mcp.ToolResponse, error) {
			res, err := goBuildTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"go_test", "Runs Go tests", func(args GoTestArgs) (*mcp.ToolResponse, error) {
			res, err := goTestTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"format_go_code", "Formats Go code using go fmt", func(args FormatGoCodeArgs) (*mcp.ToolResponse, error) {
			res, err := formatGoCodeTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
		{"lint_code", "Runs a code linter", func(args LintCodeArgs) (*mcp.ToolResponse, error) {
			res, err := lintCodeTool(args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		}},
	}

	for _, tool := range tools {
		if err := server.RegisterTool(tool.name, tool.description, tool.handler); err != nil {
			log.Printf("Error registering %s tool: %v", tool.name, err)
		}
	}
}
