package main

import (
	"bufio"
	"bytes"
	"context"
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
// Enhanced Argument Types with Better Validation
// =====================

type CalculateArgs struct {
	Operation string  `json:"operation" jsonschema:"required,enum=add,enum=subtract,enum=multiply,enum=divide,description=Mathematical operation to perform. 'add' for addition (+), 'subtract' for subtraction (-), 'multiply' for multiplication (×), 'divide' for division (÷). Example: operation='add' with a=5, b=3 gives result 8"`
	A         float64 `json:"a" jsonschema:"required,description=First number for the calculation. Can be positive, negative, or decimal. Example: 5.5"`
	B         float64 `json:"b" jsonschema:"required,description=Second number for the calculation. Cannot be zero for division. Example: 2.3"`
}

type TimeArgs struct {
	Format   string `json:"format,omitempty" jsonschema:"description=Time format string using Go time constants. Leave empty for RFC3339 (2006-01-02T15:04:05Z07:00). Common formats: '2006-01-02' for date only, '15:04:05' for time only, 'Mon Jan 2 15:04:05 2006' for readable format"`
	Timezone string `json:"timezone,omitempty" jsonschema:"description=Timezone name for time conversion. Examples: 'UTC', 'America/New_York', 'Europe/London', 'Asia/Tokyo'. Defaults to local timezone if not specified"`
}

type WeatherArgs struct {
	Longitude float64 `json:"longitude" jsonschema:"required,minimum=-180,maximum=180,description=Longitude in decimal degrees (range: -180 to 180). Example: -122.4194 for San Francisco"`
	Latitude  float64 `json:"latitude" jsonschema:"required,minimum=-90,maximum=90,description=Latitude in decimal degrees (range: -90 to 90). Example: 37.7749 for San Francisco"`
	Units     string  `json:"units,omitempty" jsonschema:"enum=metric,enum=imperial,description=Temperature units. 'metric' for Celsius (default), 'imperial' for Fahrenheit"`
}

type GitRepoArgs struct {
	Path   string `json:"path" jsonschema:"required,description=Absolute path to an existing Git repository directory. Must contain a .git folder. Example: '/home/user/myproject'"`
	Branch string `json:"branch,omitempty" jsonschema:"description=Git branch name for operations. If specified, operations will target this branch. Example: 'main', 'develop', 'feature/new-feature'"`
	Remote string `json:"remote,omitempty" jsonschema:"description=Git remote name. Defaults to 'origin' if not specified. Example: 'origin', 'upstream'"`
}

type GitCloneArgs struct {
	RepoURL   string `json:"repoUrl" jsonschema:"required,pattern=^(https?|git|ssh)://.*|.*@.*:.*,description=URL of the Git repository to clone. Supports HTTPS, SSH, and Git protocols. Example: 'https://github.com/user/repo.git'"`
	Path      string `json:"path" jsonschema:"required,description=Local directory path where the repository will be cloned. Parent directories will be created if they don't exist. Example: '/home/user/projects/newrepo'"`
	Branch    string `json:"branch,omitempty" jsonschema:"description=Specific branch to clone. If not specified, clones the default branch. Example: 'main', 'develop'"`
	Depth     int    `json:"depth,omitempty" jsonschema:"minimum=1,description=Create a shallow clone with limited history depth. Use 1 for minimal clone (latest commit only), omit for full history"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"description=Whether to recursively clone submodules. Set to true if the repository contains submodules"`
}

type ShellCommandArgs struct {
	Command []string `json:"command" jsonschema:"required"`
	Dir     string   `json:"dir" jsonschema:"required"`
}

type CLIToolArgs struct {
	Command     string            `json:"command" jsonschema:"required,minLength=1,description=Command to execute. Use full command with arguments as a single string. Example: 'ls -la /tmp', 'git status'"`
	Dir         string            `json:"dir,omitempty" jsonschema:"description=Working directory for command execution. Defaults to current directory if not specified. Example: '/home/user/project'"`
	Timeout     int               `json:"timeout,omitempty" jsonschema:"minimum=1,maximum=300,description=Command timeout in seconds (1-300). Defaults to 30 seconds if not specified"`
	Environment map[string]string `json:"environment,omitempty" jsonschema:"description=Additional environment variables as key-value pairs. Example: {'NODE_ENV': 'production', 'API_KEY': 'secret'}"`
}

type GoBuildArgs struct {
	Path       string   `json:"path" jsonschema:"required,description=Directory path containing Go code with go.mod file. Example: '/home/user/goproject'"`
	Output     string   `json:"output,omitempty" jsonschema:"description=Output binary file path. If not specified, uses default naming. Example: './bin/myapp'"`
	BuildFlags []string `json:"buildFlags,omitempty" jsonschema:"description=Additional build flags as array. Example: ['-v', '-race'] for verbose output with race detection"`
	Target     string   `json:"target,omitempty" jsonschema:"description=Specific package to build. Use '.' for current directory, './...' for all packages. Example: './cmd/server'"`
}

type GoTestArgs struct {
	Path        string `json:"path" jsonschema:"required,description=Directory path containing Go tests. Example: '/home/user/goproject'"`
	Package     string `json:"package,omitempty" jsonschema:"description=Specific package to test. Use './...' for all packages (default). Example: './internal/handler'"`
	TestPattern string `json:"testPattern,omitempty" jsonschema:"description=Run only tests matching this pattern. Example: 'TestUser', 'Test.*Integration'"`
	Verbose     bool   `json:"verbose,omitempty" jsonschema:"description=Enable verbose test output showing all test names and results"`
	Race        bool   `json:"race,omitempty" jsonschema:"description=Enable race condition detection during tests"`
}

type FormatGoCodeArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path containing Go code to format. Example: '/home/user/goproject'"`
}

