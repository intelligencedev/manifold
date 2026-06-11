package config

// Config is the top-level runtime configuration for the agent.
type Config struct {
	Workdir string `yaml:"workdir" json:"workdir"`
	// Deprecated: systemPrompt is retained only to read older config files.
	// New orchestrator system instructions should be configured through the
	// persisted orchestrator specialist, and shared prompt blocks should be
	// customized through PromptOverrides.
	SystemPrompt string `yaml:"systemPrompt" json:"systemPrompt"`
	// PromptOverrides replaces selected built-in prompt blocks.
	PromptOverrides PromptOverridesConfig `yaml:"promptOverrides" json:"promptOverrides"`
	// Rolling summarization config: enable and tuning knobs (token-based only)
	SummaryEnabled bool `yaml:"summaryEnabled" json:"summaryEnabled"`
	// Summary configures rolling chat summaries independently from the primary LLM.
	Summary SummaryConfig `yaml:"summary" json:"summary"`
	// SummaryPlainTextContextWindowTokens optionally overrides the context window
	// budget used to trigger plain-text rolling summaries. When 0, runtime code
	// must derive an effective budget from the active target and summary model.
	SummaryPlainTextContextWindowTokens int `yaml:"summaryPlainTextContextWindowTokens" json:"summaryPlainTextContextWindowTokens"`
	// SummaryContextWindowTokens sets the context window size (in tokens) used for
	// chat-memory budgeting and summarization triggering.
	//
	// If 0, agentd will derive this from the configured summary model, but will
	// cap it to a moderate default to avoid sending very large raw chat histories
	// even when the underlying model supports huge context windows.
	SummaryContextWindowTokens int `yaml:"summaryContextWindowTokens" json:"summaryContextWindowTokens"`
	// SummaryReserveBufferTokens is the number of tokens to reserve for model output
	// (including reasoning tokens for reasoning models). OpenAI recommends ~25,000
	// when experimenting with reasoning models. Default: 25000.
	SummaryReserveBufferTokens int `yaml:"summaryReserveBufferTokens" json:"summaryReserveBufferTokens"`
	// SummaryMinKeepLastMessages is the minimum number of recent messages to preserve
	// in raw form, even if the token budget is small. Default: 4.
	SummaryMinKeepLastMessages int `yaml:"summaryMinKeepLastMessages" json:"summaryMinKeepLastMessages"`
	// SummaryMaxKeepLastMessages caps the number of recent messages preserved in raw
	// form. When the chat exceeds this many messages, earlier messages will be
	// summarized even if they still fit within the model context budget.
	//
	// This helps keep the orchestrator focused on the latest user turn, while still
	// retaining prior context via a rolling summary.
	SummaryMaxKeepLastMessages int `yaml:"summaryMaxKeepLastMessages" json:"summaryMaxKeepLastMessages"`
	// SummaryMaxSummaryChunkTokens caps the size of the summary prompt in tokens.
	SummaryMaxSummaryChunkTokens int `yaml:"summaryMaxSummaryChunkTokens" json:"summaryMaxSummaryChunkTokens"`
	OutputTruncateByte           int `yaml:"outputTruncateBytes" json:"outputTruncateBytes"`
	// Maximum number of reasoning steps the agent can take
	MaxSteps int `yaml:"maxSteps" json:"maxSteps"`
	// MaxToolParallelism controls how many tool calls may run concurrently within a single step.
	// <= 0 means unbounded (run all tools in parallel); 1 forces sequential execution.
	MaxToolParallelism int           `yaml:"maxToolParallelism" json:"maxToolParallelism"`
	Harness            HarnessConfig `yaml:"harness" json:"harness"`
	LogPath            string        `yaml:"logPath" json:"logPath"`
	LogLevel           string        `yaml:"logLevel" json:"logLevel"`
	LogPayloads        bool          `yaml:"logPayloads" json:"logPayloads"`
	LogRawPrompts      bool          `yaml:"logRawPrompts" json:"logRawPrompts"`
	Exec               ExecConfig    `yaml:"exec" json:"exec"`
	// LLMClient controls which LLM provider to use and holds provider-specific settings.
	LLMClient LLMClientConfig `yaml:"llm_client" json:"llmClient"`
	// OpenAI retains the active OpenAI-compatible configuration for backward compatibility.
	OpenAI OpenAIConfig `yaml:"openai" json:"openai"`
	Obs    ObsConfig    `yaml:"obs" json:"obs"`
	Web    WebConfig    `yaml:"web" json:"web"`
	// Matrix configures the built-in Matrix gateway.
	Matrix MatrixConfig `yaml:"matrix" json:"matrix"`
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
	Specialists      []SpecialistConfig `yaml:"specialists" json:"specialists"`
	SpecialistRoutes []SpecialistRoute  `yaml:"routes" json:"routes"`
	// Databases describes pluggable backends for search, vector embeddings,
	// and graph operations. Each backend can be configured independently via
	// YAML or environment variables.
	Databases DBConfig `yaml:"databases" json:"databases"`
	// EnableTools globally enables/disables tool exposure to the main agent.
	EnableTools bool `yaml:"enableTools" json:"enableTools"`
	// RequestInfoEnabled controls whether interactive agents may ask the user
	// for missing information through request_info. Nil defaults to enabled.
	RequestInfoEnabled *bool `yaml:"requestInfoEnabled" json:"requestInfoEnabled"`
	// Top-level allow list of tool names to expose to the main orchestrator agent.
	// If empty or omitted, all registered tools are exposed.
	ToolAllowList []string `yaml:"allowTools" json:"allowTools"`
	// AutoDiscover enables deferred tool discovery. When enabled, allowTools
	// becomes the initial bootstrap set and agents can load more tools mid-run.
	AutoDiscover bool `yaml:"autoDiscover" json:"autoDiscover"`
	// MaxDiscoveredTools caps how many non-bootstrap tools a single run can load
	// when auto-discovery is enabled.
	MaxDiscoveredTools int `yaml:"maxDiscoveredTools" json:"maxDiscoveredTools"`
	// Embedding configures the embedding service endpoint for text embeddings.
	Embedding EmbeddingConfig `yaml:"embedding" json:"embedding"`
	// Reranking configures the optional external reranking service for RAG retrieval.
	Reranking RerankingConfig `yaml:"reranking" json:"reranking"`
	// Magma configures optional multi-graph agentic memory support for RAG.
	Magma MagmaConfig `yaml:"magma" json:"magma"`
	// Archaeology configures decision lineage, artifact capture, and provenance.
	Archaeology ArchaeologyConfig `yaml:"archaeology" json:"archaeology"`
	// Memory coordinates evolving memory, belief memory, and MAGMA behind a
	// single runtime toggle. The legacy top-level memory blocks remain accepted
	// as aliases for compatibility.
	Memory MemoryConfig `yaml:"memory" json:"memory"`
	// MemoryConfigured is set when the YAML contains a top-level memory block.
	MemoryConfigured bool `yaml:"-" json:"-"`
	// EvolvingMemoryConfigured is set when the YAML contains the legacy
	// top-level evolvingMemory block.
	EvolvingMemoryConfigured bool `yaml:"-" json:"-"`
	// BeliefMemoryConfigured is set when the YAML contains the legacy
	// top-level beliefMemory block.
	BeliefMemoryConfigured bool `yaml:"-" json:"-"`
	// MagmaConfigured is set when the YAML contains the legacy top-level magma
	// block.
	MagmaConfigured bool `yaml:"-" json:"-"`
	// ImageTool configures defaults for the describe_image tool.
	ImageTool ImageToolConfig `yaml:"imageTool" json:"imageTool"`
	// EvolvingMemory configures the Search-Synthesis-Evolve memory system.
	EvolvingMemory EvolvingMemoryConfig `yaml:"evolvingMemory" json:"evolvingMemory"`
	// Transit configures the shared durable memory system.
	Transit TransitConfig `yaml:"transit" json:"transit"`
	// BeliefMemory configures shared belief-memory foundations.
	BeliefMemory BeliefMemoryConfig `yaml:"beliefMemory" json:"beliefMemory"`
	// TTS configures text-to-speech defaults and endpoint.
	TTS TTSConfig `yaml:"tts" json:"tts"`
	// STT configures speech-to-text defaults and endpoint.
	STT STTConfig `yaml:"stt" json:"stt"`
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
	// CodeQA configures deterministic and LLM-assisted code quality evaluation.
	CodeQA CodeQAConfig `yaml:"codeQA" json:"codeQA"`
	// Tokenization configures accurate token counting for summarization.
	Tokenization TokenizationConfig `yaml:"tokenization" json:"tokenization"`
}

