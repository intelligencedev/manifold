// manifold/initialize.go

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	_ "embed" // Required for go:embed
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
	"github.com/pterm/pterm"

	hostinfopkg "manifold/internal/hostinfo"
	"manifold/internal/sefii"
)

//go:embed sandbox/Dockerfile
var sandboxDockerfile string

//go:embed workflows
var workflowsFS embed.FS

// downloadFile downloads a file from a URL to a local filepath.
// It creates parent directories if they don't exist.
func downloadFile(url, filePath string) error {
	// Create all parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// downloadModels downloads required reranker and embedding models
func downloadModels(config *Config) error {
	if config.DataPath == "" {
		return fmt.Errorf("data path not configured")
	}

	models := map[string]string{
		filepath.Join(config.DataPath, "models", "rerankers", "slide-bge-reranker-v2-m3.Q4_K_M.gguf"): "https://huggingface.co/mradermacher/slide-bge-reranker-v2-m3-GGUF/resolve/main/slide-bge-reranker-v2-m3.Q4_K_M.gguf",
		filepath.Join(config.DataPath, "models", "embeddings", "nomic-embed-text-v1.5.Q8_0.gguf"):     "https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q8_0.gguf",
		filepath.Join(config.DataPath, "models", "gguf", "gemma-3-4b-it-Q8_0.gguf"):                   "https://huggingface.co/unsloth/gemma-3-4b-it-GGUF/resolve/main/gemma-3-4b-it-Q8_0.gguf",
	}

	for filePath, url := range models {
		// Check if file already exists
		if _, err := os.Stat(filePath); err == nil {
			pterm.Info.Printf("Model already exists at %s\n", filePath)
			continue
		}

		pterm.Info.Printf("Downloading model from %s\n", url)
		if err := downloadFile(url, filePath); err != nil {
			return fmt.Errorf("failed to download model %s: %w", url, err)
		}
		pterm.Success.Printf("Successfully downloaded model to %s\n", filePath)
	}

	return nil
}

// extractAndCopyWorkflows extracts the embedded workflows data and copies it to the target directory
func extractAndCopyWorkflows(config *Config) error {
	if config.DataPath == "" {
		return fmt.Errorf("data path not configured")
	}

	workflowsDir := filepath.Join(config.DataPath, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		return fmt.Errorf("failed to create workflows directory: %w", err)
	}

	// Walk through the embedded workflows directory and copy each file
	err := fs.WalkDir(workflowsFS, "workflows", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == "workflows" {
			return nil
		}

		// Get the relative path from the workflows directory
		relPath, err := filepath.Rel("workflows", path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Create the destination path
		destPath := filepath.Join(workflowsDir, relPath)

		// If it's a directory, create it
		if d.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			return nil
		}

		// If it's a file, copy its contents
		data, err := workflowsFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		pterm.Success.Printf("Copied workflow file to %s\n", destPath)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to extract workflows: %w", err)
	}

	return nil
}

// unzipLlamaBinary extracts a zip archive to a destination directory.
// It ensures all paths are properly sanitized to prevent path traversal attacks.
func unzipLlamaBinary(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		// Ensure extracted path is within destination directory (prevent path traversal)
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in zip: %s", f.Name)
		}

		// Create directory if needed
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Create file
		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}

		// Extract content
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return fmt.Errorf("failed to open file in archive: %w", err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to extract file: %w", err)
		}
	}
	return nil
}

