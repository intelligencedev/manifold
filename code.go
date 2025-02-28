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
	"runtime"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// PythonCodeRequest represents the structure of the incoming Python execution request.
type PythonCodeRequest struct {
	Code         string   `json:"code"`
	Dependencies []string `json:"dependencies"`
}

// PythonCodeResponse represents the structure of the response after executing Python code.
type PythonCodeResponse struct {
	ReturnCode int    `json:"return_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
}

// runEphemeralPython creates a temporary virtual environment,
// installs any requested dependencies, executes the user code,
// and returns stdout, stderr, and return_code.
func runEphemeralPython(code string, dependencies []string) (*PythonCodeResponse, error) {
	tempDir, err := os.MkdirTemp("", "sandbox_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	venvDir := filepath.Join(tempDir, "venv")

	// Determine python command
	pythonCmd := "python3"
	if _, err := exec.LookPath(pythonCmd); err != nil {
		// fallback to python if python3 is not found
		pythonCmd = "python"
	}

	// 1. Create the venv (with context)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonCmd, "-m", "venv", venvDir)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	log.Printf("Creating venv with: %s -m venv %s", pythonCmd, venvDir)
	if err := cmd.Run(); err != nil {
		log.Printf("Venv creation error:\nSTDOUT: %s\nSTDERR: %s", outBuf.String(), errBuf.String())
		return nil, fmt.Errorf("failed to create venv: %w", err)
	}
	log.Printf("Venv created successfully in %s", venvDir)

	// Set up path to pip/python inside venv
	pipPath := filepath.Join(venvDir, "bin", "pip")
	realPythonPath := filepath.Join(venvDir, "bin", "python")

	// Windows fix (if needed)
	if runtime.GOOS == "windows" {
		pipPath = filepath.Join(venvDir, "Scripts", "pip.exe")
		realPythonPath = filepath.Join(venvDir, "Scripts", "python.exe")
	}

	// 2. Install dependencies
	successfulInstalls := []string{}
	failedInstalls := []string{}

	for _, dep := range dependencies {
		ctxDep, cancelDep := context.WithTimeout(context.Background(), 60*time.Second)

		cmdInstall := exec.CommandContext(ctxDep, pipPath, "install", dep)
		outBuf.Reset()
		errBuf.Reset()
		cmdInstall.Stdout = &outBuf
		cmdInstall.Stderr = &errBuf

		log.Printf("Installing dependency %s", dep)
		if err := cmdInstall.Run(); err != nil {
			log.Printf("Pip install error for %s:\nSTDOUT: %s\nSTDERR: %s", dep, outBuf.String(), errBuf.String())
			failedInstalls = append(failedInstalls, dep)
		} else {
			log.Printf("Installed %s successfully.", dep)
			successfulInstalls = append(successfulInstalls, dep)
		}

		cancelDep() // Cancel the context after each dependency
	}

	if len(failedInstalls) > 0 {
		log.Printf("Warning: Failed to install packages: %s", strings.Join(failedInstalls, ", "))
	}

	// 3. Write code to user_code.py
	codeFilePath := filepath.Join(tempDir, "user_code.py")
	if err := os.WriteFile(codeFilePath, []byte(code), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write user code: %w", err)
	}

	// 4. Run the code
	ctxRun, cancelRun := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelRun()

	cmdRun := exec.CommandContext(ctxRun, realPythonPath, codeFilePath)
	var runOutBuf, runErrBuf bytes.Buffer
	cmdRun.Stdout = &runOutBuf
	cmdRun.Stderr = &runErrBuf
	err = cmdRun.Run()

	response := &PythonCodeResponse{
		Stdout:     runOutBuf.String(),
		Stderr:     runErrBuf.String(),
		ReturnCode: 0,
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			response.ReturnCode = exitErr.ExitCode()
		} else {
			// Some other error (context deadline, etc.)
			return response, fmt.Errorf("error running code: %w", err)
		}
	}

	return response, nil
}

func executePythonHandler(c echo.Context) error {
	var req PythonCodeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	result, err := runEphemeralPython(req.Code, req.Dependencies)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, result)
}
