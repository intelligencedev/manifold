package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"manifold/internal/web"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// ------------------------------------------------------------------------
// Tool Metadata and Registry
// ------------------------------------------------------------------------

// ToolInfo represents the metadata returned in /tool/list.
type ToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ExampleArgs map[string]interface{} `json:"example_args,omitempty"`
}

// ToolDefinition ties together the metadata and the actual runtime logic.
type ToolDefinition struct {
	Info      ToolInfo
	HandlerFn func(args json.RawMessage) (string, error)
}

// ------------------------------------------------------------------------
// Tool Handlers
// ------------------------------------------------------------------------

func handleThink(rawArgs json.RawMessage) (string, error) {
	var args ThinkArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for think tool: %w", err)
	}

	// Create a separator line.
	separator := strings.Repeat("=", 100)

	// ANSI escape codes for magenta background and white text.
	header := fmt.Sprintf("\n%s\n\033[45m\033[97m🧠 CLAUDE-STYLE THINKING PROCESS 🧠\033[0m\n%s\n", separator, separator)
	fmt.Println(header)

	// Split the thought into paragraphs.
	paragraphs := strings.Split(args.Thought, "\n\n")
	for _, para := range paragraphs {
		trimmed := strings.TrimSpace(para)
		if trimmed == "" {
			continue
		}
		// If the paragraph starts with a numbered list, print it in green.
		if strings.HasPrefix(trimmed, "1.") || strings.HasPrefix(trimmed, "2.") ||
			strings.HasPrefix(trimmed, "3.") || strings.HasPrefix(trimmed, "4.") ||
			strings.HasPrefix(trimmed, "5.") {
			fmt.Println("\033[32m" + trimmed + "\033[0m\n")
		} else {
			// Otherwise, print in white.
			fmt.Println("\033[37m" + trimmed + "\033[0m\n")
		}
	}

	footer := fmt.Sprintf("%s\n", separator)
	fmt.Println("\033[45m\033[97m" + footer + "\033[0m\n")

	// Calculate and log the word count.
	words := strings.Fields(args.Thought)
	wordCount := len(words)
	log.Printf("Model used the Claude-style think tool (%d words)", wordCount)

	return "Thought recorded. Continue with your analysis.", nil
}

// handleHello processes the "hello" tool.
func handleHello(rawArgs json.RawMessage) (string, error) {
	var args HelloArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for hello tool: %w", err)
	}
	return fmt.Sprintf("Hello, %s!", args.Name), nil
}

// handleCalculate processes the "calculate" tool.
func handleCalculate(rawArgs json.RawMessage) (string, error) {
	var args CalculateArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for calculate tool: %w", err)
	}
	switch args.Operation {
	case "add":
		return fmt.Sprintf("Result of add: %.2f", args.A+args.B), nil
	case "subtract":
		return fmt.Sprintf("Result of subtract: %.2f", args.A-args.B), nil
	case "multiply":
		return fmt.Sprintf("Result of multiply: %.2f", args.A*args.B), nil
	case "divide":
		if args.B == 0 {
			return "", fmt.Errorf("division by zero")
		}
		return fmt.Sprintf("Result of divide: %.2f", args.A/args.B), nil
	default:
		return "", fmt.Errorf("unknown operation: %s", args.Operation)
	}
}

// handleTime processes the "time" tool.
func handleTime(rawArgs json.RawMessage) (string, error) {
	var args TimeArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for time tool: %w", err)
	}
	format := time.RFC3339
	if args.Format != "" {
		format = args.Format
	}
	return time.Now().Format(format), nil
}

// handleWeather processes the "weather" tool.
func handleWeather(rawArgs json.RawMessage) (string, error) {
	var args WeatherArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for weather tool: %w", err)
	}
	return getWeatherTool(args)
}

// handleReadFile processes the "read_file" tool.
func handleReadFile(rawArgs json.RawMessage) (string, error) {
	var args ReadFileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for read_file tool: %w", err)
	}
	return readFileTool(args)
}

// handleWriteFile processes the "write_file" tool.
func handleWriteFile(rawArgs json.RawMessage) (string, error) {
	var args WriteFileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for write_file tool: %w", err)
	}
	return writeFileTool(args)
}

// handleListDirectory processes the "list_directory" tool.
func handleListDirectory(rawArgs json.RawMessage) (string, error) {
	var args ListDirectoryArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for list_directory tool: %w", err)
	}
	return listDirectoryTool(args)
}

// handleCreateDirectory processes the "create_directory" tool.
func handleCreateDirectory(rawArgs json.RawMessage) (string, error) {
	var args CreateDirectoryArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for create_directory tool: %w", err)
	}
	return createDirectoryTool(args)
}

// handleMoveFile processes the "move_file" tool.
func handleMoveFile(rawArgs json.RawMessage) (string, error) {
	var args MoveFileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for move_file tool: %w", err)
	}
	return moveFileTool(args)
}

// handleGitInit processes the "git_init" tool.
func handleGitInit(rawArgs json.RawMessage) (string, error) {
	var args GitInitArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for git_init tool: %w", err)
	}
	return gitInitTool(args)
}

// handleGitStatus processes the "git_status" tool.
func handleGitStatus(rawArgs json.RawMessage) (string, error) {
	var args GitRepoArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for git_status tool: %w", err)
	}
	return gitStatusTool(args)
}

// handleGitAdd processes the "git_add" tool.
func handleGitAdd(rawArgs json.RawMessage) (string, error) {
	var args GitAddArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for git_add tool: %w", err)
	}
	return gitAddTool(args)
}

// handleGitCommit processes the "git_commit" tool.
func handleGitCommit(rawArgs json.RawMessage) (string, error) {
	var args GitCommitArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for git_commit tool: %w", err)
	}
	return gitCommitTool(args)
}

// handleGitPull processes the "git_pull" tool.
func handleGitPull(rawArgs json.RawMessage) (string, error) {
	var args GitRepoArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for git_pull tool: %w", err)
	}
	return gitPullTool(args)
}

// handleGitPush processes the "git_push" tool.
func handleGitPush(rawArgs json.RawMessage) (string, error) {
	var args GitRepoArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for git_push tool: %w", err)
	}
	return gitPushTool(args)
}

// handleSearchFiles processes the "search_files" tool.
func handleSearchFiles(rawArgs json.RawMessage) (string, error) {
	var args SearchFilesArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for search_files tool: %w", err)
	}
	return searchFilesTool(args)
}

// handleDeleteFile processes the "delete_file" tool.
func handleDeleteFile(rawArgs json.RawMessage) (string, error) {
	var args DeleteFileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for delete_file tool: %w", err)
	}
	return deleteFileTool(args)
}

// handleCopyFile processes the "copy_file" tool.
func handleCopyFile(rawArgs json.RawMessage) (string, error) {
	var args CopyFileArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for copy_file tool: %w", err)
	}
	return copyFileTool(args)
}

