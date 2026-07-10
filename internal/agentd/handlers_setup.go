package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	yaml "gopkg.in/yaml.v3"

	"manifold/internal/config"
	llmproviders "manifold/internal/llm/providers"
)

type setupStatusResponse struct {
	Ready              bool   `json:"ready"`
	NeedsSetup         bool   `json:"needsSetup"`
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	HasCredentials     bool   `json:"hasCredentials"`
	MemoryEnabled      bool   `json:"memoryEnabled"`
	EmbeddingRequired  bool   `json:"embeddingRequired"`
	ConfigPath         string `json:"configPath"`
	BaseURL            string `json:"baseUrl,omitempty"`
	ListenAddr         string `json:"listenAddr,omitempty"`
}

type setupCompleteRequest struct {
	Provider    string `json:"provider"`
	APIKey      string `json:"apiKey"`
	Model       string `json:"model"`
	BaseURL     string `json:"baseUrl"`
	// MemoryEnabled is optional; defaults remain off for first run.
	MemoryEnabled *bool `json:"memoryEnabled,omitempty"`
	// EmbedAPIKey is optional and only written when memory is enabled.
	EmbedAPIKey   string `json:"embedApiKey,omitempty"`
	EmbedBaseURL  string `json:"embedBaseUrl,omitempty"`
	EmbedModel    string `json:"embedModel,omitempty"`
}

func (a *app) setupStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		ready := config.HasPrimaryLLMCredentials(a.cfg)
		writeJSON(w, http.StatusOK, setupStatusResponse{
			Ready:             ready,
			NeedsSetup:        !ready,
			Provider:          strings.ToLower(strings.TrimSpace(a.cfg.LLMClient.Provider)),
			Model:             resolveLLMClientModel(a.cfg.LLMClient),
			HasCredentials:     ready,
			MemoryEnabled:     a.cfg.Memory.Enabled,
			EmbeddingRequired: a.cfg.Memory.Enabled,
			ConfigPath:        firstNonEmptyTrimmed(a.cfg.ConfigPath, findConfigYAMLPath()),
			ListenAddr:        a.listenAddr,
			BaseURL:           a.publicURL,
		})
	}
}

func (a *app) setupCompleteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if a.cfg.Auth.Enabled {
			if _, err := a.requireUserID(r); err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="sio"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		var req setupCompleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid setup payload: %w", err))
			return
		}
		if err := a.applySetupComplete(req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		ready := config.HasPrimaryLLMCredentials(a.cfg)
		writeJSON(w, http.StatusOK, setupStatusResponse{
			Ready:             ready,
			NeedsSetup:        !ready,
			Provider:          strings.ToLower(strings.TrimSpace(a.cfg.LLMClient.Provider)),
			Model:             resolveLLMClientModel(a.cfg.LLMClient),
			HasCredentials:     ready,
			MemoryEnabled:     a.cfg.Memory.Enabled,
			EmbeddingRequired: a.cfg.Memory.Enabled,
			ConfigPath:        firstNonEmptyTrimmed(a.cfg.ConfigPath, findConfigYAMLPath()),
			ListenAddr:        a.listenAddr,
			BaseURL:           a.publicURL,
		})
	}
}

