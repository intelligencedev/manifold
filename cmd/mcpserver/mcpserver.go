package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

// =====================
// Constants for OpenAI (placeholder)
// =====================
const openAIEndpoint = "https://api.openai.com/v1/chat/completions"
const openAIApiKey = "..."

// =====================
// Existing Argument Types
// =====================

// HelloArgs represents the arguments for the hello tool
type HelloArgs struct {
	Name string `json:"name" jsonschema:"required,description=The name to say hello to"`
}

// CalculateArgs represents the arguments for the calculate tool
type CalculateArgs struct {
	Operation string  `json:"operation" jsonschema:"required,enum=add,enum=subtract,enum=multiply,enum=divide,description=The mathematical operation to perform"`
	A         float64 `json:"a" jsonschema:"required,description=First number"`
	B         float64 `json:"b" jsonschema:"required,description=Second number"`
}

// TimeArgs represents the arguments for the current time tool
type TimeArgs struct {
	Format string `json:"format,omitempty" jsonschema:"description=Optional time format (default: RFC3339)"`
}

// PromptArgs represents the arguments for custom prompts
type PromptArgs struct {
	Input string `json:"input" jsonschema:"required,description=The input text to process"`
}

// WeatherArgs represents the arguments for the weather tool
type WeatherArgs struct {
	Longitude float64 `json:"longitude" jsonschema:"required,description=The longitude of the location to get the weather for"`
	Latitude  float64 `json:"latitude" jsonschema:"required,description=The latitude of the location to get the weather for"`
}

// =====================
// New File System Tools (Argument Types)
// =====================

// ReadFileArgs is used by the "read_file" tool.
type ReadFileArgs struct {
	Path string `json:"path" jsonschema:"required,description=Path to the file to read"`
}

// WriteFileArgs is used by the "write_file" tool.
type WriteFileArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Path to the file to write"`
	Content string `json:"content" jsonschema:"required,description=Content to write into the file"`
}

// ListDirectoryArgs is used by the "list_directory" tool.
type ListDirectoryArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path to list"`
}

// CreateDirectoryArgs is used by the "create_directory" tool.
type CreateDirectoryArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path to create"`
}

// MoveFileArgs is used by the "move_file" tool.
type MoveFileArgs struct {
	Source      string `json:"source" jsonschema:"required,description=Source file/directory path"`
	Destination string `json:"destination" jsonschema:"required,description=Destination file/directory path"`
}

// =====================
// Git Tool Argument Types
// =====================

// GitInitArgs is used by the "git_init" tool.
type GitInitArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory in which to initialize a Git repo"`
}

// GitRepoArgs is used by "git_status", "git_pull", "git_push".
type GitRepoArgs struct {
	Path string `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
}

// GitAddArgs is used by the "git_add" tool.
type GitAddArgs struct {
	Path     string   `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
	FileList []string `json:"fileList" jsonschema:"required,description=List of files to add (or empty to add all)"`
}

// GitCommitArgs is used by the "git_commit" tool.
type GitCommitArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
	Message string `json:"message" jsonschema:"required,description=Commit message"`
}

// --------------------------
// Helper Functions for Git
// --------------------------

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

func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

// ----------------------------
// Agent Tool Argument Type
// ----------------------------
type AgentArgs struct {
	Query    string `json:"query" jsonschema:"required,description=User's query"`
	MaxCalls int    `json:"maxCalls" jsonschema:"required,description=Number of maximum LLM calls allowed"`
}