// handleGitClone processes the "git_clone" tool.
func handleGitClone(rawArgs json.RawMessage) (string, error) {
	var args GitCloneArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for git_clone tool: %w", err)
	}
	return gitCloneTool(args)
}

// handleGitCheckout processes the "git_checkout" tool.
func handleGitCheckout(rawArgs json.RawMessage) (string, error) {
	var args GitCheckoutArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for git_checkout tool: %w", err)
	}
	return gitCheckoutTool(args)
}

// handleRunShellCommand processes the "run_shell_command" tool.
func handleRunShellCommand(rawArgs json.RawMessage) (string, error) {
	var args ShellCommandArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for run_shell_command tool: %w", err)
	}
	return runShellCommandTool(args)
}

// handleGoBuild processes the "go_build" tool.
func handleGoBuild(rawArgs json.RawMessage) (string, error) {
	var args GoBuildArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for go_build tool: %w", err)
	}
	return goBuildTool(args)
}

// handleGoTest processes the "go_test" tool.
func handleGoTest(rawArgs json.RawMessage) (string, error) {
	var args GoTestArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for go_test tool: %w", err)
	}
	return goTestTool(args)
}

// handleFormatGoCode processes the "format_go_code" tool.
func handleFormatGoCode(rawArgs json.RawMessage) (string, error) {
	var args FormatGoCodeArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for format_go_code tool: %w", err)
	}
	return formatGoCodeTool(args)
}

// handleLintCode processes the "lint_code" tool.
func handleLintCode(rawArgs json.RawMessage) (string, error) {
	var args LintCodeArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for lint_code tool: %w", err)
	}
	return lintCodeTool(args)
}

// handleWebSearch processes the "web_search" tool.
func handleWebSearch(rawArgs json.RawMessage) (string, error) {
	var args WebSearchArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for web_search tool: %w", err)
	}
	return webSearchTool(args)
}

// handleWebContent processes the "web_content" tool.
func handleWebContent(rawArgs json.RawMessage) (string, error) {
	var args WebContentArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for web_content tool: %w", err)
	}
	return webContentTool(args)
}

func handleAgent(c echo.Context, config *Config, rawArgs json.RawMessage) (string, error) {
	// 1) Unmarshal the agent arguments.
	var args AgentArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return "", fmt.Errorf("invalid arguments for agent tool: %w", err)
	}
	if args.MaxCalls <= 0 {
		args.MaxCalls = 5 // fallback
	}

	// 2) Agent memory to store partial results from tasks
	agentMemory := make(map[string]string)

	// 3) Build a stable JSON or text representation of the available tools.
	//    This can be done by iterating over the toolRegistry or using a JSON approach.
	toolsJSON := buildToolsListJSON() // replicate or adapt from mcp.go

	// 4) Prepare an initial conversation or message list.
	var conversation []Message
	conversation = append(conversation, Message{
		Role:    "system",
		Content: "You are an advanced planning assistant. ... (system instructions here)",
	})
	conversation = append(conversation, Message{
		Role:    "user",
		Content: args.Query,
	})

	// 5) Loop up to maxCalls times
	var resultsMu sync.Mutex
	for iteration := 0; iteration < args.MaxCalls; iteration++ {
		// -- PHASE 1: PLAN --
		// Build a prompt that says "Output a JSON array of tasks to run," etc.
		planPrompt := fmt.Sprintf(`...
Available Tools:
%s
User Query: %s
...`, toolsJSON, args.Query)
		conversation = append(conversation, Message{
			Role:    "user",
			Content: planPrompt,
		})

		planOutput := completionsHandler(c, config)

		tasks, parseErr := parsePlanOutput(planOutput.Error())
		if parseErr != nil {
			// handle parse error or partial
			return fmt.Sprintf("Error parsing plan output: %v", parseErr), nil
		}
		if len(tasks) == 0 {
			// Means we got "[]", so do a final answer and break
			finalAns := tryFinalAnswer(c, config, conversation, agentMemory, args)
			return finalAns, nil
		}

		// -- PHASE 2: EXECUTION --
		iterationLog := []string{}
		err := executePlanTasksConcurrently(tasks, &resultsMu, agentMemory, iterationLog)
		if err != nil {
			// partial final
			return fmt.Sprintf("Error executing tasks: %v", err), nil
		}

		// append iteration logs to conversation
		for _, line := range iterationLog {
			conversation = append(conversation, Message{Role: "assistant", Content: line})
		}

		// -- PHASE 3: Final check
		finPrompt := buildFinalizationPrompt(args.Query, iterationLog, agentMemory, toolsJSON)
		convFin := []Message{
			{Role: "system", Content: "... instructions ..."},
			{Role: "user", Content: finPrompt},
		}

		// change the context to the finalization prompt
		c.Set("conversation", convFin)

		finalAnswer := completionsHandler(c, config)

		if isConclusion(finalAnswer.Error()) {
			return finalAnswer.Error(), nil
		} else {
			// Add finalAnswer into conversation for the next iteration
			conversation = append(conversation, Message{Role: "assistant", Content: finalAnswer.Error()})
		}
	}

	// If we exhausted the loop, return partial
	partial := tryFinalAnswer(c, config, conversation, agentMemory, args)
	return partial, nil
}

// ------------------------------------------------------------------------
// Tool Registry
// ------------------------------------------------------------------------

