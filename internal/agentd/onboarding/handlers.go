package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"manifold/internal/config"
)

// StatusResponse is the public setup status payload returned by the onboarding
// endpoints.
type StatusResponse struct {
	Ready             bool   `json:"ready"`
	NeedsSetup        bool   `json:"needsSetup"`
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	HasCredentials    bool   `json:"hasCredentials"`
	MemoryEnabled     bool   `json:"memoryEnabled"`
	EmbeddingRequired bool   `json:"embeddingRequired"`
	ConfigPath        string `json:"configPath"`
	BaseURL           string `json:"baseUrl,omitempty"`
	ListenAddr        string `json:"listenAddr,omitempty"`
}

// CompleteRequest contains the provider and optional memory settings submitted
// by the setup form.
type CompleteRequest struct {
	Provider      string `json:"provider"`
	APIKey        string `json:"apiKey"`
	Model         string `json:"model"`
	BaseURL       string `json:"baseUrl"`
	MemoryEnabled *bool  `json:"memoryEnabled,omitempty"`
	EmbedAPIKey   string `json:"embedApiKey,omitempty"`
	EmbedBaseURL  string `json:"embedBaseUrl,omitempty"`
	EmbedModel    string `json:"embedModel,omitempty"`
}

// Deps contains the narrow composition-root callbacks needed by the setup
// handlers. Config mutation and prompt seeding remain outside this package.
type Deps struct {
	Config        *config.Config
	AuthEnabled   bool
	ListenAddr    string
	PublicURL     string
	ConfigPath    func() string
	ResolveModel  func(config.LLMClientConfig) string
	RequireUserID func(*http.Request) (int64, error)
	ApplyComplete func(CompleteRequest) error
	SeedPrompt    func(context.Context, int64) error
}

// StatusHandler returns the GET-only setup status handler.
func StatusHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if deps.Config == nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("config unavailable"))
			return
		}
		ready := config.HasPrimaryLLMCredentials(deps.Config)
		writeJSON(w, http.StatusOK, setupStatus(deps, ready))
	}
}

// CompleteHandler returns the POST-only setup completion handler.
func CompleteHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if deps.Config == nil || deps.ApplyComplete == nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("setup unavailable"))
			return
		}

		userID := int64(0)
		if deps.AuthEnabled {
			if deps.RequireUserID == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var err error
			userID, err = deps.RequireUserID(r)
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="sio"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		var req CompleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid setup payload: %w", err))
			return
		}
		if err := deps.ApplyComplete(req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if deps.SeedPrompt != nil {
			if err := deps.SeedPrompt(r.Context(), userID); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		ready := config.HasPrimaryLLMCredentials(deps.Config)
		writeJSON(w, http.StatusOK, setupStatus(deps, ready))
	}
}

func setupStatus(deps Deps, ready bool) StatusResponse {
	model := ""
	if deps.ResolveModel != nil {
		model = deps.ResolveModel(deps.Config.LLMClient)
	}
	configPath := deps.Config.ConfigPath
	if deps.ConfigPath != nil {
		configPath = deps.ConfigPath()
	}
	return StatusResponse{
		Ready:             ready,
		NeedsSetup:        !ready,
		Provider:          strings.ToLower(strings.TrimSpace(deps.Config.LLMClient.Provider)),
		Model:             model,
		HasCredentials:    ready,
		MemoryEnabled:     deps.Config.Memory.Enabled,
		EmbeddingRequired: deps.Config.Memory.Enabled,
		ConfigPath:        configPath,
		BaseURL:           deps.PublicURL,
		ListenAddr:        deps.ListenAddr,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
