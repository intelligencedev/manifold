package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
	"gopkg.in/yaml.v2" // Added for YAML parsing
)

// =====================
// Config structs
// =====================
type DatabaseConfig struct {
	ConnectionString string `yaml:"connection_string"`
}

type CompletionsConfig struct {
	DefaultHost string `yaml:"default_host"`
	APIKey      string `yaml:"api_key"`
}

type EmbeddingsConfig struct {
	Host         string `yaml:"host"`
	APIKey       string `yaml:"api_key"`
	Dimensions   int    `yaml:"dimensions"`
	EmbedPrefix  string `yaml:"embed_prefix"`
	SearchPrefix string `yaml:"search_prefix"`
}

type RerankerConfig struct {
	Host string `yaml:"host"`
}

type Config struct {
	Host             string            `yaml:"host"`
	Port             int               `yaml:"port"`
	DataPath         string            `yaml:"data_path"`
	AnthropicKey     string            `yaml:"anthropic_key,omitempty"`
	OpenAIAPIKey     string            `yaml:"openai_api_key,omitempty"`
	GoogleGeminiKey  string            `yaml:"google_gemini_key,omitempty"`
	HuggingFaceToken string            `yaml:"hf_token,omitempty"`
	Database         DatabaseConfig    `yaml:"database"`
	Completions      CompletionsConfig `yaml:"completions"`
	Embeddings       EmbeddingsConfig  `yaml:"embeddings"`
	Reranker         RerankerConfig    `yaml:"reranker"`
}

// Function to load the configuration file
func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}