type LintCodeArgs struct {
	Path       string `json:"path" jsonschema:"required,description=Path to file or directory to lint. Example: '/home/user/project' or '/home/user/project/main.go'"`
	LinterName string `json:"linterName,omitempty" jsonschema:"enum=golangci-lint,enum=staticcheck,enum=vet,description=Specific linter to use. Defaults to 'golangci-lint' for Go code. Example: 'golangci-lint'"`
	Fix        bool   `json:"fix,omitempty" jsonschema:"description=Attempt to automatically fix issues where possible"`
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
	Operation   string `json:"operation" jsonschema:"required,enum=read,enum=read_range,enum=search,enum=replace_line,enum=replace_range,enum=apply_patch,description=File operation type. 'read' reads entire file, 'read_range' reads specific lines, 'search' finds patterns, 'replace_line' replaces single line, 'replace_range' replaces line range, 'apply_patch' applies unified diff"`
	Path        string `json:"path" jsonschema:"required,description=Absolute or workspace-relative file path. Example: '/home/user/file.txt' or 'src/main.go'"`
	Start       int    `json:"start,omitempty" jsonschema:"minimum=1,description=Start line number (1-based) for range operations. Example: 10"`
	End         int    `json:"end,omitempty" jsonschema:"minimum=1,description=End line number (1-based, inclusive) for range operations. Example: 20"`
	Pattern     string `json:"pattern,omitempty" jsonschema:"description=Search pattern (regex or literal text). Use regex for complex patterns. Example: 'func.*main', 'TODO:'"`
	Replacement string `json:"replacement,omitempty" jsonschema:"description=Replacement text for replace operations. Use \\n for line breaks. Example: 'new function() {\\n\\treturn true;\\n}'"`
	Patch       string `json:"patch,omitempty" jsonschema:"description=Unified diff patch content for apply_patch operation"`
}

// =====================
// Enhanced Tool Implementations with Better Error Handling
// =====================

// Enhanced calculate tool with comprehensive validation
func calculateTool(args CalculateArgs) (string, error) {
	// Validate operation
	validOps := map[string]bool{"add": true, "subtract": true, "multiply": true, "divide": true}
	if !validOps[args.Operation] {
		return "", fmt.Errorf("invalid operation '%s'. Supported operations: add, subtract, multiply, divide. Example: use 'add' to compute 5 + 3 = 8", args.Operation)
	}

	// Check for division by zero
	if args.Operation == "divide" && args.B == 0 {
		return "", fmt.Errorf("division by zero is not allowed. Please provide a non-zero value for parameter 'b'. Example: a=10, b=2 gives result 5")
	}

	var result float64
	var symbol string
	switch args.Operation {
	case "add":
		result = args.A + args.B
		symbol = "+"
	case "subtract":
		result = args.A - args.B
		symbol = "-"
	case "multiply":
		result = args.A * args.B
		symbol = "×"
	case "divide":
		result = args.A / args.B
		symbol = "÷"
	}

	return fmt.Sprintf("Calculation: %.6g %s %.6g = %.6g", args.A, symbol, args.B, result), nil
}

