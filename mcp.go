package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"manifold/internal/web"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

type GenerateAndRunCodeArgs struct {
	// A text specification describing what we want the code to do.
	Spec string `json:"spec" jsonschema:"required,description=Description or purpose of the code to generate"`

	// The language to generate. Allowed values: "python", "go", "javascript"
	Language string `json:"language" jsonschema:"required,enum=python,enum=go,enum=javascript,description=Which language to generate and run"`

	// An optional list of dependencies (e.g. Python pip packages, Go modules, or npm packages).
	Dependencies []string `json:"dependencies,omitempty" jsonschema:"description=Optional list of dependencies for the chosen language"`
}

// RunMCP is the main entry point for running the MCP server with all registered tools.
// We have refactored the "agent" tool to function like a manager + team. The manager
// (LLM) plans multi-step workflows, which this code then executes step by step.
func RunMCP(config *Config) {

	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransport()

	// Create a new server with the transport
	server := mcp.NewServer(serverTransport)

	// --------------------------
	// Register Tools
	// --------------------------

	if err := server.RegisterTool(
		"generate_and_run_code",
		"Generates code in a specified language from a spec, runs it in a container, and returns code + execution result.",
		generateAndRunCodeHandler(config),
	); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("hello", "Says hello to the provided name", func(args HelloArgs) (*mcp.ToolResponse, error) {
		res, err := helloTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("calculate", "Performs basic mathematical operations", func(args CalculateArgs) (*mcp.ToolResponse, error) {
		res, err := calculateTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("time", "Returns the current time", func(args TimeArgs) (*mcp.ToolResponse, error) {
		res, err := timeTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("get_weather", "Get the weather forecast", func(args WeatherArgs) (*mcp.ToolResponse, error) {
		res, err := getWeatherTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	// File system tools
	if err := server.RegisterTool("read_file", "Reads the entire contents of a text file", func(args ReadFileArgs) (*mcp.ToolResponse, error) {
		res, err := readFileTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("write_file", "Writes text content to a file", func(args WriteFileArgs) (*mcp.ToolResponse, error) {
		res, err := writeFileTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("list_directory", "Lists files and directories", func(args ListDirectoryArgs) (*mcp.ToolResponse, error) {
		res, err := listDirectoryTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("create_directory", "Creates a directory", func(args CreateDirectoryArgs) (*mcp.ToolResponse, error) {
		res, err := createDirectoryTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("move_file", "Moves or renames a file/directory", func(args MoveFileArgs) (*mcp.ToolResponse, error) {
		res, err := moveFileTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	// Git tools
	if err := server.RegisterTool("git_init", "Initializes a new Git repository", func(args GitInitArgs) (*mcp.ToolResponse, error) {
		res, err := gitInitTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("git_status", "Shows Git status", func(args GitRepoArgs) (*mcp.ToolResponse, error) {
		res, err := gitStatusTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("git_add", "Stages file changes", func(args GitAddArgs) (*mcp.ToolResponse, error) {
		res, err := gitAddTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("git_commit", "Commits staged changes", func(args GitCommitArgs) (*mcp.ToolResponse, error) {
		res, err := gitCommitTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("git_pull", "Pulls changes", func(args GitRepoArgs) (*mcp.ToolResponse, error) {
		res, err := gitPullTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("git_push", "Pushes commits", func(args GitRepoArgs) (*mcp.ToolResponse, error) {
		res, err := gitPushTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	// Additional Tools
	if err := server.RegisterTool("read_multiple_files", "Reads the contents of multiple files", func(args ReadMultipleFilesArgs) (*mcp.ToolResponse, error) {
		res, err := readMultipleFilesTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("edit_file", "Edits a file via search and replace", func(args EditFileArgs) (*mcp.ToolResponse, error) {
		res, err := editFileTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("directory_tree", "Recursively lists the directory structure", func(args DirectoryTreeArgs) (*mcp.ToolResponse, error) {
		res, err := directoryTreeTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("search_files", "Searches for a text pattern in files", func(args SearchFilesArgs) (*mcp.ToolResponse, error) {
		res, err := searchFilesTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("get_file_info", "Returns metadata for a file or directory", func(args GetFileInfoArgs) (*mcp.ToolResponse, error) {
		res, err := getFileInfoTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("list_allowed_directories", "Lists directories allowed for access", func(args ListAllowedDirectoriesArgs) (*mcp.ToolResponse, error) {
		res, err := listAllowedDirectoriesTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("delete_file", "Deletes a file or directory", func(args DeleteFileArgs) (*mcp.ToolResponse, error) {
		res, err := deleteFileTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("copy_file", "Copies a file or directory", func(args CopyFileArgs) (*mcp.ToolResponse, error) {
		res, err := copyFileTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("git_clone", "Clones a remote Git repository", func(args GitCloneArgs) (*mcp.ToolResponse, error) {
		res, err := gitCloneTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("git_checkout", "Switches or creates a new Git branch", func(args GitCheckoutArgs) (*mcp.ToolResponse, error) {
		res, err := gitCheckoutTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("git_diff", "Shows Git diff between references", func(args GitDiffArgs) (*mcp.ToolResponse, error) {
		res, err := gitDiffTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("run_shell_command", "Executes an arbitrary shell command", func(args ShellCommandArgs) (*mcp.ToolResponse, error) {
		res, err := runShellCommandTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("go_build", "Builds a Go module", func(args GoBuildArgs) (*mcp.ToolResponse, error) {
		res, err := goBuildTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("go_test", "Runs Go tests", func(args GoTestArgs) (*mcp.ToolResponse, error) {
		res, err := goTestTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("format_go_code", "Formats Go code using go fmt", func(args FormatGoCodeArgs) (*mcp.ToolResponse, error) {
		res, err := formatGoCodeTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	if err := server.RegisterTool("lint_code", "Runs a code linter", func(args LintCodeArgs) (*mcp.ToolResponse, error) {
		res, err := lintCodeTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		panic(err)
	}

	// --------------------------
	// Register our new "agent" tool (manager + team approach)
	// --------------------------
	if err := server.RegisterTool("agent", "Agent that orchestrates a multi-step plan and executes it step by step.", agentHandler(config)); err != nil {
		panic(err)
	}

	// --------------------------
	// Start the MCP server
	// --------------------------
	if err := server.Serve(); err != nil {
		panic(err)
	}

	// Keep the server running
	select {}
}

// =====================
// Argument Types
// =====================

// HelloArgs example
type HelloArgs struct {
	Name string `json:"name" jsonschema:"required,description=The name to say hello to"`
}

type CalculateArgs struct {
	Operation string  `json:"operation" jsonschema:"required,enum=add,enum=subtract,enum=multiply,enum=divide,description=The mathematical operation to perform"`
	A         float64 `json:"a" jsonschema:"required,description=First number"`
	B         float64 `json:"b" jsonschema:"required,description=Second number"`
}

type TimeArgs struct {
	Format string `json:"format,omitempty" jsonschema:"description=Optional time format (default: RFC3339)"`
}

type WeatherArgs struct {
	Longitude float64 `json:"longitude" jsonschema:"required,description=Longitude"`
	Latitude  float64 `jsonschema:"required,description=Latitude" json:"latitude"`
}

// FS Tools
type ReadFileArgs struct {
	Path string `json:"path" jsonschema:"required,description=Path to the file to read"`
}
type WriteFileArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Path to the file to write"`
	Content string `json:"content" jsonschema:"required,description=Content to write into the file"`
}
type ListDirectoryArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path to list"`
}
type CreateDirectoryArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path to create"`
}
type MoveFileArgs struct {
	Source      string `json:"source" jsonschema:"required,description=Source path"`
	Destination string `json:"destination" jsonschema:"required,description=Destination path"`
}

// Git Tools
type GitInitArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory in which to initialize a Git repo"`
}
type GitRepoArgs struct {
	Path string `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
}
type GitAddArgs struct {
	Path     string   `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
	FileList []string `json:"fileList" jsonschema:"required,description=List of files to add"`
}
type GitCommitArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
	Message string `json:"message" jsonschema:"required,description=Commit message"`
}

// Agent
type AgentArgs struct {
	Query    string `json:"query" jsonschema:"required,description=User's query"`
	MaxCalls int    `json:"maxCalls" jsonschema:"required,description=Maximum LLM calls allowed"`
}

// ChatCompletion structs for calling OpenAI
type ChatCompletionRequest struct {
	Model       string              `json:"model"`
	Messages    []ChatCompletionMsg `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
}
type ChatCompletionMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Additional Tool Args
type ReadMultipleFilesArgs struct {
	Paths []string `json:"paths" jsonschema:"required,description=List of file paths to read"`
}
type EditFileArgs struct {
	Path         string `json:"path" jsonschema:"required,description=File path"`
	Search       string `json:"search,omitempty" jsonschema:"description=Search text"`
	Replace      string `json:"replace,omitempty" jsonschema:"description=Replace text"`
	PatchContent string `json:"patchContent,omitempty" jsonschema:"description=Unified diff patch"`
}
type DirectoryTreeArgs struct {
	Path     string `json:"path" jsonschema:"required,description=Root directory"`
	MaxDepth int    `json:"maxDepth,omitempty" jsonschema:"description=Depth limit"`
}
type SearchFilesArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Base path"`
	Pattern string `json:"pattern" jsonschema:"required,description=Text or regex pattern"`
}
type GetFileInfoArgs struct {
	Path string `json:"path" jsonschema:"required,description=Path to file or directory"`
}
type ListAllowedDirectoriesArgs struct{}
type DeleteFileArgs struct {
	Path      string `json:"path" jsonschema:"required,description=Path to delete"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"description=Delete recursively"`
}
type CopyFileArgs struct {
	Source      string `json:"source" jsonschema:"required"`
	Destination string `json:"destination" jsonschema:"required"`
	Recursive   bool   `json:"recursive,omitempty" jsonschema:"description=Copy directories recursively"`
}
type GitCloneArgs struct {
	RepoURL string `json:"repoUrl" jsonschema:"required"`
	Path    string `json:"path" jsonschema:"required"`
}
type GitCheckoutArgs struct {
	Path      string `json:"path" jsonschema:"required"`
	Branch    string `json:"branch" jsonschema:"required"`
	CreateNew bool   `json:"createNew,omitempty" jsonschema:"description=Create new branch?"`
}
type GitDiffArgs struct {
	Path    string `json:"path" jsonschema:"required"`
	FromRef string `json:"fromRef,omitempty"`
	ToRef   string `json:"toRef,omitempty"`
}
type ShellCommandArgs struct {
	Command []string `json:"command" jsonschema:"required"`
	Dir     string   `json:"dir" jsonschema:"required"`
}
type GoBuildArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of Go module"`
}
type GoTestArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of Go tests"`
}
type FormatGoCodeArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of Go code to format"`
}
type LintCodeArgs struct {
	Path       string `json:"path" jsonschema:"required,description=Dir or file to lint"`
	LinterName string `json:"linterName,omitempty" jsonschema:"description=Optional linter name"`
}
type WebSearchArgs struct {
	Query         string `json:"query" jsonschema:"required"`
	ResultSize    int    `json:"result_size,omitempty"`
	SearchBackend string `json:"search_backend,omitempty"`
	SxngURL       string `json:"sxng_url,omitempty"`
}
type WebContentArgs struct {
	URLs []string `json:"urls" jsonschema:"required,description=List of URLs"`
}

// callOpenAI is a helper that calls the completions endpoint
func callOpenAI(config *Config, messages []ChatCompletionMsg) (string, error) {
	requestBody := ChatCompletionRequest{
		Model:       config.Completions.CompletionsModel,
		Messages:    messages,
		MaxTokens:   4096,
		Temperature: 0.3,
	}
	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequest("POST", config.Completions.DefaultHost, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.Completions.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai API error, status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var completionResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		return "", fmt.Errorf("failed to parse openai response: %w", err)
	}
	if len(completionResp.Choices) == 0 {
		return "", fmt.Errorf("no completion returned by OpenAI")
	}
	return completionResp.Choices[0].Message.Content, nil
}

// agentHandler is our manager+team approach. The user calls "agent" with
// { query: "...", maxCalls: N }, and we do the following:
//
// 1) Ask the manager LLM for a multi-step plan of tool calls (in JSON).
// 2) Execute each step in code, saving outputs in a tool history.
// 3) Validate the result of each step and decide whether to continue or adjust.
// 4) Ask the manager LLM for a final summary. Return that summary to the user.
func agentHandler(config *Config) func(args AgentArgs) (*mcp.ToolResponse, error) {
	return func(args AgentArgs) (*mcp.ToolResponse, error) {
		plan, err := producePlan(config, args.Query)
		if err != nil {
			return nil, fmt.Errorf("error producing plan: %w", err)
		}

		log.Default().Printf("Plan: %s", string(args.Query))

		// Execute the plan
		var history []StepResult
		for idx, step := range plan.Steps {
			res, toolErr := callToolInServer(config, step.ToolName, step.ArgsRaw)
			sr := StepResult{
				Index:    idx,
				ToolName: step.ToolName,
				Args:     step.ArgsRaw,
				Output:   res,
				Error:    "",
			}
			if toolErr != nil {
				sr.Error = toolErr.Error()
				log.Printf("Step %d result added to history: %+v", idx, sr)
				// If a step fails, we short-circuit
				return mcp.NewToolResponse(mcp.NewTextContent(
					fmt.Sprintf("Plan step %d (%s) failed: %v", idx+1, step.ToolName, toolErr),
				)), nil
			}
			history = append(history, sr)

			// Validate this step's result before proceeding
			if idx < len(plan.Steps)-1 {
				validation, validationErr := validateStepResult(config, args.Query, history, plan, idx+1)
				if validationErr != nil {
					log.Printf("Step validation error: %v", validationErr)
					return mcp.NewToolResponse(mcp.NewTextContent(
						fmt.Sprintf("Step validation after step %d (%s) failed: %v",
							idx+1, step.ToolName, validationErr),
					)), nil
				}

				// If validation suggests we should change course
				if !validation.ContinueWithPlan {
					log.Printf("Validation suggests changing course after step %d", idx+1)

					if validation.AdditionalSteps != nil && len(validation.AdditionalSteps) > 0 {
						// Add the new steps to our plan
						insertPoint := idx + 1
						newSteps := make([]PlanStep, 0, len(plan.Steps)+len(validation.AdditionalSteps))
						newSteps = append(newSteps, plan.Steps[:insertPoint]...)
						newSteps = append(newSteps, validation.AdditionalSteps...)
						if insertPoint < len(plan.Steps) {
							newSteps = append(newSteps, plan.Steps[insertPoint:]...)
						}
						plan.Steps = newSteps
						log.Printf("Plan modified with %d additional steps", len(validation.AdditionalSteps))
					}

					if validation.ModifiedStep != nil {
						// Next step will be this modified step instead
						plan.Steps[idx+1] = *validation.ModifiedStep
						log.Printf("Next step modified based on validation")
					}

					if validation.TerminatePlan {
						log.Printf("Plan terminated early based on validation after step %d", idx+1)
						break
					}
				}
			}
		}

		// Summarize final answer
		final, err := produceFinalAnswer(config, args.Query, history)
		if err != nil {
			return nil, fmt.Errorf("error producing final answer: %w", err)
		}
		return mcp.NewToolResponse(mcp.NewTextContent(final)), nil
	}
}

// producePlan calls the manager LLM to get a multi-step plan in JSON
func producePlan(config *Config, userQuery string) (*Plan, error) {
	// List of all available tools
	availableTools := []string{
		"hello", "calculate", "time", "get_weather",
		"read_file", "write_file", "list_directory", "create_directory", "move_file",
		"git_init", "git_status", "git_add", "git_commit", "git_pull", "git_push",
		"read_multiple_files", "edit_file", "directory_tree", "search_files",
		"get_file_info", "list_allowed_directories", "delete_file", "copy_file",
		"git_clone", "git_checkout", "git_diff", "run_shell_command",
		"go_build", "go_test", "format_go_code", "lint_code", "web_search", "web_content",
		"generate_and_run_code",
	}

	availableToolsStr := strings.Join(availableTools, ", ")

	// Prepend the available tools to the query with two line breaks
	queryWithTools := fmt.Sprintf("Available tools: %s\n\n%s", availableToolsStr, userQuery)

	// Get detailed tool documentation
	toolDocs := generateToolDocumentation()

	systemPrompt := fmt.Sprintf(`You are a manager that plans multi-step tool usage. Output only the JSON plan.

%s

Only use tools from the provided in the previous list of available tools.`, toolDocs)

	prompt := fmt.Sprintf(`
You are a project manager. The user query is: %q

IMPORTANT: ONLY use the tools mentioned in the system prompt.

You MUST break down the query into multiple smaller tasks and decide if a tool is available that will satisfy the task.
If a tool is not available, you must end the conversation and inform the user that you cannot help.
If you need to use multiple tools, you must break the query into multiple steps and provide a plan for each step.
The query has more importance than your opinions, so if the query explicitly states to use a tool, you must use it.

Produce a JSON plan of steps in the format:
{
  "steps": [
    {
      "toolName": "<some_tool_name>",
      "argsRaw": "<JSON string for the tool arguments>"
    },
    ...
  ]
}

DO NOT include extraneous commentary. Just valid JSON with "toolName" and "argsRaw"
`, queryWithTools)

	messages := []ChatCompletionMsg{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	llmOutput, err := callCompletionsEndpoint(config, messages)
	if err != nil {
		return nil, err
	}

	var plan Plan
	if err := json.Unmarshal([]byte(llmOutput), &plan); err != nil {
		return nil, fmt.Errorf("plan parse error: %w\nLLM output was: %s", err, llmOutput)
	}

	// Log the plan
	log.Printf("Plan produced: %s", string(llmOutput))

	// Validate that all tools exist
	for i, step := range plan.Steps {
		toolExists := false
		for _, tool := range availableTools {
			if step.ToolName == tool {
				toolExists = true
				break
			}
		}
		if !toolExists {
			return nil, fmt.Errorf("step %d uses unknown tool: %s", i+1, step.ToolName)
		}
	}

	return &plan, nil
}

// produceFinalAnswer calls the manager again, providing the step results, and asks for a final summary
func produceFinalAnswer(config *Config, userQuery string, history []StepResult) (string, error) {

	// Summarize the steps
	var sb strings.Builder
	for _, h := range history {
		sb.WriteString(fmt.Sprintf("Step %d: Tool=%s\n", h.Index+1, h.ToolName))
		if h.Error != "" {
			sb.WriteString(fmt.Sprintf("Error: %s\n", h.Error))
		} else {
			sb.WriteString(fmt.Sprintf("Output:\n%s\n", h.Output))
		}
		sb.WriteString("\n")
	}

	systemPrompt := "You are a manager finalizing the user's answer. Be concise but thorough"

	prompt := fmt.Sprintf(`
User query: %q

We executed the following steps and got these results:
%s

Now provide a final human-readable answer to the user, summarizing or directly providing relevant tool outputs if the user requested them.
`, userQuery, sb.String())

	messages := []ChatCompletionMsg{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	answer, err := callCompletionsEndpoint(config, messages)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(answer), nil
}

// -------------------
// Data Structures
// -------------------

// Plan is the top-level structure we expect from the manager's plan JSON
type Plan struct {
	Steps []PlanStep `json:"steps"`
}

type PlanStep struct {
	ToolName string `json:"toolName"`
	ArgsRaw  string `json:"argsRaw"`
}

// StepResult records each tool call outcome
type StepResult struct {
	Index    int
	ToolName string
	Args     string
	Output   string
	Error    string
}

// StepValidationResult contains the validation outcome from the manager
type StepValidationResult struct {
	ContinueWithPlan bool       // Whether to continue with the original plan
	TerminatePlan    bool       // Whether to terminate the plan early
	ModifiedStep     *PlanStep  // Optional modified version of the next step
	AdditionalSteps  []PlanStep // Optional additional steps to insert
}

// validateStepResult asks the manager LLM to validate a step's result
// and decide whether to continue with the plan or make adjustments
// validateStepResult asks the manager LLM to validate a step's result
// and decide whether to continue with the plan or make adjustments
func validateStepResult(config *Config, userQuery string, history []StepResult, plan *Plan, nextStepIndex int) (*StepValidationResult, error) {
	// Summarize the steps executed so far (keep existing code here)
	var executedSteps strings.Builder
	// ... (code to build executedSteps remains the same) ...
	for _, h := range history {
		executedSteps.WriteString(fmt.Sprintf("Step %d: Tool=%s\n", h.Index+1, h.ToolName))
		// Limit output in prompt for brevity
		outputSummary := h.Output
		maxLen := 500 // Limit output length in prompt
		if len(outputSummary) > maxLen {
			outputSummary = outputSummary[:maxLen] + "... (truncated)"
		}
		executedSteps.WriteString(fmt.Sprintf("Args: %s\n", h.Args)) // Keep args concise if possible too
		if h.Error != "" {
			executedSteps.WriteString(fmt.Sprintf("Error: %s\n", h.Error))
		} else {
			executedSteps.WriteString(fmt.Sprintf("Output:\n%s\n", outputSummary)) // Use summary
		}
		executedSteps.WriteString("\n")
	}

	// Describe the next planned step (keep existing code here)
	var nextStepDesc string
	// ... (code to build nextStepDesc remains the same) ...
	if nextStepIndex < len(plan.Steps) {
		nextStep := plan.Steps[nextStepIndex]
		nextStepDesc = fmt.Sprintf("The next planned step is Step %d: Tool='%s', Args='%s'", nextStepIndex+1, nextStep.ToolName, nextStep.ArgsRaw)
	} else {
		nextStepDesc = "This was the last step in the current plan."
	}

	// ***** MODIFIED PROMPT BELOW *****

	// System Prompt focuses on the goal and JSON format
	systemPrompt := `You are an execution validator AI. Review the history of executed steps and the next planned step in light of the **overall goal** stated in the original user query.
Your primary task is to determine if the plan is still on track to meet the *entire* original request.
Output *only* a valid JSON object with the following structure, no commentary:
{
  "ContinueWithPlan": boolean,
  "TerminatePlan": boolean,
  "ModifiedStep": { "toolName": "...", "argsRaw": "{...}" } | null,
  "AdditionalSteps": [ { "toolName": "...", "argsRaw": "{...}" }, ... ] | []
}`

	// User Prompt emphasizes the conditions for termination and modification
	prompt := fmt.Sprintf(`
Original User Query: "%s"

Execution History:
%s
%s

Based on the history and the **entire original user query**, evaluate the plan's progress.

**CRITICAL**: Set `+"`TerminatePlan: true`"+` **ONLY** if one of the following conditions is met:
1.  The **entire original user query** has been fully satisfied by the execution history provided.
2.  A step failed critically and the plan cannot recover or proceed meaningfully towards the original goal.
3.  The current plan seems fundamentally flawed and cannot achieve the original goal even with modifications.

If `+"`TerminatePlan`"+` is `+"`false`"+`, then decide:
- Is the `+"`Next Planned Step`"+` still appropriate and sufficient to progress towards the goal? If YES, set `+"`ContinueWithPlan: true`"+` and leave `+"`ModifiedStep`"+`/`+"`AdditionalSteps`"+` as null/empty.
- Does the `+"`Next Planned Step`"+` need different arguments based on the history? If YES, set `+"`ContinueWithPlan: false`"+`, provide the corrected step in `+"`ModifiedStep`"+`.
- Are *new* steps needed *before* the `+"`Next Planned Step`"+` to correctly proceed? If YES, set `+"`ContinueWithPlan: false`"+`, provide the new steps in `+"`AdditionalSteps`"+`. `+"`ModifiedStep`"+` might be null if the original next step is still valid *after* the additions.

Respond **only** with the JSON object.
`, userQuery, executedSteps.String(), nextStepDesc) // Pass the summarized history

	messages := []ChatCompletionMsg{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	// Make the API call (Request JSON if possible)
	// ***** IMPORTANT: Ensure callCompletionsEndpoint requests JSON format if supported *****
	// You might need to modify callCompletionsEndpoint or pass a flag like:
	// validationOutput, err := callCompletionsEndpoint(config, messages, true)
	// Assuming your callCompletionsEndpoint handles JSON mode correctly:
	validationOutput, err := callCompletionsEndpoint(config, messages /*, true */) // Add JSON flag if needed
	if err != nil {
		// Log the raw output attempt before parsing error
		log.Printf("Raw validation output on API error: %s", validationOutput)
		return nil, fmt.Errorf("validation API call error: %w", err)
	}

	var validationResult StepValidationResult
	// Clean potential markdown fences if the LLM adds them despite instructions
	validationOutput = strings.TrimSpace(validationOutput)
	validationOutput = strings.TrimPrefix(validationOutput, "```json")
	validationOutput = strings.TrimSuffix(validationOutput, "```")
	validationOutput = strings.TrimSpace(validationOutput)

	if err := json.Unmarshal([]byte(validationOutput), &validationResult); err != nil {
		// Log the output that failed to parse
		log.Printf("Raw validation output on parse error: %s", validationOutput)
		return nil, fmt.Errorf("validation result parse error: %w\nLLM output was: %s", err, validationOutput)
	}

	// Log the validation decision (keep existing code here)
	log.Printf("Step validation result: continue=%v, terminate=%v, modifiedStep=%v, additionalSteps=%d",
		validationResult.ContinueWithPlan,
		validationResult.TerminatePlan,
		validationResult.ModifiedStep != nil,
		len(validationResult.AdditionalSteps))

	return &validationResult, nil
}

// --------------------------
// callToolInServer dispatches a tool call.
// --------------------------
func generateAndRunCodeHandler(config *Config) func(args GenerateAndRunCodeArgs) (*mcp.ToolResponse, error) {
	return func(args GenerateAndRunCodeArgs) (*mcp.ToolResponse, error) {
		// 1) Ask LLM to generate the code
		generatedCode, err := produceLanguageCode(config, args.Language, args.Spec)
		if err != nil {
			return nil, fmt.Errorf("error generating code: %w", err)
		}

		// 2) Execute the code in Docker
		execOutput, execErr := runCodeInContainer(args.Language, generatedCode, args.Dependencies)
		if execErr != nil {
			return nil, fmt.Errorf("error running code: %w", execErr)
		}

		// 3) Return only the execution results
		return mcp.NewToolResponse(mcp.NewTextContent(execOutput)), nil
	}
}

func callToolInServer(config *Config, toolName, jsonArgs string) (string, error) {
	switch toolName {
	case "generate_and_run_code":
		var args GenerateAndRunCodeArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}

		responseStr, err := generateAndRunCodeHandler(config)(args)
		if err != nil {
			return "", err
		}

		// Convert the response to a string
		responseBytes, err := json.Marshal(responseStr)
		if err != nil {
			return "", err
		}

		log.Printf("Response from generate_and_run_code: %s", string(responseBytes))

		return string(responseBytes), nil
	case "hello":
		var args HelloArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return helloTool(args)
	case "calculate":
		var args CalculateArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return calculateTool(args)
	case "time":
		var args TimeArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return timeTool(args)
	case "get_weather":
		var args WeatherArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return getWeatherTool(args)
	case "read_file":
		var args ReadFileArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return readFileTool(args)
	case "write_file":
		var args WriteFileArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return writeFileTool(args)
	case "list_directory":
		var args ListDirectoryArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return listDirectoryTool(args)
	case "create_directory":
		var args CreateDirectoryArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return createDirectoryTool(args)
	case "move_file":
		var args MoveFileArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return moveFileTool(args)
	case "git_init":
		var args GitInitArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitInitTool(args)
	case "git_status":
		var args GitRepoArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitStatusTool(args)
	case "git_add":
		var args GitAddArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitAddTool(args)
	case "git_commit":
		var args GitCommitArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitCommitTool(args)
	case "git_pull":
		var args GitRepoArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitPullTool(args)
	case "git_push":
		var args GitRepoArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitPushTool(args)
	case "read_multiple_files":
		var args ReadMultipleFilesArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return readMultipleFilesTool(args)
	case "edit_file":
		var args EditFileArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return editFileTool(args)
	case "directory_tree":
		var args DirectoryTreeArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return directoryTreeTool(args)
	case "search_files":
		var args SearchFilesArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return searchFilesTool(args)
	case "get_file_info":
		var args GetFileInfoArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return getFileInfoTool(args)
	case "list_allowed_directories":
		var args ListAllowedDirectoriesArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return listAllowedDirectoriesTool(args)
	case "delete_file":
		var args DeleteFileArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return deleteFileTool(args)
	case "copy_file":
		var args CopyFileArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return copyFileTool(args)
	case "git_clone":
		var args GitCloneArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitCloneTool(args)
	case "git_checkout":
		var args GitCheckoutArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitCheckoutTool(args)
	case "git_diff":
		var args GitDiffArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return gitDiffTool(args)
	case "run_shell_command":
		var args ShellCommandArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return runShellCommandTool(args)
	case "go_build":
		var args GoBuildArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return goBuildTool(args)
	case "go_test":
		var args GoTestArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return goTestTool(args)
	case "format_go_code":
		var args FormatGoCodeArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return formatGoCodeTool(args)
	case "lint_code":
		var args LintCodeArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return lintCodeTool(args)
	case "web_search":
		var args WebSearchArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return webSearchTool(args)
	case "web_content":
		var args WebContentArgs
		if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
			return "", err
		}
		return webContentTool(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// --------------
// Original tool helpers below
// (these remain the same as your existing code, e.g. helloTool, readFileTool, etc.)
// --------------

// hello tool
func helloTool(args HelloArgs) (string, error) {
	return fmt.Sprintf("Hello, %s!", args.Name), nil
}

// calculate tool
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

// time tool
func timeTool(args TimeArgs) (string, error) {
	format := time.RFC3339
	if args.Format != "" {
		format = args.Format
	}
	return time.Now().Format(format), nil
}

// get_weather tool
func getWeatherTool(args WeatherArgs) (string, error) {
	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,wind_speed_10m&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m",
		args.Latitude, args.Longitude)
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

// read_file tool
func readFileTool(args ReadFileArgs) (string, error) {
	bytes, err := ioutil.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(bytes), nil
}

// write_file tool
func writeFileTool(args WriteFileArgs) (string, error) {
	err := ioutil.WriteFile(args.Path, []byte(args.Content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("Wrote file: %s", args.Path), nil
}

// list_directory tool
func listDirectoryTool(args ListDirectoryArgs) (string, error) {
	entries, err := ioutil.ReadDir(args.Path)
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

// create_directory tool
func createDirectoryTool(args CreateDirectoryArgs) (string, error) {
	err := os.MkdirAll(args.Path, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	return fmt.Sprintf("Directory created: %s", args.Path), nil
}

// move_file tool
func moveFileTool(args MoveFileArgs) (string, error) {
	if err := os.Rename(args.Source, args.Destination); err != nil {
		return "", fmt.Errorf("failed to move file: %w", err)
	}
	return fmt.Sprintf("Moved '%s' to '%s'", args.Source, args.Destination), nil
}

// git_init tool
func gitInitTool(args GitInitArgs) (string, error) {
	info, err := os.Stat(args.Path)
	if err != nil {
		return "", fmt.Errorf("cannot access path %q: %w", args.Path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", args.Path)
	}
	output, err := runGitCommand(args.Path, "init")
	if err != nil {
		return "", err
	}
	return output, nil
}

// git_status tool
func gitStatusTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return runGitCommand(args.Path, "status", "--short", "--branch")
}

// git_add tool
func gitAddTool(args GitAddArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	if len(args.FileList) == 0 {
		args.FileList = []string{"."}
	}
	fullArgs := append([]string{"add"}, args.FileList...)
	return runGitCommand(args.Path, fullArgs...)
}

// git_commit tool
func gitCommitTool(args GitCommitArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Message) == "" {
		return "", fmt.Errorf("commit message cannot be empty")
	}
	return runGitCommand(args.Path, "commit", "-m", args.Message)
}

// git_pull tool
func gitPullTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return runGitCommand(args.Path, "pull", "--rebase")
}

// git_push tool
func gitPushTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return runGitCommand(args.Path, "push")
}

// read_multiple_files tool
func readMultipleFilesTool(args ReadMultipleFilesArgs) (string, error) {
	var sb strings.Builder
	for _, path := range args.Paths {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			sb.WriteString(fmt.Sprintf("Error reading %s: %v\n", path, err))
			continue
		}
		sb.WriteString(fmt.Sprintf("File: %s\n%s\n---\n", path, string(data)))
	}
	return sb.String(), nil
}

// edit_file tool
func editFileTool(args EditFileArgs) (string, error) {
	if args.PatchContent != "" {
		return "", fmt.Errorf("patchContent not supported in this implementation")
	}
	if args.Search == "" {
		return "", fmt.Errorf("must provide a search string for edit_file")
	}
	original, err := ioutil.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	edited := strings.ReplaceAll(string(original), args.Search, args.Replace)
	if err := ioutil.WriteFile(args.Path, []byte(edited), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("Edited file: %s", args.Path), nil
}

// directory_tree tool
func directoryTreeTool(args DirectoryTreeArgs) (string, error) {
	tree, err := buildDirectoryTree(args.Path, "", args.MaxDepth, 1)
	if err != nil {
		return "", err
	}
	return tree, nil
}

// search_files tool
func searchFilesTool(args SearchFilesArgs) (string, error) {
	matches, err := searchFilesRecursive(args.Path, args.Pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "No files found matching the pattern.", nil
	}
	return "Files matching:\n" + strings.Join(matches, "\n"), nil
}

// get_file_info tool
func getFileInfoTool(args GetFileInfoArgs) (string, error) {
	info, err := os.Stat(args.Path)
	if err != nil {
		return "", fmt.Errorf("stat error: %w", err)
	}
	return fmt.Sprintf("Name: %s\nSize: %d bytes\nMode: %s\nModified: %s\nIsDir: %t",
		info.Name(), info.Size(), info.Mode().String(), info.ModTime().Format(time.RFC3339), info.IsDir()), nil
}

// list_allowed_directories tool
func listAllowedDirectoriesTool(args ListAllowedDirectoriesArgs) (string, error) {
	// For demonstration
	return "All directories are allowed.", nil
}

// delete_file tool
func deleteFileTool(args DeleteFileArgs) (string, error) {
	if args.Recursive {
		if err := os.RemoveAll(args.Path); err != nil {
			return "", fmt.Errorf("removeAll error: %w", err)
		}
	} else {
		if err := os.Remove(args.Path); err != nil {
			return "", fmt.Errorf("remove error: %w", err)
		}
	}
	return fmt.Sprintf("Deleted: %s", args.Path), nil
}

// copy_file tool
func copyFileTool(args CopyFileArgs) (string, error) {
	info, err := os.Stat(args.Source)
	if err != nil {
		return "", fmt.Errorf("access source error: %w", err)
	}
	if info.IsDir() {
		if !args.Recursive {
			return "", fmt.Errorf("source is directory, need recursive=true to copy directories")
		}
		if err := copyDir(args.Source, args.Destination); err != nil {
			return "", fmt.Errorf("copyDir error: %w", err)
		}
	} else {
		if err := copyFile(args.Source, args.Destination); err != nil {
			return "", fmt.Errorf("copyFile error: %w", err)
		}
	}
	return fmt.Sprintf("Copied %s to %s", args.Source, args.Destination), nil
}

// git_clone tool
func gitCloneTool(args GitCloneArgs) (string, error) {
	cmd := exec.Command("git", "clone", args.RepoURL, args.Path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

// git_checkout tool
func gitCheckoutTool(args GitCheckoutArgs) (string, error) {
	var cmdArgs []string
	if args.CreateNew {
		cmdArgs = []string{"checkout", "-b", args.Branch}
	} else {
		cmdArgs = []string{"checkout", args.Branch}
	}
	return runGitCommand(args.Path, cmdArgs...)
}

// git_diff tool
func gitDiffTool(args GitDiffArgs) (string, error) {
	cmdArgs := []string{"diff"}
	if args.FromRef != "" && args.ToRef != "" {
		cmdArgs = append(cmdArgs, args.FromRef, args.ToRef)
	}
	return runGitCommand(args.Path, cmdArgs...)
}

// run_shell_command tool
func runShellCommandTool(args ShellCommandArgs) (string, error) {
	if len(args.Command) == 0 {
		return "", fmt.Errorf("empty command array")
	}
	cmd := exec.Command(args.Command[0], args.Command[1:]...)
	cmd.Dir = args.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("shell command error: %w\nOutput: %s", err, output)
	}
	return string(output), nil
}

// go_build tool
func goBuildTool(args GoBuildArgs) (string, error) {
	cmd := exec.Command("go", "build")
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build error: %w\nOutput: %s", err, output)
	}
	return string(output), nil
}

// go_test tool
func goTestTool(args GoTestArgs) (string, error) {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go test error: %w\nOutput: %s", err, output)
	}
	return string(output), nil
}

// format_go_code tool
func formatGoCodeTool(args FormatGoCodeArgs) (string, error) {
	cmd := exec.Command("go", "fmt", "./...")
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go fmt error: %w\nOutput: %s", err, output)
	}
	return string(output), nil
}

// lint_code tool
func lintCodeTool(args LintCodeArgs) (string, error) {
	var cmd *exec.Cmd
	if args.LinterName != "" {
		cmd = exec.Command(args.LinterName, "run")
	} else {
		cmd = exec.Command("golangci-lint", "run")
	}
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("lint command error: %w\nOutput: %s", err, output)
	}
	return string(output), nil
}

// web_search tool
func webSearchTool(args WebSearchArgs) (string, error) {
	results := web.SearchDDG(args.Query)
	if results == nil {
		return "", fmt.Errorf("web search error: no results")
	}
	return strings.Join(results, "\n"), nil
}

// web_content tool
func webContentTool(args WebContentArgs) (string, error) {
	if len(args.URLs) == 0 {
		return "", fmt.Errorf("no URLs provided")
	}
	var content strings.Builder
	for _, urlStr := range args.URLs {
		urlStr = strings.TrimSpace(urlStr)
		if urlStr == "" {
			continue
		}
		apiURL := "http://localhost:8080/api/web-content?urls=" + urlStr
		resp, err := http.Get(apiURL)
		if err != nil {
			return "", fmt.Errorf("web content request: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			// ignoring details
		}
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("read response: %w", err)
		}
		content.WriteString(string(bodyBytes))
		content.WriteString("\n")
	}
	return content.String(), nil
}

// runGitCommand + checkGitRepo + copyFile + copyDir + buildDirectoryTree + searchFilesRecursive remain the same
// as in your existing code.  End of file.
func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

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

func copyDir(src string, dst string) error {
	entries, err := ioutil.ReadDir(src)
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

func buildDirectoryTree(path string, prefix string, maxDepth int, currentDepth int) (string, error) {
	if maxDepth > 0 && currentDepth > maxDepth {
		return "", nil
	}
	entries, err := ioutil.ReadDir(path)
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

func searchFilesRecursive(root, pattern string) ([]string, error) {
	var matches []string
	entries, err := ioutil.ReadDir(root)
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
			data, err := ioutil.ReadFile(fullPath)
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

// -------------------------
// callOpenAI is a helper for OpenAI API compatible completions endpoint
// This function will invoke the default completions endpoint configured
// in the config file.
// -------------------------
func callCompletionsEndpoint(config *Config, messages []ChatCompletionMsg) (string, error) {
	requestBody := ChatCompletionRequest{
		Model:       config.Completions.CompletionsModel,
		Messages:    messages,
		MaxTokens:   16384,
		Temperature: 0.3,
	}

	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := config.Completions.DefaultHost
	apiKey := config.Completions.APIKey

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai API error, status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var completionResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		return "", fmt.Errorf("failed to parse openai response: %w", err)
	}

	if len(completionResp.Choices) == 0 {
		return "", fmt.Errorf("no completion returned by OpenAI")
	}

	answer := completionResp.Choices[0].Message.Content
	return answer, nil
}

// generateToolDocumentation returns a string descri	bing all available tools and their parameters
func generateToolDocumentation() string {
	// Map of tool names to their parameter documentation
	toolDocs := map[string]string{
		"generate_and_run_code":    "{ \"spec\": \"string\", \"language\": \"string\" (python|go|javascript), \"dependencies\": [\"string\", ...] } - Generate code from the spec in the chosen language, run it, and return the code plus execution result",
		"hello":                    "{ \"name\": \"string\" } - The name to say hello to",
		"calculate":                "{ \"operation\": \"string\" (add|subtract|multiply|divide), \"a\": number, \"b\": number } - Perform basic math",
		"time":                     "{ \"format\": \"string\" (optional) } - Get current time, optionally with specified format",
		"get_weather":              "{ \"longitude\": number, \"latitude\": number } - Get weather forecast",
		"read_file":                "{ \"path\": \"string\" } - Path to the file to read",
		"write_file":               "{ \"path\": \"string\", \"content\": \"string\" } - Write content to specified file",
		"list_directory":           "{ \"path\": \"string\" } - List files and directories at path",
		"create_directory":         "{ \"path\": \"string\" } - Create a directory at path",
		"move_file":                "{ \"source\": \"string\", \"destination\": \"string\" } - Move or rename a file/directory",
		"git_init":                 "{ \"path\": \"string\" } - Initialize a new Git repository",
		"git_status":               "{ \"path\": \"string\" } - Show Git status in repository at path",
		"git_add":                  "{ \"path\": \"string\", \"fileList\": [\"string\", ...] } - Stage files for commit",
		"git_commit":               "{ \"path\": \"string\", \"message\": \"string\" } - Commit staged changes",
		"git_pull":                 "{ \"path\": \"string\" } - Pull changes from remote repository",
		"git_push":                 "{ \"path\": \"string\" } - Push commits to remote repository",
		"read_multiple_files":      "{ \"paths\": [\"string\", ...] } - Read multiple files at once",
		"edit_file":                "{ \"path\": \"string\", \"search\": \"string\", \"replace\": \"string\" } - Replace text in a file",
		"directory_tree":           "{ \"path\": \"string\", \"maxDepth\": number (optional) } - Show directory structure",
		"search_files":             "{ \"path\": \"string\", \"pattern\": \"string\" } - Search for text pattern in files",
		"get_file_info":            "{ \"path\": \"string\" } - Get file/directory metadata",
		"list_allowed_directories": "{} - List directories allowed for access",
		"delete_file":              "{ \"path\": \"string\", \"recursive\": boolean (optional) } - Delete a file or directory",
		"copy_file":                "{ \"source\": \"string\", \"destination\": \"string\", \"recursive\": boolean (optional) } - Copy a file or directory",
		"git_clone":                "{ \"repoUrl\": \"string\", \"path\": \"string\" } - Clone a Git repository",
		"git_checkout":             "{ \"path\": \"string\", \"branch\": \"string\", \"createNew\": boolean (optional) } - Checkout or create a Git branch",
		"git_diff":                 "{ \"path\": \"string\", \"fromRef\": \"string\" (optional), \"toRef\": \"string\" (optional) } - Show Git diff",
		"run_shell_command":        "{ \"command\": [\"string\", ...], \"dir\": \"string\" } - Execute a shell command",
		"go_build":                 "{ \"path\": \"string\" } - Build a Go module",
		"go_test":                  "{ \"path\": \"string\" } - Run Go tests",
		"format_go_code":           "{ \"path\": \"string\" } - Format Go code",
		"lint_code":                "{ \"path\": \"string\", \"linterName\": \"string\" (optional) } - Lint code",
		"web_search":               "{ \"query\": \"string\", \"result_size\": number (optional), \"search_backend\": \"string\" (optional) } - Perform web search",
		"web_content":              "{ \"urls\": [\"string\", ...] } - Fetch web content",
	}

	var sb strings.Builder
	sb.WriteString("Available tools and their parameters:\n\n")

	// Sort the tools by name for consistent output
	toolNames := make([]string, 0, len(toolDocs))
	for tool := range toolDocs {
		toolNames = append(toolNames, tool)
	}
	sort.Strings(toolNames)

	// Build the documentation string
	for _, toolName := range toolNames {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", toolName, toolDocs[toolName]))
	}

	return sb.String()
}

func produceLanguageCode(config *Config, language, spec string) (string, error) {
	systemMsg := fmt.Sprintf(`
You are a coding assistant that produces correct, runnable %s code. 
Output only the code. Do not wrap it in markdown fences or add commentary.
`, language)

	userMsg := fmt.Sprintf(`
Generate a %s program to accomplish the following:
%s
`, language, spec)

	messages := []ChatCompletionMsg{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}

	output, err := callCompletionsEndpoint(config, messages)
	if err != nil {
		return "", err
	}

	// Minimal cleanup in case the LLM includes triple backticks or extra text
	code := strings.TrimSpace(output)
	code = strings.TrimPrefix(code, "```"+language)
	code = strings.TrimPrefix(code, "```")
	code = strings.TrimSuffix(code, "```")
	code = strings.TrimSpace(code)

	return code, nil
}

func runCodeInContainer(language, code string, dependencies []string) (string, error) {
	switch language {
	case "python":
		resp, err := runPythonInContainer(code, dependencies)
		if err != nil {
			return "", err
		}
		if resp.Error != "" {
			return "", fmt.Errorf(resp.Error)
		}
		return resp.Result, nil

	case "go":
		resp, err := runGoInContainer(code, dependencies)
		if err != nil {
			return "", err
		}
		if resp.Error != "" {
			return "", fmt.Errorf(resp.Error)
		}
		return resp.Result, nil

	case "javascript":
		resp, err := runNodeInContainer(code, dependencies)
		if err != nil {
			return "", err
		}
		if resp.Error != "" {
			return "", fmt.Errorf(resp.Error)
		}
		return resp.Result, nil

	default:
		return "", fmt.Errorf("unsupported language: %s", language)
	}
}
