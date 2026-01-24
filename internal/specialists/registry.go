package specialists

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"manifold/internal/agent/prompts"
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
	Name                       string
	Description                string
	System                     string
	Model                      string
	SummaryContextWindowTokens int
	EnableTools                bool
	ReasoningEffort            string // optional: "low"|"medium"|"high"
	ExtraParams                map[string]any

	provider llm.Provider
	tools    tools.Registry
}

type chatWithOptionsProvider interface {
	ChatWithOptions(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error)
}

// Registry holds addressable specialists by name.
type Registry struct {
	mu                   sync.RWMutex
	agents               map[string]*Agent
	systemPromptAddendum string
	workdir              string
}

// NewRegistry builds a registry from config.SpecialistConfig entries.
// The base OpenAI config is used as a default for API key/model unless
// overridden per specialist.
func NewRegistry(base config.LLMClientConfig, list []config.SpecialistConfig, httpClient *http.Client, toolsReg tools.Registry) *Registry {
	reg := &Registry{agents: make(map[string]*Agent, len(list)), workdir: ""}
	reg.ReplaceFromConfigs(base, list, httpClient, toolsReg)
	return reg
}

// SetWorkdir sets the working directory used for composing default system prompts.
func (r *Registry) SetWorkdir(workdir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workdir = workdir
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
		if len(sc.ExtraParams) > 0 {
			cfg.ExtraParams = copyAnyMap(sc.ExtraParams)
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
		if len(sc.ExtraParams) > 0 {
			cfg.ExtraParams = copyAnyMap(sc.ExtraParams)
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
		extra := map[string]any{}
		if len(sc.ExtraParams) > 0 {
			extra = copyAnyMap(sc.ExtraParams)
		} else if len(oc.ExtraParams) > 0 {
			extra = copyAnyMap(oc.ExtraParams)
		}
		if re := strings.TrimSpace(sc.ReasoningEffort); re != "" {
			if extra == nil {
				extra = map[string]any{}
			}
			extra["reasoning_effort"] = re
		}
		if len(extra) > 0 {
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

		// Prepend default system prompt to specialist's configured system prompt
		// This ensures specialists get tool usage rules, memory instructions, etc.
		specialistSystem := sc.System
		if specialistSystem != "" {
			baseSystem := prompts.DefaultSystemPrompt(r.workdir, "")
			specialistSystem = combineSystemPrompts(baseSystem, specialistSystem)
		}

		a := &Agent{
			Name:                       sc.Name,
			Description:                strings.TrimSpace(sc.Description),
			System:                     specialistSystem,
			Model:                      model,
			SummaryContextWindowTokens: sc.SummaryContextWindowTokens,
			EnableTools:                sc.EnableTools,
			ReasoningEffort:            strings.TrimSpace(sc.ReasoningEffort),
			ExtraParams:                sc.ExtraParams,
			provider:                   prov,
			tools:                      toolsView,
		}
		if a.Name != "" {
			agents[a.Name] = a
		}
	}
	addendum := buildSystemPromptAddendum(agents)
	if addendum != "" {
		for _, a := range agents {
			a.System = combineSystemPrompts(a.System, addendum)
		}
	}
	r.agents = agents
	r.systemPromptAddendum = addendum
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

// AppendToSystemPrompt appends the registry's specialist catalog to the provided
// base system prompt, returning a combined prompt. If the registry has no
// specialists, base is returned unchanged (after trimming).
func (r *Registry) AppendToSystemPrompt(base string) string {
	r.mu.RLock()
	addition := r.systemPromptAddendum
	r.mu.RUnlock()
	return combineSystemPrompts(base, addition)
}

// Get returns the named specialist.
func (r *Registry) Get(name string) (*Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[name]
	return a, ok
}

// Provider exposes the underlying LLM provider for a specialist.
func (a *Agent) Provider() llm.Provider { return a.provider }

// ToolsRegistry returns the filtered tool registry view for this specialist, or nil when tools are disabled.
func (a *Agent) ToolsRegistry() tools.Registry { return a.tools }

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

func combineSystemPrompts(base, addition string) string {
	base = strings.TrimSpace(base)
	addition = strings.TrimSpace(addition)
	switch {
	case base == "":
		return addition
	case addition == "":
		return base
	default:
		return base + "\n\n" + addition
	}
}

func buildSystemPromptAddendum(agents map[string]*Agent) string {
	if len(agents) == 0 {
		return ""
	}
	list := make([]*Agent, 0, len(agents))
	for _, a := range agents {
		if a == nil || strings.TrimSpace(a.Name) == "" {
			continue
		}
		list = append(list, a)
	}
	if len(list) == 0 {
		return ""
	}
	sort.Slice(list, func(i, j int) bool { return strings.TrimSpace(list[i].Name) < strings.TrimSpace(list[j].Name) })
	lines := make([]string, 0, len(list))
	for _, a := range list {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(a.Description)
		if desc == "" {
			desc = "no description provided"
		}
		lines = append(lines, "- "+name+": "+desc)
	}
	if len(lines) == 0 {
		return ""
	}
	return "Available specialists you can invoke:\n" + strings.Join(lines, "\n")
}

// headerTransport injects static headers into every request.
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range t.headers {
		r.Header.Set(k, v)
	}
	return t.base.RoundTrip(r)
}

// Tool sink plumbing to surface specialist tool calls to UIs
type ToolSink func(name string, payload []byte, args json.RawMessage)
type toolSinkKey struct{}

func WithToolSink(ctx context.Context, sink ToolSink) context.Context {
	return context.WithValue(ctx, toolSinkKey{}, sink)
}