// Enhanced time tool with timezone support
func timeTool(args TimeArgs) (string, error) {
	format := time.RFC3339
	if args.Format != "" {
		format = args.Format
	}

	var loc *time.Location
	var err error
	if args.Timezone != "" {
		loc, err = time.LoadLocation(args.Timezone)
		if err != nil {
			return "", fmt.Errorf("invalid timezone '%s': %w. Use format like 'UTC', 'America/New_York', or 'Europe/London'", args.Timezone, err)
		}
	} else {
		loc = time.Local
	}

	now := time.Now().In(loc)
	formatted := now.Format(format)

	return fmt.Sprintf("Current time: %s (timezone: %s)", formatted, loc.String()), nil
}

// Enhanced weather tool with better error handling and validation
func getWeatherTool(args WeatherArgs) (string, error) {
	// Validate coordinates
	if args.Latitude < -90 || args.Latitude > 90 {
		return "", fmt.Errorf("invalid latitude %.6f. Must be between -90 and 90. Example: 37.7749 for San Francisco", args.Latitude)
	}
	if args.Longitude < -180 || args.Longitude > 180 {
		return "", fmt.Errorf("invalid longitude %.6f. Must be between -180 and 180. Example: -122.4194 for San Francisco", args.Longitude)
	}

	// Build API URL with units
	units := "metric"
	if args.Units == "imperial" {
		units = "fahrenheit"
	}

	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,wind_speed_10m,weather_code&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m&temperature_unit=%s",
		args.Latitude, args.Longitude, units)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch weather data: %w. Please check your internet connection", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("weather API returned status %d. The coordinates might be invalid or the service is temporarily unavailable", resp.StatusCode)
	}

	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read weather response: %w", err)
	}

	return fmt.Sprintf("Weather data for coordinates (%.6f, %.6f) in %s units:\n%s", args.Latitude, args.Longitude, units, string(output)), nil
}

// Enhanced git operations with better validation and error messages
func gitPushTool(args GitRepoArgs) (string, error) {
	if err := validateGitRepo(args.Path); err != nil {
		return "", err
	}

	// Check if there are changes to push
	statusOutput, err := runGitCommand(args.Path, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("failed to check git status: %w", err)
	}

	// Check for uncommitted changes
	if strings.TrimSpace(statusOutput) != "" {
		return "", fmt.Errorf("repository has uncommitted changes. Please commit or stash changes before pushing. Use 'git status' to see uncommitted changes")
	}

	// Determine remote and branch
	remote := "origin"
	if args.Remote != "" {
		remote = args.Remote
	}

	var pushArgs []string
	if args.Branch != "" {
		pushArgs = []string{"push", remote, args.Branch}
	} else {
		pushArgs = []string{"push"}
	}

	output, err := runGitCommand(args.Path, pushArgs...)
	if err != nil {
		if strings.Contains(err.Error(), "rejected") {
			return "", fmt.Errorf("push rejected by remote. Try running 'git pull' first to merge remote changes: %w", err)
		}
		if strings.Contains(err.Error(), "authentication") || strings.Contains(err.Error(), "Permission denied") {
			return "", fmt.Errorf("authentication failed. Check your Git credentials or SSH keys: %w", err)
		}
		return "", fmt.Errorf("git push failed: %w", err)
	}

	return fmt.Sprintf("Successfully pushed to %s:\n%s", remote, output), nil
}

func gitPullTool(args GitRepoArgs) (string, error) {
	if err := validateGitRepo(args.Path); err != nil {
		return "", err
	}

	// Determine remote and branch
	remote := "origin"
	if args.Remote != "" {
		remote = args.Remote
	}

	var pullArgs []string
	if args.Branch != "" {
		pullArgs = []string{"pull", "--rebase", remote, args.Branch}
	} else {
		pullArgs = []string{"pull", "--rebase"}
	}

	output, err := runGitCommand(args.Path, pullArgs...)
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			return "", fmt.Errorf("merge conflicts detected during pull. Resolve conflicts manually and commit, or run 'git rebase --abort' to cancel: %w", err)
		}
		if strings.Contains(err.Error(), "authentication") {
			return "", fmt.Errorf("authentication failed. Check your Git credentials: %w", err)
		}
		return "", fmt.Errorf("git pull failed: %w", err)
	}

	return fmt.Sprintf("Successfully pulled from %s:\n%s", remote, output), nil
}

