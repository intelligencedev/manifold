package specialists

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"gptagent/internal/config"
	"gptagent/internal/llm"
	openaillm "gptagent/internal/llm/openai"
)

// Agent represents a configured specialist bound to a specific endpoint/model.
// It is designed for inference-only requests (no tool schema unless enabled).
type Agent struct {
	Name            string
	System          string
	Model           string
	EnableTools     bool
	ReasoningEffort string // optional: "low"|"medium"|"high"
	ExtraParams     map[string]any

	provider *openaillm.Client
}

// Registry holds addressable specialists by name.
type Registry struct {
	agents map[string]*Agent
}

// NewRegistry builds a registry from config.SpecialistConfig entries.
// The base OpenAI config is used as a default for API key/model unless
// overridden per specialist.
func NewRegistry(base config.OpenAIConfig, list []config.SpecialistConfig, httpClient *http.Client) *Registry {
	agents := make(map[string]*Agent, len(list))
	for _, sc := range list {
		// Derive OpenAI cfg for the specialist
		oc := config.OpenAIConfig{
			APIKey:  firstNonEmpty(sc.APIKey, base.APIKey),
			Model:   firstNonEmpty(sc.Model, base.Model),
			BaseURL: firstNonEmpty(sc.BaseURL, base.BaseURL),
		}
		// Build per-specialist HTTP client with extra headers if provided
		hc := httpClient
		if len(sc.ExtraHeaders) > 0 {
			if hc == nil {
				hc = http.DefaultClient
			}
			tr := hc.Transport
			if tr == nil {
				tr = http.DefaultTransport
			}
			hc = &http.Client{Transport: &headerTransport{base: tr, headers: sc.ExtraHeaders}}
		}
		prov := openaillm.New(oc, hc)
		a := &Agent{
			Name:            sc.Name,
			System:          sc.System,
			Model:           oc.Model,
			EnableTools:     sc.EnableTools,
			ReasoningEffort: strings.TrimSpace(sc.ReasoningEffort),
			ExtraParams:     sc.ExtraParams,
			provider:        prov,
		}
		if a.Name != "" {
			agents[a.Name] = a
		}
	}
	return &Registry{agents: agents}
}

// Names returns sorted agent names.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.agents))
	for k := range r.agents {
		out = append(out, k)
	}
	// no dependency on slices package to keep compat; simple insertion sort
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Get returns the named specialist.
func (r *Registry) Get(name string) (*Agent, bool) { a, ok := r.agents[name]; return a, ok }

// Inference performs a single-turn completion with optional history.
// If tools are disabled, no tool schema is sent at all.
// If ReasoningEffort is set, a provider-specific reasoning block is attached.
func (a *Agent) Inference(ctx context.Context, user string, history []llm.Message) (string, error) {
	if a.provider == nil {
		return "", errors.New("provider not configured")
	}
	msgs := make([]llm.Message, 0, len(history)+2)
	if sys := strings.TrimSpace(a.System); sys != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: sys})
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, llm.Message{Role: "user", Content: user})

	// Extra fields for the request: start with configured extra params
	extra := make(map[string]any, len(a.ExtraParams)+1)
	for k, v := range a.ExtraParams {
		extra[k] = v
	}
	if a.ReasoningEffort != "" && extra["reasoning"] == nil {
		extra["reasoning"] = map[string]any{"effort": a.ReasoningEffort}
	}

	var tools []llm.ToolSchema
	if !a.EnableTools {
		tools = nil // ensure omission
	} else {
		tools = []llm.ToolSchema{} // caller may extend in the future; no default
	}
	resp, err := a.provider.ChatWithOptions(ctx, msgs, tools, a.Model, extra)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// headerTransport injects static headers into every request.
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range t.headers {
		if r.Header.Get(k) == "" {
			r.Header.Set(k, v)
		}
	}
	return t.base.RoundTrip(r)
}
