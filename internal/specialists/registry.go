package specialists

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"manifold/internal/agent"
	"manifold/internal/agent/prompts"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/llm/anthropic"
	"manifold/internal/llm/google"
	openaillm "manifold/internal/llm/openai"
	"manifold/internal/tools"
	tooldiscovery "manifold/internal/tools/discovery"
	inputrequesttool "manifold/internal/tools/inputrequest"
)

// Agent represents a configured specialist bound to a specific endpoint/model.
// It is designed for inference-only requests (no tool schema unless enabled).
type Agent struct {
	Name                       string
	Description                string
	System                     string
	UserPromptContext          string
	Model                      string
	SummaryContextWindowTokens int
	EnableTools                bool
	RequestInfoEnabled         bool
	ImageGeneration            bool
	VideoGeneration            bool
	AutoDiscover               bool
	ReasoningEffort            string // optional: "low"|"medium"|"high"
	ExtraParams                map[string]any
	Harness                    *config.HarnessConfig
	MaxSteps                   int

	provider llm.Provider
	tools    tools.Registry
}

type chatWithOptionsProvider interface {
	ChatWithOptions(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error)
}

const (
	defaultImagePromptSize            = "1K"
	specialistInferenceToolCallPrefix = "specialist-call"
)

// Registry holds addressable specialists by name.
type Registry struct {
	mu                   sync.RWMutex
	agents               map[string]*Agent
	systemPromptAddendum string
	workdir              string
	base                 config.LLMClientConfig
	configs              []config.SpecialistConfig
	httpClient           *http.Client
	toolsReg             tools.Registry
	toolIndex            *tooldiscovery.ToolIndex
	autoDiscover         bool
	requestInfoEnabled   bool
	maxDiscovered        int
	promptOverrides      prompts.InstructionOverrides
	maxSteps             int
}

// NewRegistry builds a registry from config.SpecialistConfig entries.
// The base OpenAI config is used as a default for API key/model unless
// overridden per specialist.
func NewRegistry(base config.LLMClientConfig, list []config.SpecialistConfig, httpClient *http.Client, toolsReg tools.Registry) *Registry {
	return NewRegistryWithWorkdir(base, list, httpClient, toolsReg, "")

}

// NewRegistryWithWorkdir builds a registry with the provided workdir available
// during initial system prompt composition.
func NewRegistryWithWorkdir(base config.LLMClientConfig, list []config.SpecialistConfig, httpClient *http.Client, toolsReg tools.Registry, workdir string) *Registry {
	reg := &Registry{
		agents:             make(map[string]*Agent, len(list)),
		workdir:            workdir,
		base:               base,
		configs:            cloneSpecialistConfigs(list),
		httpClient:         httpClient,
		toolsReg:           toolsReg,
		requestInfoEnabled: true,
	}
	reg.rebuildLocked()
	return reg
}

// SetWorkdir sets the working directory used for composing default system prompts.
func (r *Registry) SetWorkdir(workdir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.workdir == workdir {
		return
	}
	r.workdir = workdir
	r.rebuildLocked()
}

func (r *Registry) SetToolDiscovery(index *tooldiscovery.ToolIndex, autoDiscover bool, maxDiscovered int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolIndex = index
	r.autoDiscover = autoDiscover
	r.maxDiscovered = maxDiscovered
	r.rebuildLocked()
}

func (r *Registry) SetPromptOverrides(overrides prompts.InstructionOverrides) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.promptOverrides = overrides
	r.rebuildLocked()
}

func (r *Registry) SetRequestInfoEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requestInfoEnabled = enabled
	r.rebuildLocked()
}

func (r *Registry) SetMaxSteps(maxSteps int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxSteps = maxSteps
	r.rebuildLocked()
}