func (a *app) applySetupComplete(req setupCompleteRequest) error {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(a.cfg.LLMClient.Provider))
	}
	if provider == "" {
		provider = "openai"
	}
	switch provider {
	case "openai", "anthropic", "google", "local":
	default:
		return fmt.Errorf("provider must be one of openai, anthropic, google, or local")
	}

	apiKey := strings.TrimSpace(req.APIKey)
	model := strings.TrimSpace(req.Model)
	baseURL := strings.TrimSpace(req.BaseURL)

	a.cfg.LLMClient.Provider = provider
	switch provider {
	case "anthropic":
		if apiKey == "" {
			return fmt.Errorf("apiKey is required for anthropic")
		}
		a.cfg.LLMClient.Anthropic.APIKey = apiKey
		if model != "" {
			a.cfg.LLMClient.Anthropic.Model = model
		}
		if baseURL != "" {
			a.cfg.LLMClient.Anthropic.BaseURL = baseURL
		}
	case "google":
		if apiKey == "" {
			return fmt.Errorf("apiKey is required for google")
		}
		a.cfg.LLMClient.Google.APIKey = apiKey
		if model != "" {
			a.cfg.LLMClient.Google.Model = model
		}
		if baseURL != "" {
			a.cfg.LLMClient.Google.BaseURL = baseURL
		}
	default: // openai + local
		if provider != "local" && apiKey == "" {
			return fmt.Errorf("apiKey is required for openai")
		}
		if apiKey != "" {
			a.cfg.LLMClient.OpenAI.APIKey = apiKey
		}
		if model != "" {
			a.cfg.LLMClient.OpenAI.Model = model
		}
		if baseURL != "" {
			a.cfg.LLMClient.OpenAI.BaseURL = baseURL
		}
		if provider == "local" {
			a.cfg.LLMClient.OpenAI.API = "completions"
		}
	}
	a.cfg.OpenAI = a.cfg.LLMClient.OpenAI

	if req.MemoryEnabled != nil {
		a.cfg.Memory.Enabled = *req.MemoryEnabled
		a.cfg.MemoryConfigured = true
		a.cfg.EvolvingMemory.Enabled = *req.MemoryEnabled
		a.cfg.BeliefMemory.Enabled = *req.MemoryEnabled
		a.cfg.Magma.Enabled = *req.MemoryEnabled
	}

	// Single embedding endpoint: only apply settings when memory is on.
	if a.cfg.Memory.Enabled {
		if strings.TrimSpace(req.EmbedAPIKey) != "" {
			a.cfg.Embedding.APIKey = strings.TrimSpace(req.EmbedAPIKey)
		} else if provider == "openai" && strings.TrimSpace(a.cfg.LLMClient.OpenAI.APIKey) != "" {
			// Convenience: mark OpenAI key as embedding key when memory enabled.
			a.cfg.Embedding.APIKey = a.cfg.LLMClient.OpenAI.APIKey
		}
		if strings.TrimSpace(req.EmbedBaseURL) != "" {
			a.cfg.Embedding.BaseURL = strings.TrimSpace(req.EmbedBaseURL)
		}
		if strings.TrimSpace(req.EmbedModel) != "" {
			a.cfg.Embedding.Model = strings.TrimSpace(req.EmbedModel)
		}
	}

	if err := persistPrimaryLLMConfig(a.cfg); err != nil {
		return fmt.Errorf("persist config: %w", err)
	}

	// Hot-reload primary LLM used by chat/orchestrator when possible.
	if a.httpClient != nil {
		llm, err := llmproviders.Build(*a.cfg, a.httpClient)
		if err != nil {
			log.Warn().Err(err).Msg("setup: failed to rebuild llm provider; restart may be required")
		} else {
			a.llm = llm
			if a.engine != nil {
				a.engine.LLM = llm
				a.engine.Model = resolveLLMClientModel(a.cfg.LLMClient)
			}
		}
	}
	return nil
}

func persistPrimaryLLMConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	path := firstNonEmptyTrimmed(cfg.ConfigPath, findConfigYAMLPath())
	if err := ensureConfigParentDir(path); err != nil {
		return err
	}

	root := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(b, &root)
	}

	setNestedMapValue(root, []string{"llm_client", "provider"}, cfg.LLMClient.Provider)
	switch strings.ToLower(strings.TrimSpace(cfg.LLMClient.Provider)) {
	case "anthropic":
		setNestedMapValue(root, []string{"llm_client", "anthropic", "apiKey"}, cfg.LLMClient.Anthropic.APIKey)
		if cfg.LLMClient.Anthropic.Model != "" {
			setNestedMapValue(root, []string{"llm_client", "anthropic", "model"}, cfg.LLMClient.Anthropic.Model)
		}
		if cfg.LLMClient.Anthropic.BaseURL != "" {
			setNestedMapValue(root, []string{"llm_client", "anthropic", "baseURL"}, cfg.LLMClient.Anthropic.BaseURL)
		}
	case "google":
		setNestedMapValue(root, []string{"llm_client", "google", "apiKey"}, cfg.LLMClient.Google.APIKey)
		if cfg.LLMClient.Google.Model != "" {
			setNestedMapValue(root, []string{"llm_client", "google", "model"}, cfg.LLMClient.Google.Model)
		}
		if cfg.LLMClient.Google.BaseURL != "" {
			setNestedMapValue(root, []string{"llm_client", "google", "baseURL"}, cfg.LLMClient.Google.BaseURL)
		}
	default:
		setNestedMapValue(root, []string{"llm_client", "openai", "apiKey"}, cfg.LLMClient.OpenAI.APIKey)
		if cfg.LLMClient.OpenAI.Model != "" {
			setNestedMapValue(root, []string{"llm_client", "openai", "model"}, cfg.LLMClient.OpenAI.Model)
		}
		if cfg.LLMClient.OpenAI.BaseURL != "" {
			setNestedMapValue(root, []string{"llm_client", "openai", "baseURL"}, cfg.LLMClient.OpenAI.BaseURL)
		}
		if cfg.LLMClient.OpenAI.API != "" {
			setNestedMapValue(root, []string{"llm_client", "openai", "api"}, cfg.LLMClient.OpenAI.API)
		}
	}

	setNestedMapValue(root, []string{"memory", "enabled"}, cfg.Memory.Enabled)
	if cfg.Memory.Enabled {
		if cfg.Embedding.APIKey != "" {
			setNestedMapValue(root, []string{"embedding", "apiKey"}, cfg.Embedding.APIKey)
		}
		if cfg.Embedding.BaseURL != "" {
			setNestedMapValue(root, []string{"embedding", "baseURL"}, cfg.Embedding.BaseURL)
		}
		if cfg.Embedding.Model != "" {
			setNestedMapValue(root, []string{"embedding", "model"}, cfg.Embedding.Model)
		}
		if cfg.Embedding.Path != "" {
			setNestedMapValue(root, []string{"embedding", "path"}, cfg.Embedding.Path)
		}
	}

	b, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	cfg.ConfigPath = path
	return nil
}
