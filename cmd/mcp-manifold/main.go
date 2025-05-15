package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

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
			res := searchDDG(args.Query)

			// Convert the go slice to a string
			resStr := strings.Join(res, "\n")
			if len(resStr) == 0 {
				resStr = "No results found."
			}

			// return the result
			return mcp.NewToolResponse(mcp.NewTextContent(resStr)), nil
		}},
		{"web_content", "Fetches and extracts content from web URLs", func(args WebContentArgs) (*mcp.ToolResponse, error) {
			urlsParam := args.URLs

			if urlsParam == "" {
				return nil, fmt.Errorf("URLs are required")
			}
			// Split the URLs by comma

			urls := strings.Split(urlsParam, ",")
			var wg sync.WaitGroup
			var mu sync.Mutex
			results := make(map[string]interface{})

			resultChan := make(chan *mcp.ToolResponse)
			errChan := make(chan error)

			go func() {
				for _, pageURL := range urls {
					wg.Add(1)
					go func(url string) {
						defer wg.Done()
						content, err := webGetHandler(url)
						mu.Lock()
						defer mu.Unlock()
						if err != nil {
							results[url] = map[string]string{"error": fmt.Sprintf("Error extracting web content: %v", err)}
						} else {
							results[url] = content
						}
					}(pageURL)
				}

				wg.Wait()

				jsonResult, err := json.Marshal(results)
				if err != nil {
					errChan <- fmt.Errorf("error marshaling results: %w", err)
					return
				}
				resultChan <- mcp.NewToolResponse(mcp.NewTextContent(string(jsonResult)))
			}()

			// Wait for result or timeout
			select {
			case result := <-resultChan:
				return result, nil
			case err := <-errChan:
				return nil, err
			case <-time.After(60 * time.Second):
				jsonResult, err := json.Marshal(results)
				if err != nil {
					return nil, fmt.Errorf("error marshaling results after timeout: %w", err)
				}
				return mcp.NewToolResponse(mcp.NewTextContent(string(jsonResult))), nil
			}
		}},
	}

	for _, tool := range tools {
		if err := server.RegisterTool(tool.name, tool.description, tool.handler); err != nil {
			log.Printf("Error registering %s tool: %v", tool.name, err)
		}
	}
}

// registerGitTools registers tools related to git operations
// These tools are missing in other git MCP servers tested
func registerGitTools(server *mcp.Server) {
	tools := []struct {
		name        string
		description string
		handler     interface{}
	}{
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
