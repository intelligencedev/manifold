package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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
	Latitude  float64 `json:"required,description=Latitude" json:"latitude"`
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
type GenerateAndRunCodeArgs struct {
	Spec         string   `json:"spec" jsonschema:"required,description=Description or purpose of the code to generate"`
	Language     string   `json:"language" jsonschema:"required,enum=python,enum=go,enum=javascript,description=Which language to generate and run"`
	Dependencies []string `json:"dependencies,omitempty" jsonschema:"description=Optional list of dependencies for the chosen language"`
}
type AgentArgs struct {
	Query    string `json:"query" jsonschema:"required,description=User's query"`
	MaxCalls int    `json:"maxCalls" jsonschema:"required,description=Maximum LLM calls allowed"`
}

// =====================
// Tool Implementations
// =====================

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
	err := os.WriteFile(args.Path, []byte(args.Content), 0644)
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

// git_push tool
func gitPushTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return runGitCommand(args.Path, "push")
}

// git_pull tool
func gitPullTool(args GitRepoArgs) (string, error) {
	if err := checkGitRepo(args.Path); err != nil {
		return "", err
	}
	return runGitCommand(args.Path, "pull", "--rebase")
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
func listAllowedDirectoriesTool(_ ListAllowedDirectoriesArgs) (string, error) {
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
	// This is a simplified implementation
	return fmt.Sprintf("Search results for: %s", args.Query), nil
}

// web_content tool
func webContentTool(args WebContentArgs) (string, error) {
	if len(args.URLs) == 0 {
		return "", fmt.Errorf("no URLs provided")
	}

	var content strings.Builder
	for _, url := range args.URLs {
		resp, err := http.Get(url)
		if err != nil {
			content.WriteString(fmt.Sprintf("Error fetching %s: %v\n", url, err))
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			content.WriteString(fmt.Sprintf("Error reading response from %s: %v\n", url, err))
			continue
		}

		content.WriteString(fmt.Sprintf("Content from %s:\n%s\n\n", url, string(body)))
	}

	return content.String(), nil
}

// Helper functions
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

// Helper functions for git operations
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
