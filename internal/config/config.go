package config

// Config is the top-level runtime configuration for the agent.
type Config struct {
	Workdir string
	// Kafka configuration for orchestrator integration
	Kafka KafkaConfig
	// If empty, the built-in hard-coded prompt is used.
	SystemPrompt string
	// Rolling summarization config: enable and tuning knobs
	SummaryEnabled               bool
	SummaryThreshold             int
	SummaryKeepLast              int
	SummaryMode                  string  `yaml:"summaryMode" json:"summaryMode"`
	SummaryTargetUtilizationPct  float64 `yaml:"summaryTargetUtilizationPct" json:"summaryTargetUtilizationPct"`
	SummaryMinKeepLastMessages   int     `yaml:"summaryMinKeepLastMessages" json:"summaryMinKeepLastMessages"`
	SummaryMaxSummaryChunkTokens int     `yaml:"summaryMaxSummaryChunkTokens" json:"summaryMaxSummaryChunkTokens"`
	OutputTruncateByte           int
	// Maximum number of reasoning steps the agent can take
	MaxSteps int
	// MaxToolParallelism controls how many tool calls may run concurrently within a single step.
	// <= 0 means unbounded (run all tools in parallel); 1 forces sequential execution.
	MaxToolParallelism int `yaml:"maxToolParallelism" json:"maxToolParallelism"`
	LogPath            string
	LogLevel           string
	LogPayloads        bool
	Exec               ExecConfig
	// LLMClient controls which LLM provider to use and holds provider-specific settings.
	LLMClient LLMClientConfig `yaml:"llm_client" json:"llmClient"`
	// OpenAI retains the active OpenAI-compatible configuration for backward compatibility.
	OpenAI OpenAIConfig
	Obs    ObsConfig
	Web    WebConfig
	// Auth configures optional user authentication (OIDC/OAuth2) and RBAC.
	Auth AuthConfig
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
	// EvolvingMemory configures the Search-Synthesis-Evolve memory system.
	EvolvingMemory EvolvingMemoryConfig
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
	// Projects controls per-user projects service behavior.
	Projects ProjectsConfig `yaml:"projects" json:"projects"`
}

// KafkaConfig holds Kafka connectivity and topic defaults for orchestrator
type KafkaConfig struct {
	Brokers        string `yaml:"brokers" json:"brokers"`
	CommandsTopic  string `yaml:"commandsTopic" json:"commandsTopic"`
	ResponsesTopic string `yaml:"responsesTopic" json:"responsesTopic"`
}

// ProjectsConfig controls filesystem-backed projects behavior.
type ProjectsConfig struct {
	// Encrypt enables at-rest encryption for project files using an envelope
	// scheme with a locally stored master key (under ${WORKDIR}/.keystore).
	Encrypt bool `yaml:"encrypt" json:"encrypt"`
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

// LLMClientConfig selects the LLM provider and holds provider-specific configs.
type LLMClientConfig struct {
	Provider  string          `yaml:"provider" json:"provider"`
	OpenAI    OpenAIConfig    `yaml:"openai" json:"openai"`
	Anthropic AnthropicConfig `yaml:"anthropic" json:"anthropic"`
	Google    GoogleConfig    `yaml:"google" json:"google"`
}

type OpenAIConfig struct {
	APIKey         string
	Model          string
	BaseURL        string
	SummaryModel   string `yaml:"summaryModel" json:"summaryModel"`
	SummaryBaseURL string `yaml:"summaryBaseURL" json:"summaryBaseURL"`
	// API selects which OpenAI-compatible API surface to use for chat: "completions" or "responses".
	// Defaults to "completions" if empty or unrecognized.
	API string `yaml:"api" json:"api"`
	// ExtraHeaders are added to every main agent HTTP request.
	ExtraHeaders map[string]string
	// ExtraParams are merged into the chat completions request for the main agent.
	ExtraParams map[string]any
	// LogPayloads enables verbose logging of request/response bodies with redaction.
	LogPayloads bool `yaml:"logPayloads" json:"logPayloads"`
}

// AnthropicConfig holds Anthropic provider settings.
type AnthropicConfig struct {
	APIKey  string `yaml:"apiKey" json:"apiKey"`
	Model   string `yaml:"model" json:"model"`
	BaseURL string `yaml:"baseURL" json:"baseURL"`
}

// GoogleConfig holds Google Gemini provider settings.
type GoogleConfig struct {
	APIKey  string `yaml:"apiKey" json:"apiKey"`
	Model   string `yaml:"model" json:"model"`
	BaseURL string `yaml:"baseURL" json:"baseURL"`
}

// SpecialistConfig describes a single specialist agent bound to a specific
// OpenAI-compatible endpoint and model. It can optionally specify a different
// API key and base URL than the default OpenAI config.
type SpecialistConfig struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Provider    string `yaml:"provider" json:"provider"`
	BaseURL     string `yaml:"baseURL" json:"baseURL"`
	APIKey      string `yaml:"apiKey" json:"apiKey"`
	Model       string `yaml:"model" json:"model"`
	// API, when set, overrides which API surface to use for this specialist: "completions" or "responses".
	API         string `yaml:"api" json:"api"`
	EnableTools bool   `yaml:"enableTools" json:"enableTools"`
	// Paused specialists are ignored by the orchestrator and not exposed to tools.
	Paused bool `yaml:"paused" json:"paused"`
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

type ClickHouseConfig struct {
	DSN                  string `yaml:"dsn" json:"dsn"`
	Database             string `yaml:"database" json:"database"`
	MetricsTable         string `yaml:"metricsTable" json:"metricsTable"`
	TracesTable          string `yaml:"tracesTable" json:"tracesTable"`
	LogsTable            string `yaml:"logsTable" json:"logsTable"`
	TimestampColumn      string `yaml:"timestampColumn" json:"timestampColumn"`
	ValueColumn          string `yaml:"valueColumn" json:"valueColumn"`
	ModelAttributeKey    string `yaml:"modelAttributeKey" json:"modelAttributeKey"`
	PromptMetricName     string `yaml:"promptMetricName" json:"promptMetricName"`
	CompletionMetricName string `yaml:"completionMetricName" json:"completionMetricName"`
	LookbackHours        int    `yaml:"lookbackHours" json:"lookbackHours"`
	TimeoutSeconds       int    `yaml:"timeoutSeconds" json:"timeoutSeconds"`
}

type ObsConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLP           string
	ClickHouse     ClickHouseConfig
}

