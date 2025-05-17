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

// RunPythonInContainer writes out the Python code and dependencies, then executes them using the multi-language Docker container.
func RunPythonInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	codeFilePath := filepath.Join(tempDir, "user_code.py")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write python code: %w", err)
	}

	reqFile := filepath.Join(tempDir, "requirements.txt")
	if len(dependencies) > 0 {
		reqContent := strings.Join(dependencies, "\n")
		if err := os.WriteFile(reqFile, []byte(reqContent), 0644); err != nil {
			return nil, fmt.Errorf("failed to write requirements.txt: %w", err)
		}
	} else {
		if err := os.WriteFile(reqFile, []byte(""), 0644); err != nil {
			return nil, fmt.Errorf("failed to write empty requirements.txt: %w", err)
		}
	}

	config, err := configpkg.LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

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

	return ConvertDockerResponse(dresp), nil
}

// RunGoInContainer writes out the Go code and sets up dependencies, then executes the code using the multi-language Docker container.
func RunGoInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	codeFilePath := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write go code: %w", err)
	}

	config, err := configpkg.LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	var installLines []string
	if len(dependencies) > 0 {
		for _, dep := range dependencies {
			installLines = append(installLines, fmt.Sprintf("go get %s > /dev/null 2>/dev/null", dep))
		}
	}

	cmdParts := []string{
		"cd /sandbox",
		"go mod init sandbox > /dev/null 2>/dev/null || true",
	}
	cmdParts = append(cmdParts, installLines...)
	cmdParts = append(cmdParts, "go run main.go")

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

	return ConvertDockerResponse(dresp), nil
}

// RunNodeInContainer writes out the JavaScript code and sets up dependencies, then executes it using the multi-language Docker container.
func RunNodeInContainer(code string, dependencies []string) (*CodeEvalResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	codeFilePath := filepath.Join(tempDir, "user_code.js")
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write javascript code: %w", err)
	}

	config, err := configpkg.LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	var installLines []string
	if len(dependencies) > 0 {
		installLines = append(installLines, "npm init -y > /dev/null 2>/dev/null")
		installLines = append(installLines, fmt.Sprintf("npm install %s > /dev/null 2>/dev/null", strings.Join(dependencies, " ")))
	}

	cmdParts := []string{"cd /sandbox"}
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

	return ConvertDockerResponse(dresp), nil
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
