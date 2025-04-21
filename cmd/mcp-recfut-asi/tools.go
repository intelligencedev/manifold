package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// Config represents the structure of our config.yaml file
type Config struct {
	MCPServers map[string]MCPServerConfig `yaml:"mcpServers"`
}

// MCPServerConfig represents the configuration for each MCP server
type MCPServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

// PingArgs is an empty struct as the ping endpoint doesn't require arguments
type PingArgs struct{}

// PingResponse represents the structure of the response from the ping endpoint
type PingResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// LoadConfig loads the configuration from the config.yaml file
func LoadConfig() (*Config, error) {
	// First, try to find the config.yaml in the same directory as the executable
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	execDir := filepath.Dir(execPath)
	configPath := filepath.Join(execDir, "config.yaml")

	// Check if the config file exists at the executable directory
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// If not, try to find it in the current working directory
		workDir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
		configPath = filepath.Join(workDir, "config.yaml")

		// If still not found, try looking in the dist directory
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = filepath.Join(workDir, "dist", "config.yaml")
		}
	}

	// Read the config file
	configData, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the config file
	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// GetSecurityTrailsAPIKey extracts the SecurityTrails API key from the environment variable or config
func GetSecurityTrailsAPIKey() (string, error) {
	// First, check if the API key is available as an environment variable
	apiKey := os.Getenv("SECURITYTRAILS_API_KEY")
	if apiKey != "" {
		return apiKey, nil
	}

	// If not found in environment, try to load from config
	config, err := LoadConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	// Extract the API key from the config
	securityTrailsConfig, exists := config.MCPServers["securitytrails"]
	if !exists {
		return "", fmt.Errorf("securitytrails configuration not found in config.yaml")
	}

	apiKey, exists = securityTrailsConfig.Env["SECURITYTRAILS_API_KEY"]
	if !exists || apiKey == "" {
		return "", fmt.Errorf("SecurityTrails API key not found in config.yaml")
	}

	return apiKey, nil
}

// pingTool implements the ping endpoint for SecurityTrails API
func pingTool(_ PingArgs) (string, error) {
	// Get the API key
	apiKey, err := GetSecurityTrailsAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to get SecurityTrails API key: %w", err)
	}

	// Create the HTTP request
	url := "https://api.securitytrails.com/v1/ping"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameter for API key
	q := req.URL.Query()
	q.Add("apikey", apiKey)
	req.URL.RawQuery = q.Encode()

	// Add headers
	req.Header.Add("accept", "application/json")

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the response
	var pingResp PingResponse
	if err := json.Unmarshal(body, &pingResp); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	// Return a formatted response
	response := fmt.Sprintf("SecurityTrails API Ping Response:\nSuccess: %t\nMessage: %s",
		pingResp.Success, pingResp.Message)

	return response, nil
}
