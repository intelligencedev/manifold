package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"manifold/internal/file_editor"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

// Global file editor instance
var globalEditor *file_editor.Editor

// Initialize the file editor with DATA_PATH from environment or default workspace
func init() {
	workspaceRoot := os.Getenv("DATA_PATH")
	if workspaceRoot == "" {
		workspaceRoot = "/tmp/manifold-workspace" // Default workspace
		os.MkdirAll(workspaceRoot, 0755)
	}

	var err error
	globalEditor, err = file_editor.NewEditor(workspaceRoot)
	if err != nil {
		log.Printf("Warning: Could not initialize file editor: %v", err)
	}
}

// handleCalculateTool handles the calculate tool
func handleCalculateTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	operation, _ := arguments["operation"].(string)
	a, _ := arguments["a"].(float64)
	b, _ := arguments["b"].(float64)

	args := CalculateArgs{
		Operation: operation,
		A:         a,
		B:         b,
	}

	res, err := calculateTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleTimeTool handles the time tool
func handleTimeTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	format, _ := arguments["format"].(string)

	args := TimeArgs{
		Format: format,
	}

	res, err := timeTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleWeatherTool handles the weather tool
func handleWeatherTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	latitude, _ := arguments["latitude"].(float64)
	longitude, _ := arguments["longitude"].(float64)

	args := WeatherArgs{
		Latitude:  latitude,
		Longitude: longitude,
	}

	res, err := getWeatherTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleGitPullTool handles the git pull tool
func handleGitPullTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	path, _ := arguments["path"].(string)

	args := GitRepoArgs{
		Path: path,
	}

	res, err := gitPullTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleGitPushTool handles the git push tool
func handleGitPushTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	path, _ := arguments["path"].(string)

	args := GitRepoArgs{
		Path: path,
	}

	res, err := gitPushTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleGitCloneTool handles the git clone tool
func handleGitCloneTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	repoUrl, _ := arguments["repoUrl"].(string)
	path, _ := arguments["path"].(string)

	args := GitCloneArgs{
		RepoURL: repoUrl,
		Path:    path,
	}

	res, err := gitCloneTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleShellCommandTool handles the shell command tool
// func handleShellCommandTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
// 	arguments := request.Params.Arguments
// 	commandArray, _ := arguments["command"].([]interface{})
// 	dir, _ := arguments["dir"].(string)

// 	// Convert interface slice to string slice
// 	command := make([]string, len(commandArray))
// 	for i, v := range commandArray {
// 		command[i], _ = v.(string)
// 	}

// 	args := ShellCommandArgs{
// 		Command: command,
// 		Dir:     dir,
// 	}

// 	res, err := runShellCommandTool(args)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &mcp.CallToolResult{
// 		Content: []mcp.Content{
// 			mcp.TextContent{
// 				Type: "text",
// 				Text: res,
// 			},
// 		},
// 	}, nil
// }

// handleCLITool executes a raw CLI command using the underlying shell.
func handleCLITool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	command, _ := arguments["command"].(string)
	dir, _ := arguments["dir"].(string)

	args := CLIToolArgs{
		Command: command,
		Dir:     dir,
	}

	res, err := cliTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: res},
		},
	}, nil
}

// handleGoBuildTool handles the go build tool
func handleGoBuildTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	path, _ := arguments["path"].(string)

	args := GoBuildArgs{
		Path: path,
	}

	res, err := goBuildTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleGoTestTool handles the go test tool
func handleGoTestTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	path, _ := arguments["path"].(string)

	args := GoTestArgs{
		Path: path,
	}

	res, err := goTestTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleFormatGoCodeTool handles the format go code tool
func handleFormatGoCodeTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	path, _ := arguments["path"].(string)

	args := FormatGoCodeArgs{
		Path: path,
	}

	res, err := formatGoCodeTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleLintCodeTool handles the lint code tool
func handleLintCodeTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	path, _ := arguments["path"].(string)
	linterName, _ := arguments["linterName"].(string)

	args := LintCodeArgs{
		Path:       path,
		LinterName: linterName,
	}

	res, err := lintCodeTool(args)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: res,
			},
		},
	}, nil
}

// handleFileTool handles the file tool (currently unused)
//
//nolint:unused
func handleFileTool(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args FileToolArgs
	if err := mapToStruct(request.Params.Arguments, &args); err != nil {
		return nil, err
	}
	res, err := fileTool(args)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: res}},
	}, nil
}

func mapToStruct(in map[string]interface{}, out interface{}) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// handleEditFileTool handles the new enhanced file editing tool
func handleEditFileTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if globalEditor == nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "File editor not available - initialization failed",
				},
			},
			IsError: true,
		}, nil
	}

	// Parse arguments into EditRequest
	var req file_editor.EditRequest
	if err := mapToStruct(request.Params.Arguments, &req); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error parsing arguments: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Perform the edit operation
	response, err := globalEditor.Edit(ctx, req)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Internal error: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Format the response
	return formatFileEditorResponse(response), nil
}

// formatFileEditorResponse converts EditResponse to MCP CallToolResult
func formatFileEditorResponse(response file_editor.EditResponse) *mcp.CallToolResult {
	if !response.Success {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error: %s", response.Error),
				},
			},
			IsError: true,
		}
	}

	var content []mcp.Content

	// Add main message
	if response.Message != "" {
		content = append(content, mcp.TextContent{
			Type: "text",
			Text: response.Message,
		})
	}

	// Add file content if present (for read operations)
	if response.Content != "" {
		content = append(content, mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("Content:\n%s", response.Content),
		})
	}

	// Add search matches if present
	if len(response.Matches) > 0 {
		matchText := "Matches:\n"
		for _, match := range response.Matches {
			matchText += fmt.Sprintf("Line %d: %s\n", match.LineNumber, match.Line)
		}
		content = append(content, mcp.TextContent{
			Type: "text",
			Text: matchText,
		})
	}

	// Add diff if present (for preview operations)
	if response.Diff != "" {
		content = append(content, mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("Diff preview:\n%s", response.Diff),
		})
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}
}