type WebConfig struct {
	SearXNGURL string
}

// AuthConfig holds OAuth2/OIDC and session cookie settings. If Enabled is true,
// the HTTP server will require authentication for protected endpoints.
type AuthConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Provider supports "oidc" (default) and "oauth2".
	Provider string `yaml:"provider" json:"provider"`
	// IssuerURL is the OIDC issuer discovery URL, e.g. https://accounts.google.com
	IssuerURL    string `yaml:"issuerURL" json:"issuerURL"`
	ClientID     string `yaml:"clientID" json:"clientID"`
	ClientSecret string `yaml:"clientSecret" json:"clientSecret"`
	RedirectURL  string `yaml:"redirectURL" json:"redirectURL"`
	// AllowedDomains restricts sign-in to specific email domains; empty allows any.
	AllowedDomains []string `yaml:"allowedDomains" json:"allowedDomains"`
	// CookieName is the name of the auth session cookie; default "sio_session".
	CookieName string `yaml:"cookieName" json:"cookieName"`
	// CookieSecure sets the Secure attribute on cookies. Set true in production (HTTPS).
	CookieSecure bool `yaml:"cookieSecure" json:"cookieSecure"`
	// CookieDomain optionally scopes the cookie domain; empty leaves default host-only.
	CookieDomain string `yaml:"cookieDomain" json:"cookieDomain"`
	// StateTTLSeconds bounds the lifetime of the OAuth state/PKCE cookies.
	StateTTLSeconds int `yaml:"stateTTLSeconds" json:"stateTTLSeconds"`
	// SessionTTLHours controls how long a server-side session remains valid.
	SessionTTLHours int `yaml:"sessionTTLHours" json:"sessionTTLHours"`
	// OAuth2 provides additional configuration when Provider=="oauth2".
	OAuth2 OAuth2Config `yaml:"oauth2" json:"oauth2"`
}