// MemoryConfig is the unified agent-memory configuration. The nested subsystem
// configs preserve their existing shapes while the Enabled flag controls whether
// all coordinated memory lanes are active for new runs.
type MemoryConfig struct {
	Enabled    bool                   `yaml:"enabled" json:"enabled"`
	Retrieval  MemoryRetrievalConfig  `yaml:"retrieval" json:"retrieval"`
	LLMClients MemoryLLMClientsConfig `yaml:"llmClients" json:"llmClients"`
	Evolving   EvolvingMemoryConfig   `yaml:"evolving" json:"evolving"`
	Belief     BeliefMemoryConfig     `yaml:"belief" json:"belief"`
	Magma      MagmaConfig            `yaml:"magma" json:"magma"`
}

type MemoryRetrievalConfig struct {
	MaxTokensPerPrompt int  `yaml:"maxTokensPerPrompt" json:"maxTokensPerPrompt"`
	TimeoutMs          int  `yaml:"timeoutMs" json:"timeoutMs"`
	IncludeRecent      bool `yaml:"includeRecent" json:"includeRecent"`
}

type MemoryLLMClientsConfig struct {
	Evolving           LLMClientConfig `yaml:"evolving" json:"evolving"`
	BeliefDistillation LLMClientConfig `yaml:"beliefDistillation" json:"beliefDistillation"`
	MagmaConsolidation LLMClientConfig `yaml:"magmaConsolidation" json:"magmaConsolidation"`
}

