// manifold/code.go
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// CodeEvalRequest is the unified structure for code evaluation requests.
type CodeEvalRequest struct {
	Code         string   `json:"code"`
	Language     string   `json:"language"`               // "python", "go", or "javascript"
	Dependencies []string `json:"dependencies,omitempty"` // Optional dependencies
}

// CodeEvalResponse is the response returned after code execution.
type CodeEvalResponse struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// DockerExecResponse holds the output details from the Docker container execution.
type DockerExecResponse struct {
	ReturnCode int
	Stdout     string
	Stderr     string
}

// evaluateCodeHandler is the HTTP handler that dispatches based on language.
func evaluateCodeHandler(c echo.Context) error {
	var req CodeEvalRequest
	if err := c.Bind(&req); err != nil {
		// Note: Reading req.Language here might be unreliable if Bind fails early.
		// Log the error instead.
		log.Printf("Failed to bind request: %v", err)
		return c.JSON(http.StatusBadRequest, CodeEvalResponse{
			Error: "Invalid request body: " + err.Error(),
		})
	}

	log.Printf("Received language: [%s]", req.Language)

	var resp *CodeEvalResponse
	var err error

	switch strings.ToLower(req.Language) {
	case "python":
		resp, err = runPythonInContainer(req.Code, req.Dependencies)
	case "go":
		resp, err = runGoInContainer(req.Code, req.Dependencies)
	case "javascript":
		resp, err = runNodeInContainer(req.Code, req.Dependencies)
	default:
		return c.JSON(http.StatusBadRequest, CodeEvalResponse{
			Error: "Unsupported language: " + req.Language,
		})
	}

	if err != nil {
		// Log the internal error for debugging
		log.Printf("Error processing %s request: %v", req.Language, err)
		// Return a more generic error to the client
		return c.JSON(http.StatusInternalServerError, CodeEvalResponse{
			// Avoid leaking internal details like file paths if the error comes from os calls
			Error: "Internal server error during code execution.",
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// runPythonInContainer writes out the Python code and dependencies,
// then executes them using the multi-language Docker container.
func runPythonInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	// Create a temporary directory for mounting into the container.
	tempDir, err := os.MkdirTemp("", "sandbox_python_") // More specific prefix
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the Python code.
	codeFilePath := filepath.Join(tempDir, "user_code.py")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write python code: %w", err)
	}

	// Write dependencies (if any) into requirements.txt.
	reqFile := filepath.Join(tempDir, "requirements.txt")
	reqContent := ""
	if len(dependencies) > 0 {
		reqContent = strings.Join(dependencies, "\n")
	}
	// Write the file even if empty, pip install -r is fine with an empty file.
	if err := os.WriteFile(reqFile, []byte(reqContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write requirements.txt: %w", err)
	}

	// Build the docker run command.
	dockerArgs := []string{
		"run", "--rm",
		"--network", "none", // Disable networking for added security
		"--memory", "256m", // Limit memory
		"--cpus", "0.5", // Limit CPU usage
		"-v", fmt.Sprintf("%s:/sandbox:ro", tempDir), // Mount read-only
		"code-sandbox", // Ensure this image is built.
		"/bin/bash", "-c",
		// Install dependencies quietly, run code. Capture stderr from python3.
		"cd /sandbox && pip install -q -r requirements.txt && python3 user_code.py",
	}

	dresp, err := runDockerCommand(dockerArgs, 30*time.Second) // Reduced timeout
	if err != nil {
		// Don't return the raw error from runDockerCommand if it contains internal details
		log.Printf("Docker command execution failed: %v, stderr: %s", err, dresp.Stderr)
		// Check if the error is due to timeout
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("code execution timed out")
		}
		// For other errors, return a generic message potentially including stderr
		errMsg := "failed to execute python code in container"
		if dresp != nil && dresp.Stderr != "" {
			errMsg += fmt.Sprintf(": %s", dresp.Stderr)
		}
		return nil, errors.New(errMsg)
	}

	return convertDockerResponse(dresp), nil
}

// runGoInContainer writes out the Go code and sets up dependencies,
// then executes the code using the multi-language Docker container.
// MODIFIED: Redirects stderr of `go mod init` to prevent unwanted warnings.
func runGoInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_go_") // More specific prefix
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the Go code to main.go.
	codeFilePath := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write go code: %w", err)
	}

	// Build the shell command:
	var cmdParts []string
	cmdParts = append(cmdParts, "cd /sandbox")
	// *** CHANGE HERE: Redirect stderr of go mod init to /dev/null ***
	// This prevents the "go: creating new go.mod..." warning from being captured.
	// Use `go mod init sandbox > /dev/null 2>&1 || true` to silence both stdout and stderr
	cmdParts = append(cmdParts, "go mod init sandbox > /dev/null 2>&1 || true")
	if len(dependencies) > 0 {
		for _, dep := range dependencies {
			// Add -v flag for slightly more verbose output during get if needed,
			// but generally keep it concise. Errors will still go to stderr.
			cmdParts = append(cmdParts, fmt.Sprintf("go get %s", dep))
		}
		// Tidy up dependencies after getting them
		cmdParts = append(cmdParts, "go mod tidy")
	}
	cmdParts = append(cmdParts, "go run main.go")
	cmdStr := strings.Join(cmdParts, " && ")

	dockerArgs := []string{
		"run", "--rm",
		"--network", "none", // Disable networking
		"--memory", "256m", // Limit memory
		"--cpus", "0.5", // Limit CPU
		// Mount as read-write initially needed for go mod/get, then potentially read-only run?
		// Simpler to keep rw for now.
		"-v", fmt.Sprintf("%s:/sandbox", tempDir),
		"code-sandbox",
		"/bin/sh", "-c", cmdStr,
	}

	dresp, err := runDockerCommand(dockerArgs, 60*time.Second) // Go build can take longer
	if err != nil {
		log.Printf("Docker command execution failed: %v, stderr: %s", err, dresp.Stderr)
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("code execution timed out")
		}
		errMsg := "failed to execute go code in container"
		if dresp != nil && dresp.Stderr != "" {
			errMsg += fmt.Sprintf(": %s", dresp.Stderr)
		}
		return nil, errors.New(errMsg)
	}

	// Pass the response directly to convertDockerResponse, which handles stderr correctly
	return convertDockerResponse(dresp), nil
}