func buildProvider(provider string, base config.LLMClientConfig, sc config.SpecialistConfig, httpClient *http.Client) (llm.Provider, string) {
	hc := httpClient
	if hc == nil {
		hc = http.DefaultClient
	}
	tr := hc.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	tr = &specialistTokenizeBypassTransport{base: tr}
	if len(sc.ExtraHeaders) > 0 {
		tr = &headerTransport{base: tr, headers: sc.ExtraHeaders}
	}
	hc = &http.Client{Transport: tr}

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
		isLocalProvider := strings.EqualFold(provider, "local")
		if isLocalProvider {
			oc.API = "completions"
		}
		if !isLocalProvider && strings.TrimSpace(sc.API) != "" {
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
		} else if !sc.ImageGeneration && !sc.VideoGeneration && len(oc.ExtraParams) > 0 {
			extra = copyAnyMap(oc.ExtraParams)
		}
		if re := strings.TrimSpace(sc.ReasoningEffort); !sc.ImageGeneration && !sc.VideoGeneration && re != "" {
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
	r.base = base
	r.configs = cloneSpecialistConfigs(list)
	r.httpClient = httpClient
	r.toolsReg = toolsReg
	r.rebuildLocked()
}

func (r *Registry) rebuildLocked() {
	agents := make(map[string]*Agent, len(r.configs))
	for _, sc := range r.configs {
		if sc.Paused {
			continue
		}
		provName := strings.TrimSpace(sc.Provider)
		if provName == "" {
			provName = r.base.Provider
		}
		prov, model := buildProvider(provName, r.base, sc, r.httpClient)
		if prov == nil || model == "" {
			continue
		}
		resolvedAutoDiscover := r.autoDiscover
		if sc.AutoDiscover != nil {
			resolvedAutoDiscover = *sc.AutoDiscover
		}
		resolvedRequestInfo := r.requestInfoEnabled
		if sc.RequestInfoEnabled != nil {
			resolvedRequestInfo = *sc.RequestInfoEnabled
		}
		var toolsView tools.Registry
		if sc.EnableTools && r.toolsReg != nil {
			if resolvedAutoDiscover && r.toolIndex != nil {
				toolsView = tooldiscovery.NewDiscoverableRegistry(r.toolsReg, r.toolIndex, sc.AllowTools, r.maxDiscovered)
			} else {
				toolsView = tools.NewFilteredRegistry(r.toolsReg, sc.AllowTools)
			}
			if resolvedRequestInfo {
				toolsView = tools.NewOverlayRegistry(toolsView, inputrequesttool.New())
			}
		} else {
			toolsView = nil
		}

		// Prefix shared base prompt before specialist-specific instructions.
		specialistSystem := prompts.DefaultSystemPrompt(r.workdir, sc.System, r.promptOverrides)
		if sc.EnableTools && resolvedAutoDiscover {
			specialistSystem = prompts.EnsureToolDiscoveryInstructions(specialistSystem, r.promptOverrides)
		}
		if sc.EnableTools && resolvedRequestInfo {
			specialistSystem = prompts.EnsureRequestInfoInstructions(specialistSystem)
		}

		a := &Agent{
			Name:                       sc.Name,
			Description:                strings.TrimSpace(sc.Description),
			System:                     specialistSystem,
			Model:                      model,
			SummaryContextWindowTokens: sc.SummaryContextWindowTokens,
			EnableTools:                sc.EnableTools,
			RequestInfoEnabled:         resolvedRequestInfo,
			ImageGeneration:            sc.ImageGeneration,
			VideoGeneration:            sc.VideoGeneration,
			AutoDiscover:               resolvedAutoDiscover,
			ReasoningEffort:            strings.TrimSpace(sc.ReasoningEffort),
			ExtraParams:                sc.ExtraParams,
			Harness:                    cloneHarnessConfig(sc.Harness),
			MaxSteps:                   r.maxSteps,
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
			a.UserPromptContext = addendum
		}
	}
	r.agents = agents
	r.systemPromptAddendum = addendum
}

func cloneSpecialistConfigs(list []config.SpecialistConfig) []config.SpecialistConfig {
	if len(list) == 0 {
		return nil
	}
	out := make([]config.SpecialistConfig, 0, len(list))
	for _, sc := range list {
		clone := sc
		if len(sc.AllowTools) > 0 {
			clone.AllowTools = append([]string(nil), sc.AllowTools...)
		}
		if sc.AutoDiscover != nil {
			value := *sc.AutoDiscover
			clone.AutoDiscover = &value
		}
		if sc.RequestInfoEnabled != nil {
			value := *sc.RequestInfoEnabled
			clone.RequestInfoEnabled = &value
		}
		clone.Harness = cloneHarnessConfig(sc.Harness)
		clone.ExtraHeaders = copyStringMap(sc.ExtraHeaders)
		clone.ExtraParams = copyAnyMap(sc.ExtraParams)
		out = append(out, clone)
	}
	return out
}

func cloneHarnessConfig(in *config.HarnessConfig) *config.HarnessConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.TerminalTools = append([]string(nil), in.TerminalTools...)
	out.RequiredSteps = append([]string(nil), in.RequiredSteps...)
	if len(in.ToolPrerequisites) > 0 {
		out.ToolPrerequisites = make(map[string][]config.HarnessPrerequisite, len(in.ToolPrerequisites))
		for tool, prereqs := range in.ToolPrerequisites {
			out.ToolPrerequisites[tool] = append([]config.HarnessPrerequisite(nil), prereqs...)
		}
	} else {
		out.ToolPrerequisites = nil
	}
	out.Compact.PhaseThresholds = append([]float64(nil), in.Compact.PhaseThresholds...)
	return &out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
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

// UserPromptContext returns the registry's specialist catalog as dynamic
// context suitable for prepending to the current user prompt.
func (r *Registry) UserPromptContext() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.systemPromptAddendum
}

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