// ArchaeologyConfig controls context-archaeology memory features.
type ArchaeologyConfig struct {
	Enabled                 bool                          `yaml:"enabled" json:"enabled"`
	ArchiveBeforeDelete     bool                          `yaml:"archive_before_delete" json:"archiveBeforeDelete"`
	DecisionDistiller       bool                          `yaml:"decision_distiller" json:"decisionDistiller"`
	CausalGroundingRequired bool                          `yaml:"causal_grounding_required" json:"causalGroundingRequired"`
	Reactor                 ArchaeologyReactorConfig      `yaml:"reactor" json:"reactor"`
	Retrieval               ArchaeologyRetrievalConfig    `yaml:"retrieval" json:"retrieval"`
	AutoActivate            ArchaeologyAutoActivateConfig `yaml:"auto_activate" json:"autoActivate"`
}

// ArchaeologyReactorConfig tunes belief-to-decision stale detection.
type ArchaeologyReactorConfig struct {
	ConfidenceFloor     float64 `yaml:"confidence_floor" json:"confidenceFloor"`
	ConfidenceDropDelta float64 `yaml:"confidence_drop_delta" json:"confidenceDropDelta"`
}

// ArchaeologyRetrievalConfig bounds the deterministic decision lane injected
// into the unified memory prompt context. The lane only activates when
// archaeology is enabled and the session memory toggle is on.
type ArchaeologyRetrievalConfig struct {
	// MaxDecisionsPerPrompt bounds how many decisions are rendered (default 5).
	MaxDecisionsPerPrompt int `yaml:"max_decisions_per_prompt" json:"maxDecisionsPerPrompt"`
	// MaxTokensPerPrompt bounds the rendered decision lane (default 600).
	MaxTokensPerPrompt int `yaml:"max_tokens_per_prompt" json:"maxTokensPerPrompt"`
	// TimeoutMs optionally tightens the decision lane below the shared memory
	// lane timeout. 0 means the shared timeout applies.
	TimeoutMs int `yaml:"timeout_ms" json:"timeoutMs"`
}

// ArchaeologyAutoActivateConfig gates deterministic candidate auto-activation.
// Lifecycle actions (reaffirm, revoke, supersede, stale->active) always stay
// deliberate operator/agent actions via decision_review.
type ArchaeologyAutoActivateConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// MinConfidence is the auto-activation floor (default 0.85; the LLM
	// distiller caps candidate confidence at 0.90).
	MinConfidence float64 `yaml:"min_confidence" json:"minConfidence"`
	// ConflictSimilarityFloor is the token-overlap similarity at which an
	// active in-scope decision is treated as conflicting (default 0.50).
	ConflictSimilarityFloor float64 `yaml:"conflict_similarity_floor" json:"conflictSimilarityFloor"`
}

// PromptOverridesConfig customizes built-in prompt blocks.
//
// Empty fields use the hard-coded defaults from internal/agent/prompts.
type PromptOverridesConfig struct {
	BaseSystem                 string `yaml:"baseSystem" json:"baseSystem"`
	MemoryInstructions         string `yaml:"memoryInstructions" json:"memoryInstructions"`
	ToolDiscoveryInstructions  string `yaml:"toolDiscoveryInstructions" json:"toolDiscoveryInstructions"`
	SkillDiscoveryInstructions string `yaml:"skillDiscoveryInstructions" json:"skillDiscoveryInstructions"`
}