// Global config variable that will hold our loaded configuration
var appConfig *Config

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
	Directory string   `json:"directory" jsonschema:"required"`
	Command   []string `json:"command" jsonschema:"required"`
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
// callOpenAI is a helper
// -------------------------
func callOpenAI(messages []ChatCompletionMsg) (string, error) {
	requestBody := ChatCompletionRequest{
		Model:       "gpt-4o-mini",
		Messages:    messages,
		MaxTokens:   8192,
		Temperature: 0.1,
	}

	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Using configuration values instead of hardcoded constants
	endpoint := appConfig.Completions.DefaultHost
	apiKey := appConfig.Completions.APIKey

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
	cmd := exec.Command(args.Command[0], args.Command[1:]...)
	cmd.Dir = args.Directory
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
	case "agent":
		// agent is handled separately below
		return "", fmt.Errorf("agent tool should be handled separately")
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// ---------------------------------------------------------
// Main function: registering tools and starting the server
// ---------------------------------------------------------
func main() {
	// Load configuration from config file
	configPath := "../../config.yaml" // Path to config.yaml relative to mcpserver executable

	var err error
	appConfig, err = loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Loaded configuration. Using completions endpoint: %s", appConfig.Completions.DefaultHost)

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

// agentHandler returns a closure that processes the agent query using a recursive planner+worker approach.
func agentHandler() func(args AgentArgs) (*mcp.ToolResponse, error) {
	return func(args AgentArgs) (*mcp.ToolResponse, error) {
		var conversation []ChatCompletionMsg
		// --- Step 0: Retrieve Available Tools ---
		// Instead of trying to make HTTP requests, we'll build a static list of available tools
		// This avoids the need for mcp.NewToolRequest and server.HandleToolRequest
		toolsInfo := map[string]interface{}{
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
			},
		}

		toolsJSONBytes, err := json.MarshalIndent(toolsInfo, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tools info: %v", err)
		}
		toolsJSON := string(toolsJSONBytes)

		// --- Initialize conversation context ---
		// The conversation will be built up over iterations.
		conversation = []ChatCompletionMsg{
			{Role: "system", Content: "You are an autonomous planning assistant. Use the provided tools list to decide which actions to perform."},
			{Role: "user", Content: args.Query},
		}

		// --- Track tool execution results for substitution ---
		toolResults := make(map[string]string)

		// Maximum number of iterations (recursive refinement)
		for iteration := 0; iteration < args.MaxCalls; iteration++ {
			// --- Phase 1: Planning ---
			planningPrompt := "Available Tools:\n" + toolsJSON + "\n\n" +
				`You are a planning assistant for an autonomous agent.
The current user query is: "` + args.Query + `".
Produce a JSON array of tasks that will accomplish the query. 
Each task must be an object with two keys:
  - "tool": the name of the tool to invoke (e.g., "read_file", "edit_file", "write_file", etc.)
  - "args": a JSON object with the arguments for that tool.

To reference the output of a previous tool in your plan, use the format: "$TOOL_RESULT[task_index]"
  Example: { "tool": "write_file", "args": { "path": "weather.txt", "content": "$TOOL_RESULT[1]" } }
  This would use the result from the 2nd task (index 1) as the content for the write_file tool.

Do not include any task that calls the "agent" tool.
Output only the JSON array. For example:
[
  { "tool": "read_file", "args": { "path": "/path/to/file.txt" } },
  { "tool": "edit_file", "args": { "path": "/path/to/file.txt", "search": "old", "replace": "new" } }
]`
			// Append planning prompt to conversation
			conversation = append(conversation, ChatCompletionMsg{Role: "user", Content: planningPrompt})
			planOutput, err := callOpenAI(conversation)
			if err != nil {
				return nil, fmt.Errorf("planning phase failed: %v", err)
			}

			// --- Phase 2: Clean and Parse the Plan ---
			planOutput = strings.TrimSpace(planOutput)
			// Remove markdown fences if present.
			if strings.HasPrefix(planOutput, "```json") {
				planOutput = strings.TrimPrefix(planOutput, "```json")
				planOutput = strings.TrimSuffix(planOutput, "```")
				planOutput = strings.TrimSpace(planOutput)
			}

			type PlanTask struct {
				Tool string          `json:"tool"`
				Args json.RawMessage `json:"args"`
			}
			var tasks []PlanTask
			if err := json.Unmarshal([]byte(planOutput), &tasks); err != nil {
				return nil, fmt.Errorf("failed to parse plan JSON: %v\nPlan output was: %s", err, planOutput)
			}
			// Filter out tasks that reference the "agent" tool.
			validTasks := []PlanTask{}
			for _, task := range tasks {
				if strings.ToLower(task.Tool) == "agent" {
					continue
				}
				validTasks = append(validTasks, task)
			}
			if len(validTasks) == 0 {
				return nil, fmt.Errorf("plan produced no valid tasks (all tasks were forbidden)")
			}

			// --- Phase 3: Execution (Worker) ---
			executionLog := []string{}
			for i, task := range validTasks {
				// Process the task args to substitute any references to previous tool results
				processedArgs, processErr := processPreviousToolResults(string(task.Args), toolResults)
				if processErr != nil {
					logMsg := fmt.Sprintf("Task %d (%s) argument processing failed: %v", i+1, task.Tool, processErr)
					executionLog = append(executionLog, logMsg)
					return mcp.NewToolResponse(mcp.NewTextContent(logMsg)), nil
				}

				// Call the tool with the processed arguments
				out, toolErr := callToolInServer(task.Tool, processedArgs)
				if toolErr != nil {
					logMsg := fmt.Sprintf("Task %d (%s) failed: %v", i+1, task.Tool, toolErr)
					executionLog = append(executionLog, logMsg)
					log.Printf("Execution log so far: %v", executionLog)
					// Don't append to conversation since we're returning immediately
					return mcp.NewToolResponse(mcp.NewTextContent(logMsg)), nil
				}

				// Store the tool result for potential future reference
				resultKey := fmt.Sprintf("%d", i)
				toolResults[resultKey] = out

				logMsg := fmt.Sprintf("Task %d (%s) succeeded: %s", i+1, task.Tool, out)
				executionLog = append(executionLog, logMsg)
				// Append tool response to conversation.
				conversation = append(conversation, ChatCompletionMsg{Role: "assistant", Content: logMsg})
			}
			log.Printf("Final Execution Log: %v", executionLog)

			// --- Phase 4: Finalization ---
			finalPrompt := "Available Tools:\n" + toolsJSON + "\n\n" +
				"The following tasks were executed in order:\n" + strings.Join(executionLog, "\n") +
				"\nBased on these results, provide a final summary answer for the user's query: \"" + args.Query + "\". " +
				"Output only the final summary. If the plan is not yet complete, include additional steps."
			finalMessages := []ChatCompletionMsg{
				{Role: "system", Content: "You are a finalizing assistant. Use the provided tools list and execution log to generate a final summary or further steps."},
				{Role: "user", Content: finalPrompt},
			}
			finalAnswer, err := callOpenAI(finalMessages)
			if err != nil {
				return nil, fmt.Errorf("finalization phase failed: %v", err)
			}

			// Check termination: if the final answer clearly states a final summary or does not instruct any further actions, then return it.
			if isFinal(finalAnswer) {
				return mcp.NewToolResponse(mcp.NewTextContent(finalAnswer)), nil
			}
			// Otherwise, append the final answer to conversation and iterate.
			conversation = append(conversation, ChatCompletionMsg{Role: "assistant", Content: finalAnswer})
		}
		return mcp.NewToolResponse(mcp.NewTextContent("Insufficient context gathered to answer the query (max iterations reached).")), nil
	}
}

// processPreviousToolResults processes the JSON args string and substitutes any references to previous tool results.
// It looks for patterns like $TOOL_RESULT[index] and replaces them with the actual tool result.
func processPreviousToolResults(argsJSON string, toolResults map[string]string) (string, error) {
	// Define a regex pattern to match $TOOL_RESULT[index]
	pattern := regexp.MustCompile(`\$TOOL_RESULT\[(\d+)\]`)

	// First check if there are any matches to process
	if !pattern.MatchString(argsJSON) {
		return argsJSON, nil
	}

	// Parse the JSON to work with its structure
	var argsMap map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &argsMap); err != nil {
		return "", fmt.Errorf("failed to parse args JSON for substitution: %v", err)
	}

	// Process all string values in the map
	processJSONValue(argsMap, pattern, toolResults)

	// Convert back to JSON
	processedJSON, err := json.Marshal(argsMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal processed args: %v", err)
	}

	return string(processedJSON), nil
}

