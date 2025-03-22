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
	"regexp"
	"strings"
	"sync"
	"time"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

func RunMCP(config *Config) {

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
	// Register New Tools
	// --------------------------
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

	if err := server.RegisterTool("agent",
		"Agent that uses LLM to decide which tools to call - Plan and Execute style",
		newPlanExecuteAgent(config)); err != nil {
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

// =====================
// Agent Tool Argument Type
// =====================
type AgentArgs struct {
	Query    string `json:"query" jsonschema:"required,description=User's query"`
	MaxCalls int    `json:"maxCalls" jsonschema:"required,description=Max iteration steps allowed"`
}

// planTask is the JSON structure representing a single planned task in the plan array.
type planTask struct {
	Tool      string          `json:"tool"`
	Args      json.RawMessage `json:"args"`
	TaskID    int             `json:"task_id,omitempty"`
	DependsOn []int           `json:"depends_on,omitempty"`
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

// =====================
// New Tools: Additional Argument Types
// =====================

// ReadMultipleFilesArgs is used by the "read_multiple_files" tool.
type ReadMultipleFilesArgs struct {
	Paths []string `json:"paths" jsonschema:"required,description=List of file paths to read"`
}

// EditFileArgs is used by the "edit_file" tool.
type EditFileArgs struct {
	Path         string `json:"path" jsonschema:"required,description=Path to the file to edit"`
	Search       string `json:"search,omitempty" jsonschema:"description=Text to search for"`
	Replace      string `json:"replace,omitempty" jsonschema:"description=Text to replace with"`
	PatchContent string `json:"patchContent,omitempty" jsonschema:"description=A unified diff patch to apply to the file"`
}

// DirectoryTreeArgs is used by the "directory_tree" tool.
type DirectoryTreeArgs struct {
	Path     string `json:"path" jsonschema:"required,description=Root directory for the tree"`
	MaxDepth int    `json:"maxDepth,omitempty" jsonschema:"description=Limit recursion depth (0 for unlimited)"`
}

// SearchFilesArgs is used by the "search_files" tool.
type SearchFilesArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Base path to search"`
	Pattern string `json:"pattern" jsonschema:"required,description=Text or regex pattern to find"`
}

// GetFileInfoArgs is used by the "get_file_info" tool.
type GetFileInfoArgs struct {
	Path string `json:"path" jsonschema:"required,description=Path to the file or directory"`
}

// ListAllowedDirectoriesArgs is used by the "list_allowed_directories" tool.
type ListAllowedDirectoriesArgs struct{}

// DeleteFileArgs is used by the "delete_file" tool.
type DeleteFileArgs struct {
	Path      string `json:"path" jsonschema:"required,description=File or directory path to delete"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"description=If true, delete recursively"`
}

// CopyFileArgs is used by the "copy_file" tool.
type CopyFileArgs struct {
	Source      string `json:"source" jsonschema:"required"`
	Destination string `json:"destination" jsonschema:"required"`
	Recursive   bool   `json:"recursive,omitempty" jsonschema:"description=Copy directories recursively"`
}

// GitCloneArgs is used by the "git_clone" tool.
type GitCloneArgs struct {
	RepoURL string `json:"repoUrl" jsonschema:"required"`
	Path    string `json:"path" jsonschema:"required,description=Local path to clone into"`
}

// GitCheckoutArgs is used by the "git_checkout" tool.
type GitCheckoutArgs struct {
	Path      string `json:"path" jsonschema:"required"`
	Branch    string `json:"branch" jsonschema:"required"`
	CreateNew bool   `json:"createNew,omitempty" jsonschema:"description=Create a new branch if true"`
}

// GitDiffArgs is used by the "git_diff" tool.
type GitDiffArgs struct {
	Path    string `json:"path" jsonschema:"required"`
	FromRef string `json:"fromRef,omitempty" jsonschema:"description=Starting reference"`
	ToRef   string `json:"toRef,omitempty" jsonschema:"description=Ending reference"`
}

// ShellCommandArgs is used by the "run_shell_command" tool.
type ShellCommandArgs struct {
	Command []string `json:"command" jsonschema:"required"`
	Dir     string   `json:"dir" jsonschema:"required"`
}

// GoBuildArgs is used by the "go_build" tool.
type GoBuildArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of the Go module to build"`
}

// GoTestArgs is used by the "go_test" tool.
type GoTestArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of the Go module to test"`
}