// HarnessConfig controls the optional Forge-style guarded agent loop.
type HarnessConfig struct {
	Enabled           bool                             `yaml:"enabled" json:"enabled"`
	Mode              string                           `yaml:"mode" json:"mode"`
	RescueEnabled     bool                             `yaml:"rescueEnabled" json:"rescueEnabled"`
	MaxRetriesPerStep int                              `yaml:"maxRetriesPerStep" json:"maxRetriesPerStep"`
	MaxToolErrors     int                              `yaml:"maxToolErrors" json:"maxToolErrors"`
	TerminalTools     []string                         `yaml:"terminalTools" json:"terminalTools"`
	RequiredSteps     []string                         `yaml:"requiredSteps" json:"requiredSteps"`
	ToolPrerequisites map[string][]HarnessPrerequisite `yaml:"toolPrerequisites" json:"toolPrerequisites"`
	Compact           HarnessCompactConfig             `yaml:"compact" json:"compact"`
}

// HarnessPrerequisite requires one successful tool call before another tool can run.
type HarnessPrerequisite struct {
	Tool     string `yaml:"tool" json:"tool"`
	MatchArg string `yaml:"matchArg" json:"matchArg"`
}

// HarnessCompactConfig reserves config shape for later metadata-aware compaction.
type HarnessCompactConfig struct {
	Enabled         bool      `yaml:"enabled" json:"enabled"`
	KeepRecentSteps int       `yaml:"keepRecentSteps" json:"keepRecentSteps"`
	PhaseThresholds []float64 `yaml:"phaseThresholds" json:"phaseThresholds"`
}

// CodeQAConfig controls the code-quality judge pipeline.
type CodeQAConfig struct {
	Enabled                bool     `yaml:"enabled" json:"enabled"`
	ArtifactDir            string   `yaml:"artifactDir" json:"artifactDir"`
	MaxConcurrentRuns      int      `yaml:"maxConcurrentRuns" json:"maxConcurrentRuns"`
	MaxGateParallelism     int      `yaml:"maxGateParallelism" json:"maxGateParallelism"`
	MaxJudgeParallelism    int      `yaml:"maxJudgeParallelism" json:"maxJudgeParallelism"`
	DefaultMaxDiffBytes    int      `yaml:"defaultMaxDiffBytes" json:"defaultMaxDiffBytes"`
	DefaultMaxChangedFiles int      `yaml:"defaultMaxChangedFiles" json:"defaultMaxChangedFiles"`
	AcceptThreshold        float64  `yaml:"acceptThreshold" json:"acceptThreshold"`
	MinConfidence          float64  `yaml:"minConfidence" json:"minConfidence"`
	JudgeModel             string   `yaml:"judgeModel" json:"judgeModel"`
	ProposerModel          string   `yaml:"proposerModel" json:"proposerModel"`
	AllowedCommands        []string `yaml:"allowedCommands" json:"allowedCommands"`
	HighRiskGlobs          []string `yaml:"highRiskGlobs" json:"highRiskGlobs"`
	ForbiddenGlobs         []string `yaml:"forbiddenGlobs" json:"forbiddenGlobs"`
	AllowAutoApply         bool     `yaml:"allowAutoApply" json:"allowAutoApply"`
	AllowCommitAccepted    bool     `yaml:"allowCommitAccepted" json:"allowCommitAccepted"`
}

// MatrixConfig controls the built-in Matrix gateway runtime.
type MatrixConfig struct {
	Enabled               bool               `yaml:"enabled" json:"enabled"`
	HomeserverURL         string             `yaml:"homeserverURL" json:"homeserverURL"`
	UserID                string             `yaml:"userID" json:"userID"`
	AccessToken           string             `yaml:"accessToken" json:"accessToken"`
	DeviceID              string             `yaml:"deviceID" json:"deviceID"`
	MessageRetention      int                `yaml:"messageRetention" json:"messageRetention"`
	SyncTimeoutSeconds    int                `yaml:"syncTimeoutSeconds" json:"syncTimeoutSeconds"`
	SyncRetryDelaySeconds int                `yaml:"syncRetryDelaySeconds" json:"syncRetryDelaySeconds"`
	ProcessBacklog        bool               `yaml:"processBacklog" json:"processBacklog"`
	Rooms                 []MatrixRoomConfig `yaml:"rooms" json:"rooms"`
}

