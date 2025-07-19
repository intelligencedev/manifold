package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pterm/pterm"

	hostinfopkg "manifold/internal/tools"
)

// LlamaService represents a running llama-server instance.
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

// StartLocalServices initializes and starts all required local services based on the configuration.
func StartLocalServices(config *Config) error {
	if !config.SingleNodeInstance {
		pterm.Info.Println("SingleNodeInstance is false, not starting local services")
		return nil
	}

	binaryPath, err := getLlamaServerBinaryPath(config)
	if err != nil {
		return fmt.Errorf("failed to get llama-server binary path: %w", err)
	}

	pterm.Info.Printf("Using llama-server binary at: %s\n", binaryPath)

	if err := startService(StartEmbeddingsService, config, binaryPath); err != nil {
		return fmt.Errorf("failed to start embeddings service: %v", err)
	}

	if err := startService(StartRerankerService, config, binaryPath); err != nil {
		return fmt.Errorf("failed to start reranker service: %v", err)
	}

	if err := startService(StartCompletionsService, config, binaryPath); err != nil {
		return fmt.Errorf("failed to start completions service: %v", err)
	}

	// config.Embeddings.Host = fmt.Sprintf("http://127.0.0.1:%d/v1/embeddings", 32184)
	// config.Reranker.Host = fmt.Sprintf("http://127.0.0.1:%d/v1/rerank", 32185)
	// config.Completions.DefaultHost = fmt.Sprintf("http://127.0.0.1:%d/v1", 32186)

	return nil
}

// startService is a helper function to start a specific service.
func startService(startFunc func(*Config, string) error, config *Config, binaryPath string) error {
	return startFunc(config, binaryPath)
}

// StartCompletionsService starts the local completions service using the Gemma model.
func StartCompletionsService(config *Config, binaryPath string) error {
	return startLlamaService("completions", config, binaryPath, 32186, "completions", []string{
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
		"--props",
	})
}

// StartEmbeddingsService starts the local embeddings service.
func StartEmbeddingsService(config *Config, binaryPath string) error {
	return startLlamaService("embeddings", config, binaryPath, 32184, "embeddings", []string{
		"-c", "65536",
		"-np", "8",
		"-b", "8192",
		"-ub", "8192",
		"-fa",
		"-lv", "1",
		"--embedding",
	})
}

// StartRerankerService starts the local reranker service.
func StartRerankerService(config *Config, binaryPath string) error {
	return startLlamaService("reranker", config, binaryPath, 32185, "reranker", []string{
		"-c", "65536",
		"-np", "8",
		"-b", "8192",
		"-ub", "8192",
		"-fa",
		"-lv", "1",
		"--reranking",
		"--pooling", "rank",
	})
}

