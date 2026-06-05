package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"manifold/internal/config"
)

// agentdSettings mirrors the frontend AgentdSettings shape.
type agentdSettings struct {
	OpenAISummaryModel                  string `json:"openaiSummaryModel"`
	OpenAISummaryURL                    string `json:"openaiSummaryUrl"`
	SummaryProvider                     string `json:"summaryProvider"`
	SummaryModel                        string `json:"summaryModel"`
	SummaryURL                          string `json:"summaryUrl"`
	SummaryEnabled                      bool   `json:"summaryEnabled"`
	SummaryContextWindowTokens          int    `json:"summaryContextWindowTokens"`
	SummaryPlainTextContextWindowTokens int    `json:"summaryPlainTextContextWindowTokens"`
	SummaryReserveBufferTokens          int    `json:"summaryReserveBufferTokens"`
	SummaryMinKeepLastMessages          int    `json:"summaryMinKeepLastMessages"`
	SummaryMaxKeepLastMessages          int    `json:"summaryMaxKeepLastMessages"`
	SummaryMaxSummaryChunkTokens        int    `json:"summaryMaxSummaryChunkTokens"`
	SummaryCallTimeoutSeconds           int    `json:"summaryCallTimeoutSeconds"`
	SummaryTokenBudget                  int    `json:"summaryTokenBudget"`
	RequestInfoEnabled                  bool   `json:"requestInfoEnabled"`

	PromptBaseSystem                 string `json:"promptBaseSystem"`
	PromptMemoryInstructions         string `json:"promptMemoryInstructions"`
	PromptToolDiscoveryInstructions  string `json:"promptToolDiscoveryInstructions"`
	PromptSkillDiscoveryInstructions string `json:"promptSkillDiscoveryInstructions"`

	EmbedBaseURL                        string            `json:"embedBaseUrl"`
	EmbedModel                          string            `json:"embedModel"`
	EmbedAPIKey                         string            `json:"embedApiKey"`
	EmbedAPIHeader                      string            `json:"embedApiHeader"`
	EmbedAPIHeaders                     map[string]string `json:"embedApiHeaders"`
	EmbedPath                           string            `json:"embedPath"`
	EmbedInstructionMode                string            `json:"embedInstructionMode"`
	EmbedInstructionFormat              string            `json:"embedInstructionFormat"`
	EmbedDefaultQueryInstruction        string            `json:"embedDefaultQueryInstruction"`
	EmbedRAGQueryInstruction            string            `json:"embedRagQueryInstruction"`
	EmbedEvolvingMemoryQueryInstruction string            `json:"embedEvolvingMemoryQueryInstruction"`
	EmbedTransitQueryInstruction        string            `json:"embedTransitQueryInstruction"`

	RerankEnabled     bool              `json:"rerankEnabled"`
	RerankBaseURL     string            `json:"rerankBaseUrl"`
	RerankModel       string            `json:"rerankModel"`
	RerankInstruction string            `json:"rerankInstruction"`
	RerankAPIKey      string            `json:"rerankApiKey"`
	RerankAPIHeader   string            `json:"rerankApiHeader"`
	RerankAPIHeaders  map[string]string `json:"rerankApiHeaders"`
	RerankPath        string            `json:"rerankPath"`

	AgentRunTimeoutSeconds  int `json:"agentRunTimeoutSeconds"`
	StreamRunTimeoutSeconds int `json:"streamRunTimeoutSeconds"`
	WorkflowTimeoutSeconds  int `json:"workflowTimeoutSeconds"`

	BlockBinaries                string                   `json:"blockBinaries"`
	CommandRules                 []config.ExecCommandRule `json:"commandRules"`
	SandboxEnabled               *bool                    `json:"sandboxEnabled,omitempty"`
	SandboxFailIfUnavailable     *bool                    `json:"sandboxFailIfUnavailable,omitempty"`
	SandboxNetworkEnabled        *bool                    `json:"sandboxNetworkEnabled,omitempty"`
	SandboxNetworkAllowedDomains []string                 `json:"sandboxNetworkAllowedDomains,omitempty"`
	MaxCommandSeconds            int                      `json:"maxCommandSeconds"`
	OutputTruncateBytes          int                      `json:"outputTruncateBytes"`
	MaxTerminalSessions          int                      `json:"maxTerminalSessions"`
	MaxTerminalRuntimeSeconds    int                      `json:"maxTerminalRuntimeSeconds"`
	TerminalIdleTTLSeconds       int                      `json:"terminalIdleTTLSeconds"`
	TerminalOutputBufferBytes    int                      `json:"terminalOutputBufferBytes"`

	OTELServiceName string `json:"otelServiceName"`
	ServiceVersion  string `json:"serviceVersion"`
	Environment     string `json:"environment"`
	OTLPEndpoint    string `json:"otelExporterOtlpEndpoint"`

	LogPath       string `json:"logPath"`
	LogLevel      string `json:"logLevel"`
	LogPayloads   bool   `json:"logPayloads"`
	LogRawPrompts bool   `json:"logRawPrompts"`

	SearXNGURL    string `json:"searxngUrl"`
	WebSearXNGURL string `json:"webSearxngUrl"`

	DatabaseURL string `json:"databaseUrl"`
	DBURL       string `json:"dbUrl"`
	PostgresDSN string `json:"postgresDsn"`

	SearchBackend string `json:"searchBackend"`
	SearchDSN     string `json:"searchDsn"`
	SearchIndex   string `json:"searchIndex"`

	VectorBackend string `json:"vectorBackend"`
	VectorDSN     string `json:"vectorDsn"`
	VectorIndex   string `json:"vectorIndex"`
	VectorDims    int    `json:"vectorDimensions"`
	VectorMetric  string `json:"vectorMetric"`

	GraphBackend string `json:"graphBackend"`
	GraphDSN     string `json:"graphDsn"`
}

