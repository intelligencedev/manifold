package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	yaml "gopkg.in/yaml.v3"
)

// Load reads configuration from environment variables (optionally .env).
func Load() (Config, error) {
	// Use Overload so .env values override existing OS environment variables.
	// This allows repository/local configuration to deterministically control
	// runtime behavior in development unless explicitly changed.
	_ = godotenv.Overload()

	cfg := Config{}
	// Allow overriding the agent system prompt via env var SYSTEM_PROMPT.
	cfg.SystemPrompt = strings.TrimSpace(os.Getenv("SYSTEM_PROMPT"))
	// Read environment values first (no defaults here; we'll apply defaults later)
	cfg.OpenAI.APIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	cfg.OpenAI.Model = strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	// Allow overriding API base via env (useful for proxies/self-hosted gateways)
	cfg.OpenAI.BaseURL = firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")), strings.TrimSpace(os.Getenv("OPENAI_API_BASE_URL")))
	cfg.Workdir = strings.TrimSpace(os.Getenv("WORKDIR"))
	cfg.LogPath = strings.TrimSpace(os.Getenv("LOG_PATH"))
	cfg.LogLevel = strings.TrimSpace(os.Getenv("LOG_LEVEL"))
	if v := strings.TrimSpace(os.Getenv("LOG_PAYLOADS")); v != "" {
		cfg.LogPayloads = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
		cfg.OpenAI.LogPayloads = cfg.LogPayloads
	}
	// Int env parsing without defaults; defaults applied after YAML
	if v := strings.TrimSpace(os.Getenv("MAX_COMMAND_SECONDS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Exec.MaxCommandSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("OUTPUT_TRUNCATE_BYTES")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.OutputTruncateByte = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("MAX_STEPS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.MaxSteps = n
		}
	}

	cfg.Obs.ServiceName = strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	cfg.Obs.ServiceVersion = strings.TrimSpace(os.Getenv("SERVICE_VERSION"))
	cfg.Obs.Environment = strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	cfg.Obs.OTLP = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))

	cfg.Web.SearXNGURL = strings.TrimSpace(os.Getenv("SEARXNG_URL"))
	// TTS defaults (optional)
	cfg.TTS.BaseURL = strings.TrimSpace(os.Getenv("TTS_BASE_URL"))
	cfg.TTS.Model = strings.TrimSpace(os.Getenv("TTS_MODEL"))
	cfg.TTS.Voice = strings.TrimSpace(os.Getenv("TTS_VOICE"))

	// Summary configuration via env
	if v := strings.TrimSpace(os.Getenv("SUMMARY_ENABLED")); v != "" {
		cfg.SummaryEnabled = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_THRESHOLD")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryThreshold = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SUMMARY_KEEP_LAST")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.SummaryKeepLast = n
		}
	}

	// Global enableTools via env (overrides YAML)
	if v := strings.TrimSpace(os.Getenv("ENABLE_TOOLS")); v != "" {
		cfg.EnableTools = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}

	// Database backends via environment variables
	// Optional default shared DSN (e.g., postgresql://...)
	cfg.Databases.DefaultDSN = firstNonEmpty(strings.TrimSpace(os.Getenv("DATABASE_URL")), strings.TrimSpace(os.Getenv("DB_URL")), strings.TrimSpace(os.Getenv("POSTGRES_DSN")))
	cfg.Databases.Search.Backend = strings.TrimSpace(os.Getenv("SEARCH_BACKEND"))
	cfg.Databases.Search.DSN = strings.TrimSpace(os.Getenv("SEARCH_DSN"))
	cfg.Databases.Search.Index = strings.TrimSpace(os.Getenv("SEARCH_INDEX"))
	cfg.Databases.Vector.Backend = strings.TrimSpace(os.Getenv("VECTOR_BACKEND"))
	cfg.Databases.Vector.DSN = strings.TrimSpace(os.Getenv("VECTOR_DSN"))
	cfg.Databases.Vector.Index = strings.TrimSpace(os.Getenv("VECTOR_INDEX"))
	if v := strings.TrimSpace(os.Getenv("VECTOR_DIMENSIONS")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Databases.Vector.Dimensions = n
		}
	}
	cfg.Databases.Vector.Metric = strings.TrimSpace(os.Getenv("VECTOR_METRIC"))
	cfg.Databases.Graph.Backend = strings.TrimSpace(os.Getenv("GRAPH_BACKEND"))
	cfg.Databases.Graph.DSN = strings.TrimSpace(os.Getenv("GRAPH_DSN"))

	// Embedding service configuration via environment variables
	cfg.Embedding.BaseURL = strings.TrimSpace(os.Getenv("EMBED_BASE_URL"))
	cfg.Embedding.Model = strings.TrimSpace(os.Getenv("EMBED_MODEL"))
	cfg.Embedding.APIKey = strings.TrimSpace(os.Getenv("EMBED_API_KEY"))
	cfg.Embedding.APIHeader = strings.TrimSpace(os.Getenv("EMBED_API_HEADER"))
	cfg.Embedding.Path = strings.TrimSpace(os.Getenv("EMBED_PATH"))
	if v := strings.TrimSpace(os.Getenv("EMBED_TIMEOUT")); v != "" {
		if n, err := parseInt(v); err == nil {
			cfg.Embedding.Timeout = n
		}
	}

	// Optionally load specialist agents from YAML.
	if err := loadSpecialists(&cfg); err != nil {
		return Config{}, err
	}

	// Apply defaults after merging YAML
	if cfg.OpenAI.Model == "" {
		cfg.OpenAI.Model = "gpt-4o-mini"
	}
	if cfg.Obs.ServiceName == "" {
		cfg.Obs.ServiceName = "intelligence.dev"
	}
	if cfg.Obs.Environment == "" {
		cfg.Obs.Environment = "dev"
	}
	if cfg.Web.SearXNGURL == "" {
		cfg.Web.SearXNGURL = "http://localhost:8080"
	}
	if cfg.Exec.MaxCommandSeconds == 0 {
		cfg.Exec.MaxCommandSeconds = 30
	}
	if cfg.OutputTruncateByte == 0 {
		cfg.OutputTruncateByte = 64 * 1024
	}
	if cfg.MaxSteps == 0 {
		cfg.MaxSteps = 8
	}

	// Apply embedding defaults
	if cfg.Embedding.BaseURL == "" {
		cfg.Embedding.BaseURL = "https://api.openai.com"
	}
	if cfg.Embedding.Model == "" {
		cfg.Embedding.Model = "text-embedding-3-small"
	}
	if cfg.Embedding.APIHeader == "" {
		cfg.Embedding.APIHeader = "Authorization"
	}
	if cfg.Embedding.Path == "" {
		cfg.Embedding.Path = "/v1/embeddings"
	}
	if cfg.Embedding.Timeout == 0 {
		cfg.Embedding.Timeout = 30
	}

	// Apply database defaults. If a DefaultDSN is provided and the backend is
	// unspecified, prefer "auto" so the factory can attempt Postgres.
	if cfg.Databases.Search.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Search.Backend = "auto"
		} else {
			cfg.Databases.Search.Backend = "memory"
		}
	}
	if cfg.Databases.Vector.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Vector.Backend = "auto"
		} else {
			cfg.Databases.Vector.Backend = "memory"
		}
	}
	if cfg.Databases.Graph.Backend == "" {
		if cfg.Databases.DefaultDSN != "" {
			cfg.Databases.Graph.Backend = "auto"
		} else {
			cfg.Databases.Graph.Backend = "memory"
		}
	}

	if cfg.OpenAI.APIKey == "" {
		return Config{}, errors.New("OPENAI_API_KEY is required (set in .env or environment)")
	}
	if cfg.Workdir == "" {
		return Config{}, errors.New("WORKDIR is required (set in .env or environment)")
	}

	absWD, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve WORKDIR: %w", err)
	}
	info, err := os.Stat(absWD)
	if err != nil {
		return Config{}, fmt.Errorf("stat WORKDIR: %w", err)
	}
	if !info.IsDir() {
		return Config{}, fmt.Errorf("WORKDIR must be a directory: %s", absWD)
	}
	cfg.Workdir = absWD

	// Parse blocklist
	blockStr := strings.TrimSpace(os.Getenv("BLOCK_BINARIES"))
	if blockStr != "" {
		// env overrides any YAML-defined list
		cfg.Exec.BlockBinaries = nil
		parts := strings.Split(blockStr, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if strings.Contains(p, "/") || strings.Contains(p, "\\") {
				return Config{}, fmt.Errorf("BLOCK_BINARIES must contain bare binary names only (no paths): %q", p)
			}
			cfg.Exec.BlockBinaries = append(cfg.Exec.BlockBinaries, p)
		}
	}
	return cfg, nil
}