// Inference performs a completion with optional history.
// If tools are disabled, no tool schema is sent at all.
// If ReasoningEffort is set, a provider-specific reasoning block is attached.
func (a *Agent) Inference(ctx context.Context, user string, history []llm.Message) (string, error) {
	if a.provider == nil {
		return "", errors.New("provider not configured")
	}
	if a.ImageGeneration {
		msg, err := a.provider.Chat(llm.WithImagePrompt(ctx, llm.ImagePromptOptions{Size: defaultImagePromptSize}), a.buildMessages(nil, user), nil, a.Model)
		if err != nil {
			return "", err
		}
		return msg.Content, nil
	}
	if a.VideoGeneration {
		msg, err := a.provider.Chat(llm.WithVideoPrompt(ctx, llm.VideoPromptOptions{}), a.buildMessages(nil, user), nil, a.Model)
		if err != nil {
			return "", err
		}
		return msg.Content, nil
	}
	msgs := a.buildMessages(history, user)

	// Extra fields for the request: start with configured extra params
	extra := a.mergedExtraParams()

	callWithOptions := func(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema) (llm.Message, error) {
		if p, ok := a.provider.(chatWithOptionsProvider); ok {
			return p.ChatWithOptions(ctx, messages, tools, a.Model, extra)
		}
		return a.provider.Chat(ctx, messages, tools, a.Model)
	}

	if a.EnableTools && a.tools != nil {
		for step := 0; a.inferenceStepAllowed(step); step++ {
			msg, err := callWithOptions(ctx, msgs, a.toolSchemas())
			if err != nil {
				return "", err
			}
			msg.ToolCalls = ensureSpecialistToolCallIDs(msgs, llm.NormalizeToolCalls(msg.ToolCalls), step)
			if len(msg.ToolCalls) == 0 {
				return msg.Content, nil
			}
			msgs = append(msgs, msg)
			dispatchCtx := tools.WithProvider(ctx, a.provider)
			for _, tc := range msg.ToolCalls {
				payload, err := a.tools.Dispatch(dispatchCtx, tc.Name, tc.Args)
				if err != nil {
					payload = []byte("{" + strconv.Quote("error") + ":" + strconv.Quote(err.Error()) + "}")
				}
				msgs = append(msgs, llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID})
			}
		}
		return "", agent.MaxStepsExceededError{MaxSteps: a.MaxSteps}
	}

	resp, err := callWithOptions(ctx, msgs, a.toolSchemas())
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (a *Agent) inferenceStepAllowed(step int) bool {
	return a.MaxSteps <= 0 || step < a.MaxSteps
}

func (a *Agent) toolSchemas() []llm.ToolSchema {
	if !a.EnableTools {
		return nil
	}
	if a.tools == nil {
		return []llm.ToolSchema{}
	}
	return a.tools.Schemas()
}