// EnsureCodeSandboxImage checks if the code-sandbox Docker image exists,
// and builds it if it doesn't exist using the embedded Dockerfile.
func EnsureCodeSandboxImage() error {
	// Check if Docker is installed
	_, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker is not installed or not in PATH: %w", err)
	}

	// Check if Docker is running
	checkCmd := exec.Command("docker", "info")
	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("docker is not running: %w", err)
	}

	pterm.Info.Println("Docker is available, checking for code-sandbox image...")

	// Check if the image exists
	checkImageCmd := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", "code-sandbox:latest")
	output, err := checkImageCmd.Output()
	if err == nil && len(output) > 0 {
		pterm.Success.Println("code-sandbox:latest image already exists")
		return nil
	}

	pterm.Info.Println("code-sandbox:latest image not found, building it...")

	// Create a temporary directory to store the Dockerfile
	tempDir, err := os.MkdirTemp("", "docker-build-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory for Dockerfile: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write the embedded Dockerfile to the temporary directory
	dockerfilePath := filepath.Join(tempDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(sandboxDockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	// Build the Docker image
	buildCmd := exec.Command("docker", "build", "-t", "code-sandbox:latest", "-f", dockerfilePath, ".")
	buildCmd.Dir = tempDir

	// Capture and display the build output
	var stdoutBuf, stderrBuf bytes.Buffer
	buildCmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	buildCmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	pterm.Info.Println("Building code-sandbox Docker image...")
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build code-sandbox image: %w\n%s", err, stderrBuf.String())
	}

	pterm.Success.Println("code-sandbox:latest image successfully built")
	return nil
}

// InitializeLlamaCpp downloads and sets up llama.cpp binaries if they don't exist
func InitializeLlamaCpp(config *Config) error {
	if config.DataPath == "" {
		return fmt.Errorf("data path not configured")
	}

	llamaCppDir := filepath.Join(config.DataPath, "llama-cpp")

	// Determine binary name and path based on OS
	hostInfo, err := hostinfopkg.GetHostInfo()
	if err != nil {
		return fmt.Errorf("failed to get host info: %w", err)
	}

	binaryName := "llama-server"

	// Check if binary exists in the build/bin directory
	binaryPath := filepath.Join(llamaCppDir, "build", "bin", binaryName)
	if fi, err := os.Stat(binaryPath); err == nil && !fi.IsDir() {
		// On Unix systems, check if the file is executable
		if fi.Mode()&0111 != 0 {
			pterm.Info.Printf("llama-server binary found at %s\n", binaryPath)
			return nil
		}

	}

	pterm.Info.Println("llama-server binary not found, downloading llama.cpp...")

	// Create llama-cpp directory
	if err := os.MkdirAll(llamaCppDir, 0755); err != nil {
		return fmt.Errorf("failed to create llama-cpp directory: %w", err)
	}

	// Determine OS/arch for download
	var osArch string
	switch hostInfo.OS {
	case "darwin":
		if hostInfo.Arch == "arm64" {
			osArch = "macos-arm64"
		} else {
			return fmt.Errorf("unsupported macOS architecture")
		}
	case "linux":
		osArch = "ubuntu-x64"
	default:
		return fmt.Errorf("unsupported operating system")
	}

	// Get latest release info from GitHub
	resp, err := http.Get("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
	if err != nil {
		return fmt.Errorf("failed to fetch latest release info: %w", err)
	}
	defer resp.Body.Close()

	var release map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to decode release info: %w", err)
	}

	assets, ok := release["assets"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid release assets format")
	}

	var llamaDownloadURL string
	var releaseVersion string
	if tag, ok := release["tag_name"].(string); ok {
		releaseVersion = strings.TrimPrefix(tag, "b")
	}

	for _, asset := range assets {
		assetMap, ok := asset.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := assetMap["name"].(string)
		if !ok {
			continue
		}
		downloadURL, ok := assetMap["browser_download_url"].(string)
		if !ok {
			continue
		}

		if releaseVersion != "" && strings.Contains(name, "llama-b"+releaseVersion+"-bin-"+osArch) && strings.HasSuffix(name, ".zip") {
			llamaDownloadURL = downloadURL
			break
		}
	}

	if llamaDownloadURL == "" {
		return fmt.Errorf("could not find download URL for system architecture")
	}

	// Download and extract llama.cpp
	llamaFilePath := filepath.Join(llamaCppDir, "llama.zip")
	if err := downloadFile(llamaDownloadURL, llamaFilePath); err != nil {
		return fmt.Errorf("failed to download llama.cpp: %w", err)
	}

	if err := unzipLlamaBinary(llamaFilePath, llamaCppDir); err != nil {
		os.Remove(llamaFilePath)
		return fmt.Errorf("failed to unzip llama.cpp: %w", err)
	}
	os.Remove(llamaFilePath)

	// After extraction, check if the binary is already in the build/bin directory
	buildBinDir := filepath.Join(llamaCppDir, "build", "bin")
	binaryPath = filepath.Join(buildBinDir, binaryName)

	// First, check if binary exists directly in the expected location
	if _, err := os.Stat(binaryPath); err == nil {
		// Binary already exists in the correct location
		pterm.Info.Printf("llama-server binary found at %s\n", binaryPath)

		if err := os.Chmod(binaryPath, 0755); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
		// On Linux, copy all *.so files to current working directory
		if hostInfo.OS == "linux" {
			if err := copySharedLibsToCurrentDir(buildBinDir); err != nil {
				return fmt.Errorf("failed to copy shared libraries: %w", err)
			}
		}

		return nil
	}

	// If not in build/bin, create directory structure if it doesn't exist
	if err := os.MkdirAll(buildBinDir, 0755); err != nil {
		return fmt.Errorf("failed to create build/bin directory: %w", err)
	}

	// Check if binary is in root directory
	oldBinaryPath := filepath.Join(llamaCppDir, binaryName)
	if _, err := os.Stat(oldBinaryPath); err == nil {
		// Move the binary from root to build/bin
		if err := os.Rename(oldBinaryPath, binaryPath); err != nil {
			return fmt.Errorf("failed to move binary to build/bin: %w", err)
		}
	} else {
		// Binary wasn't found in either location - try to locate it
		var binaryFound bool

		err := filepath.Walk(llamaCppDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && strings.HasSuffix(info.Name(), binaryName) {
				pterm.Info.Printf("Found llama-server binary at %s\n", path)

				// If found, copy it to the build/bin location
				srcFile, err := os.Open(path)
				if err != nil {
					return err
				}
				defer srcFile.Close()

				destFile, err := os.Create(binaryPath)
				if err != nil {
					return err
				}
				defer destFile.Close()

				_, err = io.Copy(destFile, srcFile)
				if err != nil {
					return err
				}

				binaryFound = true
				return filepath.SkipDir
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("error searching for llama-server binary: %w", err)
		}

		if !binaryFound {
			return fmt.Errorf("could not find llama-server binary in extracted files")
		}
	}

	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	// On Linux, copy all *.so files to current working directory
	if hostInfo.OS == "linux" {
		if err := copySharedLibsToCurrentDir(buildBinDir); err != nil {
			return fmt.Errorf("failed to copy shared libraries: %w", err)
		}
	}

	pterm.Success.Println("Successfully downloaded and installed llama.cpp binaries")
	return nil
}

// InitializeApplication performs necessary setup tasks, such as creating the data directory.
func InitializeApplication(config *Config) error {
	hostInfo, err := hostinfopkg.GetHostInfo()
	if err != nil {
		pterm.Error.Printf("Failed to get host information: %+v\n", err)
	} else {
		tableData := [][]string{
			{"Key", "Value"},
			{"OS", hostInfo.OS},
			{"Arch", hostInfo.Arch},
			{"CPUs", fmt.Sprintf("%d", hostInfo.CPUs)},
			{"Total Memory (GB)", fmt.Sprintf("%.2f", float64(hostInfo.Memory.Total)/(1024*1024*1024))},
		}

		// Only add GPU information if GPUs are available
		if len(hostInfo.GPUs) > 0 {
			tableData = append(tableData,
				[]string{"GPU Model", hostInfo.GPUs[0].Model},
				[]string{"GPU Cores", hostInfo.GPUs[0].TotalNumberOfCores},
				[]string{"Metal Support", hostInfo.GPUs[0].MetalSupport},
			)
		} else {
			tableData = append(tableData, []string{"GPU", "None detected"})
		}

		pterm.DefaultTable.WithData(pterm.TableData(tableData)).Render()
	}

	if config.DataPath != "" {
		if _, err := os.Stat(config.DataPath); os.IsNotExist(err) {
			pterm.Info.Printf("Data directory '%s' does not exist, creating it...\n", config.DataPath)
			if err := os.MkdirAll(config.DataPath, 0755); err != nil {
				return fmt.Errorf("failed to create data directory: %w", err)
			}
			pterm.Success.Printf("Data directory '%s' created successfully.\n", config.DataPath)
		} else if err != nil {
			return fmt.Errorf("failed to stat data directory: %w", err)
		}

		// Create model directories
		modelDirs := []string{
			filepath.Join(config.DataPath, "models"),
			filepath.Join(config.DataPath, "models", "gguf"),
			filepath.Join(config.DataPath, "models", "mlx"),
			filepath.Join(config.DataPath, "models", "embeddings"),
			filepath.Join(config.DataPath, "models", "rerankers"),
		}

		for _, dir := range modelDirs {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create model directory %s: %w", dir, err)
			}
			pterm.Success.Printf("Model directory '%s' created successfully.\n", dir)
		}

		// Create the temp directory
		tempDir := filepath.Join(config.DataPath, "tmp")
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}

		// Use WaitGroup to wait for concurrent initialization tasks
		var wg sync.WaitGroup
		var initErrors []error
		var errorsMutex sync.Mutex

		// Helper function to add errors
		addError := func(err error) {
			errorsMutex.Lock()
			initErrors = append(initErrors, err)
			errorsMutex.Unlock()
		}

		// Ensure code-sandbox Docker image exists
		wg.Add(1)
		go func() {
			defer wg.Done()
			pterm.Info.Println("Checking code-sandbox Docker image...")
			if err := EnsureCodeSandboxImage(); err != nil {
				addError(fmt.Errorf("code-sandbox image initialization error: %w", err))
				pterm.Warning.Printf("Failed to initialize code-sandbox Docker image: %v\n", err)
			}
		}()

		// Start PGVector container in a goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			pterm.Info.Println("Initializing PGVector database container...")
			if err := StartPGVectorContainer(config); err != nil {
				addError(fmt.Errorf("PGVector container initialization error: %w", err))
				pterm.Warning.Printf("Failed to initialize PGVector container: %v\n", err)
			}
		}()

		// Initialize llama.cpp in a goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := InitializeLlamaCpp(config); err != nil {
				addError(fmt.Errorf("llama.cpp initialization error: %w", err))
				pterm.Warning.Printf("Failed to initialize llama.cpp: %v\n", err)
			}
		}()

		// Download required models in a goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := downloadModels(config); err != nil {
				addError(fmt.Errorf("model download error: %w", err))
				pterm.Warning.Printf("Failed to download models: %v\n", err)
			}
		}()

		// Extract and copy workflows
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := extractAndCopyWorkflows(config); err != nil {
				addError(fmt.Errorf("workflow extraction error: %w", err))
				pterm.Warning.Printf("Failed to extract and copy workflows: %v\n", err)
			}
		}()

		// Wait for all initialization tasks to complete
		wg.Wait()

		// Check if there were any errors during initialization
		if len(initErrors) > 0 {
			pterm.Warning.Println("Some initialization tasks encountered errors:")
			for _, err := range initErrors {
				pterm.Warning.Println("- " + err.Error())
			}
			// Continue anyway since some errors might be non-fatal
		}
	}

	// Use the existing connection pool rather than creating a new connection
	ctx := context.Background()

	// We'll use the pool that was already created in main.go
	if config.DBPool == nil {
		// If the pool doesn't exist yet (which shouldn't happen), create a backup connection temporarily
		pterm.Warning.Println("Database pool not initialized, creating a temporary connection")
		db, err := Connect(ctx, config.Database.ConnectionString)
		if err != nil {
			pterm.Fatal.Println(err)
		}
		defer db.Close(ctx)

		_, err = db.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
		if err != nil {
			panic(err)
		}
		err = pgxvector.RegisterTypes(ctx, db)
		if err != nil {
			panic(err)
		}

		engine := sefii.NewEngine(db)
		engine.EnsureTable(ctx, config.Embeddings.Dimensions)
		engine.EnsureInvertedIndexTable(ctx)
	} else {
		// Use the existing connection pool
		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			pterm.Fatal.Println("Failed to acquire connection from pool:", err)
		}
		defer conn.Release()

		_, err = conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
		if err != nil {
			panic(err)
		}
		err = pgxvector.RegisterTypes(ctx, conn.Conn())
		if err != nil {
			panic(err)
		}

		engine := sefii.NewEngine(conn.Conn())
		engine.EnsureTable(ctx, config.Embeddings.Dimensions)
		engine.EnsureInvertedIndexTable(ctx)
	}

	// Start local services if SingleNodeInstance is true
	if config.SingleNodeInstance {
		pterm.Info.Println("Running in single node mode, starting local services...")
		if err := StartLocalServices(config); err != nil {
			pterm.Warning.Printf("Failed to start local services: %v\n", err)
		}
	}

	// Note: Signal handling removed from here - now centralized in main.go

	return nil
}