var toolRegistry = map[string]ToolDefinition{
	"think": {
		Info: ToolInfo{
			Name:        "think",
			Description: "Processes a thought using a reasoning process",
			ExampleArgs: map[string]interface{}{
				"thought": "Your detailed analysis and reasoning here...",
			},
		},
		HandlerFn: handleThink,
	},
	"agent": {
		Info: ToolInfo{
			Name:        "agent",
			Description: "Agent that uses LLM to decide which tools to call (Plan & Execute style)",
			ExampleArgs: map[string]interface{}{
				"query":    "User's query or high-level task",
				"maxCalls": 5,
			},
		},
		HandlerFn: func(rawArgs json.RawMessage) (string, error) {
			// Create a dummy echo.Context and provide a valid config instance
			e := echo.New()
			c := e.NewContext(nil, nil)
			config, err := LoadConfig("config.yaml")
			if err != nil {
				return "", fmt.Errorf("failed to load config: %w", err)
			}
			return handleAgent(c, config, rawArgs)
		},
	},
	"hello": {
		Info: ToolInfo{
			Name:        "hello",
			Description: "Says hello to the provided name",
			ExampleArgs: map[string]interface{}{
				"name": "Alice",
			},
		},
		HandlerFn: handleHello,
	},
	"calculate": {
		Info: ToolInfo{
			Name:        "calculate",
			Description: "Performs basic mathematical operations",
			ExampleArgs: map[string]interface{}{
				"operation": "add",
				"a":         5,
				"b":         3,
			},
		},
		HandlerFn: handleCalculate,
	},
	"time": {
		Info: ToolInfo{
			Name:        "time",
			Description: "Returns the current time",
			ExampleArgs: map[string]interface{}{
				"format": "optional time format",
			},
		},
		HandlerFn: handleTime,
	},
	"weather": {
		Info: ToolInfo{
			Name:        "weather",
			Description: "Fetches weather information for a given location",
			ExampleArgs: map[string]interface{}{
				"latitude":  37.7749,
				"longitude": -122.4194,
			},
		},
		HandlerFn: handleWeather,
	},
	"read_file": {
		Info: ToolInfo{
			Name:        "read_file",
			Description: "Reads and returns the content of a file",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/file.txt",
			},
		},
		HandlerFn: handleReadFile,
	},
	"write_file": {
		Info: ToolInfo{
			Name:        "write_file",
			Description: "Writes content to a file",
			ExampleArgs: map[string]interface{}{
				"path":    "/path/to/file.txt",
				"content": "Hello, World!",
			},
		},
		HandlerFn: handleWriteFile,
	},
	"list_directory": {
		Info: ToolInfo{
			Name:        "list_directory",
			Description: "Lists files and directories within a directory",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/directory",
			},
		},
		HandlerFn: handleListDirectory,
	},
	"create_directory": {
		Info: ToolInfo{
			Name:        "create_directory",
			Description: "Creates a new directory",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/new/directory",
			},
		},
		HandlerFn: handleCreateDirectory,
	},
	"move_file": {
		Info: ToolInfo{
			Name:        "move_file",
			Description: "Moves or renames a file or directory",
			ExampleArgs: map[string]interface{}{
				"source":      "/path/to/source",
				"destination": "/path/to/destination",
			},
		},
		HandlerFn: handleMoveFile,
	},
	"git_init": {
		Info: ToolInfo{
			Name:        "git_init",
			Description: "Initializes a new Git repository",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/repo",
			},
		},
		HandlerFn: handleGitInit,
	},
	"git_status": {
		Info: ToolInfo{
			Name:        "git_status",
			Description: "Returns the Git status of a repository",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/repo",
			},
		},
		HandlerFn: handleGitStatus,
	},
	"git_add": {
		Info: ToolInfo{
			Name:        "git_add",
			Description: "Stages changes in a Git repository",
			ExampleArgs: map[string]interface{}{
				"path":     "/path/to/repo",
				"fileList": []string{"file1.txt", "file2.txt"},
			},
		},
		HandlerFn: handleGitAdd,
	},
	"git_commit": {
		Info: ToolInfo{
			Name:        "git_commit",
			Description: "Commits changes in a Git repository",
			ExampleArgs: map[string]interface{}{
				"path":    "/path/to/repo",
				"message": "Initial commit",
			},
		},
		HandlerFn: handleGitCommit,
	},
	"git_pull": {
		Info: ToolInfo{
			Name:        "git_pull",
			Description: "Pulls changes from a remote repository",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/repo",
			},
		},
		HandlerFn: handleGitPull,
	},
	"git_push": {
		Info: ToolInfo{
			Name:        "git_push",
			Description: "Pushes changes to a remote repository",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/repo",
			},
		},
		HandlerFn: handleGitPush,
	},
	"search_files": {
		Info: ToolInfo{
			Name:        "search_files",
			Description: "Searches for files containing a specific pattern",
			ExampleArgs: map[string]interface{}{
				"path":    "/path/to/search",
				"pattern": "search term",
			},
		},
		HandlerFn: handleSearchFiles,
	},
	"delete_file": {
		Info: ToolInfo{
			Name:        "delete_file",
			Description: "Deletes a file or directory",
			ExampleArgs: map[string]interface{}{
				"path":      "/path/to/file_or_directory",
				"recursive": true,
			},
		},
		HandlerFn: handleDeleteFile,
	},
	"copy_file": {
		Info: ToolInfo{
			Name:        "copy_file",
			Description: "Copies a file or directory",
			ExampleArgs: map[string]interface{}{
				"source":      "/path/to/source",
				"destination": "/path/to/destination",
				"recursive":   true,
			},
		},
		HandlerFn: handleCopyFile,
	},
	"git_clone": {
		Info: ToolInfo{
			Name:        "git_clone",
			Description: "Clones a Git repository",
			ExampleArgs: map[string]interface{}{
				"repoUrl": "https://github.com/example/repo.git",
				"path":    "/path/to/clone",
			},
		},
		HandlerFn: handleGitClone,
	},
	"git_checkout": {
		Info: ToolInfo{
			Name:        "git_checkout",
			Description: "Checks out a Git branch",
			ExampleArgs: map[string]interface{}{
				"path":      "/path/to/repo",
				"branch":    "feature-branch",
				"createNew": true,
			},
		},
		HandlerFn: handleGitCheckout,
	},
	"run_shell_command": {
		Info: ToolInfo{
			Name:        "run_shell_command",
			Description: "Executes a shell command",
			ExampleArgs: map[string]interface{}{
				"command": []string{"ls", "-la"},
				"dir":     "/path/to/dir",
			},
		},
		HandlerFn: handleRunShellCommand,
	},
	"go_build": {
		Info: ToolInfo{
			Name:        "go_build",
			Description: "Builds a Go module",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/module",
			},
		},
		HandlerFn: handleGoBuild,
	},
	"go_test": {
		Info: ToolInfo{
			Name:        "go_test",
			Description: "Runs tests for a Go module",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/module",
			},
		},
		HandlerFn: handleGoTest,
	},
	"format_go_code": {
		Info: ToolInfo{
			Name:        "format_go_code",
			Description: "Formats Go code",
			ExampleArgs: map[string]interface{}{
				"path": "/path/to/code",
			},
		},
		HandlerFn: handleFormatGoCode,
	},
	"lint_code": {
		Info: ToolInfo{
			Name:        "lint_code",
			Description: "Lints code using a specified linter",
			ExampleArgs: map[string]interface{}{
				"path":       "/path/to/code",
				"linterName": "golangci-lint",
			},
		},
		HandlerFn: handleLintCode,
	},
	// Add web tools below
	"web_search": {
		Info: ToolInfo{
			Name:        "web_search",
			Description: "Performs a web search using the DuckDuckGo backend",
			ExampleArgs: map[string]interface{}{
				"query":          "OpenAI GPT-4",
				"result_size":    3,
				"search_backend": "ddg",
			},
		},
		HandlerFn: handleWebSearch,
	},
	"web_content": {
		Info: ToolInfo{
			Name:        "web_content",
			Description: "Fetches content from a list of URLs",
			ExampleArgs: map[string]interface{}{
				"urls": []string{"https://example.com", "https://example.org"},
			},
		},
		HandlerFn: handleWebContent,
	},
}

// ------------------------------------------------------------------------
// Tool Execution and Listing
// ------------------------------------------------------------------------

// GetAllTools returns the list of tool info for /tool/list.
func GetAllTools() []ToolInfo {
	tools := make([]ToolInfo, 0, len(toolRegistry))
	for _, def := range toolRegistry {
		tools = append(tools, def.Info)
	}
	return tools
}