// ChatCompletionRequest is used to marshal the body for OpenAI's chat completion request.
type ChatCompletionRequest struct {
	Model       string              `json:"model"`
	Messages    []ChatCompletionMsg `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
}

// ChatCompletionMsg represents a message with role and content for the Chat API.
type ChatCompletionMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse is used to unmarshal the OpenAI chat completion response.
type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// -------------------------
// callOpenAI is a helper
// -------------------------
func callOpenAI(messages []ChatCompletionMsg) (string, error) {
	requestBody := ChatCompletionRequest{
		Model:       "gpt-4o-mini",
		Messages:    messages,
		MaxTokens:   8192,
		Temperature: 0.3,
	}

	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", openAIEndpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+openAIApiKey)

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

//
// ---------------------------------------------------------
// Tool helper functions: each tool has a function that takes its
// typed arguments and returns a string result. In addition, each
// wrapper (callXXXTool) unmarshals JSON and calls the underlying
// function.
// ---------------------------------------------------------
//

// hello tool
func helloTool(args HelloArgs) (string, error) {
	return fmt.Sprintf("Hello, %s!", args.Name), nil
}

func callHelloTool(jsonArgs string) (string, error) {
	var args HelloArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return helloTool(args)
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

func callCalculateTool(jsonArgs string) (string, error) {
	var args CalculateArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return calculateTool(args)
}

// time tool
func timeTool(args TimeArgs) (string, error) {
	format := time.RFC3339
	if args.Format != "" {
		format = args.Format
	}
	return time.Now().Format(format), nil
}

func callTimeTool(jsonArgs string) (string, error) {
	var args TimeArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return timeTool(args)
}

// get_weather tool
func getWeatherTool(args WeatherArgs) (string, error) {
	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,wind_speed_10m&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m",
		args.Latitude, args.Longitude,
	)
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

func callGetWeatherTool(jsonArgs string) (string, error) {
	var args WeatherArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return getWeatherTool(args)
}

// read_file tool
func readFileTool(args ReadFileArgs) (string, error) {
	bytes, err := ioutil.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(bytes), nil
}

func callReadFileTool(jsonArgs string) (string, error) {
	var args ReadFileArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return readFileTool(args)
}

// write_file tool
func writeFileTool(args WriteFileArgs) (string, error) {
	err := ioutil.WriteFile(args.Path, []byte(args.Content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("Wrote file: %s", args.Path), nil
}

func callWriteFileTool(jsonArgs string) (string, error) {
	var args WriteFileArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return writeFileTool(args)
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

func callListDirectoryTool(jsonArgs string) (string, error) {
	var args ListDirectoryArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return listDirectoryTool(args)
}

// create_directory tool
func createDirectoryTool(args CreateDirectoryArgs) (string, error) {
	err := os.MkdirAll(args.Path, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	return fmt.Sprintf("Directory created: %s", args.Path), nil
}

func callCreateDirectoryTool(jsonArgs string) (string, error) {
	var args CreateDirectoryArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return createDirectoryTool(args)
}

// move_file tool
func moveFileTool(args MoveFileArgs) (string, error) {
	err := os.Rename(args.Source, args.Destination)
	if err != nil {
		return "", fmt.Errorf("failed to move/rename: %w", err)
	}
	return fmt.Sprintf("Moved/renamed '%s' to '%s'", args.Source, args.Destination), nil
}

func callMoveFileTool(jsonArgs string) (string, error) {
	var args MoveFileArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return moveFileTool(args)
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

func callGitInitTool(jsonArgs string) (string, error) {
	var args GitInitArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitInitTool(args)
}

// git_status tool
func gitStatusTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	output, err := runGitCommand(args.Path, "status", "--short", "--branch")
	if err != nil {
		return "", err
	}
	return output, nil
}

func callGitStatusTool(jsonArgs string) (string, error) {
	var args GitRepoArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitStatusTool(args)
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
	output, err := runGitCommand(args.Path, fullArgs...)
	if err != nil {
		return "", err
	}
	return output, nil
}

func callGitAddTool(jsonArgs string) (string, error) {
	var args GitAddArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitAddTool(args)
}

// git_commit tool
func gitCommitTool(args GitCommitArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Message) == "" {
		return "", fmt.Errorf("commit message cannot be empty")
	}
	output, err := runGitCommand(args.Path, "commit", "-m", args.Message)
	if err != nil {
		return "", err
	}
	return output, nil
}

func callGitCommitTool(jsonArgs string) (string, error) {
	var args GitCommitArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitCommitTool(args)
}

// git_pull tool
func gitPullTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	output, err := runGitCommand(args.Path, "pull", "--rebase")
	if err != nil {
		return "", err
	}
	return output, nil
}

func callGitPullTool(jsonArgs string) (string, error) {
	var args GitRepoArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitPullTool(args)
}

// git_push tool
func gitPushTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	output, err := runGitCommand(args.Path, "push")
	if err != nil {
		return "", err
	}
	return output, nil
}

func callGitPushTool(jsonArgs string) (string, error) {
	var args GitRepoArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitPushTool(args)
}

// callToolInServer dispatches a tool call based on the tool name and JSON arguments.
func callToolInServer(toolName, jsonArgs string) (string, error) {
	switch toolName {
	case "hello":
		return callHelloTool(jsonArgs)
	case "calculate":
		return callCalculateTool(jsonArgs)
	case "time":
		return callTimeTool(jsonArgs)
	case "get_weather":
		return callGetWeatherTool(jsonArgs)
	case "read_file":
		return callReadFileTool(jsonArgs)
	case "write_file":
		return callWriteFileTool(jsonArgs)
	case "list_directory":
		return callListDirectoryTool(jsonArgs)
	case "create_directory":
		return callCreateDirectoryTool(jsonArgs)
	case "move_file":
		return callMoveFileTool(jsonArgs)
	case "git_init":
		return callGitInitTool(jsonArgs)
	case "git_status":
		return callGitStatusTool(jsonArgs)
	case "git_add":
		return callGitAddTool(jsonArgs)
	case "git_commit":
		return callGitCommitTool(jsonArgs)
	case "git_pull":
		return callGitPullTool(jsonArgs)
	case "git_push":
		return callGitPushTool(jsonArgs)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// ---------------------------------------------------------
// Main function: registering tools and starting the server
// ---------------------------------------------------------
func main() {
	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransport()

	// Create a new server with the transport
	server := mcp.NewServer(serverTransport)

	// --------------------------
	// Existing Tools
	// --------------------------
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

	// --------------------------
	// Register our new "agent" tool:
	// --------------------------
	if err := server.RegisterTool("agent", "Agent that uses LLM to decide which tools to call", agentHandler()); err != nil {
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

// agentHandler returns a closure that processes the agent query.
func agentHandler() func(args AgentArgs) (*mcp.ToolResponse, error) {
	return func(args AgentArgs) (*mcp.ToolResponse, error) {
		// First, always retrieve the list of available tools
		toolsList, err := callToolInServer("listTools", "{}")
		if err != nil {
			toolsList = fmt.Sprintf("Error retrieving available tools: %v", err)
		}

		// Initialize conversation context including the list of tools.
		conversation := []ChatCompletionMsg{
			{
				Role: "system",
				Content: `Below is a list of file system, Git, and agent operations you can perform.
When answering queries, you MUST use the available tools rather than relying on internal knowledge.
After each tool call, evaluate whether the aggregated tool responses provide enough context to answer the user's original query.
If yes, respond with:
  FINAL_ANSWER: <your final answer>
If not, respond with:
  CALL_TOOL: <toolName>
  ARGS: <json arguments for that tool>
In particular, if a query requires real-time data (e.g., "What time is it?"), you MUST invoke the "time" tool.
The available tools are:
` + toolsList,
			},
			{
				Role:    "user",
				Content: args.Query,
			},
		}

		// Loop until a final answer is generated or maxCalls is reached.
		for callCount := 0; callCount < args.MaxCalls; callCount++ {
			assistantReply, err := callOpenAI(conversation)
			if err != nil {
				return nil, err
			}
			assistantReply = strings.TrimSpace(assistantReply)
			upperReply := strings.ToUpper(assistantReply)

			// If a final answer directive is returned, output it.
			if strings.HasPrefix(upperReply, "FINAL_ANSWER:") {
				finalAnswer := strings.TrimSpace(assistantReply[len("FINAL_ANSWER:"):])
				return mcp.NewToolResponse(mcp.NewTextContent(finalAnswer)), nil
			}

			// If a tool call is requested, extract the tool name and its JSON args.
			if strings.HasPrefix(upperReply, "CALL_TOOL:") {
				lines := strings.SplitN(assistantReply, "\n", 2)
				toolName := strings.TrimSpace(strings.TrimPrefix(lines[0], "CALL_TOOL:"))
				var jsonArgs string
				if len(lines) > 1 && strings.Contains(strings.ToUpper(lines[1]), "ARGS:") {
					jsonArgs = strings.TrimSpace(strings.TrimPrefix(lines[1], "ARGS:"))
				} else {
					continue
				}

				// Execute the requested tool.
				result, err := callToolInServer(toolName, jsonArgs)
				if err != nil {
					result = fmt.Sprintf("Error calling tool %s: %v", toolName, err)
				}

				// Append the tool's response to the conversation context.
				toolResponseMsg := fmt.Sprintf("Tool Response (%s): %s", toolName, result)
				conversation = append(conversation, ChatCompletionMsg{
					Role:    "assistant",
					Content: toolResponseMsg,
				})
				// Continue the loop so the LLM can decide if more tool calls are needed.
				continue
			}

			// If no recognizable directive is found, assume it's the final answer.
			return mcp.NewToolResponse(mcp.NewTextContent(assistantReply)), nil
		}

		// If maxCalls are exhausted without a final answer:
		return mcp.NewToolResponse(mcp.NewTextContent("Insufficient context gathered to answer the query.")), nil
	}
}