// loadSpecialists populates cfg.Specialists by reading a YAML file if present.
// The file path can be specified with SPECIALISTS_CONFIG. If not set, the
// loader will look for config.yaml or config.yml in the current working directory.
func loadSpecialists(cfg *Config) error {
	// Allow disabling via env
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SPECIALISTS_DISABLED")), "true") {
		return nil
	}
	var paths []string
	if p := strings.TrimSpace(os.Getenv("SPECIALISTS_CONFIG")); p != "" {
		paths = append(paths, p)
	}
	// Default to local config file next to the running binary / current dir
	paths = append(paths, "config.yaml", "config.yml")
	var data []byte
	var chosen string
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			data = b
			chosen = p
			break
		}
		if os.IsNotExist(err) {
			continue
		}
		// Unexpected read error
		return fmt.Errorf("read %s: %w", p, err)
	}
	if len(data) == 0 {
		return nil // optional
	}
	// Two accepted shapes:
	//   specialists: [ {name: ..., ...}, ... ]
	// or directly a list: [ {name: ..., ...} ]
	type openAIYAML struct {
		APIKey       string            `yaml:"apiKey"`
		Model        string            `yaml:"model"`
		BaseURL      string            `yaml:"baseURL"`
		ExtraHeaders map[string]string `yaml:"extraHeaders"`
		ExtraParams  map[string]any    `yaml:"extraParams"`
		LogPayloads  bool              `yaml:"logPayloads"`
	}
	type execYAML struct {
		BlockBinaries     []string `yaml:"blockBinaries"`
		MaxCommandSeconds int      `yaml:"maxCommandSeconds"`
	}
	type obsYAML struct {
		ServiceName    string `yaml:"serviceName"`
		ServiceVersion string `yaml:"serviceVersion"`
		Environment    string `yaml:"environment"`
		OTLP           string `yaml:"otlp"`
	}
	type webYAML struct {
		SearXNGURL string `yaml:"searXNGURL"`
	}
	type dbSearchYAML struct {
		Backend string `yaml:"backend"`
		DSN     string `yaml:"dsn"`
		Index   string `yaml:"index"`
	}
	type dbVectorYAML struct {
		Backend    string `yaml:"backend"`
		DSN        string `yaml:"dsn"`
		Index      string `yaml:"index"`
		Dimensions int    `yaml:"dimensions"`
		Metric     string `yaml:"metric"`
	}
	type dbGraphYAML struct {
		Backend string `yaml:"backend"`
		DSN     string `yaml:"dsn"`
	}
	type databasesYAML struct {
		DefaultDSN string       `yaml:"defaultDSN"`
		Search     dbSearchYAML `yaml:"search"`
		Vector     dbVectorYAML `yaml:"vector"`
		Graph      dbGraphYAML  `yaml:"graph"`
	}
	type mcpServerYAML struct {
		Name             string            `yaml:"name"`
		Command          string            `yaml:"command"`
		Args             []string          `yaml:"args"`
		Env              map[string]string `yaml:"env"`
		KeepAliveSeconds int               `yaml:"keepAliveSeconds"`
	}
	type mcpYAML struct {
		Servers []mcpServerYAML `yaml:"servers"`
	}
	type embeddingYAML struct {
		BaseURL        string `yaml:"baseURL"`
		Model          string `yaml:"model"`
		APIKey         string `yaml:"apiKey"`
		APIHeader      string `yaml:"apiHeader"`
		Path           string `yaml:"path"`
		TimeoutSeconds int    `yaml:"timeoutSeconds"`
	}
	type ttsYAML struct {
		BaseURL string `yaml:"baseURL"`
		Model   string `yaml:"model"`
		Voice   string `yaml:"voice"`
		Format  string `yaml:"format"`
	}
	type wrap struct {
		// SystemPrompt is an optional top-level YAML field to override the
		// default system prompt used by the agent.
		SystemPrompt     string             `yaml:"systemPrompt"`
		Specialists      []SpecialistConfig `yaml:"specialists"`
		Routes           []SpecialistRoute  `yaml:"routes"`
		OpenAI           openAIYAML         `yaml:"openai"`
		Workdir          string             `yaml:"workdir"`
		OutputTrunc      int                `yaml:"outputTruncateBytes"`
		SummaryEnabled   bool               `yaml:"summaryEnabled"`
		SummaryThreshold int                `yaml:"summaryThreshold"`
		SummaryKeepLast  int                `yaml:"summaryKeepLast"`
		Exec             execYAML           `yaml:"exec"`
		Obs              obsYAML            `yaml:"obs"`
		Web              webYAML            `yaml:"web"`
		Databases        databasesYAML      `yaml:"databases"`
		MCP              mcpYAML            `yaml:"mcp"`
		Embedding        embeddingYAML      `yaml:"embedding"`
		TTS              ttsYAML            `yaml:"tts"`
		EnableTools      *bool              `yaml:"enableTools"`
	}
	var w wrap
	// Expand ${VAR} with environment variables before parsing.
	data = []byte(os.ExpandEnv(string(data)))
	if err := yaml.Unmarshal(data, &w); err == nil {
		// Specialists and routes
		if len(w.Specialists) > 0 {
			cfg.Specialists = w.Specialists
		}
		if len(w.Routes) > 0 {
			cfg.SpecialistRoutes = w.Routes
		}
		// OpenAI extras always merged
		if len(w.OpenAI.ExtraHeaders) > 0 {
			cfg.OpenAI.ExtraHeaders = w.OpenAI.ExtraHeaders
		}
		if len(w.OpenAI.ExtraParams) > 0 {
			cfg.OpenAI.ExtraParams = w.OpenAI.ExtraParams
		}
		if !cfg.LogPayloads && w.OpenAI.LogPayloads {
			cfg.LogPayloads = true
			cfg.OpenAI.LogPayloads = true
		}
		// OpenAI core: only if empty (env overrides YAML)
		if cfg.OpenAI.APIKey == "" && strings.TrimSpace(w.OpenAI.APIKey) != "" {
			cfg.OpenAI.APIKey = strings.TrimSpace(w.OpenAI.APIKey)
		}
		if cfg.OpenAI.Model == "" && strings.TrimSpace(w.OpenAI.Model) != "" {
			cfg.OpenAI.Model = strings.TrimSpace(w.OpenAI.Model)
		}
		if cfg.OpenAI.BaseURL == "" && strings.TrimSpace(w.OpenAI.BaseURL) != "" {
			cfg.OpenAI.BaseURL = strings.TrimSpace(w.OpenAI.BaseURL)
		}
		// Workdir and others only if empty
		if cfg.Workdir == "" && strings.TrimSpace(w.Workdir) != "" {
			cfg.Workdir = strings.TrimSpace(w.Workdir)
		}
		// YAML-provided system prompt is applied only if env did not already
		// provide an override.
		if cfg.SystemPrompt == "" && strings.TrimSpace(w.SystemPrompt) != "" {
			cfg.SystemPrompt = w.SystemPrompt
		}

		if cfg.OutputTruncateByte == 0 && w.OutputTrunc > 0 {
			cfg.OutputTruncateByte = w.OutputTrunc
		}
		// Summary config from YAML if not provided via env
		if !cfg.SummaryEnabled && w.SummaryEnabled {
			cfg.SummaryEnabled = true
		}
		if cfg.SummaryThreshold == 0 && w.SummaryThreshold > 0 {
			cfg.SummaryThreshold = w.SummaryThreshold
		}
		if cfg.SummaryKeepLast == 0 && w.SummaryKeepLast > 0 {
			cfg.SummaryKeepLast = w.SummaryKeepLast
		}
		if cfg.Exec.MaxCommandSeconds == 0 && w.Exec.MaxCommandSeconds > 0 {
			cfg.Exec.MaxCommandSeconds = w.Exec.MaxCommandSeconds
		}
		if len(cfg.Exec.BlockBinaries) == 0 && len(w.Exec.BlockBinaries) > 0 {
			cfg.Exec.BlockBinaries = append([]string{}, w.Exec.BlockBinaries...)
		}
		if cfg.Obs.ServiceName == "" && w.Obs.ServiceName != "" {
			cfg.Obs.ServiceName = w.Obs.ServiceName
		}
		if cfg.Obs.ServiceVersion == "" && w.Obs.ServiceVersion != "" {
			cfg.Obs.ServiceVersion = w.Obs.ServiceVersion
		}
		if cfg.Obs.Environment == "" && w.Obs.Environment != "" {
			cfg.Obs.Environment = w.Obs.Environment
		}
		if cfg.Obs.OTLP == "" && w.Obs.OTLP != "" {
			cfg.Obs.OTLP = w.Obs.OTLP
		}
		if cfg.Web.SearXNGURL == "" && strings.TrimSpace(w.Web.SearXNGURL) != "" {
			cfg.Web.SearXNGURL = strings.TrimSpace(w.Web.SearXNGURL)
		}
		// Databases: only assign fields which are empty to allow env to override
		if cfg.Databases.DefaultDSN == "" && strings.TrimSpace(w.Databases.DefaultDSN) != "" {
			cfg.Databases.DefaultDSN = strings.TrimSpace(w.Databases.DefaultDSN)
		}
		if cfg.Databases.Search.Backend == "" && strings.TrimSpace(w.Databases.Search.Backend) != "" {
			cfg.Databases.Search.Backend = strings.TrimSpace(w.Databases.Search.Backend)
		}
		if cfg.Databases.Search.DSN == "" && strings.TrimSpace(w.Databases.Search.DSN) != "" {
			cfg.Databases.Search.DSN = strings.TrimSpace(w.Databases.Search.DSN)
		}
		if cfg.Databases.Search.Index == "" && strings.TrimSpace(w.Databases.Search.Index) != "" {
			cfg.Databases.Search.Index = strings.TrimSpace(w.Databases.Search.Index)
		}
		if cfg.Databases.Vector.Backend == "" && strings.TrimSpace(w.Databases.Vector.Backend) != "" {
			cfg.Databases.Vector.Backend = strings.TrimSpace(w.Databases.Vector.Backend)
		}
		if cfg.Databases.Vector.DSN == "" && strings.TrimSpace(w.Databases.Vector.DSN) != "" {
			cfg.Databases.Vector.DSN = strings.TrimSpace(w.Databases.Vector.DSN)
		}
		if cfg.Databases.Vector.Index == "" && strings.TrimSpace(w.Databases.Vector.Index) != "" {
			cfg.Databases.Vector.Index = strings.TrimSpace(w.Databases.Vector.Index)
		}
		if cfg.Databases.Vector.Dimensions == 0 && w.Databases.Vector.Dimensions > 0 {
			cfg.Databases.Vector.Dimensions = w.Databases.Vector.Dimensions
		}
		if cfg.Databases.Vector.Metric == "" && strings.TrimSpace(w.Databases.Vector.Metric) != "" {
			cfg.Databases.Vector.Metric = strings.TrimSpace(w.Databases.Vector.Metric)
		}
		if cfg.Databases.Graph.Backend == "" && strings.TrimSpace(w.Databases.Graph.Backend) != "" {
			cfg.Databases.Graph.Backend = strings.TrimSpace(w.Databases.Graph.Backend)
		}
		if cfg.Databases.Graph.DSN == "" && strings.TrimSpace(w.Databases.Graph.DSN) != "" {
			cfg.Databases.Graph.DSN = strings.TrimSpace(w.Databases.Graph.DSN)
		}
		// Embedding: only assign fields which are empty to allow env to override
		if cfg.Embedding.BaseURL == "" && strings.TrimSpace(w.Embedding.BaseURL) != "" {
			cfg.Embedding.BaseURL = strings.TrimSpace(w.Embedding.BaseURL)
		}
		if cfg.Embedding.Model == "" && strings.TrimSpace(w.Embedding.Model) != "" {
			cfg.Embedding.Model = strings.TrimSpace(w.Embedding.Model)
		}
		if cfg.Embedding.APIKey == "" && strings.TrimSpace(w.Embedding.APIKey) != "" {
			cfg.Embedding.APIKey = strings.TrimSpace(w.Embedding.APIKey)
		}
		if cfg.Embedding.APIHeader == "" && strings.TrimSpace(w.Embedding.APIHeader) != "" {
			cfg.Embedding.APIHeader = strings.TrimSpace(w.Embedding.APIHeader)
		}
		if cfg.Embedding.Path == "" && strings.TrimSpace(w.Embedding.Path) != "" {
			cfg.Embedding.Path = strings.TrimSpace(w.Embedding.Path)
		}
		if cfg.Embedding.Timeout == 0 && w.Embedding.TimeoutSeconds > 0 {
			cfg.Embedding.Timeout = w.Embedding.TimeoutSeconds
		}
		// MCP servers
		if len(w.MCP.Servers) > 0 {
			cfg.MCP.Servers = make([]MCPServerConfig, 0, len(w.MCP.Servers))
			for _, s := range w.MCP.Servers {
				cfg.MCP.Servers = append(cfg.MCP.Servers, MCPServerConfig{
					Name:             strings.TrimSpace(s.Name),
					Command:          strings.TrimSpace(s.Command),
					Args:             append([]string{}, s.Args...),
					Env:              s.Env,
					KeepAliveSeconds: s.KeepAliveSeconds,
				})
			}
		}
		// Global enableTools only if not set via env (env takes precedence)
		if !cfg.EnableTools && w.EnableTools != nil {
			cfg.EnableTools = *w.EnableTools
		}
		// TTS defaults from YAML if not set by env
		if cfg.TTS.BaseURL == "" && strings.TrimSpace(w.TTS.BaseURL) != "" {
			cfg.TTS.BaseURL = strings.TrimSpace(w.TTS.BaseURL)
		}
		if cfg.TTS.Model == "" && strings.TrimSpace(w.TTS.Model) != "" {
			cfg.TTS.Model = strings.TrimSpace(w.TTS.Model)
		}
		if cfg.TTS.Voice == "" && strings.TrimSpace(w.TTS.Voice) != "" {
			cfg.TTS.Voice = strings.TrimSpace(w.TTS.Voice)
		}
		return nil
	}
	// Fallback: try list at root
	var list []SpecialistConfig
	if err := yaml.Unmarshal(data, &list); err == nil && len(list) > 0 {
		cfg.Specialists = list
		return nil
	}
	return fmt.Errorf("%s: could not parse specialists configuration", chosen)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func intFromEnv(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := parseInt(v); err == nil {
			return n
		}
	}
	return def
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
