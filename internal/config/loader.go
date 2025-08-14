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

	cfg.Obs.ServiceName = strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	cfg.Obs.ServiceVersion = strings.TrimSpace(os.Getenv("SERVICE_VERSION"))
	cfg.Obs.Environment = strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	cfg.Obs.OTLP = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))

	cfg.Web.SearXNGURL = strings.TrimSpace(os.Getenv("SEARXNG_URL"))

	// Optionally load specialist agents from YAML.
	if err := loadSpecialists(&cfg); err != nil {
		return Config{}, err
	}

	// Apply defaults after merging YAML
	if cfg.OpenAI.Model == "" {
		cfg.OpenAI.Model = "gpt-4o-mini"
	}
	if cfg.Obs.ServiceName == "" {
		cfg.Obs.ServiceName = "singularityio"
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
	type wrap struct {
		Specialists []SpecialistConfig `yaml:"specialists"`
		Routes      []SpecialistRoute  `yaml:"routes"`
		OpenAI      openAIYAML         `yaml:"openai"`
		Workdir     string             `yaml:"workdir"`
		OutputTrunc int                `yaml:"outputTruncateBytes"`
		Exec        execYAML           `yaml:"exec"`
		Obs         obsYAML            `yaml:"obs"`
		Web         webYAML            `yaml:"web"`
		MCP         mcpYAML            `yaml:"mcp"`
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
		if cfg.OutputTruncateByte == 0 && w.OutputTrunc > 0 {
			cfg.OutputTruncateByte = w.OutputTrunc
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
