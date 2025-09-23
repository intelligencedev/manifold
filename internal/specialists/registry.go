package specialists

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"intelligence.dev/internal/config"
	"intelligence.dev/internal/llm"
	openaillm "intelligence.dev/internal/llm/openai"
	"intelligence.dev/internal/tools"
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
	tools    tools.Registry
}

// Registry holds addressable specialists by name.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*Agent
}

// NewRegistry builds a registry from config.SpecialistConfig entries.
// The base OpenAI config is used as a default for API key/model unless
// overridden per specialist.
func NewRegistry(base config.OpenAIConfig, list []config.SpecialistConfig, httpClient *http.Client, toolsReg tools.Registry) *Registry {
	reg := &Registry{agents: make(map[string]*Agent, len(list))}
	reg.ReplaceFromConfigs(base, list, httpClient, toolsReg)
	return reg
}

// ReplaceFromConfigs rebuilds the registry from configs (skips paused specialists).
func (r *Registry) ReplaceFromConfigs(base config.OpenAIConfig, list []config.SpecialistConfig, httpClient *http.Client, toolsReg tools.Registry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	agents := make(map[string]*Agent, len(list))
	for _, sc := range list {
		if sc.Paused {
			continue
		}
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
		var toolsView tools.Registry
		if sc.EnableTools && toolsReg != nil {
			toolsView = tools.NewFilteredRegistry(toolsReg, sc.AllowTools)
		} else {
			toolsView = nil
		}

		a := &Agent{
			Name:            sc.Name,
			System:          sc.System,
			Model:           oc.Model,
			EnableTools:     sc.EnableTools,
			ReasoningEffort: strings.TrimSpace(sc.ReasoningEffort),
			ExtraParams:     sc.ExtraParams,
			provider:        prov,
			tools:           toolsView,
		}
		if a.Name != "" {
			agents[a.Name] = a
		}
	}
	r.agents = agents
}

// Names returns sorted agent names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
func (r *Registry) Get(name string) (*Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[name]
	return a, ok
}

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
	if a.ReasoningEffort != "" && extra["reasoning_effort"] == nil {
		// Provider expects a simple enum string for reasoning_effort ("low"|"medium"|"high").
		// Previously we sent an object which caused a 400 invalid_type error.
		extra["reasoning_effort"] = a.ReasoningEffort
	}

	// If tools are enabled and a tools registry is attached, include schemas
	// and perform a single-step execution: run the first tool call (if any)
	// and return its payload directly. If tools are disabled we must not
	// include any schemas nor attempt dispatch.
	if a.EnableTools && a.tools != nil {
		messages := msgs
		// Optional tool sink for UIs to display tool calls
		var sink ToolSink
		if v := ctx.Value(toolSinkKey{}); v != nil {
			if f, ok := v.(ToolSink); ok {
				sink = f
			}
		}
		msg, err := a.provider.ChatWithOptions(ctx, messages, a.tools.Schemas(), a.Model, extra)
		if err != nil {
			return "", err
		}
		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}
		tc := msg.ToolCalls[0]
		// Propagate the specialist's provider to the tool dispatch context so
		// tools that make LLM calls (describe_image, llm_transform, etc.) can
		// use the same provider/model/baseURL as the specialist.
		dispatchCtx := ctx
		if a.provider != nil {
			dispatchCtx = tools.WithProvider(ctx, a.provider)
		}
		payload, err := a.tools.Dispatch(dispatchCtx, tc.Name, tc.Args)
		if err != nil {
			payload = []byte("{" + strconv.Quote("error") + ":" + strconv.Quote(err.Error()) + "}")
		}
		if sink != nil {
			sink(tc.Name, payload, tc.Args)
		}
		return string(payload), nil
	}

	var schemas []llm.ToolSchema
	if !a.EnableTools {
		schemas = nil // ensure omission
	} else {
		schemas = []llm.ToolSchema{} // no attached registry; send empty
	}
	resp, err := a.provider.ChatWithOptions(ctx, msgs, schemas, a.Model, extra)
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

// Tool sink plumbing to surface specialist tool calls to UIs
type ToolSink func(name string, payload []byte, args json.RawMessage)
type toolSinkKey struct{}

func WithToolSink(ctx context.Context, sink ToolSink) context.Context {
	return context.WithValue(ctx, toolSinkKey{}, sink)
}
