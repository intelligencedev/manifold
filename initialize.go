// manifold/initialize.go

package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"manifold/internal/sefii"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
	"github.com/pterm/pterm"
)

// downloadModelFile downloads a file from a URL to a local filepath
func downloadModelFile(url, filePath string) error {
	// Create all parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
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
		if err := downloadModelFile(url, filePath); err != nil {
			return fmt.Errorf("failed to download model %s: %w", url, err)
		}
		pterm.Success.Printf("Successfully downloaded model to %s\n", filePath)
	}

	return nil
}

// downloadLlamaBinary downloads a file from a URL to a local filepath
func downloadLlamaBinary(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// unzipLlamaBinary extracts a zip archive to a destination directory
func unzipLlamaBinary(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Ensure extracted path is within destination directory
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// InitializeLlamaCpp downloads and sets up llama.cpp binaries if they don't exist
func InitializeLlamaCpp(config *Config) error {
	if config.DataPath == "" {
		return fmt.Errorf("data path not configured")
	}

	llamaCppDir := filepath.Join(config.DataPath, "llama-cpp")

	// Determine binary name and path based on OS
	hostInfo, err := GetHostInfo()
	if err != nil {
		return fmt.Errorf("failed to get host info: %w", err)
	}

	binaryName := "llama-server"
	if hostInfo.OS == "windows" {
		binaryName = "llama-server.exe"
	}

	// Check if binary exists in the build/bin directory
	binaryPath := filepath.Join(llamaCppDir, "build", "bin", binaryName)
	if fi, err := os.Stat(binaryPath); err == nil && !fi.IsDir() {
		// On Unix systems, check if the file is executable
		if hostInfo.OS != "windows" {
			if fi.Mode()&0111 != 0 {
				pterm.Info.Printf("llama-server binary found at %s\n", binaryPath)
				return nil
			}
		} else {
			// On Windows just check if file exists
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
	case "windows":
		osArch = "win-cuda-cu12.4-x64"
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
	if err := downloadLlamaBinary(llamaDownloadURL, llamaFilePath); err != nil {
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

		// Make the binary executable on Unix systems if needed
		if hostInfo.OS != "windows" {
			if err := os.Chmod(binaryPath, 0755); err != nil {
				return fmt.Errorf("failed to make binary executable: %w", err)
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

	// Make the binary executable on Unix systems
	if hostInfo.OS != "windows" {
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	pterm.Success.Println("Successfully downloaded and installed llama.cpp binaries")
	return nil
}

// InitializeApplication performs necessary setup tasks, such as creating the data directory.
func InitializeApplication(config *Config) error {
	hostInfo, err := GetHostInfo()
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

	ctx := context.Background()
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

			files, err := ioutil.ReadDir(modelDir)
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
