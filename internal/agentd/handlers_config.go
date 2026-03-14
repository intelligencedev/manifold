package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// agentdSettings mirrors the frontend AgentdSettings shape.
type agentdSettings struct {
	OpenAISummaryModel         string `json:"openaiSummaryModel"`
	OpenAISummaryURL           string `json:"openaiSummaryUrl"`
	SummaryEnabled             bool   `json:"summaryEnabled"`
	SummaryReserveBufferTokens int    `json:"summaryReserveBufferTokens"`

	EmbedBaseURL    string            `json:"embedBaseUrl"`
	EmbedModel      string            `json:"embedModel"`
	EmbedAPIKey     string            `json:"embedApiKey"`
	EmbedAPIHeader  string            `json:"embedApiHeader"`
	EmbedAPIHeaders map[string]string `json:"embedApiHeaders"`
	EmbedPath       string            `json:"embedPath"`

	AgentRunTimeoutSeconds  int `json:"agentRunTimeoutSeconds"`
	StreamRunTimeoutSeconds int `json:"streamRunTimeoutSeconds"`
	WorkflowTimeoutSeconds  int `json:"workflowTimeoutSeconds"`

	BlockBinaries       string `json:"blockBinaries"`
	MaxCommandSeconds   int    `json:"maxCommandSeconds"`
	OutputTruncateBytes int    `json:"outputTruncateBytes"`

	OTELServiceName string `json:"otelServiceName"`
	ServiceVersion  string `json:"serviceVersion"`
	Environment     string `json:"environment"`
	OTLPEndpoint    string `json:"otelExporterOtlpEndpoint"`

	LogPath     string `json:"logPath"`
	LogLevel    string `json:"logLevel"`
	LogPayloads bool   `json:"logPayloads"`

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

	// Indicate that a restart is required for some changes to fully apply.
	w.Header().Set("X-Needs-Restart", "true")
	writeJSON(w, http.StatusOK, payload)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
