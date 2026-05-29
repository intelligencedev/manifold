package config

type DBConfig struct {
	// DefaultDSN is an optional shared connection string. If a per-subsystem
	// DSN is not provided, this value will be used. When set, the factory can
	// automatically select a Postgres backend if reachable.
	DefaultDSN           string       `yaml:"defaultDSN" json:"defaultDSN"`
	Embedded             bool         `yaml:"embedded" json:"embedded"`
	EmbeddedPort         uint32       `yaml:"embeddedPort" json:"embeddedPort"`
	EmbeddedDataDir      string       `yaml:"embeddedDataDir" json:"embeddedDataDir"`
	EmbeddedVersion      string       `yaml:"embeddedVersion" json:"embeddedVersion"`
	EmbeddedExtensions   []string     `yaml:"embeddedExtensions" json:"embeddedExtensions"`
	EmbeddedExtensionURL string       `yaml:"embeddedExtensionURL" json:"embeddedExtensionURL"`
	Search               SearchConfig `yaml:"search" json:"search"`
	Vector               VectorConfig `yaml:"vector" json:"vector"`
	Graph                GraphConfig  `yaml:"graph" json:"graph"`
	Chat                 ChatConfig   `yaml:"chat" json:"chat"`
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
	// PathDependent marks this server as requiring per-user instances with project
	// path injection. Only applies when auth is enabled.
	// When true, placeholders like {{PROJECT_DIR}} in Args and Env will be expanded
	// to the user's active project workspace path. Note: We use {{...}} syntax instead
	// of ${...} because the config loader runs os.ExpandEnv which expands ${...}.
	PathDependent bool `yaml:"pathDependent" json:"pathDependent"`

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
	BaseURL      string                     `yaml:"baseURL" json:"baseURL"`
	Model        string                     `yaml:"model" json:"model"`
	APIKey       string                     `yaml:"apiKey" json:"apiKey"`
	APIHeader    string                     `yaml:"apiHeader" json:"apiHeader"` // e.g., "Authorization"
	Headers      map[string]string          `yaml:"headers" json:"headers"`     // optional additional headers
	Path         string                     `yaml:"path" json:"path"`           // default: /v1/embeddings
	Timeout      int                        `yaml:"timeoutSeconds" json:"timeoutSeconds"`
	Instructions EmbeddingInstructionConfig `yaml:"instructions" json:"instructions"`
}

// RerankingConfig configures the optional reranking service endpoint.
type RerankingConfig struct {
	Enabled     bool              `yaml:"enabled" json:"enabled"`
	BaseURL     string            `yaml:"baseURL" json:"baseURL"`
	Model       string            `yaml:"model" json:"model"`
	Instruction string            `yaml:"instruction" json:"instruction"` // optional query-side instruction
	APIKey      string            `yaml:"apiKey" json:"apiKey"`
	APIHeader   string            `yaml:"apiHeader" json:"apiHeader"` // e.g., "Authorization"
	Headers     map[string]string `yaml:"headers" json:"headers"`     // optional additional headers
	Path        string            `yaml:"path" json:"path"`           // default: /v1/rerank
	Timeout     int               `yaml:"timeoutSeconds" json:"timeoutSeconds"`
}

// MagmaConfig configures optional multi-graph agentic memory.
type MagmaConfig struct {
	Enabled       bool                     `yaml:"enabled" json:"enabled"`
	Consolidation MagmaConsolidationConfig `yaml:"consolidation" json:"consolidation"`
	Graphs        MagmaGraphsConfig        `yaml:"graphs" json:"graphs"`
	Retrieval     MagmaRetrievalConfig     `yaml:"retrieval" json:"retrieval"`
	Lifecycle     MagmaLifecycleConfig     `yaml:"lifecycle" json:"lifecycle"`
}

type MagmaConsolidationConfig struct {
	Model        string             `yaml:"model" json:"model"`
	BatchSize    int                `yaml:"batchSize" json:"batchSize"`
	MaxQueueSize int                `yaml:"maxQueueSize" json:"maxQueueSize"`
	WorkerCount  int                `yaml:"workerCount" json:"workerCount"`
	Prompts      MagmaPromptsConfig `yaml:"prompts" json:"prompts"`
}

type MagmaPromptsConfig struct {
	ConsolidationExtraction string `yaml:"consolidationExtraction" json:"consolidationExtraction"`
	IntentClassification    string `yaml:"intentClassification" json:"intentClassification"`
}

type MagmaGraphsConfig struct {
	Semantic MagmaSemanticGraphConfig `yaml:"semantic" json:"semantic"`
	Temporal MagmaTemporalGraphConfig `yaml:"temporal" json:"temporal"`
	Causal   MagmaCausalGraphConfig   `yaml:"causal" json:"causal"`
	Entity   MagmaEntityGraphConfig   `yaml:"entity" json:"entity"`
}

type MagmaSemanticGraphConfig struct {
	Enabled             bool    `yaml:"enabled" json:"enabled"`
	TopK                int     `yaml:"topK" json:"topK"`
	SimilarityThreshold float64 `yaml:"similarityThreshold" json:"similarityThreshold"`
}

type MagmaTemporalGraphConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	DateResolution string `yaml:"dateResolution" json:"dateResolution"`
}

type MagmaCausalGraphConfig struct {
	Enabled      bool    `yaml:"enabled" json:"enabled"`
	LLMThreshold float64 `yaml:"llmThreshold" json:"llmThreshold"`
}

type MagmaEntityGraphConfig struct {
	Enabled     bool `yaml:"enabled" json:"enabled"`
	CoReference bool `yaml:"coReference" json:"coReference"`
}