// startLlamaService is a generic function to start a LlamaService.
func startLlamaService(name string, config *Config, binaryPath string, port int, serviceType string, additionalArgs []string) error {
	servicesMutex.Lock()
	defer servicesMutex.Unlock()

	if _, exists := services[name]; exists {
		return nil
	}

	modelPath := getModelPath(config, name)
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("%s model not found at %s", name, modelPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmdArgs := append([]string{
		"-m", modelPath,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
	}, additionalArgs...)

	cmd := exec.CommandContext(ctx, binaryPath, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	pterm.Info.Printf("Starting %s service on port %d with model %s\n", name, port, modelPath)
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start %s service: %w", name, err)
	}

	time.Sleep(2 * time.Second)

	if cmd.Process == nil || cmd.Process.Signal(syscall.Signal(0)) != nil {
		cancel()
		return fmt.Errorf("%s service process failed to start", name)
	}

	pterm.Success.Printf("%s service started with PID %d\n", name, cmd.Process.Pid)

	shutdownChan := make(chan struct{})
	services[name] = &LlamaService{
		Name:         name,
		Process:      cmd,
		ModelPath:    modelPath,
		Port:         port,
		Type:         serviceType,
		ShutdownChan: shutdownChan,
		Ctx:          ctx,
		Cancel:       cancel,
	}

	go monitorProcess(name, cmd, shutdownChan)
	return nil
}

// getModelPath returns the model path for a given service name.
func getModelPath(config *Config, serviceName string) string {
	switch serviceName {
	case "completions":
		return filepath.Join(config.DataPath, "models", "gguf", "gemma-3-4b-it-Q8_0.gguf")
	case "embeddings":
		return filepath.Join(config.DataPath, "models", "embeddings", "nomic-embed-text-v1.5.Q8_0.gguf")
	case "reranker":
		return filepath.Join(config.DataPath, "models", "rerankers", "slide-bge-reranker-v2-m3.Q4_K_M.gguf")
	default:
		return ""
	}
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

	hostInfo, err := hostinfopkg.GetHostInfo()
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

// StartPGVectorContainer starts a Docker container running PGVector if Docker is available
func StartPGVectorContainer(config *Config) error {
	// Ensure the pg-manifold Docker image exists
	if err := EnsurePGVectorImage(); err != nil {
		return fmt.Errorf("failed to ensure pg-manifold image: %w", err)
	}

	pterm.Info.Println("Docker is available, checking PGVector container...")

	// Container configuration
	containerName := "pg-manifold"
	volumeName := "postgres-data"

	// Check if container is already running
	checkContainerCmd := exec.Command("docker", "ps", "-q", "--filter", "name="+containerName)
	output, err := checkContainerCmd.Output()
	if err == nil && len(output) > 0 {
		pterm.Success.Printf("PGVector container '%s' is already running\n", containerName)
		return nil
	}

	// Check if container exists but is not running
	checkStoppedCmd := exec.Command("docker", "ps", "-a", "-q", "--filter", "name="+containerName)
	output, err = checkStoppedCmd.Output()
	if err == nil && len(output) > 0 {
		pterm.Info.Printf("PGVector container '%s' exists but is not running, starting it...\n", containerName)
		startCmd := exec.Command("docker", "start", containerName)
		if err := startCmd.Run(); err != nil {
			return fmt.Errorf("failed to start existing PGVector container: %w", err)
		}
		pterm.Success.Printf("PGVector container '%s' started\n", containerName)
	} else {
		// Parse database config for credentials from connection string
		username := "postgres" // default
		password := "postgres" // default
		dbname := "manifold"   // default

		if config != nil && config.Database.ConnectionString != "" {
			if u, p, db, ok := parseConnectionString(config.Database.ConnectionString); ok {
				username = u
				password = p
				dbname = db
			} else {
				pterm.Warning.Println("Could not parse database connection string, using default credentials")
			}
		} else {
			pterm.Warning.Println("No database connection string provided, using default credentials")
		}

		// Create and run new container using intelligencedev/pg-manifold:latest image
		pterm.Info.Printf("Creating new PGVector container: %s with image intelligencedev/pg-manifold:latest\n", containerName)

		runCmd := exec.Command("docker", "run", "-d",
			"--name", containerName,
			"-p", "5432:5432",
			"-v", volumeName+":/var/lib/postgresql/data",
			"-e", "POSTGRES_USER="+username,
			"-e", "POSTGRES_PASSWORD="+password,
			"-e", "POSTGRES_DB="+dbname,
			"intelligencedev/pg-manifold:latest")

		if output, err := runCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to start pgvector container: %w\n%s", err, string(output))
		}

		pterm.Success.Printf("PGVector container '%s' created and started\n", containerName)
	}

	// Wait for the database to be ready
	pterm.Info.Println("Waiting for PGVector database to initialize...")

	// Use full connection string from config if available
	connStr := "postgres://postgres:postgres@localhost:5432/manifold" // Default connection string
	if config != nil && config.Database.ConnectionString != "" {
		connStr = config.Database.ConnectionString
	}

	// Try to connect to the database with timeout
	if err := waitForDatabaseReady(connStr, 60*time.Second); err != nil {
		pterm.Warning.Printf("Database container started but connection check failed: %v\n", err)
		pterm.Warning.Println("Proceeding anyway, but database might not be fully initialized")
	} else {
		pterm.Success.Println("PGVector database is ready to accept connections")
	}

	return nil
}

// waitForDatabaseReady attempts to connect to the database until it succeeds or times out
func waitForDatabaseReady(connStr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// Create a context with timeout for the overall operation
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for time.Now().Before(deadline) {
		// Use a short timeout for each connection attempt
		attemptCtx, attemptCancel := context.WithTimeout(ctx, 3*time.Second)

		// Try to connect
		conn, err := pgx.Connect(attemptCtx, connStr)
		if err == nil {
			// Successfully connected
			conn.Close(attemptCtx)
			attemptCancel()
			return nil
		}

		attemptCancel()

		// Check if our overall deadline is reached
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for database: %w", ctx.Err())
		default:
			// Wait a bit before trying again
			pterm.Info.Println("Waiting for database to be ready...")
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("timed out waiting for database to be ready after %v", timeout)
}

// StopPGVectorContainer stops the PGVector container if it's running
func StopPGVectorContainer() error {
	containerName := "pg-manifold"

	// Check if container is running
	checkCmd := exec.Command("docker", "ps", "-q", "--filter", "name="+containerName)
	output, err := checkCmd.Output()
	if err != nil || len(output) == 0 {
		// Container is not running
		pterm.Info.Printf("PGVector container '%s' is not running, nothing to stop\n", containerName)
		return nil
	}

	pterm.Info.Printf("Gracefully stopping PGVector container '%s'...\n", containerName)

	// Use timeout to ensure graceful shutdown of the database
	stopCmd := exec.Command("docker", "stop", "--time", "10", containerName)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		pterm.Warning.Printf("Error while stopping container: %v\n%s", err, string(output))

		// Try to force kill if graceful stop fails
		killCmd := exec.Command("docker", "kill", containerName)
		if err := killCmd.Run(); err != nil {
			return fmt.Errorf("failed to kill PGVector container: %w", err)
		}
		pterm.Warning.Printf("PGVector container '%s' had to be forcefully killed\n", containerName)
	}

	// Verify the container has actually stopped
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		// Check if the container is still running
		checkCmd := exec.Command("docker", "ps", "-q", "--filter", "name="+containerName)
		output, err := checkCmd.Output()
		if err == nil && len(output) == 0 {
			// Container is no longer running
			pterm.Success.Printf("PGVector container '%s' gracefully stopped\n", containerName)
			return nil
		}

		// Container is still running, wait a bit and try again
		pterm.Warning.Printf("PGVector container '%s' is still running, waiting...\n", containerName)
		time.Sleep(1 * time.Second)
	}

	// If we get here, the container might be stuck
	pterm.Error.Printf("Failed to stop PGVector container '%s' after multiple attempts\n", containerName)

	// Last resort: try to force kill
	killCmd := exec.Command("docker", "kill", containerName)
	if err := killCmd.Run(); err != nil {
		return fmt.Errorf("failed to kill PGVector container after multiple stop attempts: %w", err)
	}

	pterm.Warning.Printf("PGVector container '%s' was forcefully killed after failing to stop gracefully\n", containerName)
	return nil
}

// parseConnectionString attempts to extract username, password and database name from a connection string
func parseConnectionString(conn string) (user, pass, db string, ok bool) {
	user, pass, db = "postgres", "postgres", "manifold"
	at := strings.Index(conn, "@")
	if at == -1 {
		return user, pass, db, false
	}
	creds := conn[strings.Index(conn, "//")+2 : at]
	hostAndDb := conn[at+1:]
	colon := strings.Index(creds, ":")
	if colon != -1 {
		// If username is empty, keep default
		if creds[:colon] != "" {
			user = creds[:colon]
		}
		// If password is empty, keep default
		if creds[colon+1:] != "" {
			pass = creds[colon+1:]
		}
	} else if creds != "" {
		user = creds
	}
	// If creds is empty (i.e., postgres://@host:5432/db), keep defaults and ok=true
	slash := strings.LastIndex(hostAndDb, "/")
	if slash != -1 && slash+1 < len(hostAndDb) {
		db = hostAndDb[slash+1:]
		q := strings.Index(db, "?")
		if q != -1 {
			db = db[:q]
		}
	}
	return user, pass, db, true
}
