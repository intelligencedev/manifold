package services

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

//go:embed pgsql.Dockerfile
var pgsqlDockerfile string

// EnsurePGVectorImage checks if the pg-manifold Docker image exists and builds it if necessary.
func EnsurePGVectorImage() error {
	// Check if Docker is installed
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is not installed or not in PATH: %w", err)
	}

	// Check if Docker is running
	checkCmd := exec.Command("docker", "info")
	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("docker is not running: %w", err)
	}

	logrus.Info("Docker is available, checking for pg-manifold image...")

	// Check if the image exists
	checkImageCmd := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", "intelligencedev/pg-manifold:latest")
	output, err := checkImageCmd.Output()
	if err == nil && len(output) > 0 {
		logrus.Info("intelligencedev/pg-manifold:latest image already exists")
		return nil
	}

	logrus.Info("intelligencedev/pg-manifold:latest image not found, building it...")

	tempDir, err := os.MkdirTemp("", "pgvector-build-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory for Dockerfile: %w", err)
	}
	defer os.RemoveAll(tempDir)

	dockerfilePath := filepath.Join(tempDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(pgsqlDockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	buildCmd := exec.Command("docker", "build", "-t", "intelligencedev/pg-manifold:latest", "-f", dockerfilePath, ".")
	buildCmd.Dir = tempDir

	var stdoutBuf, stderrBuf bytes.Buffer
	buildCmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	buildCmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	logrus.Info("Building intelligencedev/pg-manifold Docker image...")
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build pg-manifold image: %w\n%s", err, stderrBuf.String())
	}

	logrus.Info("intelligencedev/pg-manifold:latest image successfully built")
	return nil
}