func (a *app) agentdConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			a.handleGetAgentdConfig(w, r)
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			a.handleUpdateAgentdConfig(w, r)
		default:
			w.Header().Set("Allow", "GET, POST, PUT, PATCH")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) handleGetAgentdConfig(w http.ResponseWriter, r *http.Request) {
	// Auth gate if enabled
	if a.cfg.Auth.Enabled {
		if _, err := a.requireUserID(r); err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	writeJSON(w, http.StatusOK, currentAgentdSettings(a.cfg))
}

func (a *app) handleUpdateAgentdConfig(w http.ResponseWriter, r *http.Request) {
	// Auth gate if enabled
	if a.cfg.Auth.Enabled {
		if _, err := a.requireUserID(r); err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	var payload agentdSettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload = normalizeAgentdSettings(payload)

	if err := applyAgentdSettings(a.cfg, payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Persist to config.yaml to survive restarts.
	if err := persistToConfigYAML(payload); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("persist config.yaml: %w", err))
		return
	}
	if err := a.refreshSummaryRuntime(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("refresh summary runtime: %w", err))
		return
	}
	if a.specRegistry != nil {
		a.specRegistry.SetPromptOverrides(promptInstructionOverrides(a.cfg))
		a.specRegistry.SetRequestInfoEnabled(config.RequestInfoEnabled(a.cfg.RequestInfoEnabled))
	}
	a.refreshEngineSystemPrompt()

	// Indicate that a restart is required for some changes to fully apply.
	w.Header().Set("X-Needs-Restart", "true")
	writeJSON(w, http.StatusOK, currentAgentdSettings(a.cfg))
}

func (a *app) refreshSummaryRuntime() error {
	if a == nil || a.cfg == nil {
		return nil
	}
	summaryLLM, summaryModel, err := buildSummaryLLM(a.cfg, a.httpClient)
	if err != nil {
		return err
	}
	a.summaryLLM = summaryLLM
	if a.mgr != nil {
		if err := a.initChatMemory(a.cfg, *a.mgr, summaryLLM, summaryModel); err != nil {
			return err
		}
	}
	if a.engine != nil {
		a.engine.SummaryEnabled = a.cfg.Summary.Enabled || a.cfg.SummaryEnabled
		a.engine.SummaryReserveBufferTokens = firstPositiveInt(a.cfg.Summary.ReserveBufferTokens, a.cfg.SummaryReserveBufferTokens)
		a.engine.SummaryMinKeepLastMessages = firstPositiveInt(a.cfg.Summary.MinKeepLastMessages, a.cfg.SummaryMinKeepLastMessages)
		a.engine.SummaryMaxSummaryChunkTokens = firstPositiveInt(a.cfg.Summary.MaxSummaryChunkTokens, a.cfg.SummaryMaxSummaryChunkTokens)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
