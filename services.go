package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/pterm/pterm"
)

// LlamaService represents a running llama-server instance
type LlamaService struct {
	Name         string
	Process      *exec.Cmd
	ModelPath    string
	Port         int
	Type         string // "embeddings" or "reranker"
	ShutdownChan chan struct{}
	Ctx          context.Context
	Cancel       context.CancelFunc
}

var (
	servicesMutex sync.Mutex
	services      = make(map[string]*LlamaService)
)

// StartLocalServices starts all required local services based on config
func StartLocalServices(config *Config) error {
	// Only start local services if SingleNodeInstance is true
	if !config.SingleNodeInstance {
		pterm.Info.Println("SingleNodeInstance is false, not starting local services")
		return nil
	}

	// First, make sure the llama-server binary is available
	binaryPath, err := getLlamaServerBinaryPath(config)
	if err != nil {
		return fmt.Errorf("failed to get llama-server binary path: %w", err)
	}

	pterm.Info.Printf("Using llama-server binary at: %s\n", binaryPath)

	// In single node mode, we start local services regardless of host configuration
	// Start embeddings service
	if err := StartEmbeddingsService(config, binaryPath); err != nil {
		return fmt.Errorf("failed to start embeddings service: %v", err)
	}

	// Start reranker service
	if err := StartRerankerService(config, binaryPath); err != nil {
		return fmt.Errorf("failed to start reranker service: %v", err)
	}

	// Start completions service
	if err := StartCompletionsService(config, binaryPath); err != nil {
		return fmt.Errorf("failed to start completions service: %v", err)
	}

	// Override any existing host configurations
	config.Embeddings.Host = fmt.Sprintf("http://127.0.0.1:%d/v1/embeddings", 32184)
	config.Reranker.Host = fmt.Sprintf("http://127.0.0.1:%d/v1/rerank", 32185)
	config.Completions.DefaultHost = fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", 32186)

	return nil
}

// StartCompletionsService starts the local completions service using the Gemma model
func StartCompletionsService(config *Config, binaryPath string) error {
	servicesMutex.Lock()
	defer servicesMutex.Unlock()

	// Check if service is already running
	if _, exists := services["completions"]; exists {
		return nil
	}

	// Default port for completions service
	const completionsPort = 32186

	modelPath := filepath.Join(config.DataPath, "models", "gguf", "gemma-3-4b-it.Q8_0.gguf")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("completions model not found at %s", modelPath)
	}

	// Create context with cancellation for proper shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Log the full command we're about to execute with all the parameters from the example command
	cmdArgs := []string{
		"-m", modelPath,
		"--temp", "1.0",
		"--ctx-size", "16384",
		"--min-p", "0.01",
		"--top-p", "0.95",
		"--top-k", "64",
		"--repeat-penalty", "1.0",
		"-t", "-1",
		"-ngl", "99",
		"--parallel", "4",
		"--batch-size", "2048",
		"--ubatch-size", "512",
		"--threads-http", "4",
		"-fa",
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", completionsPort),
		"--props",
	}
	pterm.Debug.Printf("Starting completions service with command: %s %v\n", binaryPath, cmdArgs)

	// Prepare command with all necessary arguments
	cmd := exec.CommandContext(ctx, binaryPath, cmdArgs...)

	// Capture stdout and stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	pterm.Info.Printf("Starting completions service on port %d with model %s\n", completionsPort, modelPath)
	if err := cmd.Start(); err != nil {
		cancel() // Clean up the context if command fails to start
		return fmt.Errorf("failed to start completions service: %w", err)
	}

	// Give some time for the server to initialize
	time.Sleep(2 * time.Second)

	// Verify the process is still running
	if cmd.Process == nil {
		cancel()
		return fmt.Errorf("completions service process failed to start")
	}

	// Try to check if process is still alive
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		cancel()
		return fmt.Errorf("completions service process died immediately after start: %v", err)
	}

	pterm.Success.Printf("Completions service started with PID %d\n", cmd.Process.Pid)

	// Store service information
	shutdownChan := make(chan struct{})
	services["completions"] = &LlamaService{
		Name:         "completions",
		Process:      cmd,
		ModelPath:    modelPath,
		Port:         completionsPort,
		Type:         "completions",
		ShutdownChan: shutdownChan,
		Ctx:          ctx,
		Cancel:       cancel,
	}

	// Set up process monitoring
	go monitorProcess("completions", cmd, shutdownChan)

	// Update config to use local service
	config.Completions.DefaultHost = fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", completionsPort)
	return nil
}

