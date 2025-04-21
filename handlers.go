// handlers.go
//
// Unified MCP handler that merges our **internal tools** with the public
// GitHub MCP‑Server tools so they can all be invoked from a single endpoint.
//
//   • Internal tools are registered exactly as before.
//   • All external tools are discovered at runtime, then re‑registered in the
//     same server under a “gh_” prefix (e.g. “gh_list_directory”).
//   • The single /mcp endpoint now understands the full, merged tool‑set.
//
// NOTE: any router setup (main.go, etc.) should point POST /mcp to
//       executeMCPCombinedHandler.
//
// ---------------------------------------------------------------------------

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/labstack/echo/v4"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

// ---------------------------------------------------------------------------
// Static assets &  basic config endpoint
// ---------------------------------------------------------------------------

// configHandler returns the parsed config.yaml.
func configHandler(c echo.Context) error {
	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("config load: %v", err)})
	}
	return c.JSON(http.StatusOK, cfg)
}

// getFileSystem serves the SPA bundle embedded via //go:embed in frontendDist.
func getFileSystem() http.FileSystem {
	sub, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("embed FS error: %v", err)
	}
	return http.FS(sub)
}

// ---------------------------------------------------------------------------
// External (GitHub) MCP server helpers
// ---------------------------------------------------------------------------

const githubMCPImage = "ghcr.io/github/github-mcp-server:latest"

// startExternalMCP launches the GitHub MCP server in Docker and returns
// a *mcp.Client wired to its stdio plus a cleanup func.
func startExternalMCP(ctx context.Context) (*mcp.Client, func() error, error) {
	githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	if githubPAT == "" {
		return nil, nil, fmt.Errorf("env GITHUB_PERSONAL_ACCESS_TOKEN not set")
	}

	args := []string{
		"run", "-i", "--rm",
		"-e", "GITHUB_PERSONAL_ACCESS_TOKEN=" + githubPAT,
		githubMCPImage,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("docker start: %w", err)
	}

	cleanup := func() error { return cmd.Process.Kill() }

	tr := stdio.NewStdioServerTransportWithIO(stdout, stdin)
	client := mcp.NewClient(tr)

	if _, err := client.Initialize(ctx); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("ext‑client init: %w", err)
	}

	tools, err := client.ListTools(ctx, nil) // trigger tool discovery
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("ext‑client list tools: %w", err)
	}
	if len(tools.Tools) == 0 {
		cleanup()
		return nil, nil, fmt.Errorf("ext‑client no tools found")
	}
	log.Printf("External MCP tools: %v", tools.Tools)

	return client, cleanup, nil
}

// proxyArgs wraps an untyped map so MCP can auto‐generate a JSON schema.
type proxyArgs struct {
	// The external tool's arguments.
	Args map[string]interface{} `json:"args"`
}

