// internal/codeeval/codeeval.go
package codeeval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CodeEvalRequest is the structure for code evaluation requests
type CodeEvalRequest struct {
	Language     string   `json:"language"`
	Code         string   `json:"code"`
	Dependencies []string `json:"dependencies"`
}

// CodeEvalResponse is the structure for code evaluation responses
type CodeEvalResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

// RunPythonInContainer executes Python code in a sandboxed container.
func RunPythonInContainer(code string, dependencies []string) (string, error) {
	return runCodeInContainer("python", code, dependencies)
}

// RunGoInContainer executes Go code in a sandboxed container.
func RunGoInContainer(code string, dependencies []string) (string, error) {
	return runCodeInContainer("go", code, dependencies)
}

// RunNodeInContainer executes JavaScript code in a sandboxed container.
func RunNodeInContainer(code string, dependencies []string) (string, error) {
	return runCodeInContainer("node", code, dependencies)
}

// runCodeInContainer is a helper function that runs code in a sandboxed container.
func runCodeInContainer(language, code string, dependencies []string) (string, error) {
	// Create a temporary directory to store the code file
	tempDir, err := os.MkdirTemp("", "code-execution")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Determine file extension and setup command based on language
	var fileExt, setupCmd, runCmd string
	var args []string

	switch language {
	case "python":
		fileExt = ".py"
		// Create requirements.txt if dependencies are specified
		if len(dependencies) > 0 {
			requirementsPath := filepath.Join(tempDir, "requirements.txt")
			if err := os.WriteFile(requirementsPath, []byte(strings.Join(dependencies, "\n")), 0644); err != nil {
				return "", fmt.Errorf("failed to write requirements.txt: %w", err)
			}
			setupCmd = "pip install -r requirements.txt"
		}
		runCmd = "python"
		args = []string{"code" + fileExt}
	case "go":
		fileExt = ".go"
		if len(dependencies) > 0 {
			// Format go.mod content
			goModContent := "module code\n\ngo 1.20\n\n"
			for _, dep := range dependencies {
				goModContent += "require " + dep + " v0.0.0\n"
			}
			goModPath := filepath.Join(tempDir, "go.mod")
			if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
				return "", fmt.Errorf("failed to write go.mod: %w", err)
			}
			setupCmd = "go mod tidy"
		}
		runCmd = "go"
		args = []string{"run", "code" + fileExt}
	case "node":
		fileExt = ".js"
		if len(dependencies) > 0 {
			// Create package.json
			pkg := map[string]interface{}{
				"name":         "code-execution",
				"version":      "1.0.0",
				"dependencies": map[string]string{},
			}
			deps := pkg["dependencies"].(map[string]string)
			for _, dep := range dependencies {
				deps[dep] = "latest"
			}
			pkgJSON, err := json.Marshal(pkg)
			if err != nil {
				return "", fmt.Errorf("failed to create package.json: %w", err)
			}
			pkgPath := filepath.Join(tempDir, "package.json")
			if err := os.WriteFile(pkgPath, pkgJSON, 0644); err != nil {
				return "", fmt.Errorf("failed to write package.json: %w", err)
			}
			setupCmd = "npm install"
		}
		runCmd = "node"
		args = []string{"code" + fileExt}
	default:
		return "", fmt.Errorf("unsupported language: %s", language)
	}

	// Write the code to a file
	codePath := filepath.Join(tempDir, "code"+fileExt)
	if err := os.WriteFile(codePath, []byte(code), 0644); err != nil {
		return "", fmt.Errorf("failed to write code file: %w", err)
	}

	// Run setup command if needed
	if setupCmd != "" {
		setupCmdParts := strings.Split(setupCmd, " ")
		cmd := exec.Command(setupCmdParts[0], setupCmdParts[1:]...)
		cmd.Dir = tempDir
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to run setup command: %w", err)
		}
	}

	// Run the code
	cmd := exec.Command(runCmd, args...)
	cmd.Dir = tempDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("execution error: %w\nStderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