// OAuth2Config contains the endpoints and mapping hints required for plain OAuth2 providers.
type OAuth2Config struct {
	AuthURL             string   `yaml:"authURL" json:"authURL"`
	TokenURL            string   `yaml:"tokenURL" json:"tokenURL"`
	UserInfoURL         string   `yaml:"userInfoURL" json:"userInfoURL"`
	LogoutURL           string   `yaml:"logoutURL" json:"logoutURL"`
	LogoutRedirectParam string   `yaml:"logoutRedirectParam" json:"logoutRedirectParam"`
	Scopes              []string `yaml:"scopes" json:"scopes"`
	ProviderName        string   `yaml:"providerName" json:"providerName"`
	DefaultRoles        []string `yaml:"defaultRoles" json:"defaultRoles"`
	EmailField          string   `yaml:"emailField" json:"emailField"`
	NameField           string   `yaml:"nameField" json:"nameField"`
	PictureField        string   `yaml:"pictureField" json:"pictureField"`
	SubjectField        string   `yaml:"subjectField" json:"subjectField"`
	RolesField          string   `yaml:"rolesField" json:"rolesField"`
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
	Chat       ChatConfig   `yaml:"chat" json:"chat"`
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

// ChatConfig configures the chat history backend.
type ChatConfig struct {
	Backend string `yaml:"backend" json:"backend"`
	DSN     string `yaml:"dsn" json:"dsn"`
}

// MCPConfig is the root configuration for MCP clients.
type MCPConfig struct {
	Servers []MCPServerConfig `yaml:"servers" json:"servers"`
}

// MCPServerConfig describes a single MCP server to connect to.
// If Command is set, Manifold will launch the server via stdio.
// If URL is set, Manifold will connect over Streamable HTTP to the remote endpoint.
type MCPServerConfig struct {
	// Name is a unique identifier for this server, used to prefix tool names.
	Name string `yaml:"name" json:"name"`
	// Command is the executable to run for this server (stdio transport).
	Command string `yaml:"command" json:"command"`
	// Args are passed to the command.
	Args []string `yaml:"args" json:"args"`
	// Env are additional environment variables to set for the command.
	Env map[string]string `yaml:"env" json:"env"`
	// KeepAliveSeconds configures client ping interval; 0 disables keepalive.
	KeepAliveSeconds int `yaml:"keepAliveSeconds" json:"keepAliveSeconds"`

	// URL is the remote MCP endpoint (HTTP streamable transport), e.g., https://example.com/mcp
	URL string `yaml:"url" json:"url"`
	// Headers are additional HTTP headers to send on requests.
	Headers map[string]string `yaml:"headers" json:"headers"`
	// BearerToken, when set, is sent as Authorization: Bearer <token>.
	BearerToken string `yaml:"bearerToken" json:"bearerToken"`
	// Origin header to send when connecting to remote MCP servers. Some servers validate Origin.
	Origin string `yaml:"origin" json:"origin"`
	// ProtocolVersion optionally forces a specific MCP-Protocol-Version header.
	ProtocolVersion string `yaml:"protocolVersion" json:"protocolVersion"`
	// HTTP controls timeouts, TLS, and proxy settings for remote connections.
	HTTP MCPHTTPClientConfig `yaml:"http" json:"http"`
}

// MCPHTTPClientConfig configures the HTTP client used for remote MCP servers.
type MCPHTTPClientConfig struct {
	TimeoutSeconds int          `yaml:"timeoutSeconds" json:"timeoutSeconds"`
	ProxyURL       string       `yaml:"proxyURL" json:"proxyURL"`
	TLS            MCPTLSConfig `yaml:"tls" json:"tls"`
}

// MCPTLSConfig provides TLS settings for remote MCP connections.
type MCPTLSConfig struct {
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify" json:"insecureSkipVerify"`
	CAFile             string `yaml:"caFile" json:"caFile"`
	CertFile           string `yaml:"certFile" json:"certFile"`
	KeyFile            string `yaml:"keyFile" json:"keyFile"`
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

// EvolvingMemoryConfig configures the Search-Synthesis-Evolve memory system.
type EvolvingMemoryConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`             // enable evolving memory
	MaxSize       int    `yaml:"maxSize" json:"maxSize"`             // max entries (default 1000)
	TopK          int    `yaml:"topK" json:"topK"`                   // retrieval top-k (default 4)
	WindowSize    int    `yaml:"windowSize" json:"windowSize"`       // ExpRecent window (default 20)
	EnableRAG     bool   `yaml:"enableRAG" json:"enableRAG"`         // enable ExpRAG retrieval
	ReMemEnabled  bool   `yaml:"reMemEnabled" json:"reMemEnabled"`   // enable Think-Act-Refine mode
	MaxInnerSteps int    `yaml:"maxInnerSteps" json:"maxInnerSteps"` // ReMem max inner loops (default 5)
	Model         string `yaml:"model" json:"model"`                 // LLM model for summarization

	// Smart pruning options (advanced)
	EnableSmartPrune bool    `yaml:"enableSmartPrune" json:"enableSmartPrune"` // enable similarity-based dedup & relevance pruning
	PruneThreshold   float64 `yaml:"pruneThreshold" json:"pruneThreshold"`     // similarity threshold for duplicate detection (default 0.95)
	RelevanceDecay   float64 `yaml:"relevanceDecay" json:"relevanceDecay"`     // daily decay factor for relevance (default 0.99)
	MinRelevance     float64 `yaml:"minRelevance" json:"minRelevance"`         // minimum relevance to avoid pruning (default 0.1)
}
