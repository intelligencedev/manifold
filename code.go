// manifold/code.go
package main

import (
	"bytes"
	"context"
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
		log.Printf("Received language: [%s]", req.Language)
		return c.JSON(http.StatusBadRequest, CodeEvalResponse{
			Error: "Invalid request body: " + err.Error(),
		})
	}

	// Trim and lower-case to avoid trailing spaces, etc.
	lang := strings.ToLower(strings.TrimSpace(req.Language))
	log.Printf("Received language: [%s]", lang)

	var (
		resp *CodeEvalResponse
		err  error
	)

	switch lang {
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
		return c.JSON(http.StatusInternalServerError, CodeEvalResponse{
			Error: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// runPythonInContainer writes out the Python code and dependencies,
// then executes them using the multi-language Docker container.
func runPythonInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the Python code.
	codeFilePath := filepath.Join(tempDir, "user_code.py")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write python code: %w", err)
	}

	// Write dependencies (if any) into requirements.txt, but don't log them in output.
	reqFile := filepath.Join(tempDir, "requirements.txt")
	if len(dependencies) > 0 {
		reqContent := strings.Join(dependencies, "\n")
		if err := os.WriteFile(reqFile, []byte(reqContent), 0644); err != nil {
			return nil, fmt.Errorf("failed to write requirements.txt: %w", err)
		}
	} else {
		// Create an empty requirements file to avoid pip error if none exist
		if err := os.WriteFile(reqFile, []byte(""), 0644); err != nil {
			return nil, fmt.Errorf("failed to write empty requirements.txt: %w", err)
		}
	}

	// Get the data path from config
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Redirect pip install output to /dev/null (only show errors if they occur).
	// Then run the user code normally (output captured).
	cmdStr := `
cd /sandbox &&
pip install -r requirements.txt > /dev/null 2>/dev/null &&
python3 user_code.py
`

	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/sandbox", tempDir),
		"-v", fmt.Sprintf("%s:/mnt", config.DataPath),
		"code-sandbox",
		"/bin/bash", "-c", cmdStr,
	}

	dresp, err := runDockerCommand(dockerArgs, 60*time.Second)
	if err != nil {
		return nil, err
	}

	return convertDockerResponse(dresp), nil
}

// runGoInContainer writes out the Go code and sets up dependencies,
// then executes the code using the multi-language Docker container.
func runGoInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the Go code to main.go.
	codeFilePath := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write go code: %w", err)
	}

	// Get the data path from config
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// If dependencies are given, run 'go get' quietly; otherwise just run 'go run'.
	var installLines []string
	if len(dependencies) > 0 {
		for _, dep := range dependencies {
			installLines = append(installLines, fmt.Sprintf("go get %s > /dev/null 2>/dev/null", dep))
		}
	}

	// Build final command. Silence all module logs, but keep errors in stderr.
	cmdParts := []string{
		"cd /sandbox",
		"go mod init sandbox > /dev/null 2>/dev/null || true", // ignore if it already exists
	}
	cmdParts = append(cmdParts, installLines...)
	cmdParts = append(cmdParts, "go run main.go") // Show user code output

	cmdStr := strings.Join(cmdParts, " && ")

	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/sandbox", tempDir),
		"-v", fmt.Sprintf("%s:/mnt", config.DataPath),
		"code-sandbox",
		"/bin/sh", "-c", cmdStr,
	}

	dresp, err := runDockerCommand(dockerArgs, 60*time.Second)
	if err != nil {
		return nil, err
	}

	return convertDockerResponse(dresp), nil
}

// runNodeInContainer writes out the JavaScript code and sets up dependencies,
// then executes it using the multi-language Docker container.
func runNodeInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the JavaScript code.
	codeFilePath := filepath.Join(tempDir, "user_code.js")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write javascript code: %w", err)
	}

	// Get the data path from config
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// If dependencies exist, init and install them quietly (redirect to /dev/null).
	var installLines []string
	if len(dependencies) > 0 {
		installLines = append(installLines, "npm init -y > /dev/null 2>/dev/null")
		installLines = append(installLines, fmt.Sprintf("npm install %s > /dev/null 2>/dev/null",
			strings.Join(dependencies, " ")))
	}

	// Finally, run the user script (output is captured).
	cmdParts := []string{
		"cd /sandbox",
	}
	cmdParts = append(cmdParts, installLines...)
	cmdParts = append(cmdParts, "node user_code.js")

	cmdStr := strings.Join(cmdParts, " && ")

	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/sandbox", tempDir),
		"-v", fmt.Sprintf("%s:/mnt", config.DataPath),
		"code-sandbox",
		"/bin/sh", "-c", cmdStr,
	}

	dresp, err := runDockerCommand(dockerArgs, 60*time.Second)
	if err != nil {
		return nil, err
	}

	return convertDockerResponse(dresp), nil
}

// runDockerCommand executes the given Docker command with a timeout and returns the response.
func runDockerCommand(dockerArgs []string, timeout time.Duration) (*DockerExecResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	log.Printf("Running docker command: docker %s", strings.Join(dockerArgs, " "))
	err := cmd.Run()

	response := &DockerExecResponse{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	if err != nil {
		// If it's an exit error, capture its exit code.
		// We'll return it so we can produce an error in the final response.
		if exitErr, ok := err.(*exec.ExitError); ok {
			response.ReturnCode = exitErr.ExitCode()
		} else {
			return response, fmt.Errorf("docker command error: %w", err)
		}
	}

	return response, nil
}

// convertDockerResponse converts the raw Docker execution response into a CodeEvalResponse.
func convertDockerResponse(dresp *DockerExecResponse) *CodeEvalResponse {
	// If the exit code was non-zero, treat it as an error and return stderr.
	if dresp.ReturnCode != 0 {
		return &CodeEvalResponse{
			Error: dresp.Stderr,
		}
	}

	// Otherwise, return only stdout (the user script output). No warnings appended.
	return &CodeEvalResponse{
		Result: dresp.Stdout,
	}
}
