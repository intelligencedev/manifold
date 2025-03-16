package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

func configHandler(c echo.Context) error {
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load config"})
	}
	return c.JSON(http.StatusOK, config)
}

func getFileSystem() http.FileSystem {
	fsys, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to get file system: %v", err)
	}
	return http.FS(fsys)
}

// executeMCPHandler handles the MCP execution request using an MCP server.
func executeMCPHandler(c echo.Context) error {
	// Parse the JSON payload from the request.
	var payload map[string]interface{}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
	}

	log.Printf("Received MCP payload: %+v", payload)

	// Determine the action specified in the payload.
	action, ok := payload["action"].(string)
	if !ok || action == "" {
		action = "listTools"
	}

	// Load config for the MCP server
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load config"})
	}

	// Create paired pipes for client/server communication
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	// Create a transport for the client
	clientTransport := stdio.NewStdioServerTransportWithIO(clientReader, clientWriter)
	client := mcp.NewClient(clientTransport)

	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransportWithIO(serverReader, serverWriter)
	server := mcp.NewServer(serverTransport)

	// Register all tools on the server
	registerMCPTools(server, config)

	// Start the server in a goroutine
	go func() {
		if err := server.Serve(); err != nil {
			log.Printf("MCP server error: %v", err)
		}
	}()

	// Initialize the client
	if _, err := client.Initialize(context.Background()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to initialize MCP client: %v", err)})
	}

	// Prepare a variable to hold the result
	var result interface{}

	// Choose action based on the payload
	switch action {
	case "listTools":
		tools, err := client.ListTools(context.Background(), nil)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to list tools: %v", err)})
		}
		result = tools
	case "execute":
		// Expect payload to include "tool" and "args"
		toolName, ok := payload["tool"].(string)
		if !ok || toolName == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing tool name for execution"})
		}
		args, ok := payload["args"].(map[string]interface{})
		if !ok {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing or invalid arguments for tool execution"})
		}
		toolResp, err := client.CallTool(context.Background(), toolName, args)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to call tool: %v", err)})
		}
		result = toolResp
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Action '%s' not implemented", action)})
	}

	// Return the result as a JSON response
	return c.JSON(http.StatusOK, result)
}

// registerMCPTools extracts the tool registration part from RunMCP function
func registerMCPTools(server *mcp.Server, config *Config) {
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
		{"agent", "Agent that uses LLM to decide which tools to call", agentHandler(config)},
	}

	for _, tool := range tools {
		if err := server.RegisterTool(tool.name, tool.description, tool.handler); err != nil {
			log.Printf("Error registering %s tool: %v", tool.name, err)
		}
	}
}