// MatrixRoomConfig describes how a single Matrix room should route traffic.
type MatrixRoomConfig struct {
	RoomID           string            `yaml:"roomID" json:"roomID"`
	DefaultTarget    string            `yaml:"defaultTarget" json:"defaultTarget"`
	AllowUnmentioned bool              `yaml:"allowUnmentioned" json:"allowUnmentioned"`
	Mentions         map[string]string `yaml:"mentions" json:"mentions"`
	SystemPromptRef  string            `yaml:"systemPromptRef" json:"systemPromptRef"`
	MessageRetention int               `yaml:"messageRetention" json:"messageRetention"`
	MaxConcurrent    int               `yaml:"maxConcurrent" json:"maxConcurrent"`
}

// BeliefMemoryConfig controls the shared belief-memory subsystem.
type BeliefMemoryConfig struct {
	Enabled                     bool                           `yaml:"enabled" json:"enabled"`
	EnableDistillation          bool                           `yaml:"enableDistillation" json:"enableDistillation"`
	EnableRetrieval             bool                           `yaml:"enableRetrieval" json:"enableRetrieval"`
	EnableConstraintEnforcement bool                           `yaml:"enableConstraintEnforcement" json:"enableConstraintEnforcement"`
	MaxBeliefsPerPrompt         int                            `yaml:"maxBeliefsPerPrompt" json:"maxBeliefsPerPrompt"`
	MaxEvidencePerBelief        int                            `yaml:"maxEvidencePerBelief" json:"maxEvidencePerBelief"`
	DefaultConfidence           float64                        `yaml:"defaultConfidence" json:"defaultConfidence"`
	PromotionThreshold          float64                        `yaml:"promotionThreshold" json:"promotionThreshold"`
	LLMClient                   LLMClientConfig                `yaml:"llmClient" json:"llmClient"`
	Distillation                BeliefMemoryDistillationConfig `yaml:"distillation" json:"distillation"`
	Retrieval                   BeliefMemoryRetrievalConfig    `yaml:"retrieval" json:"retrieval"`
	Lifecycle                   BeliefMemoryLifecycleConfig    `yaml:"lifecycle" json:"lifecycle"`
	Enforcement                 BeliefMemoryEnforcementConfig  `yaml:"enforcement" json:"enforcement"`
	// EnableRAGEvidence blends RAG retrieval results into the belief router as a
	// dedicated evidence lane. Hard/soft constraints, approved policies, and
	// scoped beliefs continue to take precedence; RAG hits are surfaced as a
	// clearly delimited untrusted-evidence block in the prompt.
	EnableRAGEvidence       bool    `yaml:"enableRAGEvidence" json:"enableRAGEvidence"`
	MaxRAGEvidencePerPrompt int     `yaml:"maxRAGEvidencePerPrompt" json:"maxRAGEvidencePerPrompt"`
	RAGRetrievalK           int     `yaml:"ragRetrievalK" json:"ragRetrievalK"`
	RAGMinScore             float64 `yaml:"ragMinScore" json:"ragMinScore"`
}

type BeliefMemoryDistillationConfig struct {
	Mode                    string  `yaml:"mode" json:"mode"`
	MaxCandidatesPerEpisode int     `yaml:"maxCandidatesPerEpisode" json:"maxCandidatesPerEpisode"`
	MinCandidateConfidence  float64 `yaml:"minCandidateConfidence" json:"minCandidateConfidence"`
	AutoApplyMinConfidence  float64 `yaml:"autoApplyMinConfidence" json:"autoApplyMinConfidence"`
}

type BeliefMemoryRetrievalConfig struct {
	MinConfidence         float64 `yaml:"minConfidence" json:"minConfidence"`
	MaxTokensPerPrompt    int     `yaml:"maxTokensPerPrompt" json:"maxTokensPerPrompt"`
	IncludeContradictions bool    `yaml:"includeContradictions" json:"includeContradictions"`
}

type BeliefMemoryLifecycleConfig struct {
	MinEvidenceForPromotion     int     `yaml:"minEvidenceForPromotion" json:"minEvidenceForPromotion"`
	MaxEvidenceAgainstPromotion int     `yaml:"maxEvidenceAgainstPromotion" json:"maxEvidenceAgainstPromotion"`
	StaleAfterDays              int     `yaml:"staleAfterDays" json:"staleAfterDays"`
	StaleConfidenceDecay        float64 `yaml:"staleConfidenceDecay" json:"staleConfidenceDecay"`
	AllowOrgPromotion           bool    `yaml:"allowOrgPromotion" json:"allowOrgPromotion"`
}

