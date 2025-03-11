package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

func configHandler(c echo.Context) error {
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load config"})
	}
	return c.JSON(http.StatusOK, config)
}

func getFileSystem() http.FileSystem {
	fsys, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to get file system: %v", err)
	}
	return http.FS(fsys)
}

func downloadLlamaHandler(c echo.Context) error {
	cudaVersion := c.FormValue("cuda")
	osArch := c.FormValue("osarch")
	if cudaVersion == "" || osArch == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Both 'cuda' and 'osarch' parameters are required."})
	}
	if cudaVersion != "cu11" && cudaVersion != "cu12" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid 'cuda' parameter. Supported values are 'cu11' and 'cu12'."})
	}
	validArchs := map[string]bool{
		"macos-arm64":         true,
		"ubuntu-x64":          true,
		"win-cuda-cu11.7-x64": true,
		"win-cuda-cu12.4-x64": true,
	}
	if !validArchs[osArch] {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid 'osarch' parameter. Supported values are 'macos-arm64', 'ubuntu-x64', 'win-cuda-cu11.7-x64', 'win-cuda-cu12.4-x64'."})
	}
	resp, err := http.Get("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch latest release info from GitHub."})
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("GitHub API request failed with status: %s", resp.Status)})
	}
	var release map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to decode GitHub API response."})
	}
	assets, ok := release["assets"].([]interface{})
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to find assets in the release info."})
	}
	var cudartDownloadURL, llamaDownloadURL string
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
		if strings.Contains(name, "cudart-llama-bin-win-"+cudaVersion) && strings.HasSuffix(name, ".zip") {
			cudartDownloadURL = downloadURL
		}
		if releaseVersion != "" && strings.Contains(name, "llama-b"+releaseVersion+"-bin-"+osArch) && strings.HasSuffix(name, ".zip") {
			llamaDownloadURL = downloadURL
		}
	}
	if cudartDownloadURL == "" || llamaDownloadURL == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Could not find download URLs for the specified 'cuda' and 'osarch'."})
	}
	tempDir, err := os.MkdirTemp("", "llama-downloads")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create temporary directory."})
	}
	defer os.RemoveAll(tempDir)
	cudartFilePath := filepath.Join(tempDir, "cudart.zip")
	llamaFilePath := filepath.Join(tempDir, "llama.zip")
	if err := downloadFile(cudartDownloadURL, cudartFilePath); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to download cudart: %v", err)})
	}
	if err := downloadFile(llamaDownloadURL, llamaFilePath); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to download llama: %v", err)})
	}
	return c.JSON(http.StatusOK, map[string]string{
		"message":          "Successfully downloaded llama.cpp release files.",
		"cudart_file_path": cudartFilePath,
		"llama_file_path":  llamaFilePath,
	})
}

// Helper function to download a file
func downloadFile(url, filepath string) error {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

// executeMCPHandler handles the MCP execution request using an MCP server.
func executeMCPHandler(c echo.Context) error {
	// Parse the JSON payload from the request.
	var payload map[string]interface{}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid payload"})
	}

	log.Printf("Received MCP payload: %+v", payload)

	// Determine the action specified in the payload.
	action, ok := payload["action"].(string)
	if !ok || action == "" {
		action = "listTools"
	}

	// Launch the MCP server process (using the real server example).
	cmd := exec.Command("go", "run", "cmd/mcpserver/mcpserver.go")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get server stdin pipe"})
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get server stdout pipe"})
	}

	if err := cmd.Start(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to start MCP server process"})
	}
	// Ensure the server is terminated when done.
	defer cmd.Process.Kill()

	// Create an MCP client using the stdio transport.
	clientTransport := stdio.NewStdioServerTransportWithIO(stdout, stdin)
	client := mcp.NewClient(clientTransport)

	// Initialize the client.
	if _, err := client.Initialize(context.Background()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to initialize MCP client: %v", err)})
	}

	// Prepare a variable to hold the result.
	var result interface{}

	// Choose action based on the payload.
	switch action {
	case "listTools":
		tools, err := client.ListTools(context.Background(), nil)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to list tools: %v", err)})
		}
		result = tools
	case "execute":
		// Expect payload to include "tool" and "args"
		toolName, ok := payload["tool"].(string)
		if !ok || toolName == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing tool name for execution"})
		}
		args, ok := payload["args"].(map[string]interface{})
		if !ok {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing or invalid arguments for tool execution"})
		}
		toolResp, err := client.CallTool(context.Background(), toolName, args)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Failed to call tool: %v", err)})
		}
		result = toolResp
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Action '%s' not implemented", action)})
	}

	// Return the result as a JSON response.
	return c.JSON(http.StatusOK, result)
}