// mergeExternalTools registers every tool from the GitHub MCP server
// under a “gh_<toolName>” prefix.  The handler simply forwards the
// incoming map[string]interface{} to ext.CallTool.
func mergeExternalTools(ctx context.Context, server *mcp.Server, ext *mcp.Client) error {
	resp, err := ext.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list external tools: %w", err)
	}

	type ProxyArgs struct {
		Args map[string]interface{} `json:"args"`
	}

	for _, t := range resp.Tools {
		name := t.Name
		shadow := "gh_" + name
		desc := fmt.Sprintf("proxy to GitHub MCP tool %q", name)
		inputSchema := t.InputSchema

		// Print the argument names and types
		log.Printf("Tool %s: %v", name, inputSchema)

		inputSchemaMap, ok := inputSchema.(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid input schema format for tool %s", name)
		}
		properties, ok := inputSchemaMap["properties"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid properties in input schema for tool %s", name)
		}
		for propName, propValue := range properties {
			propMap, ok := propValue.(map[string]interface{})
			if !ok {
				return fmt.Errorf("invalid property format for tool %s", name)
			}
			propDesc, _ := propMap["description"].(string)
			propType, _ := propMap["type"].(string)
			log.Printf("Tool %s property: %s (%s) - %s", name, propName, propType, propDesc)
		}

		// Register the tool with a handler that takes ProxyArgs
		toolHandler := func(args ProxyArgs) (*mcp.ToolResponse, error) {
			res, err := ext.CallTool(ctx, name, args.Args)
			if err != nil {
				return nil, fmt.Errorf("call external tool %s: %w", name, err)
			}
			return res, nil
		}

		if err := server.RegisterTool(
			shadow,
			desc,
			toolHandler,
		); err != nil {
			return fmt.Errorf("register external tool %s: %w", name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Unified MCP HTTP handler
// ---------------------------------------------------------------------------

// executeMCPCombinedHandler sets up:
//
//  1. our in‑process MCP server (+ tools)
//  2. the external GitHub MCP client
//  3. proxies external tools into our server
//  4. executes the requested MCP action
//
// All of this occurs for *one* HTTP request.  The external Docker
// process is torn down afterwards; you may cache it if desired.
func executeMCPCombinedHandler(c echo.Context) error {
	ctx := c.Request().Context()

	// ---- payload & config --------------------------------------------------
	payload, err := parsePayload(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest,
			map[string]string{"error": "invalid JSON payload"})
	}
	action := getAction(payload)

	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("config load: %v", err)})
	}

	// ---- wire internal client/server --------------------------------------
	client, server, err := setupMCPCommunication()
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("setup comms: %v", err)})
	}
	registerMCPTools(server, cfg)

	// ---- launch & merge external tools ------------------------------------
	extClient, cleanup, err := startExternalMCP(ctx)
	if err != nil {
		log.Printf("⚠️  external MCP unavailable: %v (continuing with internal tools only)", err)
	} else {
		defer cleanup()
		if err := mergeExternalTools(ctx, server, extClient); err != nil {
			log.Printf("merge external tools: %v", err)
		}
	}

	extTools, err := extClient.ListTools(ctx, nil) // trigger tool discovery
	if err != nil {
		log.Printf("ext client list tools: %v", err)
	} else {
		// Convert tools to JSON for complete data representation
		toolsJSON, jsonErr := json.MarshalIndent(extTools.Tools, "", "  ")
		if jsonErr != nil {
			log.Printf("Failed to convert external tools to JSON: %v", jsonErr)

			// log the toolsJSON as a string
			toolsJSONStr := fmt.Sprintf("%v", extTools.Tools)
			log.Printf("External MCP tools (JSON):\n%s", toolsJSONStr)
		} else {
			log.Printf("External MCP tools (JSON):\n%s", string(toolsJSON))
		}
	}

	// ---- start server + initialize client ---------------------------------
	go startMCPServer(server)

	if _, err := client.Initialize(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("client init: %v", err)})
	}

	// ---- execute action ----------------------------------------------------
	result, err := handleAction(client, action, payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, result)
}

// ------------------------------------------------------------------
// MCP-specific handler functions
// ------------------------------------------------------------------

// executeMCPInternalHandler runs only the internal MCP tools.
func executeMCPInternalHandler(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse the incoming payload
	payload, err := parsePayload(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest,
			map[string]string{"error": "invalid JSON payload"})
	}
	action := getAction(payload)

	// Load configuration
	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("config load: %v", err)})
	}

	// Set up client/server communication
	client, server, err := setupMCPCommunication()
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("setup comms: %v", err)})
	}

	// Register only internal tools
	registerMCPTools(server, cfg)

	// Start server and initialize client
	go startMCPServer(server)

	if _, err := client.Initialize(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("client init: %v", err)})
	}

	// Execute the requested action
	result, err := handleAction(client, action, payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, result)
}