type BeliefMemoryEnforcementConfig struct {
	AutoEnable                   bool    `yaml:"autoEnable" json:"autoEnable"`
	SoftPolicyThreshold          float64 `yaml:"softPolicyThreshold" json:"softPolicyThreshold"`
	HardConstraintThreshold      float64 `yaml:"hardConstraintThreshold" json:"hardConstraintThreshold"`
	HardConstraintMinEvidenceFor int     `yaml:"hardConstraintMinEvidenceFor" json:"hardConstraintMinEvidenceFor"`
}

// TokenizationConfig controls how tokens are counted for summarization decisions.
type TokenizationConfig struct {
	// Enabled activates accurate token counting using provider APIs when available.
	// When false, falls back to heuristic (chars/4).
	Enabled bool `yaml:"enabled" json:"enabled"`
	// CacheSize is the maximum number of token counts to cache. Default: 1000.
	CacheSize int `yaml:"cacheSize" json:"cacheSize"`
	// CacheTTLSeconds is how long cached token counts remain valid. Default: 3600 (1 hour).
	CacheTTLSeconds int `yaml:"cacheTTLSeconds" json:"cacheTTLSeconds"`
	// FallbackToHeuristic allows falling back to heuristic on tokenization errors.
	// Default: true.
	FallbackToHeuristic bool `yaml:"fallbackToHeuristic" json:"fallbackToHeuristic"`
}

