package agents

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	configpkg "manifold/internal/config"
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

func runInContainer(codeFile string, code string, install []string, runCmd string, deps []string, cfg *configpkg.Config) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := os.WriteFile(filepath.Join(tempDir, codeFile), []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write code: %w", err)
	}

	if len(deps) > 0 {
		reqFile := filepath.Join(tempDir, "requirements.txt")
		reqContent := strings.Join(deps, "\n")
		if err := os.WriteFile(reqFile, []byte(reqContent), 0644); err != nil {
			return nil, fmt.Errorf("failed to write requirements.txt: %w", err)
		}
		install = append(install, "pip install -r requirements.txt > /dev/null 2>/dev/null")
	}

	cmdParts := []string{"cd /sandbox"}
	cmdParts = append(cmdParts, install...)
	cmdParts = append(cmdParts, runCmd)
	cmdStr := strings.Join(cmdParts, " && ")

	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/sandbox", tempDir),
		"-v", fmt.Sprintf("%s:/mnt", cfg.DataPath),
		"code-sandbox",
		"/bin/sh", "-c", cmdStr,
	}

	dresp, err := runDockerCommand(dockerArgs, 60*time.Second)
	if err != nil {
		return nil, err
	}

	return ConvertDockerResponse(dresp), nil
}

// RunPythonInContainer writes out the Python code and dependencies, then executes them using the multi-language Docker container.
func RunPythonInContainer(cfg *configpkg.Config, code string, dependencies []string) (*CodeEvalResponse, error) {
	return runInContainer("user_code.py", code, nil, "python3 user_code.py", dependencies, cfg)
}

// RunGoInContainer writes out the Go code and sets up dependencies, then executes the code using the multi-language Docker container.
func RunGoInContainer(cfg *configpkg.Config, code string, dependencies []string) (*CodeEvalResponse, error) {
	var install []string
	install = append(install, "go mod init sandbox > /dev/null 2>/dev/null || true")
	for _, dep := range dependencies {
		install = append(install, fmt.Sprintf("go get %s > /dev/null 2>/dev/null", dep))
	}
	return runInContainer("main.go", code, install, "go run main.go", nil, cfg)
}

// RunNodeInContainer writes out the JavaScript code and sets up dependencies, then executes it using the multi-language Docker container.
func RunNodeInContainer(cfg *configpkg.Config, code string, dependencies []string) (*CodeEvalResponse, error) {
	var install []string
	if len(dependencies) > 0 {
		install = append(install, "npm init -y > /dev/null 2>/dev/null")
		install = append(install, fmt.Sprintf("npm install %s > /dev/null 2>/dev/null", strings.Join(dependencies, " ")))
	}
	return runInContainer("user_code.js", code, install, "node user_code.js", nil, cfg)
}

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
		if exitErr, ok := err.(*exec.ExitError); ok {
			response.ReturnCode = exitErr.ExitCode()
		} else {
			return response, fmt.Errorf("docker command error: %w", err)
		}
	}

	return response, nil
}

// ConvertDockerResponse converts the raw Docker execution response into a CodeEvalResponse.
func ConvertDockerResponse(dresp *DockerExecResponse) *CodeEvalResponse {
	if dresp.ReturnCode != 0 {
		return &CodeEvalResponse{Error: dresp.Stderr}
	}
	return &CodeEvalResponse{Result: dresp.Stdout}
}