// runNodeInContainer writes out the JavaScript code and sets up dependencies,
// then executes it using the multi-language Docker container.
func runNodeInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_node_") // More specific prefix
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the JavaScript code.
	codeFilePath := filepath.Join(tempDir, "user_code.js")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write javascript code: %w", err)
	}

	// Build the shell command:
	var cmdParts []string
	cmdParts = append(cmdParts, "cd /sandbox")
	if len(dependencies) > 0 {
		// Use --silent or --quiet for npm init and install to reduce noise
		cmdParts = append(cmdParts, "npm init -y --silent")
		// Install dependencies quietly. Errors will still show.
		cmdParts = append(cmdParts, fmt.Sprintf("npm install --silent %s", strings.Join(dependencies, " ")))
	}
	cmdParts = append(cmdParts, "node user_code.js")
	cmdStr := strings.Join(cmdParts, " && ")

	dockerArgs := []string{
		"run", "--rm",
		"--network", "none", // Disable networking
		"--memory", "256m", // Limit memory
		"--cpus", "0.5", // Limit CPU
		"-v", fmt.Sprintf("%s:/sandbox:ro", tempDir), // Mount read-only is fine for node after install
		"code-sandbox",
		"/bin/sh", "-c", cmdStr,
	}
	// If dependencies exist, we need the mount to be read-write for npm install
	if len(dependencies) > 0 {
		dockerArgs = []string{
			"run", "--rm",
			"--network", "none", "--memory", "256m", "--cpus", "0.5",
			"-v", fmt.Sprintf("%s:/sandbox", tempDir), // Read-write needed for npm install
			"code-sandbox",
			"/bin/sh", "-c", cmdStr,
		}
	}

	dresp, err := runDockerCommand(dockerArgs, 30*time.Second) // Reduced timeout
	if err != nil {
		log.Printf("Docker command execution failed: %v, stderr: %s", err, dresp.Stderr)
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("code execution timed out")
		}
		errMsg := "failed to execute node code in container"
		if dresp != nil && dresp.Stderr != "" {
			errMsg += fmt.Sprintf(": %s", dresp.Stderr)
		}
		return nil, errors.New(errMsg)
	}

	return convertDockerResponse(dresp), nil
}

