package config

// Config is the top-level runtime configuration for the agent.
type Config struct {
    Workdir            string
    OutputTruncateByte int
    LogPath            string
    LogLevel           string
    LogPayloads        bool
    Exec               ExecConfig
    OpenAI             OpenAIConfig
    Obs                ObsConfig
    Web                WebConfig
    // MCP defines Model Context Protocol client configuration. If configured,
    // the application will connect to the listed servers and expose their tools
    // in the agent tool registry.
    MCP                MCPConfig
	// Specialists defines additional OpenAI-compatible endpoints/models
	// that can be targeted directly for inference-only requests.
	// Each specialist may have its own base URL, API key, model, optional
	// reasoning effort, and dedicated system instructions. Tools can be
	// disabled per specialist so the request contains no tool schema at all.
	Specialists      []SpecialistConfig
	SpecialistRoutes []SpecialistRoute
}

type ExecConfig struct {
	BlockBinaries     []string
	MaxCommandSeconds int
}

type OpenAIConfig struct {
    APIKey  string
    Model   string
    BaseURL string
    // ExtraHeaders are added to every main agent HTTP request.
    ExtraHeaders map[string]string
    // ExtraParams are merged into the chat completions request for the main agent.
    ExtraParams map[string]any
    // LogPayloads enables verbose logging of request/response bodies with redaction.
    LogPayloads bool `yaml:"logPayloads" json:"logPayloads"`
}

// SpecialistConfig describes a single specialist agent bound to a specific
// OpenAI-compatible endpoint and model. It can optionally specify a different
// API key and base URL than the default OpenAI config.
type SpecialistConfig struct {
	Name            string            `yaml:"name" json:"name"`
	BaseURL         string            `yaml:"baseURL" json:"baseURL"`
	APIKey          string            `yaml:"apiKey" json:"apiKey"`
	Model           string            `yaml:"model" json:"model"`
	EnableTools     bool              `yaml:"enableTools" json:"enableTools"`
	ReasoningEffort string            `yaml:"reasoningEffort" json:"reasoningEffort"`
	System          string            `yaml:"system" json:"system"`
	ExtraHeaders    map[string]string `yaml:"extraHeaders" json:"extraHeaders"`
	ExtraParams     map[string]any    `yaml:"extraParams" json:"extraParams"`
}

// SpecialistRoute defines simple pre-dispatch rules. If the user's prompt
// matches any of the contains substrings or regex patterns, the router will
// dispatch directly to the given specialist name.
type SpecialistRoute struct {
	Name     string   `yaml:"name" json:"name"`
	Contains []string `yaml:"contains" json:"contains"`
	Regex    []string `yaml:"regex" json:"regex"`
}

type ObsConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLP           string
}

type WebConfig struct {
    SearXNGURL string
}

// MCPConfig is the root configuration for MCP clients.
type MCPConfig struct {
    Servers []MCPServerConfig `yaml:"servers" json:"servers"`
}

// MCPServerConfig describes a single MCP server to connect to via stdio/command.
// Only stdio via exec.Command is supported for now.
type MCPServerConfig struct {
    // Name is a unique identifier for this server, used to prefix tool names.
    Name string `yaml:"name" json:"name"`
    // Command is the executable to run for this server. Required.
    Command string `yaml:"command" json:"command"`
    // Args are passed to the command.
    Args []string `yaml:"args" json:"args"`
    // Env are additional environment variables to set for the command.
    Env map[string]string `yaml:"env" json:"env"`
    // KeepAliveSeconds configures client ping interval; 0 disables keepalive.
    KeepAliveSeconds int `yaml:"keepAliveSeconds" json:"keepAliveSeconds"`
}