// FormatGoCodeArgs is used by the "format_go_code" tool.
type FormatGoCodeArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory of the Go code to format"`
}

// LintCodeArgs is used by the "lint_code" tool.
type LintCodeArgs struct {
	Path       string `json:"path" jsonschema:"required,description=Directory or file to lint"`
	LinterName string `json:"linterName,omitempty" jsonschema:"description=Name of the linter to use (optional)"`
}

// WebSearchArgs is used by the "web_search" tool.
type WebSearchArgs struct {
	Query         string `json:"query" jsonschema:"required,description=Search query text"`
	ResultSize    int    `json:"result_size,omitempty" jsonschema:"description=Number of results to return (default: 3)"`
	SearchBackend string `json:"search_backend,omitempty" jsonschema:"description=Search backend to use (default: ddg, alternative: sxng)"`
	SxngURL       string `json:"sxng_url,omitempty" jsonschema:"description=URL of SearXNG instance when using sxng backend"`
}

// WebContentArgs is used by the "web_content" tool.
type WebContentArgs struct {
	URLs []string `json:"urls" jsonschema:"required,description=List of URLs to fetch content from"`
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

// -------------------------
// Helper Functions for File Copy and Directory Tree
// -------------------------

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
// in the config file, therefore the model and API key are not required
// as arguments unless
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

	// Using configuration values instead of hardcoded constants
	endpoint := config.Completions.DefaultHost
	apiKey := config.Completions.APIKey

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("completions request failed: %w", err)
	}
	defer resp.Body.Close()

	var completionResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		return "", fmt.Errorf("failed to parse completions response: %w", err)
	}

	if len(completionResp.Choices) == 0 {
		return "", fmt.Errorf("no completion returned by endpoint")
	}

	answer := completionResp.Choices[0].Message.Content
	return answer, nil
}

// ---------------------------------------------------------
// Tool helper functions: each tool has a function that takes its
// typed arguments and returns a string result. In addition, each
// wrapper (callXXXTool) unmarshals JSON and calls the underlying
// function.
// ---------------------------------------------------------

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

// -------------------------
// New Tools Implementations
// -------------------------

// read_multiple_files tool
func readMultipleFilesTool(args ReadMultipleFilesArgs) (string, error) {
	var result strings.Builder
	for _, path := range args.Paths {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			result.WriteString(fmt.Sprintf("Error reading %s: %v\n", path, err))
		} else {
			result.WriteString(fmt.Sprintf("File: %s\n%s\n------\n", path, string(data)))
		}
	}
	return result.String(), nil
}

func callReadMultipleFilesTool(jsonArgs string) (string, error) {
	var args ReadMultipleFilesArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return readMultipleFilesTool(args)
}

// edit_file tool
func editFileTool(args EditFileArgs) (string, error) {
	if args.PatchContent != "" {
		return "", fmt.Errorf("patchContent not supported in this implementation")
	}
	if args.Search == "" {
		return "", fmt.Errorf("must provide a search string for edit_file")
	}
	// Read file
	original, err := ioutil.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	edited := strings.ReplaceAll(string(original), args.Search, args.Replace)
	err = ioutil.WriteFile(args.Path, []byte(edited), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write edited file: %w", err)
	}
	return fmt.Sprintf("Edited file: %s", args.Path), nil
}

func callEditFileTool(jsonArgs string) (string, error) {
	var args EditFileArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return editFileTool(args)
}

// directory_tree tool
func directoryTreeTool(args DirectoryTreeArgs) (string, error) {
	tree, err := buildDirectoryTree(args.Path, "", args.MaxDepth, 1)
	if err != nil {
		return "", err
	}
	return tree, nil
}

func callDirectoryTreeTool(jsonArgs string) (string, error) {
	var args DirectoryTreeArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return directoryTreeTool(args)
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
	return "Files matching pattern:\n" + strings.Join(matches, "\n"), nil
}

func callSearchFilesTool(jsonArgs string) (string, error) {
	var args SearchFilesArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return searchFilesTool(args)
}

// get_file_info tool
func getFileInfoTool(args GetFileInfoArgs) (string, error) {
	info, err := os.Stat(args.Path)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	return fmt.Sprintf("Name: %s\nSize: %d bytes\nMode: %s\nModified: %s\nIsDir: %t",
		info.Name(), info.Size(), info.Mode().String(), info.ModTime().Format(time.RFC3339), info.IsDir()), nil
}

