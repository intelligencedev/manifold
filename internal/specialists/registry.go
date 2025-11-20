package specialists

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/llm/anthropic"
	"manifold/internal/llm/google"
	openaillm "manifold/internal/llm/openai"
	"manifold/internal/tools"
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

	provider llm.Provider
	tools    tools.Registry
}

type chatWithOptionsProvider interface {
	ChatWithOptions(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error)
}

// Registry holds addressable specialists by name.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*Agent
}

// NewRegistry builds a registry from config.SpecialistConfig entries.
// The base OpenAI config is used as a default for API key/model unless
// overridden per specialist.
func NewRegistry(base config.LLMClientConfig, list []config.SpecialistConfig, httpClient *http.Client, toolsReg tools.Registry) *Registry {
	reg := &Registry{agents: make(map[string]*Agent, len(list))}
	reg.ReplaceFromConfigs(base, list, httpClient, toolsReg)
	return reg
}

func buildProvider(provider string, base config.LLMClientConfig, sc config.SpecialistConfig, httpClient *http.Client) (llm.Provider, string) {
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

	switch strings.ToLower(provider) {
	case "google":
		cfg := base.Google
		if strings.TrimSpace(sc.BaseURL) != "" {
			cfg.BaseURL = strings.TrimSpace(sc.BaseURL)
		}
		if strings.TrimSpace(sc.APIKey) != "" {
			cfg.APIKey = strings.TrimSpace(sc.APIKey)
		}
		if strings.TrimSpace(sc.Model) != "" {
			cfg.Model = strings.TrimSpace(sc.Model)
		}
		prov, err := google.New(cfg, hc)
		if err != nil {
			return nil, ""
		}
		return prov, cfg.Model
	case "anthropic":
		cfg := base.Anthropic
		if strings.TrimSpace(sc.BaseURL) != "" {
			cfg.BaseURL = strings.TrimSpace(sc.BaseURL)
		}
		if strings.TrimSpace(sc.APIKey) != "" {
			cfg.APIKey = strings.TrimSpace(sc.APIKey)
		}
		if strings.TrimSpace(sc.Model) != "" {
			cfg.Model = strings.TrimSpace(sc.Model)
		}
		prov := anthropic.New(cfg, hc)
		return prov, cfg.Model
	default:
		oc := base.OpenAI
		if strings.ToLower(provider) == "local" {
			oc.API = "completions"
		}
		if strings.TrimSpace(sc.API) != "" {
			oc.API = strings.TrimSpace(sc.API)
		}
		if strings.TrimSpace(sc.BaseURL) != "" {
			oc.BaseURL = strings.TrimSpace(sc.BaseURL)
		}
		if strings.TrimSpace(sc.APIKey) != "" {
			oc.APIKey = strings.TrimSpace(sc.APIKey)
		}
		if strings.TrimSpace(sc.Model) != "" {
			oc.Model = strings.TrimSpace(sc.Model)
		}
		if len(sc.ExtraParams) > 0 || strings.TrimSpace(sc.ReasoningEffort) != "" || len(oc.ExtraParams) > 0 {
			extra := map[string]any{}
			for k, v := range oc.ExtraParams {
				extra[k] = v
			}
			for k, v := range sc.ExtraParams {
				extra[k] = v
			}
			if re := strings.TrimSpace(sc.ReasoningEffort); re != "" {
				extra["reasoning_effort"] = re
			}
			oc.ExtraParams = extra
		}
		prov := openaillm.New(oc, hc)
		return prov, oc.Model
	}
}

// ReplaceFromConfigs rebuilds the registry from configs (skips paused specialists).
func (r *Registry) ReplaceFromConfigs(base config.LLMClientConfig, list []config.SpecialistConfig, httpClient *http.Client, toolsReg tools.Registry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	agents := make(map[string]*Agent, len(list))
	for _, sc := range list {
		if sc.Paused {
			continue
		}
		provName := strings.TrimSpace(sc.Provider)
		if provName == "" {
			provName = base.Provider
		}
		prov, model := buildProvider(provName, base, sc, httpClient)
		if prov == nil || model == "" {
			continue
		}
		var toolsView tools.Registry
		if sc.EnableTools && toolsReg != nil {
			toolsView = tools.NewFilteredRegistry(toolsReg, sc.AllowTools)
		} else {
			toolsView = nil
		}

		a := &Agent{
			Name:            sc.Name,
			System:          sc.System,
			Model:           model,
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
	msgs := a.buildMessages(history, user)

	// Extra fields for the request: start with configured extra params
	extra := a.mergedExtraParams()

	schemas := []llm.ToolSchema(nil)
	if a.EnableTools {
		if a.tools != nil {
			schemas = a.tools.Schemas()
		} else {
			schemas = []llm.ToolSchema{}
		}
	}
	callWithOptions := func(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema) (llm.Message, error) {
		if p, ok := a.provider.(chatWithOptionsProvider); ok {
			return p.ChatWithOptions(ctx, messages, tools, a.Model, extra)
		}
		return a.provider.Chat(ctx, messages, tools, a.Model)
	}

	if a.EnableTools && a.tools != nil {
		var sink ToolSink
		if v := ctx.Value(toolSinkKey{}); v != nil {
			if f, ok := v.(ToolSink); ok {
				sink = f
			}
		}
		msg, err := callWithOptions(ctx, msgs, schemas)
		if err != nil {
			return "", err
		}
		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}
		tc := msg.ToolCalls[0]
		dispatchCtx := tools.WithProvider(ctx, a.provider)
		payload, err := a.tools.Dispatch(dispatchCtx, tc.Name, tc.Args)
		if err != nil {
			payload = []byte("{" + strconv.Quote("error") + ":" + strconv.Quote(err.Error()) + "}")
		}
		if sink != nil {
			sink(tc.Name, payload, tc.Args)
		}
		return string(payload), nil
	}

	resp, err := callWithOptions(ctx, msgs, schemas)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Stream performs a best-effort streaming completion. Tool schemas are omitted
// to avoid multi-step tool execution loops during live streaming.
func (a *Agent) Stream(ctx context.Context, user string, history []llm.Message, handler llm.StreamHandler) error {
	if a.provider == nil {
		return errors.New("provider not configured")
	}
	msgs := a.buildMessages(history, user)
	// Streaming path intentionally skips tool schemas to avoid executing tools
	// mid-stream. This keeps the UX similar to a plain chat completion.
	return a.provider.ChatStream(ctx, msgs, nil, a.Model, handler)
}

func (a *Agent) buildMessages(history []llm.Message, user string) []llm.Message {
	msgs := make([]llm.Message, 0, len(history)+2)
	if sys := strings.TrimSpace(a.System); sys != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: sys})
	}
	msgs = append(msgs, history...)
	if strings.TrimSpace(user) != "" {
		msgs = append(msgs, llm.Message{Role: "user", Content: user})
	}
	return msgs
}

func (a *Agent) mergedExtraParams() map[string]any {
	if len(a.ExtraParams) == 0 && a.ReasoningEffort == "" {
		return nil
	}
	extra := make(map[string]any, len(a.ExtraParams)+1)
	for k, v := range a.ExtraParams {
		extra[k] = v
	}
	if a.ReasoningEffort != "" && extra["reasoning_effort"] == nil {
		extra["reasoning_effort"] = a.ReasoningEffort
	}
	return extra
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