// ExecuteToolByName looks up the tool, parses the JSON arguments, and calls the correct handler.
func ExecuteToolByName(toolName string, rawArgs json.RawMessage) (string, error) {
	def, ok := toolRegistry[toolName]
	if !ok {
		return "", fmt.Errorf("unrecognized tool name: %s", toolName)
	}
	return def.HandlerFn(rawArgs)
}

// ------------------------------------------------------------------------
// Tool Argument Structs
// ------------------------------------------------------------------------

type ThinkArgs struct {
	Thought string `json:"thought" jsonschema:"required,description=The thought or reasoning text to be processed"`
}

type AgentArgs struct {
	Query    string `json:"query" jsonschema:"required,description=User's query"`
	MaxCalls int    `json:"maxCalls" jsonschema:"required,description=Max iteration steps allowed"`
}

type HelloArgs struct {
	Name string `json:"name"`
}

type CalculateArgs struct {
	Operation string  `json:"operation"` // add, subtract, etc.
	A         float64 `json:"a"`
	B         float64 `json:"b"`
}

type TimeArgs struct {
	Format string `json:"format,omitempty"`
}

// PromptArgs represents the arguments for custom prompts.
type PromptArgs struct {
	Input string `json:"input" jsonschema:"required,description=The input text to process"`
}

// WeatherArgs represents the arguments for the weather tool.
type WeatherArgs struct {
	Longitude float64 `json:"longitude" jsonschema:"required,description=The longitude of the location to get the weather for"`
	Latitude  float64 `json:"latitude" jsonschema:"required,description=The latitude of the location to get the weather for"`
}

// ReadFileArgs is used by the read_file tool.
type ReadFileArgs struct {
	Path string `json:"path" jsonschema:"required,description=Path to the file to read"`
}

// WriteFileArgs is used by the write_file tool.
type WriteFileArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Path to the file to write"`
	Content string `json:"content" jsonschema:"required,description=Content to write into the file"`
}

// ListDirectoryArgs is used by the list_directory tool.
type ListDirectoryArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path to list"`
}

// CreateDirectoryArgs is used by the create_directory tool.
type CreateDirectoryArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path to create"`
}

// MoveFileArgs is used by the move_file tool.
type MoveFileArgs struct {
	Source      string `json:"source" jsonschema:"required,description=Source file/directory path"`
	Destination string `json:"destination" jsonschema:"required,description=Destination file/directory path"`
}

// GitInitArgs is used by the git_init tool.
type GitInitArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory in which to initialize a Git repo"`
}

// GitRepoArgs is used by git_status, git_pull, and git_push tools.
type GitRepoArgs struct {
	Path string `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
}

// GitAddArgs is used by the git_add tool.
type GitAddArgs struct {
	Path     string   `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
	FileList []string `json:"fileList" jsonschema:"required,description=List of files to add (or empty to add all)"`
}

// GitCommitArgs is used by the git_commit tool.
type GitCommitArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
	Message string `json:"message" jsonschema:"required,description=Commit message"`
}

// ReadMultipleFilesArgs is used by the read_multiple_files tool.
type ReadMultipleFilesArgs struct {
	Paths []string `json:"paths" jsonschema:"required,description=List of file paths to read"`
}

// EditFileArgs is used by the edit_file tool.
type EditFileArgs struct {
	Path         string `json:"path" jsonschema:"required,description=Path to the file to edit"`
	Search       string `json:"search,omitempty" jsonschema:"description=Text to search for"`
	Replace      string `json:"replace,omitempty" jsonschema:"description=Text to replace with"`
	PatchContent string `json:"patchContent,omitempty" jsonschema:"description=A unified diff patch to apply to the file"`
}

// DirectoryTreeArgs is used by the directory_tree tool.
type DirectoryTreeArgs struct {
	Path     string `json:"path" jsonschema:"required,description=Root directory for the tree"`
	MaxDepth int    `json:"maxDepth,omitempty" jsonschema:"description=Limit recursion depth (0 for unlimited)"`
}

// SearchFilesArgs is used by the search_files tool.
type SearchFilesArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Base path to search"`
	Pattern string `json:"pattern" jsonschema:"required,description=Text or regex pattern to find"`
}

// GetFileInfoArgs is used by the get_file_info tool.
type GetFileInfoArgs struct {
	Path string `json:"path" jsonschema:"required,description=Path to the file or directory"`
}

// ListAllowedDirectoriesArgs is used by the list_allowed_directories tool.
type ListAllowedDirectoriesArgs struct{}

// DeleteFileArgs is used by the delete_file tool.
type DeleteFileArgs struct {
	Path      string `json:"path" jsonschema:"required,description=File or directory path to delete"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"description=If true, delete recursively"`
}

// CopyFileArgs is used by the copy_file tool.
type CopyFileArgs struct {
	Source      string `json:"source" jsonschema:"required"`
	Destination string `json:"destination" jsonschema:"required"`
	Recursive   bool   `json:"recursive,omitempty" jsonschema:"description=Copy directories recursively"`
}

// GitCloneArgs is used by the git_clone tool.
type GitCloneArgs struct {
	RepoURL string `json:"repoUrl" jsonschema:"required"`
	Path    string `json:"path" jsonschema:"required,description=Local path to clone into"`
}

// GitCheckoutArgs is used by the git_checkout tool.
type GitCheckoutArgs struct {
	Path      string `json:"path" jsonschema:"required"`
	Branch    string `json:"branch" jsonschema:"required"`
	CreateNew bool   `json:"createNew,omitempty" jsonschema:"description=Create a new branch if true"`
}

// GitDiffArgs is used by the git_diff tool.
type GitDiffArgs struct {
	Path    string `json:"path" jsonschema:"required"`
	FromRef string `json:"fromRef,omitempty" jsonschema:"description=Starting reference"`
	ToRef   string `json:"toRef,omitempty" jsonschema:"description=Ending reference"`
}

// ShellCommandArgs is used by the run_shell_command tool.
type ShellCommandArgs struct {
	Command []string `json:"command" jsonschema:"required"`
	Dir     string   `json:"dir" jsonschema:"required"`
}

// GoBuildArgs is used by the go_build tool.
type GoBuildArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of the Go module to build"`
}

// GoTestArgs is used by the go_test tool.
type GoTestArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of the Go module to test"`
}

// FormatGoCodeArgs is used by the format_go_code tool.
type FormatGoCodeArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of the Go code to format"`
}

// LintCodeArgs is used by the lint_code tool.
type LintCodeArgs struct {
	Path       string `json:"path" jsonschema:"required,description=Directory or file to lint"`
	LinterName string `json:"linterName,omitempty" jsonschema:"description=Name of the linter to use (optional)"`
}

