package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// =====================
// Argument Types
// =====================

type CalculateArgs struct {
	Operation string  `json:"operation" jsonschema:"required,enum=add,enum=subtract,enum=multiply,enum=divide,description=The mathematical operation to perform"`
	A         float64 `json:"a" jsonschema:"required,description=First number"`
	B         float64 `json:"b" jsonschema:"required,description=Second number"`
}

type TimeArgs struct {
	Format string `json:"format,omitempty" jsonschema:"description=Optional time format (default: RFC3339)"`
}

type WeatherArgs struct {
	Longitude float64 `json:"longitude" jsonschema:"required,description=Longitude in decimal degrees"`
	Latitude  float64 `json:"latitude"  jsonschema:"required,description=Latitude in decimal degrees"`
}

type GitRepoArgs struct {
	Path string `json:"path" jsonschema:"required,description=Local path to an existing Git repo"`
}

// Git Tools

type GitCloneArgs struct {
	RepoURL string `json:"repoUrl" jsonschema:"required"`
	Path    string `json:"path" jsonschema:"required"`
}

type ShellCommandArgs struct {
	Command []string `json:"command" jsonschema:"required"`
	Dir     string   `json:"dir" jsonschema:"required"`
}

// CLIToolArgs defines the input for the cli tool which simply passes a raw
// command string to the underlying shell.
type CLIToolArgs struct {
	Command string `json:"command" jsonschema:"required,description=Raw CLI command to execute"`
	Dir     string `json:"dir,omitempty" jsonschema:"description=Optional working directory"`
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
	Query      string `json:"query" jsonschema:"required"`
	ResultSize int    `json:"result_size,omitempty"` // default 5, max 10
}

type WebContentArgs struct {
	URLs string `json:"urls" jsonschema:"required,description=Comma separated list of URLs"`
}

type GenerateAndRunCodeArgs struct {
	Spec         string   `json:"spec" jsonschema:"required,description=Description or purpose of the code to generate"`
	Language     string   `json:"language" jsonschema:"required,enum=python,enum=go,enum=javascript,description=Which language to generate and run"`
	Dependencies []string `json:"dependencies,omitempty" jsonschema:"description=Optional list of dependencies for the chosen language"`
}

type FileToolArgs struct {
	Operation   string `json:"operation"   jsonschema:"required,enum=read,enum=read_range,enum=search,enum=replace_line,enum=replace_range,enum=apply_patch"`
	Path        string `json:"path"        jsonschema:"required,description=Absolute or workspace-relative file path"`
	Start       int    `json:"start,omitempty"       jsonschema:"description=Start line (1-based) for range/replace operations"`
	End         int    `json:"end,omitempty"         jsonschema:"description=End line (inclusive) for range/replace operations"`
	Pattern     string `json:"pattern,omitempty"     jsonschema:"description=Regex or plain text to search for"`
	Replacement string `json:"replacement,omitempty" jsonschema:"description=Replacement text for replace_line / replace_range"`
	Patch       string `json:"patch,omitempty"       jsonschema:"description=Unified diff patch content for apply_patch"`
}

// =====================
// Tool Implementations
// =====================

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

// git_clone tool
func gitCloneTool(args GitCloneArgs) (string, error) {
	cmd := exec.Command("git", "clone", args.RepoURL, args.Path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
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

// cli tool
func cliTool(args CLIToolArgs) (string, error) {
	if strings.TrimSpace(args.Command) == "" {
		return "", fmt.Errorf("command required")
	}
	cmd := exec.Command("bash", "-c", args.Command)
	if args.Dir != "" {
		cmd.Dir = args.Dir
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cli command error: %w\nOutput: %s", err, output)
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

// =====================
// NEW: file_tool – unified high‑precision file operations
// =====================

func fileTool(args FileToolArgs) (string, error) {
	switch args.Operation {

	// ----------------------------
	// READ ENTIRE FILE
	// ----------------------------
	case "read":
		data, err := os.ReadFile(args.Path)
		if err != nil {
			return "", err
		}
		return string(data), nil // <-- FIX: convert []byte to string

	// ----------------------------
	// READ RANGE
	// ----------------------------
	case "read_range":
		if args.Start <= 0 || args.End < args.Start {
			return "", fmt.Errorf("invalid start/end")
		}
		data, err := os.ReadFile(args.Path)
		if err != nil {
			return "", err
		}
		lines := bytes.Split(data, []byte("\n"))
		if args.Start > len(lines) {
			return "", fmt.Errorf("start line beyond EOF")
		}
		if args.End > len(lines) {
			args.End = len(lines)
		}
		var out strings.Builder
		for i := args.Start; i <= args.End; i++ {
			fmt.Fprintf(&out, "%d: %s\n", i, lines[i-1])
		}
		return out.String(), nil // <-- FIX: return string, not []byte

	// ----------------------------
	// SEARCH
	// ----------------------------
	case "search":
		if args.Pattern == "" {
			return "", fmt.Errorf("pattern required")
		}
		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return "", fmt.Errorf("invalid regex: %w", err)
		}
		f, err := os.Open(args.Path)
		if err != nil {
			return "", err
		}
		defer f.Close()

		var out strings.Builder
		sc := bufio.NewScanner(f)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			if re.Match(sc.Bytes()) {
				fmt.Fprintf(&out, "%d: %s\n", lineNo, sc.Text())
			}
		}
		if err := sc.Err(); err != nil {
			return "", err
		}
		if out.Len() == 0 {
			return "no matches", nil
		}
		return out.String(), nil

	// ----------------------------
	// REPLACE SINGLE LINE
	// ----------------------------
	case "replace_line":
		if args.Start <= 0 || args.Replacement == "" {
			return "", fmt.Errorf("start line and replacement required")
		}
		return replaceRange(args.Path, args.Start, args.Start, args.Replacement)

	// ----------------------------
	// REPLACE RANGE
	// ----------------------------
	case "replace_range":
		if args.Start <= 0 || args.End < args.Start || args.Replacement == "" {
			return "", fmt.Errorf("start/end and replacement required")
		}
		return replaceRange(args.Path, args.Start, args.End, args.Replacement)

	// ----------------------------
	// APPLY PATCH
	// ----------------------------
	case "apply_patch":
		if args.Patch == "" {
			return "", fmt.Errorf("patch content required")
		}
		cmd := exec.Command("patch", args.Path)
		cmd.Stdin = strings.NewReader(args.Patch)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("patch command error: %w\nOutput: %s", err, out)
		}
		return string(out), nil

	default:
		return "", fmt.Errorf("unknown operation %q", args.Operation)
	}
}

// ---------- helper ----------
func replaceRange(path string, start, end int, replacement string) (string, error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := bytes.Split(input, []byte("\n"))
	if start > len(lines) {
		return "", fmt.Errorf("start line beyond EOF")
	}
	if end > len(lines) {
		end = len(lines)
	}
	var buf bytes.Buffer
	// lines BEFORE start
	for i := 0; i < start-1; i++ {
		buf.Write(lines[i])
		buf.WriteByte('\n')
	}
	// replacement
	buf.WriteString(replacement)
	if replacement != "" && replacement[len(replacement)-1] != '\n' {
		buf.WriteByte('\n')
	}
	// lines AFTER end
	for i := end; i < len(lines); i++ {
		buf.Write(lines[i])
		if i < len(lines)-1 {
			buf.WriteByte('\n')
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("file %s updated (lines %d-%d)", path, start, end), nil
}

// =====================
// Git helper functions
// =====================

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