// executeMCPGitHubHandler runs only the GitHub external MCP tools.
func executeMCPGitHubHandler(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse the incoming payload
	payload, err := parsePayload(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest,
			map[string]string{"error": "invalid JSON payload"})
	}

	// For GitHub MCP, we only support tool execution, not listing
	if toolName, ok := payload["tool"].(string); !ok || toolName == "" {
		return c.JSON(http.StatusBadRequest,
			map[string]string{"error": "missing 'tool' field in payload"})
	}

	// Get arguments from payload
	argsRaw, ok := payload["args"]
	if !ok {
		return c.JSON(http.StatusBadRequest,
			map[string]string{"error": "missing 'args' field in payload"})
	}

	// Start the external GitHub MCP server
	extClient, cleanup, err := startExternalMCP(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("failed to start GitHub MCP: %v", err)})
	}
	defer cleanup()

	// Execute the tool directly
	toolName := payload["tool"].(string)
	result, err := extClient.CallTool(ctx, toolName, argsRaw)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("GitHub MCP tool execution failed: %v", err)})
	}

	return c.JSON(http.StatusOK, result)
}

// listMCPToolsHandler returns a list of all available MCP tools (both internal and GitHub).
func listMCPToolsHandler(c echo.Context) error {
	ctx := c.Request().Context()
	var tools struct {
		Internal []string `json:"internal"`
		GitHub   []string `json:"github"`
	}

	// Set up internal MCP server to get internal tools
	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("config load: %v", err)})
	}

	client, server, err := setupMCPCommunication()
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("setup comms: %v", err)})
	}
	registerMCPTools(server, cfg)
	go startMCPServer(server)

	if _, err := client.Initialize(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("client init: %v", err)})
	}

	// Get internal tools
	internalResp, err := client.ListTools(ctx, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("listing internal tools: %v", err)})
	}

	for _, tool := range internalResp.Tools {
		tools.Internal = append(tools.Internal, tool.Name)
	}

	// Get GitHub MCP tools if possible
	extClient, cleanup, err := startExternalMCP(ctx)
	if err != nil {
		// Return just the internal tools if GitHub MCP is unavailable
		log.Printf("GitHub MCP unavailable: %v", err)
	} else {
		defer cleanup()
		extResp, err := extClient.ListTools(ctx, nil)
		if err != nil {
			log.Printf("Failed to list GitHub tools: %v", err)
		} else {
			for _, tool := range extResp.Tools {
				tools.GitHub = append(tools.GitHub, tool.Name)
			}
		}
	}

	return c.JSON(http.StatusOK, tools)
}

// listInternalMCPToolsHandler returns only the internal MCP tools.
func listInternalMCPToolsHandler(c echo.Context) error {
	ctx := c.Request().Context()

	// Load configuration
	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("config load: %v", err)})
	}

	// Set up client/server
	client, server, err := setupMCPCommunication()
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("setup comms: %v", err)})
	}

	// Register only internal tools
	registerMCPTools(server, cfg)

	// Start server and initialize client
	go startMCPServer(server)
	if _, err := client.Initialize(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("client init: %v", err)})
	}

	// Get tools list
	resp, err := client.ListTools(ctx, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("listing internal tools: %v", err)})
	}

	return c.JSON(http.StatusOK, resp)
}

// listGitHubMCPToolsHandler returns only the GitHub external MCP tools.
func listGitHubMCPToolsHandler(c echo.Context) error {
	ctx := c.Request().Context()

	// Start the external GitHub MCP server
	extClient, cleanup, err := startExternalMCP(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("failed to start GitHub MCP: %v", err)})
	}
	defer cleanup()

	// Get tools list
	resp, err := extClient.ListTools(ctx, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			map[string]string{"error": fmt.Sprintf("listing GitHub tools: %v", err)})
	}

	return c.JSON(http.StatusOK, resp)
}

