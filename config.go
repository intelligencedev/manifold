// manifold/config.go

package main

import (
	"fmt"
	"os"

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

type Config struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	DataPath        string `yaml:"data_path"`
	JaegerHost      string `yaml:"jaeger_host"`
	OpenAIAPIKey    string `yaml:"openai_api_key,omitempty"`
	GoogleGeminiKey string `yaml:"google_gemini_key,omitempty"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}