// WebSearchArgs is used by the web_search tool.
type WebSearchArgs struct {
	Query         string `json:"query" jsonschema:"required,description=Search query text"`
	ResultSize    int    `json:"result_size,omitempty" jsonschema:"description=Number of results to return (default: 3)"`
	SearchBackend string `json:"search_backend,omitempty" jsonschema:"description=Search backend to use (default: ddg, alternative: sxng)"`
	SxngURL       string `json:"sxng_url,omitempty" jsonschema:"description=URL of SearXNG instance when using sxng backend"`
}

// WebContentArgs is used by the web_content tool.
type WebContentArgs struct {
	URLs []string `json:"urls" jsonschema:"required,description=List of URLs to fetch content from"`
}

// ---------------------------------------------------------------------
// Generic Helpers
// ---------------------------------------------------------------------

// callTool unmarshals JSON arguments into the given type and executes the tool function.
func callTool[T any](jsonArgs string, toolFunc func(T) (string, error)) (string, error) {
	var args T
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return toolFunc(args)
}

// runCommand executes an external command with the given name, arguments and working directory.
func runCommand(name, dir string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s command failed: %w\nOutput: %s", name, err, string(output))
	}
	return string(output), nil
}

// Exported for testing so that tests can override the command execution.
var RunCommand = runCommand

// ---------------------------------------------------------------------
// Helper Functions: Git
// ---------------------------------------------------------------------

// checkGitRepo verifies whether the given path is a valid Git repository.
func checkGitRepo(repoPath string) error {
	info, err := os.Stat(repoPath)
	if err != nil {
		return fmt.Errorf("cannot access path %q: %w", repoPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %q is not a directory", repoPath)
	}
	gitDir := filepath.Join(repoPath, ".git")
	gitInfo, err := os.Stat(gitDir)
	if err != nil {
		return fmt.Errorf("this path does not appear to be a Git repo (missing .git folder): %w", err)
	}
	if !gitInfo.IsDir() {
		return fmt.Errorf(".git is not a directory")
	}
	return nil
}

// ---------------------------------------------------------------------
// Helper Functions: Files & Directories
// ---------------------------------------------------------------------

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// copyDir recursively copies a directory from src to dst.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildDirectoryTree builds a text representation of a directory tree.
func buildDirectoryTree(path, prefix string, maxDepth, currentDepth int) (string, error) {
	if maxDepth > 0 && currentDepth > maxDepth {
		return "", nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	var result strings.Builder
	for _, entry := range entries {
		result.WriteString(prefix)
		if entry.IsDir() {
			result.WriteString("[DIR] " + entry.Name() + "\n")
			subtree, err := buildDirectoryTree(filepath.Join(path, entry.Name()), prefix+"    ", maxDepth, currentDepth+1)
			if err != nil {
				return "", err
			}
			result.WriteString(subtree)
		} else {
			result.WriteString("[FILE] " + entry.Name() + "\n")
		}
	}
	return result.String(), nil
}

// searchFilesRecursive searches recursively for files that contain the given pattern.
func searchFilesRecursive(root, pattern string) ([]string, error) {
	var matches []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		fullPath := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			subMatches, err := searchFilesRecursive(fullPath, pattern)
			if err != nil {
				return nil, err
			}
			matches = append(matches, subMatches...)
		} else {
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			if strings.Contains(string(data), pattern) {
				matches = append(matches, fullPath)
			}
		}
	}
	return matches, nil
}

// ---------------------------------------------------------------------
// Tool Implementations
// ---------------------------------------------------------------------

// HelloTool returns a greeting message.
func helloTool(args HelloArgs) (string, error) {
	return fmt.Sprintf("Hello, %s!", args.Name), nil
}

// CalculateTool performs a mathematical operation.
func calculateTool(args CalculateArgs) (string, error) {
	var result float64
	switch args.Operation {
	case "add":
		result = args.A + args.B
	case "subtract":
		result = args.A - args.B
	case "multiply":
		result = args.A * args.B
	case "divide":
		if args.B == 0 {
			return "", fmt.Errorf("division by zero")
		}
		result = args.A / args.B
	default:
		return "", fmt.Errorf("unknown operation: %s", args.Operation)
	}
	return fmt.Sprintf("Result of %s: %.2f", args.Operation, result), nil
}

// TimeTool returns the current time in the specified format.
func timeTool(args TimeArgs) (string, error) {
	format := time.RFC3339
	if args.Format != "" {
		format = args.Format
	}
	return time.Now().Format(format), nil
}

// GetWeatherTool fetches weather information from the Open-Meteo API.
func getWeatherTool(args WeatherArgs) (string, error) {
	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,wind_speed_10m&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m", args.Latitude, args.Longitude)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// ReadFileTool reads and returns the content of a file.
func readFileTool(args ReadFileArgs) (string, error) {
	bytes, err := os.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(bytes), nil
}

// WriteFileTool writes content to a file.
func writeFileTool(args WriteFileArgs) (string, error) {
	err := os.WriteFile(args.Path, []byte(args.Content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("Wrote file: %s", args.Path), nil
}

// ListDirectoryTool lists files and directories within a directory.
func listDirectoryTool(args ListDirectoryArgs) (string, error) {
	entries, err := os.ReadDir(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}
	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			lines = append(lines, "[DIR]  "+e.Name())
		} else {
			lines = append(lines, "[FILE] "+e.Name())
		}
	}
	return strings.Join(lines, "\n"), nil
}

// CreateDirectoryTool creates a new directory.
func createDirectoryTool(args CreateDirectoryArgs) (string, error) {
	err := os.MkdirAll(args.Path, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	return fmt.Sprintf("Directory created: %s", args.Path), nil
}

// MoveFileTool moves or renames a file or directory.
func moveFileTool(args MoveFileArgs) (string, error) {
	err := os.Rename(args.Source, args.Destination)
	if err != nil {
		return "", fmt.Errorf("failed to move/rename: %w", err)
	}
	return fmt.Sprintf("Moved/renamed '%s' to '%s'", args.Source, args.Destination), nil
}

// GitInitTool initializes a new Git repository.
func gitInitTool(args GitInitArgs) (string, error) {
	info, err := os.Stat(args.Path)
	if err != nil {
		return "", fmt.Errorf("cannot access path %q: %w", args.Path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", args.Path)
	}
	return RunCommand("git", args.Path, "init")
}

// GitStatusTool returns the Git status.
func gitStatusTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return RunCommand("git", args.Path, "status", "--short", "--branch")
}

// GitAddTool stages changes in a Git repository.
func gitAddTool(args GitAddArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	if len(args.FileList) == 0 {
		args.FileList = []string{"."}
	}
	fullArgs := append([]string{"add"}, args.FileList...)
	return RunCommand("git", args.Path, fullArgs...)
}

// GitCommitTool commits changes in a Git repository.
func gitCommitTool(args GitCommitArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Message) == "" {
		return "", fmt.Errorf("commit message cannot be empty")
	}
	return RunCommand("git", args.Path, "commit", "-m", args.Message)
}

// GitPullTool pulls changes from a remote repository.
func gitPullTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return RunCommand("git", args.Path, "pull", "--rebase")
}

