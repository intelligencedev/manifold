package tools

import (
        "bytes"
        "context"
        "fmt"
        logpkg "manifold/internal/logging"
        "os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	configpkg "manifold/internal/config"
)

// CodeEvalRequest describes a code evaluation request.
type CodeEvalRequest struct {
	Code         string   `json:"code"`
	Language     string   `json:"language"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// CodeEvalResponse is returned after code execution.
type CodeEvalResponse struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// DockerExecResponse captures raw docker output.
type DockerExecResponse struct {
	ReturnCode int
	Stdout     string
	Stderr     string
}

func runInContainer(codeFile, code string, install []string, runCmd string, deps []string, cfg *configpkg.Config) (*CodeEvalResponse, error) {
	tempDir := cfg.DataPath + "/tmp"

	// Clean up the directory before writing new files
	// if err := os.RemoveAll(tempDir); err != nil {
	// 	return nil, fmt.Errorf("failed to clean temp directory: %w", err)
	// }
	// if err := os.MkdirAll(tempDir, 0755); err != nil {
	// 	return nil, fmt.Errorf("failed to create temp directory: %w", err)
	// }
	// Write the code to a file

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

	var cmdParts []string
	cmdParts = append(cmdParts, install...)
	cmdParts = append(cmdParts, runCmd)
	cmdStr := strings.Join(cmdParts, " && ")

	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/app/projects", tempDir),
		"-w", "/app/projects",
		"code-sandbox",
		"/bin/sh", "-c", cmdStr,
	}

	dresp, err := runDockerCommand(dockerArgs, 60*time.Second)
	if err != nil {
		return nil, err
	}

	return ConvertDockerResponse(dresp), nil
}

func runInContainerRaw(codeFile, code string, install []string, runCmd string, deps []string, cfg *configpkg.Config) (string, error) {
	resp, err := runInContainer(codeFile, code, install, runCmd, deps, cfg)
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Result, nil
}

// RunPython executes Python code inside the sandbox.
func RunPython(cfg *configpkg.Config, code string, dependencies []string) (*CodeEvalResponse, error) {
	return runInContainer("user_code.py", code, nil, "python3 user_code.py", dependencies, cfg)
}

// RunPythonRaw executes Python code and returns only stdout.
func RunPythonRaw(cfg *configpkg.Config, code string, dependencies []string) (string, error) {
	return runInContainerRaw("user_code.py", code, nil, "python3 user_code.py", dependencies, cfg)
}

// RunGo executes Go code inside the sandbox.
func RunGo(cfg *configpkg.Config, code string, dependencies []string) (*CodeEvalResponse, error) {
	var install []string
	install = append(install, "go mod init sandbox > /dev/null 2>/dev/null || true")
	for _, dep := range dependencies {
		install = append(install, fmt.Sprintf("go get %s > /dev/null 2>/dev/null", dep))
	}
	return runInContainer("main.go", code, install, "go run main.go", nil, cfg)
}

// RunGoRaw executes Go code and returns only stdout.
func RunGoRaw(cfg *configpkg.Config, code string, dependencies []string) (string, error) {
	var install []string
	install = append(install, "go mod init sandbox > /dev/null 2>/dev/null || true")
	for _, dep := range dependencies {
		install = append(install, fmt.Sprintf("go get %s > /dev/null 2>/dev/null", dep))
	}
	return runInContainerRaw("main.go", code, install, "go run main.go", nil, cfg)
}

// RunNode executes JavaScript code inside the sandbox.
func RunNode(cfg *configpkg.Config, code string, dependencies []string) (*CodeEvalResponse, error) {
	var install []string
	if len(dependencies) > 0 {
		install = append(install, "npm init -y > /dev/null 2>/dev/null")
		install = append(install, fmt.Sprintf("npm install %s > /dev/null 2>/dev/null", strings.Join(dependencies, " ")))
	}
	return runInContainer("user_code.js", code, install, "node user_code.js", nil, cfg)
}

// RunNodeRaw executes JavaScript code and returns only stdout.
func RunNodeRaw(cfg *configpkg.Config, code string, dependencies []string) (string, error) {
	var install []string
	if len(dependencies) > 0 {
		install = append(install, "npm init -y > /dev/null 2>/dev/null")
		install = append(install, fmt.Sprintf("npm install %s > /dev/null 2>/dev/null", strings.Join(dependencies, " ")))
	}
	return runInContainerRaw("user_code.js", code, install, "node user_code.js", nil, cfg)
}

func runDockerCommand(dockerArgs []string, timeout time.Duration) (*DockerExecResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

    logpkg.Log.Debugf("running docker command: docker %s", strings.Join(dockerArgs, " "))
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

// ConvertDockerResponse converts raw docker output to a CodeEvalResponse.
func ConvertDockerResponse(dresp *DockerExecResponse) *CodeEvalResponse {
	if dresp.ReturnCode != 0 {
		return &CodeEvalResponse{Error: dresp.Stderr}
	}
	return &CodeEvalResponse{Result: dresp.Stdout}
}