// Enhanced git clone with comprehensive options and validation
func gitCloneTool(args GitCloneArgs) (string, error) {
	// Validate repository URL
	if !isValidRepoURL(args.RepoURL) {
		return "", fmt.Errorf("invalid repository URL '%s'. Must be a valid HTTPS, SSH, or Git URL. Examples: 'https://github.com/user/repo.git', 'git@github.com:user/repo.git'", args.RepoURL)
	}

	// Validate and prepare target path
	if args.Path == "" {
		return "", fmt.Errorf("target path is required. Example: '/home/user/projects/myrepo'")
	}

	// Check if target directory already exists and is not empty
	if info, err := os.Stat(args.Path); err == nil {
		if info.IsDir() {
			entries, err := os.ReadDir(args.Path)
			if err != nil {
				return "", fmt.Errorf("cannot read target directory: %w", err)
			}
			if len(entries) > 0 {
				return "", fmt.Errorf("target directory '%s' already exists and is not empty. Choose a different path or remove existing files", args.Path)
			}
		} else {
			return "", fmt.Errorf("target path '%s' exists but is not a directory", args.Path)
		}
	}

	// Build clone command
	cmdArgs := []string{"clone"}

	if args.Depth > 0 {
		cmdArgs = append(cmdArgs, "--depth", fmt.Sprintf("%d", args.Depth))
	}

	if args.Recursive {
		cmdArgs = append(cmdArgs, "--recursive")
	}

	if args.Branch != "" {
		cmdArgs = append(cmdArgs, "--branch", args.Branch)
	}

	cmdArgs = append(cmdArgs, args.RepoURL, args.Path)

	// Execute with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git clone timed out after 5 minutes. The repository might be very large or network is slow")
		}
		if strings.Contains(err.Error(), "authentication") {
			return "", fmt.Errorf("authentication failed. Check repository permissions and credentials: %w\nOutput: %s", err, string(output))
		}
		return "", fmt.Errorf("git clone failed: %w\nOutput: %s\nCommand: git %s", err, string(output), strings.Join(cmdArgs, " "))
	}

	return fmt.Sprintf("Successfully cloned repository to %s:\n%s", args.Path, string(output)), nil
}

// Enhanced CLI tool with timeout and environment support
func cliTool(args CLIToolArgs) (string, error) {
	if strings.TrimSpace(args.Command) == "" {
		return "", fmt.Errorf("command cannot be empty. Example: 'ls -la', 'git status'")
	}

	// Set default timeout
	timeout := 30
	if args.Timeout > 0 {
		timeout = args.Timeout
	}

	// Create command with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)

	// Set working directory
	if args.Dir != "" {
		if _, err := os.Stat(args.Dir); os.IsNotExist(err) {
			return "", fmt.Errorf("working directory '%s' does not exist. Please provide a valid directory path", args.Dir)
		}
		cmd.Dir = args.Dir
	}

	// Set environment variables
	if args.Environment != nil {
		env := os.Environ()
		for key, value := range args.Environment {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		cmd.Env = env
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %d seconds. Consider increasing timeout or optimizing the command", timeout)
		}
		// Include both error and output for better debugging
		return "", fmt.Errorf("command failed: %w\nCommand: %s\nOutput: %s", err, args.Command, string(output))
	}

	return fmt.Sprintf("Command executed successfully:\n%s", string(output)), nil
}

// Enhanced Go build tool
func goBuildTool(args GoBuildArgs) (string, error) {
	// Validate path exists
	if _, err := os.Stat(args.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("path '%s' does not exist. Please provide a valid Go project directory", args.Path)
	}

	// Check for go.mod
	modPath := filepath.Join(args.Path, "go.mod")
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		return "", fmt.Errorf("go.mod not found in '%s'. This directory is not a Go module. Run 'go mod init <module-name>' first", args.Path)
	}

	cmdArgs := []string{"build"}
	cmdArgs = append(cmdArgs, args.BuildFlags...)

	if args.Output != "" {
		cmdArgs = append(cmdArgs, "-o", args.Output)
	}

	if args.Target != "" {
		cmdArgs = append(cmdArgs, args.Target)
	}

	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("go build failed: %w\nOutput: %s\nCommand: go %s", err, string(output), strings.Join(cmdArgs, " "))
	}

	if len(output) == 0 {
		return "Go build completed successfully (no output)", nil
	}

	return fmt.Sprintf("Go build completed successfully:\n%s", string(output)), nil
}