// ProjectsConfig controls project storage and workspace behavior.
type ProjectsConfig struct {
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

// STTConfig holds speech-to-text specific configuration.
type STTConfig struct {
	// BaseURL is the HTTP base for STT requests. Requests will be POSTed to
	// ${BaseURL}/v1/audio/transcriptions if set.
	BaseURL string `yaml:"baseURL" json:"baseURL"`
	// Model is the default STT model to use when transcribing audio.
	Model string `yaml:"model" json:"model"`
}

type ExecConfig struct {
	BlockBinaries             []string          `yaml:"blockBinaries" json:"blockBinaries"`
	CommandRules              []ExecCommandRule `yaml:"commandRules" json:"commandRules"`
	Sandbox                   ExecSandboxConfig `yaml:"sandbox" json:"sandbox"`
	MaxCommandSeconds         int               `yaml:"maxCommandSeconds" json:"maxCommandSeconds"`
	MaxTerminalSessions       int               `yaml:"maxTerminalSessions" json:"maxTerminalSessions"`
	MaxTerminalRuntimeSeconds int               `yaml:"maxTerminalRuntimeSeconds" json:"maxTerminalRuntimeSeconds"`
	TerminalIdleTTLSeconds    int               `yaml:"terminalIdleTTLSeconds" json:"terminalIdleTTLSeconds"`
	TerminalOutputBufferBytes int               `yaml:"terminalOutputBufferBytes" json:"terminalOutputBufferBytes"`
}

type ExecCommandRule struct {
	ID            string   `yaml:"id,omitempty" json:"id,omitempty"`
	Decision      string   `yaml:"decision" json:"decision"`
	Pattern       []string `yaml:"pattern" json:"pattern"`
	Contexts      []string `yaml:"contexts,omitempty" json:"contexts,omitempty"`
	Justification string   `yaml:"justification,omitempty" json:"justification,omitempty"`
}

type ExecSandboxConfig struct {
	Enabled           *bool                    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	FailIfUnavailable *bool                    `yaml:"failIfUnavailable,omitempty" json:"failIfUnavailable,omitempty"`
	Network           ExecSandboxNetworkConfig `yaml:"network" json:"network"`
}

type ExecSandboxNetworkConfig struct {
	Enabled        *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	AllowedDomains []string `yaml:"allowedDomains,omitempty" json:"allowedDomains,omitempty"`
}

// SummaryConfig controls rolling chat summaries and their dedicated LLM client.
type SummaryConfig struct {
	Enabled                      bool            `yaml:"enabled" json:"enabled"`
	CallTimeoutSeconds           int             `yaml:"callTimeoutSeconds" json:"callTimeoutSeconds"`
	ContextWindowTokens          int             `yaml:"contextWindowTokens" json:"contextWindowTokens"`
	PlainTextContextWindowTokens int             `yaml:"plainTextContextWindowTokens" json:"plainTextContextWindowTokens"`
	ReserveBufferTokens          int             `yaml:"reserveBufferTokens" json:"reserveBufferTokens"`
	MinKeepLastMessages          int             `yaml:"minKeepLastMessages" json:"minKeepLastMessages"`
	MaxKeepLastMessages          int             `yaml:"maxKeepLastMessages" json:"maxKeepLastMessages"`
	MaxSummaryChunkTokens        int             `yaml:"maxSummaryChunkTokens" json:"maxSummaryChunkTokens"`
	LLMClient                    LLMClientConfig `yaml:"llm_client" json:"llmClient"`
}

// LLMClientConfig selects the LLM provider and holds provider-specific configs.
type LLMClientConfig struct {
	Provider  string          `yaml:"provider" json:"provider"`
	OpenAI    OpenAIConfig    `yaml:"openai" json:"openai"`
	Anthropic AnthropicConfig `yaml:"anthropic" json:"anthropic"`
	Google    GoogleConfig    `yaml:"google" json:"google"`
}

type OpenAIConfig struct {
	APIKey         string `yaml:"apiKey" json:"apiKey"`
	Model          string `yaml:"model" json:"model"`
	BaseURL        string `yaml:"baseURL" json:"baseURL"`
	SummaryModel   string `yaml:"summaryModel" json:"summaryModel"`
	SummaryBaseURL string `yaml:"summaryBaseURL" json:"summaryBaseURL"`
	// API selects which OpenAI-compatible API surface to use for chat: "completions" or "responses".
	// Defaults to "responses" if empty.
	API string `yaml:"api" json:"api"`
	// ExtraHeaders are added to every main agent HTTP request.
	ExtraHeaders map[string]string `yaml:"extraHeaders" json:"extraHeaders"`
	// ExtraParams are merged into the chat completions request for the main agent.
	ExtraParams map[string]any `yaml:"extraParams" json:"extraParams"`
	// LogPayloads enables verbose logging of request/response bodies with redaction.
	LogPayloads bool `yaml:"logPayloads" json:"logPayloads"`
}

// AnthropicConfig holds Anthropic provider settings.
type AnthropicConfig struct {
	APIKey  string `yaml:"apiKey" json:"apiKey"`
	Model   string `yaml:"model" json:"model"`
	BaseURL string `yaml:"baseURL" json:"baseURL"`
	// MaxTokens overrides the per-request max output tokens for Anthropic.
	// When unset (0), Manifold uses a conservative default.
	MaxTokens int64 `yaml:"maxTokens" json:"maxTokens"`
	// ExtraParams are merged into the Anthropic request body.
	ExtraParams map[string]any `yaml:"extraParams" json:"extraParams"`
	// PromptCache enables Anthropic prompt caching via cache_control on supported blocks.
	// When enabled, Manifold will attach ephemeral cache_control directives to
	// selected request parts (system prompt, tool schema, and/or message blocks).
	PromptCache AnthropicPromptCacheConfig `yaml:"promptCache" json:"promptCache"`
}

// AnthropicPromptCacheConfig controls Anthropic prompt caching (cache_control).
//
// Anthropic currently supports ephemeral prompt caching with a fixed TTL value.
// We expose knobs for where Manifold should apply cache_control.
type AnthropicPromptCacheConfig struct {
	// Enabled turns on prompt caching directives in outgoing requests.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// CacheSystem applies cache_control to the final system prompt block.
	CacheSystem bool `yaml:"cacheSystem" json:"cacheSystem"`
	// CacheTools applies cache_control to the final tool definition.
	CacheTools bool `yaml:"cacheTools" json:"cacheTools"`
	// CacheMessages applies cache_control to the latest eligible message text block.
	// This is usually only useful when sending large, stable context content.
	CacheMessages bool `yaml:"cacheMessages" json:"cacheMessages"`
}

// GoogleConfig holds Google Gemini provider settings.
type GoogleConfig struct {
	APIKey       string                   `yaml:"apiKey" json:"apiKey"`
	Model        string                   `yaml:"model" json:"model"`
	BaseURL      string                   `yaml:"baseURL" json:"baseURL"`
	Timeout      int                      `yaml:"timeoutSeconds" json:"timeoutSeconds"`
	ContextCache GoogleContextCacheConfig `yaml:"contextCache" json:"contextCache"`
	ExtraParams  map[string]any           `yaml:"extraParams" json:"extraParams"`
}

// GoogleContextCacheConfig controls Gemini explicit context caching.
type GoogleContextCacheConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	CachedContent string `yaml:"cachedContent" json:"cachedContent"`
	AutoCreate    bool   `yaml:"autoCreate" json:"autoCreate"`
	TTLSeconds    int    `yaml:"ttlSeconds" json:"ttlSeconds"`
	CacheSystem   bool   `yaml:"cacheSystem" json:"cacheSystem"`
	CacheTools    bool   `yaml:"cacheTools" json:"cacheTools"`
	DisplayName   string `yaml:"displayName" json:"displayName"`
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
	// SummaryContextWindowTokens overrides the summary context window size (in tokens)
	// for this specialist. Zero means use the global fallback.
	SummaryContextWindowTokens int `yaml:"summaryContextWindowTokens" json:"summaryContextWindowTokens"`
	// API, when set, overrides which API surface to use for this specialist: "completions" or "responses".
	API                string `yaml:"api" json:"api"`
	EnableTools        bool   `yaml:"enableTools" json:"enableTools"`
	RequestInfoEnabled *bool  `yaml:"requestInfoEnabled" json:"requestInfoEnabled"`
	// ImageGeneration routes specialist chat requests through provider image generation.
	ImageGeneration bool `yaml:"imageGeneration" json:"imageGeneration"`
	// AutoDiscover overrides the global auto-discovery setting for this specialist.
	// Nil means inherit the global default.
	AutoDiscover *bool `yaml:"autoDiscover" json:"autoDiscover"`
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
	// Harness optionally overrides the top-level Forge harness settings for this specialist.
	Harness *HarnessConfig `yaml:"harness,omitempty" json:"harness,omitempty"`
}

