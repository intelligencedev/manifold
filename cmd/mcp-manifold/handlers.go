package main

import (
	"context"
	"encoding/json"

	mcp "github.com/mark3labs/mcp-go/mcp"
)

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
func handleShellCommandTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	arguments := request.Params.Arguments
	commandArray, _ := arguments["command"].([]interface{})
	dir, _ := arguments["dir"].(string)

	// Convert interface slice to string slice
	command := make([]string, len(commandArray))
	for i, v := range commandArray {
		command[i], _ = v.(string)
	}

	args := ShellCommandArgs{
		Command: command,
		Dir:     dir,
	}

	res, err := runShellCommandTool(args)
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

func handleFileTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
