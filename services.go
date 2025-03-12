package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type LlamaService struct {
	Name       string
	Process    *exec.Cmd
	ConfigPath string
	Port       int
}

var (
	servicesMutex sync.Mutex
	services      = make(map[string]*LlamaService)
)

// StartEmbeddingsService starts the local embeddings service if no remote host is configured
func StartEmbeddingsService(config *Config) error {
	if config.Embeddings.Host != "" {
		return nil // Remote host is configured, don't start local service
	}

	servicesMutex.Lock()
	defer servicesMutex.Unlock()

	// Default port for embeddings service
	const embeddingsPort = 32184

	modelPath := filepath.Join(config.DataPath, "models", "embeddings", "nomic-embed-text-v1.5.Q8_0.gguf")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("embeddings model not found at %s", modelPath)
	}

	llamaBinary := filepath.Join(config.DataPath, "llama-cpp", "main")
	cmd := exec.Command(llamaBinary,
		"-m", modelPath,
		"-c", "65536",
		"-np", "8",
		"-b", "8192",
		"-ub", "8192",
		"-fa",
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", embeddingsPort),
		"-lv", "1",
		"--embedding")

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start embeddings service: %w", err)
	}

	services["embeddings"] = &LlamaService{
		Name:    "embeddings",
		Process: cmd,
		Port:    embeddingsPort,
	}

	// Update config to use local service
	config.Embeddings.Host = fmt.Sprintf("http://127.0.0.1:%d/v1/embeddings", embeddingsPort)
	return nil
}

// StartRerankerService starts the local reranker service if no remote host is configured
func StartRerankerService(config *Config) error {
	if config.Reranker.Host != "" {
		return nil // Remote host is configured, don't start local service
	}

	servicesMutex.Lock()
	defer servicesMutex.Unlock()

	// Default port for reranker service
	const rerankerPort = 32185

	modelPath := filepath.Join(config.DataPath, "models", "rerankers", "slide-bge-reranker-v2-m3.Q4_K_M.gguf")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("reranker model not found at %s", modelPath)
	}

	llamaBinary := filepath.Join(config.DataPath, "llama-cpp", "main")
	cmd := exec.Command(llamaBinary,
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

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start reranker service: %w", err)
	}

	services["reranker"] = &LlamaService{
		Name:    "reranker",
		Process: cmd,
		Port:    rerankerPort,
	}

	// Update config to use local service
	config.Reranker.Host = fmt.Sprintf("http://127.0.0.1:%d/v1/rerank", rerankerPort)
	return nil
}

// StopAllServices stops all running llama.cpp services
func StopAllServices() {
	servicesMutex.Lock()
	defer servicesMutex.Unlock()

	for _, service := range services {
		if service.Process != nil && service.Process.Process != nil {
			service.Process.Process.Kill()
		}
	}
}
