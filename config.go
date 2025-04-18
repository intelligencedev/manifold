// manifold/config.go

package main

import (
	"fmt"
	"os"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v2"
)

type ServiceConfig struct {
	Name      string   `yaml:"name"`
	Host      string   `yaml:"host"`
	Port      int      `yaml:"port"`
	Command   string   `yaml:"command"`
	GPULayers string   `yaml:"gpu_layers,omitempty"`
	Args      []string `yaml:"args,omitempty"`
	Model     string   `yaml:"model,omitempty"`
}

type ToolConfig struct {
	Name       string                 `yaml:"name"`
	Parameters map[string]interface{} `yaml:"parameters"`
}

type DatabaseConfig struct {
	ConnectionString string `yaml:"connection_string"`
}

type CompletionsConfig struct {
	DefaultHost      string `yaml:"default_host"`
	CompletionsModel string `yaml:"completions_model"`
	APIKey           string `yaml:"api_key"`
}

type EmbeddingsConfig struct {
	Host         string `yaml:"host"`
	APIKey       string `yaml:"api_key"`
	Dimensions   int    `yaml:"dimensions"`
	EmbedPrefix  string `yaml:"embed_prefix"`
	SearchPrefix string `yaml:"search_prefix"`
}

type RerankerConfig struct {
	Host string `yaml:"host"`
}

type Config struct {
	Host                      string            `yaml:"host"`
	Port                      int               `yaml:"port"`
	DataPath                  string            `yaml:"data_path"`
	SingleNodeInstance        bool              `yaml:"single_node_instance,omitempty"`
	GitHubPersonalAccessToken string            `yaml:"github_personal_access_token"`
	AnthropicKey              string            `yaml:"anthropic_key,omitempty"`
	OpenAIAPIKey              string            `yaml:"openai_api_key,omitempty"`
	GoogleGeminiKey           string            `yaml:"google_gemini_key,omitempty"`
	HuggingFaceToken          string            `yaml:"hf_token,omitempty"`
	Database                  DatabaseConfig    `yaml:"database"`
	Completions               CompletionsConfig `yaml:"completions"`
	Embeddings                EmbeddingsConfig  `yaml:"embeddings"`
	Reranker                  RerankerConfig    `yaml:"reranker"`
}

// LoadConfig reads the configuration from a YAML file, unmarshals it into a Config struct,
// logs the outcome using pterm, and prints the loaded configuration as pretty printed JSON.
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		pterm.Error.Printf("Error reading config file: %v\n", err)
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		pterm.Error.Printf("Error unmarshaling config: %v\n", err)
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	pterm.Success.Println("Configuration loaded successfully.")
	return &config, nil
}