// runDockerCommand executes the given Docker command with a timeout and returns the response.
func runDockerCommand(dockerArgs []string, timeout time.Duration) (*DockerExecResponse, error) {
	// Use context.WithTimeoutCause for better error messages on timeout
	ctx, cancel := context.WithTimeoutCause(context.Background(), timeout, fmt.Errorf("docker execution timed out after %v", timeout))
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	log.Printf("Running docker command: docker %s", strings.Join(dockerArgs, " "))
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)
	log.Printf("Docker command finished in %v. Exit code: %d", duration, cmd.ProcessState.ExitCode())

	response := &DockerExecResponse{
		Stdout:     stdoutBuf.String(),
		Stderr:     stderrBuf.String(),
		ReturnCode: -1, // Default for cases where we don't get an ExitError
	}

	if err != nil {
		// Check specifically for context deadline exceeded (timeout)
		if ctx.Err() == context.DeadlineExceeded {
			response.Stderr += fmt.Sprintf("\nError: Execution timed out after %v", timeout)
			// Use a specific return code for timeout, e.g., 124 (common for timeout)
			// Or keep it simple and rely on the error message. Let's add to stderr.
			response.ReturnCode = 124
			return response, ctx.Err() // Return the context error
		}
		// If it's a command exit error, capture the code
		if exitErr, ok := err.(*exec.ExitError); ok {
			response.ReturnCode = exitErr.ExitCode()
		} else {
			// Other errors (e.g., command not found, permission issues before execution)
			response.Stderr += fmt.Sprintf("\nError running command: %v", err)
			return response, fmt.Errorf("docker command setup/execution error: %w", err)
		}
		// For non-zero exit codes, still return the response object along with the error
		return response, err
	} else {
		// Success case
		response.ReturnCode = 0
	}

	log.Printf("Docker stdout: %s", response.Stdout)
	log.Printf("Docker stderr: %s", response.Stderr)
	return response, nil
}

// convertDockerResponse converts the raw Docker execution response into a CodeEvalResponse.
// NO CHANGE NEEDED HERE: It already correctly separates Result/Error based on ReturnCode.
// The unwanted warning is now prevented from reaching stderr in the first place.
func convertDockerResponse(dresp *DockerExecResponse) *CodeEvalResponse {
	res := &CodeEvalResponse{}
	// Prioritize Stderr as the error message if the execution failed
	if dresp.ReturnCode != 0 {
		// Combine stdout and stderr for the error message, as context might be in stdout
		errOutput := strings.TrimSpace(dresp.Stdout + "\n" + dresp.Stderr)
		if errOutput == "" {
			errOutput = fmt.Sprintf("Command failed with exit code %d", dresp.ReturnCode)
		}
		res.Error = errOutput
	} else {
		res.Result = dresp.Stdout
		// If there's still stderr content on success, it represents actual warnings
		// from the compiler/runtime (like `go get` or `go run`), not the silenced `go mod init`.
		// Keep appending these as warnings.
		if trimmedStderr := strings.TrimSpace(dresp.Stderr); trimmedStderr != "" {
			res.Result += "\n/* Warnings:\n" + trimmedStderr + "\n*/\n"
		}
	}
	return res
}