func CreateModelsTable(ctx context.Context, db *pgx.Conn) error {
	_, err := db.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS models (
            id SERIAL PRIMARY KEY,
            name TEXT UNIQUE,
            path TEXT UNIQUE,
            model_type TEXT,
            temperature FLOAT,
            top_p FLOAT,
            top_k INT,
            repetition_penalty FLOAT,
            ctx INT
        )
    `)
	if err != nil {
		return fmt.Errorf("failed to create models table: %w", err)
	}

	return nil
}

func ScanGGUFModels(modelsDir string) ([]LanguageModel, error) {
	var ggufModels []LanguageModel

	ggufPath := filepath.Join(modelsDir, "models-gguf")
	entries, err := os.ReadDir(ggufPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read models-gguf directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			modelName := entry.Name()
			modelDir := filepath.Join(ggufPath, modelName)

			files, err := os.ReadDir(modelDir)
			if err != nil {
				pterm.Error.Printf("Failed to read directory %s: %v\n", modelDir, err)
				continue
			}

			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".gguf") {
					fullPath := filepath.Join(modelDir, file.Name())
					ggufModels = append(ggufModels, LanguageModel{
						Name:              modelName,
						Path:              fullPath,
						ModelType:         "gguf",
						Temperature:       0.6,
						TopP:              0.9,
						TopK:              50,
						RepetitionPenalty: 1.1,
						Ctx:               4096,
					})
					break
				}
			}
		}
	}

	return ggufModels, nil
}

func ScanMLXModels(modelsDir string) ([]LanguageModel, error) {
	var mlxModels []LanguageModel

	mlxPath := filepath.Join(modelsDir, "models-mlx")
	entries, err := os.ReadDir(mlxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read models-mlx directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			modelName := entry.Name()
			modelDir := filepath.Join(mlxPath, modelName)

			files, err := os.ReadDir(modelDir)
			if err != nil {
				pterm.Error.Printf("Failed to read directory %s: %v\n", modelDir, err)
				continue
			}

			var safetensorsPath string
			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".safetensors") {
					fullPath := filepath.Join(modelDir, file.Name())
					safetensorsPath = fullPath
					break
				}
			}

			if safetensorsPath != "" {
				mlxModels = append(mlxModels, LanguageModel{
					Name:              modelName,
					Path:              safetensorsPath,
					ModelType:         "mlx",
					Temperature:       0.5,
					TopP:              0.9,
					TopK:              50,
					RepetitionPenalty: 1.1,
					Ctx:               4096,
				})
			}
		}
	}

	return mlxModels, nil
}

// copySharedLibsToCurrentDir copies all *.so files from the source directory to the current working directory
func copySharedLibsToCurrentDir(sourceDir string) error {
	// Get current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	pterm.Info.Printf("Copying shared libraries from %s to %s\n", sourceDir, currentDir)

	// Read the source directory
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	// Keep track if we copied any files
	filesCopied := false

	// Copy each .so file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if the file is a shared library
		if strings.HasSuffix(entry.Name(), ".so") {
			srcPath := filepath.Join(sourceDir, entry.Name())
			destPath := filepath.Join(currentDir, entry.Name())

			// Open source file
			srcFile, err := os.Open(srcPath)
			if err != nil {
				return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
			}

			// Create destination file
			destFile, err := os.Create(destPath)
			if err != nil {
				srcFile.Close()
				return fmt.Errorf("failed to create destination file %s: %w", destPath, err)
			}

			// Copy the file contents
			_, err = io.Copy(destFile, srcFile)
			srcFile.Close()
			destFile.Close()

			if err != nil {
				return fmt.Errorf("failed to copy file %s to %s: %w", srcPath, destPath, err)
			}

			filesCopied = true
			pterm.Info.Printf("Copied %s to %s\n", srcPath, destPath)
		}
	}

	if filesCopied {
		pterm.Success.Println("Successfully copied shared libraries to working directory")
	} else {
		pterm.Info.Println("No shared libraries found to copy")
	}

	return nil
}