// Enhanced Go test tool
func goTestTool(args GoTestArgs) (string, error) {
	// Validate path exists
	if _, err := os.Stat(args.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("path '%s' does not exist. Please provide a valid Go project directory", args.Path)
	}

	cmdArgs := []string{"test"}

	if args.Verbose {
		cmdArgs = append(cmdArgs, "-v")
	}

	if args.Race {
		cmdArgs = append(cmdArgs, "-race")
	}

	if args.TestPattern != "" {
		cmdArgs = append(cmdArgs, "-run", args.TestPattern)
	}

	target := "./..."
	if args.Package != "" {
		target = args.Package
	}
	cmdArgs = append(cmdArgs, target)

	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Test failures are normal, include output
		return fmt.Sprintf("Go test completed with failures:\n%s", string(output)), nil
	}

	return fmt.Sprintf("Go test completed successfully:\n%s", string(output)), nil
}

// Enhanced format Go code tool
func formatGoCodeTool(args FormatGoCodeArgs) (string, error) {
	if _, err := os.Stat(args.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("path '%s' does not exist. Please provide a valid Go project directory", args.Path)
	}

	cmd := exec.Command("go", "fmt", "./...")
	cmd.Dir = args.Path
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("go fmt failed: %w\nOutput: %s", err, string(output))
	}

	if len(output) == 0 {
		return "Go code formatting completed successfully (no changes needed)", nil
	}

	return fmt.Sprintf("Go code formatting completed:\n%s", string(output)), nil
}

// Enhanced lint code tool
func lintCodeTool(args LintCodeArgs) (string, error) {
	if _, err := os.Stat(args.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("path '%s' does not exist. Please provide a valid file or directory path", args.Path)
	}

	linter := "golangci-lint"
	if args.LinterName != "" {
		linter = args.LinterName
	}

	cmdArgs := []string{"run"}
	if args.Fix {
		cmdArgs = append(cmdArgs, "--fix")
	}
	cmdArgs = append(cmdArgs, args.Path)

	cmd := exec.Command(linter, cmdArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Linting errors are expected, include output
		return fmt.Sprintf("Linting completed with issues:\n%s", string(output)), nil
	}

	if len(output) == 0 {
		return "Linting completed successfully (no issues found)", nil
	}

	return fmt.Sprintf("Linting completed:\n%s", string(output)), nil
}

// =====================
// Enhanced file_tool – unified high‑precision file operations
// =====================