// executeMCPToolHandler executes a specific MCP tool by name, whether internal or GitHub.
func executeMCPToolHandler(c echo.Context) error {
	ctx := c.Request().Context()
	toolName := c.Param("toolName")
	if toolName == "" {
		return c.JSON(http.StatusBadRequest,
			map[string]string{"error": "missing tool name parameter"})
	}

	// Parse payload for tool arguments
	payload, err := parsePayload(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest,
			map[string]string{"error": "invalid JSON payload"})
	}

	// Get tool arguments from the payload
	args, ok := payload["args"]
	if !ok {
		return c.JSON(http.StatusBadRequest,
			map[string]string{"error": "missing 'args' field in payload"})
	}

	// Determine if this is a GitHub tool (starts with "gh_")
	if strings.HasPrefix(toolName, "gh_") {
		// For GitHub tools, remove the "gh_" prefix and use the GitHub client
		githubToolName := strings.TrimPrefix(toolName, "gh_")

		// Start the external GitHub MCP server
		extClient, cleanup, err := startExternalMCP(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError,
				map[string]string{"error": fmt.Sprintf("failed to start GitHub MCP: %v", err)})
		}
		defer cleanup()

		// Execute the tool
		result, err := extClient.CallTool(ctx, githubToolName, args)
		if err != nil {
			return c.JSON(http.StatusInternalServerError,
				map[string]string{"error": fmt.Sprintf("GitHub MCP tool execution failed: %v", err)})
		}

		return c.JSON(http.StatusOK, result)
	} else {
		// This is an internal tool
		// Load configuration
		cfg, err := LoadConfig("config.yaml")
		if err != nil {
			return c.JSON(http.StatusInternalServerError,
				map[string]string{"error": fmt.Sprintf("config load: %v", err)})
		}

		// Set up client/server
		client, server, err := setupMCPCommunication()
		if err != nil {
			return c.JSON(http.StatusInternalServerError,
				map[string]string{"error": fmt.Sprintf("setup comms: %v", err)})
		}

		// Register only internal tools
		registerMCPTools(server, cfg)

		// Start server and initialize client
		go startMCPServer(server)
		if _, err := client.Initialize(ctx); err != nil {
			return c.JSON(http.StatusInternalServerError,
				map[string]string{"error": fmt.Sprintf("client init: %v", err)})
		}

		// Execute the tool
		result, err := client.CallTool(ctx, toolName, args)
		if err != nil {
			return c.JSON(http.StatusInternalServerError,
				map[string]string{"error": fmt.Sprintf("internal MCP tool execution failed: %v", err)})
		}

		return c.JSON(http.StatusOK, result)
	}
}

// ---------------------------------------------------------------------------
// The helper functions below are unchanged from the original handlers.go
// ---------------------------------------------------------------------------

// parsePayload parses arbitrary JSON into map[string]interface{}.
func parsePayload(c echo.Context) (map[string]interface{}, error) {
	var payload map[string]interface{}
	if err := c.Bind(&payload); err != nil {
		return nil, err
	}
	log.Printf("MCP payload: %+v", payload)
	return payload, nil
}

// getAction extracts the "action" key (defaults to "listTools").
func getAction(p map[string]interface{}) string {
	if a, ok := p["action"].(string); ok && a != "" {
		return a
	}
	return "listTools"
}

// setupMCPCommunication builds an in‑memory pipe between client & server.
func setupMCPCommunication() (*mcp.Client, *mcp.Server, error) {
	cR, sW := io.Pipe() // client reads / server writes
	sR, cW := io.Pipe() // server reads / client writes

	clientTr := stdio.NewStdioServerTransportWithIO(cR, cW)
	serverTr := stdio.NewStdioServerTransportWithIO(sR, sW)

	return mcp.NewClient(clientTr), mcp.NewServer(serverTr), nil
}

// startMCPServer runs the server in a goroutine.
func startMCPServer(s *mcp.Server) {
	if err := s.Serve(); err != nil {
		log.Printf("MCP server error: %v", err)
	}
}

// handleAction processes the action specified in the payload.
func handleAction(client *mcp.Client, action string, payload map[string]interface{}) (interface{}, error) {
	switch action {
	case "listTools":
		return client.ListTools(context.Background(), nil)
	case "execute":
		toolName, ok := payload["tool"].(string)
		if !ok || toolName == "" {
			return nil, fmt.Errorf("missing tool name for execution")
		}
		argsRaw, ok := payload["args"]
		if !ok {
			return nil, fmt.Errorf("missing args for tool execution")
		}
		return client.CallTool(context.Background(), toolName, argsRaw)
	default:
		return nil, fmt.Errorf("action '%s' not implemented", action)
	}
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