// processJSONValue recursively processes values in a map or slice to substitute tool results
func processJSONValue(value interface{}, pattern *regexp.Regexp, toolResults map[string]string) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		// Process each key-value pair in the map
		for key, val := range v {
			v[key] = processJSONValue(val, pattern, toolResults)
		}
		return v
	case []interface{}:
		// Process each element in the slice
		for i, val := range v {
			v[i] = processJSONValue(val, pattern, toolResults)
		}
		return v
	case string:
		// Replace any $TOOL_RESULT[index] in the string
		return pattern.ReplaceAllStringFunc(v, func(match string) string {
			// Extract the index from the match
			submatches := pattern.FindStringSubmatch(match)
			if len(submatches) < 2 {
				return match // Keep original if no index found
			}
			index := submatches[1]

			// Get the result for the index
			result, exists := toolResults[index]
			if !exists {
				log.Printf("Warning: Tool result for index %s not found", index)
				return match // Keep original if result not found
			}

			return result
		})
	default:
		// Return unchanged for other types
		return v
	}
}

// isFinal returns true if the final answer is considered complete.
// For example, if it starts with "FINAL_ANSWER:" or does not mention further tool calls.
func isFinal(answer string) bool {
	lower := strings.ToLower(answer)
	if strings.HasPrefix(lower, "final_answer:") {
		return true
	}
	// If the answer does not contain any indication of further actions, consider it final.
	if !strings.Contains(lower, "call_tool:") && !strings.Contains(lower, "next step") {
		return true
	}
	return false
}