func callGetFileInfoTool(jsonArgs string) (string, error) {
	var args GetFileInfoArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return getFileInfoTool(args)
}

// list_allowed_directories tool
func listAllowedDirectoriesTool(args ListAllowedDirectoriesArgs) (string, error) {
	// In a real system, this might pull from configuration.
	return "All directories are allowed.", nil
}

func callListAllowedDirectoriesTool(jsonArgs string) (string, error) {
	var args ListAllowedDirectoriesArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return listAllowedDirectoriesTool(args)
}

// delete_file tool
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

func callDeleteFileTool(jsonArgs string) (string, error) {
	var args DeleteFileArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return deleteFileTool(args)
}

// copy_file tool
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

func callCopyFileTool(jsonArgs string) (string, error) {
	var args CopyFileArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return copyFileTool(args)
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

func callGitCloneTool(jsonArgs string) (string, error) {
	var args GitCloneArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitCloneTool(args)
}

// git_checkout tool
func gitCheckoutTool(args GitCheckoutArgs) (string, error) {
	var cmd *exec.Cmd
	if args.CreateNew {
		cmd = exec.Command("git", "checkout", "-b", args.Branch)
	} else {
		cmd = exec.Command("git", "checkout", args.Branch)
	}
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git checkout failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

func callGitCheckoutTool(jsonArgs string) (string, error) {
	var args GitCheckoutArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitCheckoutTool(args)
}

// git_diff tool
func gitDiffTool(args GitDiffArgs) (string, error) {
	var diffArgs []string
	diffArgs = append(diffArgs, "diff")
	if args.FromRef != "" && args.ToRef != "" {
		diffArgs = append(diffArgs, args.FromRef, args.ToRef)
	}
	output, err := runGitCommand(args.Path, diffArgs...)
	if err != nil {
		return "", err
	}
	return output, nil
}

func callGitDiffTool(jsonArgs string) (string, error) {
	var args GitDiffArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return gitDiffTool(args)
}

// run_shell_command tool
func runShellCommandTool(args ShellCommandArgs) (string, error) {
	// Handle either string or []string format for Command
	cmdParts := args.Command

	if len(cmdParts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	cmd.Dir = args.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("shell command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

func callShellCommandTool(jsonArgs string) (string, error) {
	var args ShellCommandArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return runShellCommandTool(args)
}

// go_build tool
func goBuildTool(args GoBuildArgs) (string, error) {
	cmd := exec.Command("go", "build")
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

func callGoBuildTool(jsonArgs string) (string, error) {
	var args GoBuildArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return goBuildTool(args)
}

// go_test tool
func goTestTool(args GoTestArgs) (string, error) {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go test failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

func callGoTestTool(jsonArgs string) (string, error) {
	var args GoTestArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return goTestTool(args)
}

// format_go_code tool
func formatGoCodeTool(args FormatGoCodeArgs) (string, error) {
	cmd := exec.Command("go", "fmt", "./...")
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go fmt failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

func callFormatGoCodeTool(jsonArgs string) (string, error) {
	var args FormatGoCodeArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return formatGoCodeTool(args)
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
		return "", fmt.Errorf("lint command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

func callLintCodeTool(jsonArgs string) (string, error) {
	var args LintCodeArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return lintCodeTool(args)
}

// web_search tool
func webSearchTool(args WebSearchArgs) (string, error) {
	results := web.SearchDDG(args.Query)
	if results == nil {
		return "", fmt.Errorf("error performing web search")
	}

	log.Printf("Web search results!!!!!!!!!!: %v", results)

	return strings.Join(results, "\n"), nil
}

func callWebSearchTool(jsonArgs string) (string, error) {
	var args WebSearchArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return webSearchTool(args)
}

// web_content tool
func webContentTool(args WebContentArgs) (string, error) {
	// Validate input
	if len(args.URLs) == 0 {
		return "", fmt.Errorf("error: no URLs provided. The web_content tool requires at least one URL in the 'urls' array")
	}

	log.Printf("Fetching content from URLs: %v", args.URLs)

	// Split the URLs and fetch content from each
	urls := strings.Join(args.URLs, "/\n")

	var content strings.Builder

	urlList := strings.Split(urls, "\n")
	for _, urlStr := range urlList {
		urlStr = strings.TrimSpace(urlStr)
		if urlStr == "" {
			continue
		}

		apiURL := "http://localhost:8080/api/web-content?urls=" + urlStr

		// Create a client with the same 5-minute timeout
		client := &http.Client{
			Timeout: 5 * time.Minute,
		}

		resp, err := client.Get(apiURL)
		if err != nil {
			return "", fmt.Errorf("web content request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// For now we ignore non-OK responses.
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}

		content.WriteString(string(bodyBytes))
	}

	return content.String(), nil
}

func callWebContentTool(jsonArgs string) (string, error) {
	var args WebContentArgs
	if err := json.Unmarshal([]byte(jsonArgs), &args); err != nil {
		return "", err
	}
	return webContentTool(args)
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
	// New Tools
	case "read_multiple_files":
		return callReadMultipleFilesTool(jsonArgs)
	case "edit_file":
		return callEditFileTool(jsonArgs)
	case "directory_tree":
		return callDirectoryTreeTool(jsonArgs)
	case "search_files":
		return callSearchFilesTool(jsonArgs)
	case "get_file_info":
		return callGetFileInfoTool(jsonArgs)
	case "list_allowed_directories":
		return callListAllowedDirectoriesTool(jsonArgs)
	case "delete_file":
		return callDeleteFileTool(jsonArgs)
	case "copy_file":
		return callCopyFileTool(jsonArgs)
	case "git_clone":
		return callGitCloneTool(jsonArgs)
	case "git_checkout":
		return callGitCheckoutTool(jsonArgs)
	case "git_diff":
		return callGitDiffTool(jsonArgs)
	case "run_shell_command":
		return callShellCommandTool(jsonArgs)
	case "go_build":
		return callGoBuildTool(jsonArgs)
	case "go_test":
		return callGoTestTool(jsonArgs)
	case "format_go_code":
		return callFormatGoCodeTool(jsonArgs)
	case "lint_code":
		return callLintCodeTool(jsonArgs)
	case "web_search":
		return callWebSearchTool(jsonArgs)
	case "web_content":
		return callWebContentTool(jsonArgs)
	case "agent":
		// agent is handled separately below
		return "", fmt.Errorf("agent tool should be handled separately")
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// newPlanExecuteAgent returns a function (tool handler) that implements the recommended plan-execute loop.
func newPlanExecuteAgent(config *Config) func(args AgentArgs) (*mcp.ToolResponse, error) {
	return func(args AgentArgs) (*mcp.ToolResponse, error) {
		if args.MaxCalls <= 0 {
			args.MaxCalls = 5 // fallback
		}

		// A map/dict to store intermediate outputs from each executed tool
		agentMemory := make(map[string]string)

		// Build a stable JSON representation of the available tools
		toolsJSON := buildToolsListJSON()

		// Initialize a conversation or structured context for the LLM
		// This conversation can store system messages, user queries, partial logs, etc.
		var conversation []ChatCompletionMsg
		conversation = append(conversation, ChatCompletionMsg{
			Role:    "system",
			Content: "You are an advanced autonomous planning assistant. You can plan tasks by referencing the tool list, then execute them. Use as few steps as possible to accomplish the query. You store partial results in your memory, referencing them if needed.",
		})
		conversation = append(conversation, ChatCompletionMsg{Role: "user", Content: args.Query})

		// We'll loop up to args.MaxCalls times:
		for iteration := 0; iteration < args.MaxCalls; iteration++ {
			// ---------- PHASE 1: PLANNING ----------
			planningPrompt := fmt.Sprintf(`
Available Tools (JSON):
%s

You must produce a raw text JSON array of tasks, for instance:
[
  { "tool": "some_tool", "args": { "key": "value" }, "depends_on": [] },
  { "tool": "another_tool", "args": { "whatever": 123 }, "depends_on": [0] }
]

A task must NEVER depend on itself.

Any "depends_on" array references zero-based indices of previous tasks in this plan that must complete first.
Please plan the tasks to solve the user's query: "%s"

Return only valid JSON array. If no more steps are needed, respond with an empty JSON array "[]".
IMPORTANT: NEVER respond using markdown syntax. ONLY raw JSON is permitted. NEVER wrap in triple backticks.
`, toolsJSON, args.Query)

			planningMsg := ChatCompletionMsg{
				Role:    "user",
				Content: planningPrompt,
			}
			conversation = append(conversation, planningMsg)

			planOutput, planErr := callCompletionsEndpoint(config, conversation)
			if planErr != nil {
				return nil, fmt.Errorf("planning step failed: %w", planErr)
			}

			planOutput = strings.TrimSpace(planOutput)
			log.Printf("Planning raw output: %s", planOutput)

			// Attempt to parse as JSON array of planTask
			tasks, parseErr := parsePlanOutput(planOutput)
			if parseErr != nil {
				// In a real scenario, we might try to re-prompt or finalize with partial result:
				log.Printf("Failed to parse plan: %v", parseErr)
				// Return partial or fallback
				return mcp.NewToolResponse(mcp.NewTextContent(
					fmt.Sprintf("Could not parse plan. Partial: %v", parseErr),
				)), nil
			}

			if len(tasks) == 0 {
				// Means the LLM returned "[]", i.e. no further tasks. Let's do a final answer attempt.
				finalAns := tryFinalAnswer(config, conversation, agentMemory, args)
				return mcp.NewToolResponse(mcp.NewTextContent(finalAns)), nil
			}

			// ---------- PHASE 2: EXECUTION ----------
			// We'll track logs of each step to feed back into conversation if needed
			iterationLog := make([]string, 0, len(tasks))

			// concurrency approach: we can run tasks in waves, respecting depends_on
			// Build adjacency / dependency references:
			resultsMu := sync.Mutex{} // Protects agentMemory writes

			if err := executePlanTasksConcurrently(tasks, &resultsMu, agentMemory, iterationLog); err != nil {
				// If there's a catastrophic error, we can finalize or return partial:
				log.Printf("Execution encountered an error: %v", err)
				// We continue to final answer attempt or break:
				finalAns := tryFinalAnswer(config, conversation, agentMemory, args)
				return mcp.NewToolResponse(mcp.NewTextContent(
					finalAns + "\n(Encountered partial errors in tasks.)",
				)), nil
			}

			// Append iteration logs into conversation so next iteration can see them
			for _, stepResult := range iterationLog {
				conversation = append(conversation, ChatCompletionMsg{Role: "assistant", Content: stepResult})
			}

			// ---------- PHASE 3: CHECK IF COMPLETE ----------
			// We'll let the LLM see our iteration log & memory and see if final answer is possible
			finalPrompt := buildFinalizationPrompt(args.Query, iterationLog, agentMemory, toolsJSON)
			convFin := append([]ChatCompletionMsg{
				{Role: "system", Content: "You have a log of the tasks just executed. Provide final or decide if more steps needed."},
			}, ChatCompletionMsg{Role: "user", Content: finalPrompt})

			finalAnswer, finalErr := callCompletionsEndpoint(config, convFin)
			if finalErr != nil {
				return mcp.NewToolResponse(
					mcp.NewTextContent(fmt.Sprintf("Error finalizing: %v", finalErr)),
				), nil
			}

			// If finalAnswer is truly a concluding answer (no further tasks), we return it:
			if isConclusion(finalAnswer) {
				return mcp.NewToolResponse(mcp.NewTextContent(finalAnswer)), nil
			}

			// Otherwise, we add the finalAnswer to conversation so the next iteration can see it
			conversation = append(conversation, ChatCompletionMsg{Role: "assistant", Content: finalAnswer})
		}

		// If we exhaust iteration limit, return partial result:
		partial := tryFinalAnswer(config, conversation, agentMemory, args)
		msg := fmt.Sprintf("Reached max iterations (%d). Partial answer:\n%s", args.MaxCalls, partial)
		return mcp.NewToolResponse(mcp.NewTextContent(msg)), nil
	}
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
	config *Config,
	conversation []ChatCompletionMsg,
	agentMemory map[string]string,
	args AgentArgs,
) string {
	finalPrompt := "We have possibly partial data. Summarize the best final answer to the user's query: \"" + args.Query + "\". " +
		"If data is incomplete, do your best."
	conv := append(conversation, ChatCompletionMsg{Role: "user", Content: finalPrompt})
	ans, err := callCompletionsEndpoint(config, conv)
	if err != nil {
		return "Could not finalize answer: " + err.Error()
	}
	return ans
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