// StartEmbeddingsService starts the local embeddings service
func StartEmbeddingsService(config *Config, binaryPath string) error {
	servicesMutex.Lock()
	defer servicesMutex.Unlock()

	// Check if service is already running
	if _, exists := services["embeddings"]; exists {
		return nil
	}

	// Default port for embeddings service
	const embeddingsPort = 32184

	modelPath := filepath.Join(config.DataPath, "models", "embeddings", "nomic-embed-text-v1.5.Q8_0.gguf")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("embeddings model not found at %s", modelPath)
	}

	// Create context with cancellation for proper shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Log the full command we're about to execute
	cmdArgs := []string{
		"-m", modelPath,
		"-c", "65536",
		"-np", "8",
		"-b", "8192",
		"-ub", "8192",
		"-fa",
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", embeddingsPort),
		"-lv", "1",
		"--embedding",
	}
	pterm.Debug.Printf("Starting embeddings service with command: %s %v\n", binaryPath, cmdArgs)

	// Prepare command with all necessary arguments
	cmd := exec.CommandContext(ctx, binaryPath, cmdArgs...)

	// Capture stdout and stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	pterm.Info.Printf("Starting embeddings service on port %d with model %s\n", embeddingsPort, modelPath)
	if err := cmd.Start(); err != nil {
		cancel() // Clean up the context if command fails to start
		return fmt.Errorf("failed to start embeddings service: %w", err)
	}

	// Give some time for the server to initialize
	time.Sleep(2 * time.Second)

	// Verify the process is still running
	if cmd.Process == nil {
		cancel()
		return fmt.Errorf("embeddings service process failed to start")
	}

	// Try to check if process is still alive
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		cancel()
		return fmt.Errorf("embeddings service process died immediately after start: %v", err)
	}

	pterm.Success.Printf("Embeddings service started with PID %d\n", cmd.Process.Pid)

	// Store service information
	shutdownChan := make(chan struct{})
	services["embeddings"] = &LlamaService{
		Name:         "embeddings",
		Process:      cmd,
		ModelPath:    modelPath,
		Port:         embeddingsPort,
		Type:         "embeddings",
		ShutdownChan: shutdownChan,
		Ctx:          ctx,
		Cancel:       cancel,
	}

	// Set up process monitoring
	go monitorProcess("embeddings", cmd, shutdownChan)

	// Update config to use local service
	config.Embeddings.Host = fmt.Sprintf("http://127.0.0.1:%d/v1/embeddings", embeddingsPort)
	return nil
}