type MagmaRetrievalConfig struct {
	DefaultHops          int    `yaml:"defaultHops" json:"defaultHops"`
	DefaultMaxNodes      int    `yaml:"defaultMaxNodes" json:"defaultMaxNodes"`
	IntentClassification string `yaml:"intentClassification" json:"intentClassification"`
	ContextFormat        string `yaml:"contextFormat" json:"contextFormat"`
}

type MagmaLifecycleConfig struct {
	PruneIntervalMinutes   int     `yaml:"pruneIntervalMinutes" json:"pruneIntervalMinutes"`
	EventTTLHours          int     `yaml:"eventTTLHours" json:"eventTTLHours"`
	MaxEdgesPerSourceRel   int     `yaml:"maxEdgesPerSourceRel" json:"maxEdgesPerSourceRel"`
	MinSemanticWeight      float64 `yaml:"minSemanticWeight" json:"minSemanticWeight"`
	LowConfidenceThreshold float64 `yaml:"lowConfidenceThreshold" json:"lowConfidenceThreshold"`
	RequireReviewApproval  bool    `yaml:"requireReviewApproval" json:"requireReviewApproval"`
}

// EmbeddingInstructionConfig configures query-side embedding instructions.
type EmbeddingInstructionConfig struct {
	Mode                string `yaml:"mode" json:"mode"` // auto, enabled, disabled
	Format              string `yaml:"format" json:"format"`
	DefaultQuery        string `yaml:"defaultQuery" json:"defaultQuery"`
	RAGQuery            string `yaml:"ragQuery" json:"ragQuery"`
	EvolvingMemoryQuery string `yaml:"evolvingMemoryQuery" json:"evolvingMemoryQuery"`
	TransitQuery        string `yaml:"transitQuery" json:"transitQuery"`
}

// ImageToolConfig configures the describe_image tool defaults.
type ImageToolConfig struct {
	// BaseURL overrides the LLM endpoint used by describe_image. Empty means use the invoking provider endpoint.
	BaseURL string `yaml:"baseURL" json:"baseURL"`
	// Model overrides the LLM model used by describe_image. Empty means use the invoking provider model.
	Model string `yaml:"model" json:"model"`
}

// EvolvingMemoryConfig configures the Search-Synthesis-Evolve memory system.
type EvolvingMemoryConfig struct {
	Enabled                bool            `yaml:"enabled" json:"enabled"`                               // enable evolving memory
	Provider               string          `yaml:"provider" json:"provider"`                             // optional provider override (openai, anthropic, google, local)
	LLMClient              LLMClientConfig `yaml:"llmClient" json:"llmClient"`                           // optional dedicated provider configuration for evolving memory
	PersistDebounceMs      int             `yaml:"persistDebounceMs" json:"persistDebounceMs"`           // debounce async persistence writes (default 250ms)
	SessionTTLMinutes      int             `yaml:"sessionTTLMinutes" json:"sessionTTLMinutes"`           // evict idle per-session memory after this many minutes (default 60)
	JanitorIntervalMinutes int             `yaml:"janitorIntervalMinutes" json:"janitorIntervalMinutes"` // cleanup cadence for idle per-session memory (default 15)
	MaxSize                int             `yaml:"maxSize" json:"maxSize"`                               // max entries (default 1000)
	TopK                   int             `yaml:"topK" json:"topK"`                                     // retrieval top-k (default 4)
	WindowSize             int             `yaml:"windowSize" json:"windowSize"`                         // ExpRecent window (default 20)
	EnableRAG              bool            `yaml:"enableRAG" json:"enableRAG"`                           // enable ExpRAG retrieval
	ReMemEnabled           bool            `yaml:"reMemEnabled" json:"reMemEnabled"`                     // enable Think-Act-Refine mode
	MaxInnerSteps          int             `yaml:"maxInnerSteps" json:"maxInnerSteps"`                   // ReMem max inner loops (default 5)
	Model                  string          `yaml:"model" json:"model"`                                   // LLM model for summarization

	// Smart pruning options (advanced)
	EnableSmartPrune            bool    `yaml:"enableSmartPrune" json:"enableSmartPrune"`                       // enable similarity-based dedup & relevance pruning
	PruneThreshold              float64 `yaml:"pruneThreshold" json:"pruneThreshold"`                           // similarity threshold for duplicate detection (default 0.95)
	RelevanceDecay              float64 `yaml:"relevanceDecay" json:"relevanceDecay"`                           // daily decay factor for relevance (default 0.99)
	MinRelevance                float64 `yaml:"minRelevance" json:"minRelevance"`                               // minimum relevance to avoid pruning (default 0.1)
	PruneQualityFloor           int     `yaml:"pruneQualityFloor" json:"pruneQualityFloor"`                     // protect successful frequently reused memories (default 3)
	PromotionAccessThreshold    int     `yaml:"promotionAccessThreshold" json:"promotionAccessThreshold"`       // promote successful procedural memories after N accesses (default 5)
	StoreJanitorIntervalMinutes int     `yaml:"storeJanitorIntervalMinutes" json:"storeJanitorIntervalMinutes"` // durable expired-memory sweep cadence (default 60)
}

// TransitConfig configures the shared durable memory system.
type TransitConfig struct {
	Enabled            bool `yaml:"enabled" json:"enabled"`
	DefaultSearchLimit int  `yaml:"defaultSearchLimit" json:"defaultSearchLimit"`
	DefaultListLimit   int  `yaml:"defaultListLimit" json:"defaultListLimit"`
	MaxBatchSize       int  `yaml:"maxBatchSize" json:"maxBatchSize"`
	EnableVectorSearch bool `yaml:"enableVectorSearch" json:"enableVectorSearch"`
}