// GitPushTool pushes changes to a remote repository.
func gitPushTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return RunCommand("git", args.Path, "push")
}

// ReadMultipleFilesTool reads multiple files and concatenates their content.
func readMultipleFilesTool(args ReadMultipleFilesArgs) (string, error) {
	var result strings.Builder
	for _, path := range args.Paths {
		data, err := os.ReadFile(path)
		if err != nil {
			result.WriteString(fmt.Sprintf("Error reading %s: %v\n", path, err))
		} else {
			result.WriteString(fmt.Sprintf("File: %s\n%s\n------\n", path, string(data)))
		}
	}
	return result.String(), nil
}

// EditFileTool edits a file by replacing occurrences of a search string.
func editFileTool(args EditFileArgs) (string, error) {
	if args.PatchContent != "" {
		return "", fmt.Errorf("patchContent not supported in this implementation")
	}
	if args.Search == "" {
		return "", fmt.Errorf("must provide a search string for edit_file")
	}
	original, err := os.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	edited := strings.ReplaceAll(string(original), args.Search, args.Replace)
	err = os.WriteFile(args.Path, []byte(edited), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write edited file: %w", err)
	}
	return fmt.Sprintf("Edited file: %s", args.Path), nil
}

// DirectoryTreeTool generates a directory tree structure.
func directoryTreeTool(args DirectoryTreeArgs) (string, error) {
	return buildDirectoryTree(args.Path, "", args.MaxDepth, 1)
}

// SearchFilesTool searches for files containing a specific pattern.
func searchFilesTool(args SearchFilesArgs) (string, error) {
	matches, err := searchFilesRecursive(args.Path, args.Pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "No files found matching the pattern.", nil
	}
	return "Files matching pattern:\n" + strings.Join(matches, "\n"), nil
}