// StartRerankerService starts the local reranker service
func StartRerankerService(config *Config, binaryPath string) error {
	servicesMutex.Lock()
	defer servicesMutex.Unlock()

	// Check if service is already running
	if _, exists := services["reranker"]; exists {
		return nil
	}

	// Default port for reranker service
	const rerankerPort = 32185

	modelPath := filepath.Join(config.DataPath, "models", "rerankers", "slide-bge-reranker-v2-m3.Q4_K_M.gguf")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("reranker model not found at %s", modelPath)
	}

	// Create context with cancellation for proper shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Prepare command with all necessary arguments
	cmd := exec.CommandContext(ctx, binaryPath,
		"-m", modelPath,
		"-c", "65536",
		"-np", "8",
		"-b", "8192",
		"-ub", "8192",
		"-fa",
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", rerankerPort),
		"-lv", "1",
		"--reranking",
		"--pooling", "rank")

	// Capture stdout and stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	pterm.Info.Printf("Starting reranker service on port %d with model %s\n", rerankerPort, modelPath)
	if err := cmd.Start(); err != nil {
		cancel() // Clean up the context if command fails to start
		return fmt.Errorf("failed to start reranker service: %w", err)
	}

	// Give some time for the server to initialize
	time.Sleep(2 * time.Second)

	// Store service information
	shutdownChan := make(chan struct{})
	services["reranker"] = &LlamaService{
		Name:         "reranker",
		Process:      cmd,
		ModelPath:    modelPath,
		Port:         rerankerPort,
		Type:         "reranker",
		ShutdownChan: shutdownChan,
		Ctx:          ctx,
		Cancel:       cancel,
	}

	// Set up process monitoring
	go monitorProcess("reranker", cmd, shutdownChan)

	// Update config to use local service
	config.Reranker.Host = fmt.Sprintf("http://127.0.0.1:%d/v1/rerank", rerankerPort)
	return nil
}

// StopAllServices gracefully stops all running services
func StopAllServices() {
	servicesMutex.Lock()
	defer servicesMutex.Unlock()

	for name, service := range services {
		pterm.Info.Printf("Stopping %s service...\n", name)
		if service.Cancel != nil {
			service.Cancel() // Signal the process to stop via context
		}

		// Wait for process to terminate gracefully
		done := make(chan struct{})
		go func() {
			if service.Process != nil && service.Process.Process != nil {
				service.Process.Wait()
			}
			close(done)
		}()

		// Give it some time to terminate gracefully
		select {
		case <-done:
			// Process terminated
		case <-time.After(5 * time.Second):
			// Timeout: force kill if needed
			if service.Process != nil && service.Process.Process != nil {
				pterm.Warning.Printf("Force killing %s service\n", name)
				service.Process.Process.Kill()
			}
		}

		// Close the shutdown channel if it exists
		if service.ShutdownChan != nil {
			close(service.ShutdownChan)
		}

		pterm.Success.Printf("%s service stopped\n", name)
	}

	// Clear the services map
	services = make(map[string]*LlamaService)
}

// monitorProcess monitors a process and logs when it exits
func monitorProcess(name string, cmd *exec.Cmd, shutdownChan chan struct{}) {
	// Wait for process to complete
	err := cmd.Wait()

	// Check if we're shutting down normally
	select {
	case <-shutdownChan:
		// Normal shutdown, nothing to do
		return
	default:
		// Process terminated unexpectedly
		if err != nil {
			pterm.Error.Printf("%s service terminated unexpectedly: %v\n", name, err)
		} else {
			pterm.Warning.Printf("%s service terminated unexpectedly\n", name)
		}

		// Clean up from services map
		servicesMutex.Lock()
		delete(services, name)
		servicesMutex.Unlock()
	}
}

// getLlamaServerBinaryPath returns the path to the llama-server binary
func getLlamaServerBinaryPath(config *Config) (string, error) {
	if config.DataPath == "" {
		return "", fmt.Errorf("data path not configured")
	}

	hostInfo, err := GetHostInfo()
	if err != nil {
		return "", fmt.Errorf("failed to get host info: %w", err)
	}

	// Determine binary name based on OS
	binaryName := "llama-server"
	if hostInfo.OS == "windows" {
		binaryName = "llama-server.exe"
	}

	// Check for binary in expected locations
	locations := []string{
		filepath.Join(config.DataPath, "llama-cpp", "build", "bin", binaryName),
		filepath.Join(config.DataPath, "llama-cpp", binaryName),
	}

	for _, path := range locations {
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			// On Unix systems, check if the file is executable
			if hostInfo.OS != "windows" {
				if fi.Mode()&0111 != 0 {
					return path, nil
				}
			} else {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("llama-server binary not found in expected locations")
}