func fileTool(args FileToolArgs) (string, error) {
	switch args.Operation {

	case "read":
		if _, err := os.Stat(args.Path); os.IsNotExist(err) {
			return "", fmt.Errorf("file '%s' does not exist. Please check the file path", args.Path)
		}
		data, err := os.ReadFile(args.Path)
		if err != nil {
			return "", fmt.Errorf("failed to read file '%s': %w", args.Path, err)
		}
		lineCount := strings.Count(string(data), "\n") + 1
		return fmt.Sprintf("File content (%d lines):\n%s", lineCount, string(data)), nil

	case "read_range":
		if args.Start <= 0 || args.End < args.Start {
			return "", fmt.Errorf("invalid line range. Start must be > 0 and End must be >= Start. Example: start=1, end=10")
		}
		data, err := os.ReadFile(args.Path)
		if err != nil {
			return "", fmt.Errorf("failed to read file '%s': %w", args.Path, err)
		}
		lines := bytes.Split(data, []byte("\n"))
		if args.Start > len(lines) {
			return "", fmt.Errorf("start line %d exceeds file length (%d lines)", args.Start, len(lines))
		}
		if args.End > len(lines) {
			args.End = len(lines)
		}
		var out strings.Builder
		for i := args.Start; i <= args.End; i++ {
			fmt.Fprintf(&out, "%d: %s\n", i, lines[i-1])
		}
		return fmt.Sprintf("Lines %d-%d of %s:\n%s", args.Start, args.End, args.Path, out.String()), nil

	case "search":
		if args.Pattern == "" {
			return "", fmt.Errorf("search pattern is required. Example: 'func main', 'TODO:', or regex like 'func.*Test'")
		}

		file, err := os.Open(args.Path)
		if err != nil {
			return "", fmt.Errorf("failed to open file '%s': %w", args.Path, err)
		}
		defer file.Close()

		// Try to compile as regex, fall back to literal search
		var regex *regexp.Regexp
		regex, err = regexp.Compile(args.Pattern)
		if err != nil {
			// Use literal search
			regex = regexp.MustCompile(regexp.QuoteMeta(args.Pattern))
		}

		var matches strings.Builder
		scanner := bufio.NewScanner(file)
		lineNum := 0
		matchCount := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if regex.MatchString(line) {
				matchCount++
				fmt.Fprintf(&matches, "%d: %s\n", lineNum, line)
			}
		}

		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("error reading file '%s': %w", args.Path, err)
		}

		if matchCount == 0 {
			return fmt.Sprintf("No matches found for pattern '%s' in file '%s'", args.Pattern, args.Path), nil
		}

		return fmt.Sprintf("Found %d matches for pattern '%s' in file '%s':\n%s", matchCount, args.Pattern, args.Path, matches.String()), nil

	case "replace_line":
		if args.Start <= 0 || args.Replacement == "" {
			return "", fmt.Errorf("start line number and replacement text are required. Example: start=5, replacement='new line content'")
		}
		return replaceRange(args.Path, args.Start, args.Start, args.Replacement)

	case "replace_range":
		if args.Start <= 0 || args.End < args.Start || args.Replacement == "" {
			return "", fmt.Errorf("valid start/end line numbers and replacement text are required. Example: start=1, end=3, replacement='new content'")
		}
		return replaceRange(args.Path, args.Start, args.End, args.Replacement)

	case "apply_patch":
		if args.Patch == "" {
			return "", fmt.Errorf("patch content is required for apply_patch operation")
		}
		cmd := exec.Command("patch", args.Path)
		cmd.Stdin = strings.NewReader(args.Patch)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("patch command failed: %w\nOutput: %s", err, out)
		}
		return fmt.Sprintf("Patch applied successfully to %s:\n%s", args.Path, string(out)), nil

	default:
		return "", fmt.Errorf("unknown operation '%s'. Supported operations: read, read_range, search, replace_line, replace_range, apply_patch", args.Operation)
	}
}

// Enhanced helper function with better error handling
func replaceRange(path string, start, end int, replacement string) (string, error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", path, err)
	}

	lines := bytes.Split(input, []byte("\n"))
	totalLines := len(lines)

	if start > totalLines {
		return "", fmt.Errorf("start line %d exceeds file length (%d lines)", start, totalLines)
	}
	if end > totalLines {
		end = totalLines
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
		return "", fmt.Errorf("failed to write file '%s': %w", path, err)
	}

	return fmt.Sprintf("Successfully updated file %s (lines %d-%d replaced)", path, start, end), nil
}

// =====================
// Enhanced Helper Functions
// =====================

func validateGitRepo(repoPath string) error {
	if repoPath == "" {
		return fmt.Errorf("repository path is required")
	}

	info, err := os.Stat(repoPath)
	if err != nil {
		return fmt.Errorf("cannot access path '%s': %w. Please check that the path exists", repoPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path '%s' is not a directory. Git repositories must be directories", repoPath)
	}

	gitDir := filepath.Join(repoPath, ".git")
	gitInfo, err := os.Stat(gitDir)
	if err != nil {
		return fmt.Errorf("path '%s' is not a Git repository (missing .git directory). Initialize with 'git init' or clone a repository", repoPath)
	}
	if !gitInfo.IsDir() {
		return fmt.Errorf("path '%s' has invalid .git (not a directory). Repository may be corrupted", repoPath)
	}

	return nil
}

func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %w\nCommand: git %s\nOutput: %s", err, strings.Join(args, " "), string(output))
	}
	return string(output), nil
}

func isValidRepoURL(url string) bool {
	patterns := []string{
		`^https?://.*\.git$`,
		`^https?://github\.com/[^/]+/[^/]+/?$`,
		`^https?://gitlab\.com/[^/]+/[^/]+/?$`,
		`^https?://bitbucket\.org/[^/]+/[^/]+/?$`,
		`^git@.*:.*\.git$`,
		`^ssh://.*\.git$`,
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, url); matched {
			return true
		}
	}
	return false
}

func checkGitRepo(repoPath string) error {
	return validateGitRepo(repoPath)
}