// SpecialistRoute defines simple pre-dispatch rules. If the user's prompt
// matches any of the contains substrings or regex patterns, the router will
type SpecialistRoute struct {
	Name     string   `yaml:"name" json:"name"`
	Contains []string `yaml:"contains" json:"contains"`
	Regex    []string `yaml:"regex" json:"regex"`
}

func RequestInfoEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
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
	ServiceName    string           `yaml:"serviceName" json:"serviceName"`
	ServiceVersion string           `yaml:"serviceVersion" json:"serviceVersion"`
	Environment    string           `yaml:"environment" json:"environment"`
	OTLP           string           `yaml:"otlp" json:"otlp"`
	Local          LocalObsConfig   `yaml:"local" json:"local"`
	ClickHouse     ClickHouseConfig `yaml:"clickhouse" json:"clickhouse"`
}

type LocalObsConfig struct {
	Enabled              *bool `yaml:"enabled" json:"enabled"`
	MetricsWindowMinutes int   `yaml:"metricsWindowMinutes" json:"metricsWindowMinutes"`
	MetricsBucketSeconds int   `yaml:"metricsBucketSeconds" json:"metricsBucketSeconds"`
	MaxLogs              int   `yaml:"maxLogs" json:"maxLogs"`
	MaxTraces            int   `yaml:"maxTraces" json:"maxTraces"`
	MaxSpansPerTrace     int   `yaml:"maxSpansPerTrace" json:"maxSpansPerTrace"`
}

func (c LocalObsConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

type WebConfig struct {
	SearXNGURL string `yaml:"searXNGURL" json:"searXNGURL"`
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
	// OIDC provides additional configuration when Provider=="oidc".
	OIDC OIDCConfig `yaml:"oidc" json:"oidc"`
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
	DisablePKCE         bool     `yaml:"disablePKCE" json:"disablePKCE"` // Disable PKCE for providers that don't support it well
}

// OIDCConfig contains optional provider-specific controls for OIDC providers.
type OIDCConfig struct {
	Scopes              []string        `yaml:"scopes" json:"scopes"`
	ResponseMode        string          `yaml:"responseMode" json:"responseMode"`
	TokenAuthStyle      string          `yaml:"tokenAuthStyle" json:"tokenAuthStyle"`
	ProviderName        string          `yaml:"providerName" json:"providerName"`
	LogoutURL           string          `yaml:"logoutURL" json:"logoutURL"`
	LogoutRedirectParam string          `yaml:"logoutRedirectParam" json:"logoutRedirectParam"`
	Apple               AppleOIDCConfig `yaml:"apple" json:"apple"`
}

// AppleOIDCConfig configures Sign in with Apple client-secret JWT generation.
type AppleOIDCConfig struct {
	TeamID               string `yaml:"teamID" json:"teamID"`
	KeyID                string `yaml:"keyID" json:"keyID"`
	PrivateKeyPath       string `yaml:"privateKeyPath" json:"privateKeyPath"`
	PrivateKey           string `yaml:"privateKey" json:"privateKey"`
	ClientSecretTTLHours int    `yaml:"clientSecretTTLHours" json:"clientSecretTTLHours"`
}

// DBConfig contains sub-config for each pluggable database backend.
