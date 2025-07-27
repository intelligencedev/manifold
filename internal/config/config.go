// manifold/config.go

package config

import (
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
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

type ReactAgentConfig struct {
	MaxSteps int  `yaml:"max_steps"`
	Memory   bool `yaml:"memory"`
	NumTools int  `yaml:"num_tools"`
}

type FleetWorker struct {
	Name         string  `json:"name"`
	Model        string  `json:"model,omitempty"`
	Role         string  `json:"role"`
	Endpoint     string  `json:"endpoint"`
	CtxSize      int     `json:"ctx_size"`
	Temperature  float64 `json:"temperature"`
	ApiKey       string  `json:"api_key,omitempty"`
	Instructions string  `json:"instructions"`
	MaxSteps     int     `json:"max_steps"`
	Memory       bool    `json:"memory"`
}

type AgentFleet struct {
	Workers []FleetWorker `json:"workers"`
}

type AgenticMemoryConfig struct {
	Enabled bool `yaml:"enabled"`
}

// A2AConfig defines settings for the Agent2Agent protocol.
type A2AConfig struct {
	// Role specifies the node's role in the cluster ("master" or "worker").
	Role string `yaml:"role"`
	// Token is the shared secret used for authenticating A2A requests.
	Token string `yaml:"token"`
	// Nodes lists the URLs of remote nodes participating in the cluster.
	Nodes []string `yaml:"nodes"`
}

type CompletionsConfig struct {
	DefaultHost      string           `yaml:"default_host"`
	SummaryHost      string           `yaml:"summary_host,omitempty"`
	KeywordsHost     string           `yaml:"keywords_host,omitempty"`
	Backend          string           `yaml:"backend"` // e.g., "openai", "llamacpp", "mlx"
	CompletionsModel string           `yaml:"completions_model"`
	Temperature      float64          `yaml:"temperature"`
	CtxSize          int              `yaml:"ctx_size"`
	APIKey           string           `yaml:"api_key"`
	ReactAgentConfig ReactAgentConfig `yaml:"agent"`
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

type AuthConfig struct {
	SecretKey   string `yaml:"secret_key"`
	TokenExpiry int    `yaml:"token_expiry"` // Token expiry in hours
}

type WebSearchToolConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Backend    string `yaml:"backend"`            // e.g., "serpapi", "bing"
	Endpoint   string `yaml:"endpoint,omniempty"` // API endpoint for the search service
	ResultSize int    `yaml:"result_size"`        // Number of results to fetch
}

type IngestionConfig struct {
	MaxWorkers  int  `yaml:"max_workers"`
	UseAdvanced bool `yaml:"use_advanced_splitting"`
}

type ToolsConfig struct {
	Search WebSearchToolConfig
}

// TelemetryConfig controls OpenTelemetry settings.
type TelemetryConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Endpoint    string `yaml:"endpoint"`
	Insecure    bool   `yaml:"insecure"`
	ServiceName string `yaml:"service_name"`
}

type Config struct {
	Host                      string              `yaml:"host"`
	Port                      int                 `yaml:"port"`
	DataPath                  string              `yaml:"data_path"`
	SingleNodeInstance        bool                `yaml:"single_node_instance,omitempty"`
	GitHubPersonalAccessToken string              `yaml:"github_personal_access_token"`
	AnthropicKey              string              `yaml:"anthropic_key,omitempty"`
	OpenAIAPIKey              string              `yaml:"openai_api_key,omitempty"`
	GoogleGeminiKey           string              `yaml:"google_gemini_key,omitempty"`
	HuggingFaceToken          string              `yaml:"hf_token,omitempty"`
	Database                  DatabaseConfig      `yaml:"database"`
	DBPool                    *pgxpool.Pool       `yaml:"-"` // PgxPool is not serialized, used for database connections
	Completions               CompletionsConfig   `yaml:"completions"`
	Embeddings                EmbeddingsConfig    `yaml:"embeddings"`
	Reranker                  RerankerConfig      `yaml:"reranker"`
	Auth                      AuthConfig          `yaml:"auth"`
	AgentFleet                AgentFleet          `yaml:"agent_fleet,omitempty"`
	AgenticMemory             AgenticMemoryConfig `yaml:"agentic_memory"`
	A2A                       A2AConfig           `yaml:"a2a,omitempty"`
	Tools                     ToolsConfig         `yaml:"tools,omitempty"`
	OTel                      TelemetryConfig     `yaml:"otel"`
	Ingestion                 IngestionConfig     `yaml:"ingestion"`
}

// LoadConfig reads the configuration from a YAML file, unmarshals it into a Config struct,
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

	// Set default values for Auth if not provided
	if config.Auth.SecretKey == "" {
		config.Auth.SecretKey = "your-secret-key" // Default fallback (should be changed in production)
		pterm.Warning.Println("No JWT secret key provided in config, using default (insecure).")
	}

	if config.Auth.TokenExpiry <= 0 {
		config.Auth.TokenExpiry = 72 // Default to 72 hours
		pterm.Info.Println("No token expiry specified, using default (72 hours).")
	}

	// Set default values for Ingestion if not provided
	if config.Ingestion.MaxWorkers <= 0 {
		config.Ingestion.MaxWorkers = 4 // Default to 4 workers
		pterm.Info.Println("No max_workers specified for ingestion, using default (4).")
	}

	// Default to using advanced splitting for better code structure awareness
	if !config.Ingestion.UseAdvanced {
		config.Ingestion.UseAdvanced = true
		pterm.Info.Println("Advanced splitting enabled by default for better code structure preservation.")
	}

	if config.OTel.ServiceName == "" {
		config.OTel.ServiceName = "manifold"
	}

	pterm.Success.Println("Configuration loaded successfully.")
	return &config, nil
}
