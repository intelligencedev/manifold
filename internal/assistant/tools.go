package assistant

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
	"strings"
	"time"
)

// ---------------------------------------------------------------------
// Common Types
// ---------------------------------------------------------------------

// ToolRequest represents a request to a tool.
type ToolRequest struct {
	Command string   `json:"command"` // Command to execute.
	Args    []string `json:"args"`    // Arguments to the command.
}

// ToolResponse represents a response from a tool.
type ToolResponse struct {
	Output string `json:"output"` // Output of the command.
	Error  string `json:"error"`  // Error message, if any.
}

// Tool interface defines the methods that a tool must implement.
type Tool interface {
	// Execute sends a request to the tool and returns the response.
	Execute(req *ToolRequest) (*ToolResponse, error)
}

// ---------------------------------------------------------------------
// Tool Argument Structs
// ---------------------------------------------------------------------

// HelloArgs represents the arguments for the hello tool.
type HelloArgs struct {
	Name string `json:"name" jsonschema:"required,description=The name to say hello to"`
}

// CalculateArgs represents the arguments for the calculate tool.
type CalculateArgs struct {
	Operation string  `json:"operation" jsonschema:"required,enum=add,enum=subtract,enum=multiply,enum=divide,description=The mathematical operation to perform"`
	A         float64 `json:"a" jsonschema:"required,description=First number"`
	B         float64 `json:"b" jsonschema:"required,description=Second number"`
}

// TimeArgs represents the arguments for the current time tool.
type TimeArgs struct {
	Format string `json:"format,omitempty" jsonschema:"description=Optional time format (default: RFC3339)"`
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
	return runCommand("git", args.Path, "init")
}

// GitStatusTool returns the Git status.
func gitStatusTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return runCommand("git", args.Path, "status", "--short", "--branch")
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
	return runCommand("git", args.Path, fullArgs...)
}

// GitCommitTool commits changes in a Git repository.
func gitCommitTool(args GitCommitArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Message) == "" {
		return "", fmt.Errorf("commit message cannot be empty")
	}
	return runCommand("git", args.Path, "commit", "-m", args.Message)
}

// GitPullTool pulls changes from a remote repository.
func gitPullTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return runCommand("git", args.Path, "pull", "--rebase")
}

// GitPushTool pushes changes to a remote repository.
func gitPushTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return runCommand("git", args.Path, "push")
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
	return runCommand("git", "", "clone", args.RepoURL, args.Path)
}

// GitCheckoutTool checks out a Git branch.
func gitCheckoutTool(args GitCheckoutArgs) (string, error) {
	var cmdArgs []string
	if args.CreateNew {
		cmdArgs = []string{"checkout", "-b", args.Branch}
	} else {
		cmdArgs = []string{"checkout", args.Branch}
	}
	return runCommand("git", args.Path, cmdArgs...)
}

// GitDiffTool shows Git diff between two references.
func gitDiffTool(args GitDiffArgs) (string, error) {
	diffArgs := []string{"diff"}
	if args.FromRef != "" && args.ToRef != "" {
		diffArgs = append(diffArgs, args.FromRef, args.ToRef)
	}
	return runCommand("git", args.Path, diffArgs...)
}

// RunShellCommandTool executes a shell command.
func runShellCommandTool(args ShellCommandArgs) (string, error) {
	if len(args.Command) == 0 {
		return "", fmt.Errorf("empty command")
	}
	return runCommand(args.Command[0], args.Dir, args.Command[1:]...)
}

// GoBuildTool builds a Go module.
func goBuildTool(args GoBuildArgs) (string, error) {
	return runCommand("go", args.Path, "build")
}

// GoTestTool runs tests for a Go module.
func goTestTool(args GoTestArgs) (string, error) {
	return runCommand("go", args.Path, "test", "./...")
}

// FormatGoCodeTool formats Go code.
func formatGoCodeTool(args FormatGoCodeArgs) (string, error) {
	return runCommand("go", args.Path, "fmt", "./...")
}

// LintCodeTool lints code using a specified linter.
func lintCodeTool(args LintCodeArgs) (string, error) {
	if args.LinterName != "" {
		return runCommand(args.LinterName, args.Path, "run")
	}
	return runCommand("golangci-lint", args.Path, "run")
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