func ensureSpecialistToolCallIDs(msgs []llm.Message, toolCalls []llm.ToolCall, step int) []llm.ToolCall {
	if len(toolCalls) == 0 {
		return toolCalls
	}
	used := make(map[string]struct{}, len(toolCalls))
	for _, msg := range msgs {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if id := strings.TrimSpace(tc.ID); id != "" {
				used[id] = struct{}{}
			}
		}
	}
	for i := range toolCalls {
		id := strings.TrimSpace(toolCalls[i].ID)
		if id == "" {
			id = specialistToolCallID(step, i, 0)
		}
		if _, exists := used[id]; exists {
			for suffix := 1; ; suffix++ {
				candidate := specialistToolCallID(step, i, suffix)
				if _, usedCandidate := used[candidate]; !usedCandidate {
					id = candidate
					break
				}
			}
		}
		toolCalls[i].ID = id
		used[id] = struct{}{}
	}
	return toolCalls
}

func specialistToolCallID(step, index, suffix int) string {
	id := specialistInferenceToolCallPrefix + "-" + strconv.Itoa(step) + "-" + strconv.Itoa(index+1)
	if suffix > 0 {
		id += "-" + strconv.Itoa(suffix)
	}
	return id
}

// Stream performs a best-effort streaming completion. Tool schemas are omitted
// to avoid multi-step tool execution loops during live streaming.
func (a *Agent) Stream(ctx context.Context, user string, history []llm.Message, handler llm.StreamHandler) error {
	if a.provider == nil {
		return errors.New("provider not configured")
	}
	if a.ImageGeneration {
		return a.provider.ChatStream(llm.WithImagePrompt(ctx, llm.ImagePromptOptions{Size: defaultImagePromptSize}), a.buildMessages(nil, user), nil, a.Model, handler)
	}
	if a.VideoGeneration {
		return a.provider.ChatStream(llm.WithVideoPrompt(ctx, llm.VideoPromptOptions{}), a.buildMessages(nil, user), nil, a.Model, handler)
	}
	msgs := a.buildMessages(history, user)
	// Streaming path intentionally skips tool schemas to avoid executing tools
	// mid-stream. This keeps the UX similar to a plain chat completion.
	return a.provider.ChatStream(ctx, msgs, nil, a.Model, handler)
}

func (a *Agent) buildMessages(history []llm.Message, user string) []llm.Message {
	if a.ImageGeneration || a.VideoGeneration {
		if strings.TrimSpace(user) == "" {
			return nil
		}
		return []llm.Message{{Role: "user", Content: user}}
	}
	msgs := make([]llm.Message, 0, len(history)+2)
	if sys := strings.TrimSpace(a.System); sys != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: sys})
	}
	msgs = append(msgs, history...)
	if strings.TrimSpace(user) != "" {
		msgs = append(msgs, llm.Message{Role: "user", Content: user})
	}
	msgs = prependToCurrentUserMessage(msgs, a.UserPromptContext)
	return msgs
}

func prependToCurrentUserMessage(msgs []llm.Message, section string) []llm.Message {
	section = strings.TrimSpace(section)
	if section == "" {
		return msgs
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		content := strings.TrimSpace(msgs[i].Content)
		if content == "" {
			msgs[i].Content = section
		} else {
			msgs[i].Content = section + "\n\n" + msgs[i].Content
		}
		return msgs
	}
	return append(msgs, llm.Message{Role: "user", Content: section})
}

func (a *Agent) mergedExtraParams() map[string]any {
	if len(a.ExtraParams) == 0 && a.ReasoningEffort == "" {
		return nil
	}
	extra := make(map[string]any, len(a.ExtraParams)+1)
	maps.Copy(extra, a.ExtraParams)
	if a.ReasoningEffort != "" && extra["reasoning_effort"] == nil {
		extra["reasoning_effort"] = a.ReasoningEffort
	}
	return extra
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

type specialistTokenizeBypassTransport struct {
	base http.RoundTripper
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range t.headers {
		r.Header.Set(k, v)
	}
	return t.base.RoundTrip(r)
}

func (t *specialistTokenizeBypassTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req != nil && req.Method == http.MethodPost && strings.HasSuffix(strings.TrimSpace(req.URL.Path), "/tokenize") {
		body := `{"tokens":[]}`
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)),
			Request:       req,
		}, nil
	}
	return t.base.RoundTrip(req)
}
