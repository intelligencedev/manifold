package config

// Config is the top-level runtime configuration for the agent.
type Config struct {
	Workdir string
	// If empty, the built-in hard-coded prompt is used.
	SystemPrompt string
	// Rolling summarization config: enable and tuning knobs
	SummaryEnabled     bool
	SummaryThreshold   int
	SummaryKeepLast    int
	OutputTruncateByte int
	// Maximum number of reasoning steps the agent can take
	MaxSteps    int
	LogPath     string
	LogLevel    string
	LogPayloads bool
	Exec        ExecConfig
	OpenAI      OpenAIConfig
	Obs         ObsConfig
	Web         WebConfig
	// MCP defines Model Context Protocol client configuration. If configured,
	// the application will connect to the listed servers and expose their tools
	// in the agent tool registry.
	MCP MCPConfig
	// Specialists defines additional OpenAI-compatible endpoints/models
	// that can be targeted directly for inference-only requests.
	// Each specialist may have its own base URL, API key, model, optional
	// reasoning effort, and dedicated system instructions. Tools can be
	// disabled per specialist so the request contains no tool schema at all.
	Specialists      []SpecialistConfig
	SpecialistRoutes []SpecialistRoute
	// Databases describes pluggable backends for search, vector embeddings,
	// and graph operations. Each backend can be configured independently via
	// YAML or environment variables.
	Databases DBConfig
	// EnableTools globally enables/disables tool exposure to the main agent.
	EnableTools bool `yaml:"enableTools" json:"enableTools"`
	// Top-level allow list of tool names to expose to the main orchestrator agent.
	// If empty or omitted, all registered tools are exposed.
	ToolAllowList []string `yaml:"allowTools" json:"allowTools"`
	// Embedding configures the embedding service endpoint for text embeddings.
	Embedding EmbeddingConfig
	// TTS configures text-to-speech defaults and endpoint.
	TTS TTSConfig `yaml:"tts" json:"tts"`
	// AgentRunTimeoutSeconds sets an upper wall-clock bound for a single agent
	// Run() invocation. 0 or negative disables the global timeout (recommended
	// for long-running, tool-bounded workflows where per-tool timeouts and
	// MaxSteps already provide safety).
	AgentRunTimeoutSeconds int `yaml:"agentRunTimeoutSeconds" json:"agentRunTimeoutSeconds"`
	// StreamRunTimeoutSeconds optionally bounds streaming /agent/run style
	// operations. 0 disables.
	StreamRunTimeoutSeconds int `yaml:"streamRunTimeoutSeconds" json:"streamRunTimeoutSeconds"`
	// WorkflowTimeoutSeconds bounds orchestrator workflow execution; 0 disables.
	WorkflowTimeoutSeconds int `yaml:"workflowTimeoutSeconds" json:"workflowTimeoutSeconds"`
}

// TTSConfig holds text-to-speech specific configuration.
type TTSConfig struct {
	// BaseURL is the HTTP base for TTS requests. Requests will be POSTed to
	// ${BaseURL}/v1/audio/speech if set.
	BaseURL string `yaml:"baseURL" json:"baseURL"`
	// Model is the default TTS model to use when creating speech.
	Model string `yaml:"model" json:"model"`
	// Voice is the default voice name to request from the TTS endpoint.
	Voice string `yaml:"voice" json:"voice"`
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
	Name        string `yaml:"name" json:"name"`
	BaseURL     string `yaml:"baseURL" json:"baseURL"`
	APIKey      string `yaml:"apiKey" json:"apiKey"`
	Model       string `yaml:"model" json:"model"`
	EnableTools bool   `yaml:"enableTools" json:"enableTools"`
	// AllowTools is an optional allow-list of tool names exposed to this specialist.
	// If empty, all tools are exposed (subject to EnableTools). If non-empty, only
	// listed tools will be included in the tool schema and available for dispatch.
	AllowTools      []string          `yaml:"allowTools" json:"allowTools"`
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

// DBConfig contains sub-config for each pluggable database backend.
type DBConfig struct {
	// DefaultDSN is an optional shared connection string. If a per-subsystem
	// DSN is not provided, this value will be used. When set, the factory can
	// automatically select a Postgres backend if reachable.
	DefaultDSN string       `yaml:"defaultDSN" json:"defaultDSN"`
	Search     SearchConfig `yaml:"search" json:"search"`
	Vector     VectorConfig `yaml:"vector" json:"vector"`
	Graph      GraphConfig  `yaml:"graph" json:"graph"`
}

// SearchConfig configures the full-text search backend.
type SearchConfig struct {
	// Backend selects the implementation, e.g. "auto", "memory", "none", "postgres".
	Backend string `yaml:"backend" json:"backend"`
	// DSN is a connection string or URL for the backend (if applicable).
	DSN string `yaml:"dsn" json:"dsn"`
	// Index is an optional index/collection name.
	Index string `yaml:"index" json:"index"`
}

// VectorConfig configures the vector store backend.
type VectorConfig struct {
	Backend    string `yaml:"backend" json:"backend"`
	DSN        string `yaml:"dsn" json:"dsn"`
	Index      string `yaml:"index" json:"index"`
	Dimensions int    `yaml:"dimensions" json:"dimensions"`
	Metric     string `yaml:"metric" json:"metric"`
}

// GraphConfig configures the graph database backend.
type GraphConfig struct {
	Backend string `yaml:"backend" json:"backend"`
	DSN     string `yaml:"dsn" json:"dsn"`
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

// EmbeddingConfig configures the embedding service endpoint.
type EmbeddingConfig struct {
	BaseURL   string `yaml:"baseURL" json:"baseURL"`
	Model     string `yaml:"model" json:"model"`
	APIKey    string `yaml:"apiKey" json:"apiKey"`
	APIHeader string `yaml:"apiHeader" json:"apiHeader"` // e.g., "Authorization"
	Path      string `yaml:"path" json:"path"`           // default: /v1/embeddings
	Timeout   int    `yaml:"timeoutSeconds" json:"timeoutSeconds"`
}