// GetFileInfoTool returns information about a file or directory.
func getFileInfoTool(args GetFileInfoArgs) (string, error) {
	info, err := os.Stat(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	return fmt.Sprintf("Name: %s\nSize: %d bytes\nMode: %s\nModified: %s\nIsDir: %t",
		info.Name(), info.Size(), info.Mode().String(), info.ModTime().Format(time.RFC3339), info.IsDir()), nil
}

// ListAllowedDirectoriesTool returns allowed directories. (Stub implementation)
func listAllowedDirectoriesTool(args ListAllowedDirectoriesArgs) (string, error) {
	return "All directories are allowed.", nil
}

// DeleteFileTool deletes a file or directory.
func deleteFileTool(args DeleteFileArgs) (string, error) {
	var err error
	if args.Recursive {
		err = os.RemoveAll(args.Path)
	} else {
		err = os.Remove(args.Path)
	}
	if err != nil {
		return "", fmt.Errorf("failed to delete %s: %w", args.Path, err)
	}
	return fmt.Sprintf("Deleted: %s", args.Path), nil
}

// CopyFileTool copies a file or directory.
func copyFileTool(args CopyFileArgs) (string, error) {
	info, err := os.Stat(args.Source)
	if err != nil {
		return "", fmt.Errorf("failed to access source: %w", err)
	}
	if info.IsDir() {
		if !args.Recursive {
			return "", fmt.Errorf("source is a directory, set recursive to true to copy directories")
		}
		err = copyDir(args.Source, args.Destination)
	} else {
		err = copyFile(args.Source, args.Destination)
	}
	if err != nil {
		return "", fmt.Errorf("failed to copy: %w", err)
	}
	return fmt.Sprintf("Copied from %s to %s", args.Source, args.Destination), nil
}

// GitCloneTool clones a Git repository.
func gitCloneTool(args GitCloneArgs) (string, error) {
	return RunCommand("git", "", "clone", args.RepoURL, args.Path)
}

// GitCheckoutTool checks out a Git branch.
func gitCheckoutTool(args GitCheckoutArgs) (string, error) {
	var cmdArgs []string
	if args.CreateNew {
		cmdArgs = []string{"checkout", "-b", args.Branch}
	} else {
		cmdArgs = []string{"checkout", args.Branch}
	}
	return RunCommand("git", args.Path, cmdArgs...)
}

// GitDiffTool shows Git diff between two references.
func gitDiffTool(args GitDiffArgs) (string, error) {
	diffArgs := []string{"diff"}
	if args.FromRef != "" && args.ToRef != "" {
		diffArgs = append(diffArgs, args.FromRef, args.ToRef)
	}
	return RunCommand("git", args.Path, diffArgs...)
}

// RunShellCommandTool executes a shell command.
func runShellCommandTool(args ShellCommandArgs) (string, error) {
	if len(args.Command) == 0 {
		return "", fmt.Errorf("empty command")
	}
	return RunCommand(args.Command[0], args.Dir, args.Command[1:]...)
}

// GoBuildTool builds a Go module.
func goBuildTool(args GoBuildArgs) (string, error) {
	return RunCommand("go", args.Path, "build")
}

// GoTestTool runs tests for a Go module.
func goTestTool(args GoTestArgs) (string, error) {
	return RunCommand("go", args.Path, "test", "./...")
}

// FormatGoCodeTool formats Go code.
func formatGoCodeTool(args FormatGoCodeArgs) (string, error) {
	return RunCommand("go", args.Path, "fmt", "./...")
}

// LintCodeTool lints code using a specified linter.
func lintCodeTool(args LintCodeArgs) (string, error) {
	if args.LinterName != "" {
		return RunCommand(args.LinterName, args.Path, "run")
	}
	return RunCommand("golangci-lint", args.Path, "run")
}

// WebSearchTool performs a web search using the DuckDuckGo (ddg) backend.
func webSearchTool(args WebSearchArgs) (string, error) {
	results := web.SearchDDG(args.Query)
	if results == nil {
		return "", fmt.Errorf("error performing web search")
	}
	log.Printf("Web search results: %v", results)
	return strings.Join(results, "\n"), nil
}

// WebContentTool fetches content from a list of URLs.
func webContentTool(args WebContentArgs) (string, error) {
	if len(args.URLs) == 0 {
		return "", fmt.Errorf("no URLs provided; at least one URL is required")
	}
	log.Printf("Fetching content from URLs: %v", args.URLs)
	var content strings.Builder
	for _, urlStr := range args.URLs {
		urlStr = strings.TrimSpace(urlStr)
		if urlStr == "" {
			continue
		}
		apiURL := "http://localhost:8080/api/web-content?urls=" + urlStr
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Get(apiURL)
		if err != nil {
			return "", fmt.Errorf("web content request failed: %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}
		content.WriteString(string(bodyBytes))
	}
	return content.String(), nil
}

// Define the planTask struct
type planTask struct {
	TaskID    int    `json:"task_id"`
	Tool      string `json:"tool"`
	Args      string `json:"args"`
	DependsOn []int  `json:"depends_on"`
}

// parsePlanOutput tries to decode the LLM plan output into a list of planTask.
func parsePlanOutput(planText string) ([]planTask, error) {
	// handle triple backticks or code fence if present
	planText = strings.TrimPrefix(planText, "```json")
	planText = strings.TrimSuffix(planText, "```")

	var tasks []planTask
	if err := json.Unmarshal([]byte(planText), &tasks); err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}
	// Remove any tasks that are "agent" or invalid
	filtered := make([]planTask, 0, len(tasks))
	for i, t := range tasks {
		if strings.ToLower(t.Tool) == "agent" {
			log.Printf("Dropping plan task %d referencing 'agent' tool", i)
			continue
		}
		// If needed: set default TaskID if not set
		if t.TaskID == 0 {
			t.TaskID = i
		}
		filtered = append(filtered, t)
	}
	return filtered, nil
}

// executePlanTasksConcurrently runs tasks in concurrency waves based on their dependencies.
func executePlanTasksConcurrently(
	tasks []planTask,
	resultsMu *sync.Mutex,
	agentMemory map[string]string,
	iterationLog []string,
) error {
	completed := make(map[int]bool)
	for {
		// Find tasks that are not done, but whose dependencies are satisfied
		var readyBatch []int
		for i, t := range tasks {
			if completed[i] {
				continue
			}
			depsSatisfied := true
			for _, dep := range t.DependsOn {
				if dep < 0 || dep >= len(tasks) {
					// invalid dependency
					depsSatisfied = false
					break
				}
				if !completed[dep] {
					depsSatisfied = false
					break
				}
			}
			if depsSatisfied {
				readyBatch = append(readyBatch, i)
			}
		}
		if len(readyBatch) == 0 {
			// if no tasks are ready but not all tasks are done, we have a dependency deadlock
			doneCount := 0
			for _, v := range completed {
				if v {
					doneCount++
				}
			}
			if doneCount < len(tasks) {
				return fmt.Errorf("dependency deadlock or all tasks blocked. done=%d total=%d", doneCount, len(tasks))
			}
			// Otherwise all tasks are complete
			return nil
		}

		// Run this wave of tasks in parallel
		var wg sync.WaitGroup
		for _, idx := range readyBatch {
			wg.Add(1)
			go func(taskIndex int) {
				defer wg.Done()

				t := tasks[taskIndex]
				// 1) process references in the arguments
				processedArgs, pErr := processPreviousToolResults(string(t.Args), agentMemory)
				if pErr != nil {
					resultsMu.Lock()
					iterationLog = append(iterationLog, fmt.Sprintf(
						"Task %d (%s) argument-parse error: %v", taskIndex, t.Tool, pErr,
					))
					// store error in memory
					agentMemory[fmt.Sprintf("task_%d", taskIndex)] = fmt.Sprintf("Error: %v", pErr)
					resultsMu.Unlock()
					return
				}

				// 2) run the tool
				out, err := callToolInServer(t.Tool, processedArgs)
				resultsMu.Lock()
				defer resultsMu.Unlock()

				if err != nil {
					iterationLog = append(iterationLog, fmt.Sprintf(
						"Task %d (%s) failed: %v", taskIndex, t.Tool, err,
					))
					agentMemory[fmt.Sprintf("task_%d", taskIndex)] = fmt.Sprintf("Error: %v", err)
					// We do NOT return here. We let other tasks keep going if they can.
				} else {
					iterationLog = append(iterationLog, fmt.Sprintf(
						"Task %d (%s) output:\n%s", taskIndex, t.Tool, out,
					))
					agentMemory[fmt.Sprintf("task_%d", taskIndex)] = out
				}
				completed[taskIndex] = true
			}(idx)
		}

		wg.Wait()

		// If we've completed all tasks, we can exit
		doneCount := 0
		for _, v := range completed {
			if v {
				doneCount++
			}
		}
		if doneCount == len(tasks) {
			break
		}
	}
	return nil
}

// tryFinalAnswer attempts to get a final or partial answer if possible.
func tryFinalAnswer(
	c echo.Context,
	config *Config,
	conversation []Message,
	agentMemory map[string]string,
	args AgentArgs,
) string {
	finalPrompt := "We have possibly partial data. Summarize the best final answer to the user's query: \"" + args.Query + "\". " +
		"If data is incomplete, do your best."
	conv := append(conversation, Message{Role: "user", Content: finalPrompt})

	// Set the c body to conv
	c.Set("body", conv)

	if err := completionsHandler(c, config); err != nil {
		return "Error generating final answer: " + err.Error()
	}
	return "Final answer generated successfully."
}

// buildFinalizationPrompt returns a finalization question after an iteration’s tasks:
func buildFinalizationPrompt(query string, iterationLog []string, agentMemory map[string]string, toolsJSON string) string {
	sb := strings.Builder{}
	sb.WriteString("We ran tasks:\n")
	for _, line := range iterationLog {
		sb.WriteString(" - " + line + "\n")
	}
	sb.WriteString("\nCurrent memory:\n")
	for k, v := range agentMemory {
		sb.WriteString(fmt.Sprintf("  [%s]: %s\n", k, v))
	}
	sb.WriteString(fmt.Sprintf("\nUser query: '%s'\n", query))
	sb.WriteString(`Decide if we can produce a final answer. If yes, output it. If we still need more steps, propose them (but do not list "agent" as a tool).
If final, write "FINAL_ANSWER:" at the beginning. Otherwise mention next tasks or "call_tool: ..." 
`)
	return sb.String()
}

// isConclusion checks if the final answer is recognized as “finished”.
func isConclusion(answer string) bool {
	lower := strings.ToLower(strings.TrimSpace(answer))
	if strings.HasPrefix(lower, "final_answer") {
		return true
	}
	// Simple heuristic: if it doesn't mention "call_tool" or "further steps"
	if !strings.Contains(lower, "call_tool:") && !strings.Contains(lower, "next step") {
		// Possibly final
		return true
	}
	return false
}

// buildToolsListJSON can produce a static JSON list of the tools.
func buildToolsListJSON() string {
	// For brevity, we create a short list. In your real code, gather from the server’s registry or keep a static list.
	data := map[string]interface{}{
		"tools": []map[string]interface{}{
			{"name": "hello", "description": "Says hello to the provided name"},
			{"name": "calculate", "description": "Performs basic mathematical operations"},
			{"name": "time", "description": "Returns the current time"},
			{"name": "get_weather", "description": "Get the weather forecast"},
			{"name": "read_file", "description": "Reads the entire contents of a text file"},
			{"name": "write_file", "description": "Writes text content to a file"},
			{"name": "list_directory", "description": "Lists files and directories"},
			{"name": "create_directory", "description": "Creates a directory"},
			{"name": "move_file", "description": "Moves or renames a file/directory"},
			{"name": "git_init", "description": "Initializes a new Git repository"},
			{"name": "git_status", "description": "Shows Git status"},
			{"name": "git_add", "description": "Stages file changes"},
			{"name": "git_commit", "description": "Commits staged changes"},
			{"name": "git_pull", "description": "Pulls changes"},
			{"name": "git_push", "description": "Pushes commits"},
			{"name": "read_multiple_files", "description": "Reads the contents of multiple files"},
			{"name": "edit_file", "description": "Edits a file via search and replace"},
			{"name": "directory_tree", "description": "Recursively lists the directory structure"},
			{"name": "search_files", "description": "Searches for a text pattern in files"},
			{"name": "get_file_info", "description": "Returns metadata for a file or directory"},
			{"name": "list_allowed_directories", "description": "Lists directories allowed for access"},
			{"name": "delete_file", "description": "Deletes a file or directory"},
			{"name": "copy_file", "description": "Copies a file or directory"},
			{"name": "git_clone", "description": "Clones a remote Git repository"},
			{"name": "git_checkout", "description": "Switches or creates a new Git branch"},
			{"name": "git_diff", "description": "Shows Git diff between references"},
			{"name": "run_shell_command", "description": "Executes an arbitrary shell command"},
			{"name": "go_build", "description": "Builds a Go module"},
			{"name": "go_test", "description": "Runs Go tests"},
			{"name": "format_go_code", "description": "Formats Go code using go fmt"},
			{"name": "lint_code", "description": "Runs a code linter"},
			{"name": "web_search", "description": "Performs a web search"},
			{"name": "web_content", "description": "Fetches and extracts content from web URLs"},
		},
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
}

// processPreviousToolResults looks for placeholders like $TOOL_RESULT[i] and replaces
// them with the content from agentMemory["task_i"].
func processPreviousToolResults(argsJSON string, agentMemory map[string]string) (string, error) {
	pattern := regexp.MustCompile(`\$TOOL_RESULT\[(\d+)\]`)
	if !pattern.MatchString(argsJSON) {
		return argsJSON, nil
	}

	var generic interface{}
	if err := json.Unmarshal([]byte(argsJSON), &generic); err != nil {
		return "", fmt.Errorf("failed to parse JSON args for substitution: %v", err)
	}

	updated := walkAndReplace(generic, pattern, agentMemory)
	processed, err := json.Marshal(updated)
	if err != nil {
		return "", fmt.Errorf("failed to re-marshal updated JSON: %v", err)
	}
	return string(processed), nil
}

// walkAndReplace is a recursive function that replaces placeholders in string fields.
func walkAndReplace(value interface{}, pattern *regexp.Regexp, agentMemory map[string]string) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		for k, subVal := range v {
			v[k] = walkAndReplace(subVal, pattern, agentMemory)
		}
		return v
	case []interface{}:
		for i, subVal := range v {
			v[i] = walkAndReplace(subVal, pattern, agentMemory)
		}
		return v
	case string:
		return pattern.ReplaceAllStringFunc(v, func(m string) string {
			sub := pattern.FindStringSubmatch(m)
			if len(sub) < 2 {
				return m
			}
			idx := sub[1]
			key := "task_" + idx
			if res, ok := agentMemory[key]; ok {
				return res
			}
			return m
		})
	default:
		return v
	}
}

func callToolInServer(toolName, jsonArgs string) (string, error) {
	switch toolName {
	case "hello":
		return helloTool(HelloArgs{Name: jsonArgs})
	case "calculate":
		return calculateTool(CalculateArgs{Operation: "add", A: 1, B: 2})
	case "time":
		return timeTool(TimeArgs{Format: "RFC3339"})
	case "get_weather":
		return getWeatherTool(WeatherArgs{Latitude: 37.7749, Longitude: -122.4194})
	case "read_file":
		return readFileTool(ReadFileArgs{Path: jsonArgs})
	case "write_file":
		return writeFileTool(WriteFileArgs{Path: jsonArgs, Content: "Hello"})
	case "list_directory":
		return listDirectoryTool(ListDirectoryArgs{Path: jsonArgs})
	case "create_directory":
		return createDirectoryTool(CreateDirectoryArgs{Path: jsonArgs})
	case "move_file":
		return moveFileTool(MoveFileArgs{Source: jsonArgs, Destination: "dest"})
	case "git_init":
		return gitInitTool(GitInitArgs{Path: jsonArgs})
	case "git_status":
		return gitStatusTool(GitRepoArgs{Path: jsonArgs})
	case "git_add":
		return gitAddTool(GitAddArgs{Path: jsonArgs, FileList: []string{"file"}})
	case "git_commit":
		return gitCommitTool(GitCommitArgs{Path: jsonArgs, Message: "commit"})
	case "git_pull":
		return gitPullTool(GitRepoArgs{Path: jsonArgs})
	case "git_push":
		return gitPushTool(GitRepoArgs{Path: jsonArgs})
	// New Tools
	case "read_multiple_files":
		return readMultipleFilesTool(ReadMultipleFilesArgs{Paths: []string{jsonArgs}})
	case "edit_file":
		return editFileTool(EditFileArgs{Path: jsonArgs, Search: "old", Replace: "new"})
	case "directory_tree":
		return directoryTreeTool(DirectoryTreeArgs{Path: jsonArgs})
	case "search_files":
		return searchFilesTool(SearchFilesArgs{Path: jsonArgs, Pattern: "pattern"})
	case "get_file_info":
		return getFileInfoTool(GetFileInfoArgs{Path: jsonArgs})
	case "list_allowed_directories":
		return listAllowedDirectoriesTool(ListAllowedDirectoriesArgs{})
	case "delete_file":
		return deleteFileTool(DeleteFileArgs{Path: jsonArgs})
	case "copy_file":
		return copyFileTool(CopyFileArgs{Source: jsonArgs, Destination: "dest"})
	case "git_clone":
		return gitCloneTool(GitCloneArgs{RepoURL: jsonArgs, Path: "path"})
	case "git_checkout":
		return gitCheckoutTool(GitCheckoutArgs{Path: jsonArgs, Branch: "branch"})
	case "git_diff":
		return gitDiffTool(GitDiffArgs{Path: jsonArgs})
	case "run_shell_command":
		return runShellCommandTool(ShellCommandArgs{Command: []string{"ls"}, Dir: jsonArgs})
	case "go_build":
		return goBuildTool(GoBuildArgs{Path: jsonArgs})
	case "go_test":
		return goTestTool(GoTestArgs{Path: jsonArgs})
	case "format_go_code":
		return formatGoCodeTool(FormatGoCodeArgs{Path: jsonArgs})
	case "lint_code":
		return lintCodeTool(LintCodeArgs{Path: jsonArgs})
	case "web_search":
		return webSearchTool(WebSearchArgs{Query: jsonArgs})
	case "web_content":
		return webContentTool(WebContentArgs{URLs: []string{jsonArgs}})
	case "agent":
		// agent is handled separately below
		return "", fmt.Errorf("agent tool should be handled separately")
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}
