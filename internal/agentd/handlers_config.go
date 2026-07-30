package agentd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"manifold/internal/config"
	llmproviders "manifold/internal/llm/providers"
	"manifold/internal/observability"

	yaml "gopkg.in/yaml.v3"
)

// agentdSettings mirrors the frontend AgentdSettings shape.
type agentdSettings struct {
	// ServerConfig exposes the complete effective configuration for inspection.
	// It is intentionally read-only here; the focused settings above remain the
	// supported live-edit surface and secrets are redacted by writeJSON.
	ServerConfig *config.Config `json:"serverConfig"`
	// ConfigSource is the redacted source configuration. Saving this field
	// updates config.yaml and takes effect after restart.
	ConfigSource string `json:"configSource"`
	// ConfigPatch updates one or more top-level configuration groups. It powers
	// the focused advanced editors without requiring an operator to edit YAML.
	ConfigPatch map[string]any `json:"configPatch"`

	// Primary LLM (propagates to chat/summary/specialists by default).
	LLMProvider string `json:"llmProvider"`
	LLMAPIKey   string `json:"llmApiKey"`
	LLMModel    string `json:"llmModel"`
	LLMBaseURL  string `json:"llmBaseUrl"`

	// Unified memory master switch. Embeddings only apply when true.
	MemoryEnabled bool `json:"memoryEnabled"`

	LexMinifyEnabled                bool `json:"lexMinifyEnabled"`
	LexMinifyLevel                  int  `json:"lexMinifyLevel"`
	LexMinifyZones                  int  `json:"lexMinifyZones"`
	LexMinifyCurrentRequestMaxLevel int  `json:"lexMinifyCurrentRequestMaxLevel"`

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
	writeJSON(w, http.StatusOK, a.currentAgentdSettings())
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	focusedSettings := hasFocusedSettingsIntent(body)
	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawPayload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// serverConfig is read-only inspection data echoed by the frontend. It may
	// contain redacted values that cannot be decoded into config.Config.
	delete(rawPayload, "serverConfig")
	sanitizedBody, err := json.Marshal(rawPayload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var payload agentdSettings
	if err := json.Unmarshal(sanitizedBody, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !focusedSettings && strings.TrimSpace(payload.ConfigSource) != "" {
		if err := persistConfigSource(payload.ConfigSource); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		w.Header().Set("X-Needs-Restart", "true")
		writeJSON(w, http.StatusOK, a.currentAgentdSettings())
		return
	}
	if !focusedSettings && len(payload.ConfigPatch) > 0 {
		if err := persistConfigPatch(payload.ConfigPatch); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		w.Header().Set("X-Needs-Restart", "true")
		writeJSON(w, http.StatusOK, a.currentAgentdSettings())
		return
	}
	payload = normalizeAgentdSettings(payload)
	payload.CommandRules = nil

	if err := applyAgentdSettings(a.cfg, payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.refreshLexMinifyRuntime()

	// Rebuild primary LLM when provider credentials changed.
	if a.httpClient != nil {
		if llm, err := llmproviders.Build(*a.cfg, a.httpClient); err == nil {
			a.llm = llm
			if a.engine != nil {
				a.engine.LLM = llm
				a.engine.Model = resolveLLMClientModel(a.cfg.LLMClient)
			}
		}
	}

	// Persist to config.yaml to survive restarts.
	if err := persistToConfigYAML(a.cfg, payload); err != nil {
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
	writeJSON(w, http.StatusOK, a.currentAgentdSettings())
}

func hasFocusedSettingsIntent(body []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return false
	}
	for field := range fields {
		switch field {
		case "serverConfig", "configSource", "configPatch":
		default:
			return true
		}
	}
	return false
}

func (a *app) currentAgentdSettings() agentdSettings {
	settings := currentAgentdSettings(a.cfg)
	settings.ServerConfig = a.cfg
	settings.ConfigSource = currentConfigSource()
	return settings
}

func currentConfigSource() string {
	contents, err := os.ReadFile(findConfigYAMLPath())
	if err != nil {
		return ""
	}
	var root map[string]any
	if yaml.Unmarshal(contents, &root) != nil {
		return string(contents)
	}
	redacted, ok := observability.RedactValue(root).(map[string]any)
	if !ok {
		return string(contents)
	}
	contents, err = yaml.Marshal(redacted)
	if err != nil {
		return ""
	}
	return string(contents)
}

func persistConfigSource(source string) error {
	if err := validateConfigSource(source); err != nil {
		return err
	}
	var next map[string]any
	if err := yaml.Unmarshal([]byte(source), &next); err != nil {
		return err
	}

	existing := readConfigMap()
	restoreRedactedConfigValues(next, existing)
	return writeConfigMap(next)
}

func persistConfigPatch(patch map[string]any) error {
	if len(patch) == 0 {
		return fmt.Errorf("configuration patch is required")
	}
	root := readConfigMap()
	for group, next := range patch {
		if strings.TrimSpace(group) == "" {
			return fmt.Errorf("configuration group name is required")
		}
		if group == "__root" {
			values, ok := next.(map[string]any)
			if !ok {
				return fmt.Errorf("root configuration patch must be an object")
			}
			for key, value := range values {
				key = configRootKey(root, key)
				if value == "[REDACTED]" {
					continue
				}
				root[key] = value
			}
			continue
		}
		group = configRootKey(root, group)
		if next == "[REDACTED]" {
			continue
		}
		if nested, ok := next.(map[string]any); ok {
			if existing, ok := root[group].(map[string]any); ok {
				restoreRedactedConfigValues(nested, existing)
			}
		}
		root[group] = next
	}
	return writeConfigMap(root)
}

func configRootKey(root map[string]any, requested string) string {
	for key := range root {
		if normalizedConfigKey(key) == normalizedConfigKey(requested) {
			return key
		}
	}
	return requested
}

func normalizedConfigKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.NewReplacer("_", "", "-", "").Replace(key)
}

func readConfigMap() map[string]any {
	root := map[string]any{}
	if contents, err := os.ReadFile(findConfigYAMLPath()); err == nil {
		_ = yaml.Unmarshal(contents, &root)
	}
	return root
}

func writeConfigMap(root map[string]any) error {
	contents, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	path := findConfigYAMLPath()
	if err := ensureConfigParentDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o644)
}

func validateConfigSource(source string) error {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(source), &root); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	if len(root) == 0 {
		return fmt.Errorf("configuration must contain a top-level mapping")
	}
	return nil
}

func restoreRedactedConfigValues(next, existing map[string]any) {
	for key, value := range next {
		if value == "[REDACTED]" {
			if original, ok := existing[key]; ok {
				next[key] = original
			}
			continue
		}
		nested, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if original, ok := existing[key].(map[string]any); ok {
			restoreRedactedConfigValues(nested, original)
		}
	}
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
		a.engine.SummaryCallTimeout = summaryCallTimeout(a.cfg)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(observability.RedactValue(v))
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
