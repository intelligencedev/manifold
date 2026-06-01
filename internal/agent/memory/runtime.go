package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/llm"
	"manifold/internal/policy"
)

const (
	defaultRuntimeMemoryTokens = 2200
	defaultRuntimeTimeout      = 700 * time.Millisecond
)

// Runtime coordinates the distinct agent memory systems behind one runtime
// toggle. Evolving memory remains the lesson layer, belief memory remains the
// trusted-claims layer, and MAGMA remains the graph-recall layer.
type Runtime struct {
	Config RuntimeConfig

	Evolving       *EvolvingMemory
	Belief         belief.Retriever
	PolicyProvider policy.ContextProvider
	Magma          MagmaRetriever

	BeliefMaxBeliefs          int
	BeliefPromptTokenBudget   int
	BeliefMinConfidence       float64
	BeliefContradictions      bool
	MagmaIntentClassification string
	MagmaContextFormat        string
	MagmaMaxHops              int
	MagmaMaxNodes             int

	RecordEvolving func(ctx context.Context, record EpisodeRecord) (string, error)
	RecordBeliefs  func(ctx context.Context, record EpisodeRecord, evolvingEntryID string) error
}

type RuntimeConfig struct {
	Enabled            bool
	MaxTokensPerPrompt int
	Timeout            time.Duration
	IncludeRecent      bool
}

type RunSettings struct {
	Enabled bool
}

type MagmaRetriever interface {
	RetrieveMagmaContext(ctx context.Context, req MagmaRequest) (MagmaContext, error)
}

type MagmaRequest struct {
	Query                string
	Tenant               string
	MaxHops              int
	MaxNodes             int
	ContextFormat        string
	IntentClassification string
}

type MagmaContext struct {
	Text  string
	Items int
}

type Request struct {
	UserInput   string
	UserID      int64
	SessionID   string
	ProjectID   string
	ObjectiveID string
	Role        string
}

type ContextBlock struct {
	Text          string
	TokenEstimate int
	Truncated     bool
}

type Diagnostics struct {
	Enabled       bool
	DurationMs    int64
	TokenEstimate int
	Truncated     bool
	Lanes         map[string]LaneDiagnostics
}

type LaneDiagnostics struct {
	Enabled    bool
	Returned   bool
	TimedOut   bool
	Error      string
	DurationMs int64
	Items      int
	Tokens     int
}

type EpisodeRecord struct {
	StartedAt      time.Time
	UserInput      string
	Final          string
	RunErr         error
	ReasoningTrace []string
}

type runtimeLaneFunc func(context.Context) (string, int, error)

type runtimeLaneResult struct {
	name  string
	text  string
	items int
	diag  LaneDiagnostics
}

type runtimeLaneStart struct {
	ctx     context.Context
	timeout time.Duration
	wg      *sync.WaitGroup
	results chan<- runtimeLaneResult
	diag    *Diagnostics
	name    string
	enabled bool
	fn      runtimeLaneFunc
}

func (r *Runtime) Enabled() bool {
	return r != nil && r.Config.Enabled
}

func (r *Runtime) Clone() *Runtime {
	if r == nil {
		return nil
	}
	clone := *r
	return &clone
}

func (r *Runtime) PrepareContext(ctx context.Context, req Request) (ContextBlock, Diagnostics, error) {
	start := time.Now()
	diag := Diagnostics{Enabled: r.Enabled(), Lanes: map[string]LaneDiagnostics{}}
	if !r.Enabled() {
		return ContextBlock{}, diag, nil
	}

	sections := r.collectContextSections(ctx, req, r.laneTimeout(), &diag)
	block := buildRuntimeContext(orderedRuntimeSections(sections), maxRuntimeTokens(r.Config.MaxTokensPerPrompt))
	diag.DurationMs = time.Since(start).Milliseconds()
	diag.TokenEstimate = block.TokenEstimate
	diag.Truncated = block.Truncated
	return block, diag, nil
}

func (r *Runtime) collectContextSections(ctx context.Context, req Request, timeout time.Duration, diag *Diagnostics) map[string]string {
	results := make(chan runtimeLaneResult, 5)
	var wg sync.WaitGroup
	r.startRuntimeLane(runtimeLaneStart{ctx: ctx, timeout: timeout, wg: &wg, results: results, diag: diag, name: "policy", enabled: r.PolicyProvider != nil, fn: func(laneCtx context.Context) (string, int, error) {
		return r.retrievePolicyContext(laneCtx, req)
	}})
	r.startRuntimeLane(runtimeLaneStart{ctx: ctx, timeout: timeout, wg: &wg, results: results, diag: diag, name: "belief", enabled: r.Belief != nil, fn: func(laneCtx context.Context) (string, int, error) {
		return r.retrieveBeliefContext(laneCtx, req)
	}})
	r.startRuntimeLane(runtimeLaneStart{ctx: ctx, timeout: timeout, wg: &wg, results: results, diag: diag, name: "magma", enabled: r.Magma != nil, fn: func(laneCtx context.Context) (string, int, error) {
		return r.retrieveMagmaContext(laneCtx, req)
	}})
	r.startRuntimeLane(runtimeLaneStart{ctx: ctx, timeout: timeout, wg: &wg, results: results, diag: diag, name: "evolving", enabled: r.Evolving != nil, fn: func(laneCtx context.Context) (string, int, error) {
		return r.retrieveEvolvingContext(laneCtx, req)
	}})
	r.startRuntimeLane(runtimeLaneStart{ctx: ctx, timeout: timeout, wg: &wg, results: results, diag: diag, name: "recent", enabled: r.Evolving != nil && r.Config.IncludeRecent, fn: r.retrieveRecentContext})
	go func() {
		wg.Wait()
		close(results)
	}()
	return collectRuntimeSections(results, diag)
}

func (r *Runtime) startRuntimeLane(opts runtimeLaneStart) {
	if !opts.enabled {
		opts.diag.Lanes[opts.name] = LaneDiagnostics{Enabled: false}
		return
	}
	opts.wg.Go(func() {
		laneStart := time.Now()
		laneCtx, cancel := context.WithTimeout(opts.ctx, opts.timeout)
		defer cancel()
		text, items, err := opts.fn(laneCtx)
		ld := LaneDiagnostics{
			Enabled:    true,
			Returned:   strings.TrimSpace(text) != "",
			TimedOut:   laneCtx.Err() == context.DeadlineExceeded,
			DurationMs: time.Since(laneStart).Milliseconds(),
			Items:      items,
			Tokens:     llm.EstimateTokens(text),
		}
		if err != nil {
			ld.Error = err.Error()
		}
		opts.results <- runtimeLaneResult{name: opts.name, text: text, items: items, diag: ld}
	})
}

func collectRuntimeSections(results <-chan runtimeLaneResult, diag *Diagnostics) map[string]string {
	sections := map[string]string{}
	for res := range results {
		diag.Lanes[res.name] = res.diag
		if strings.TrimSpace(res.text) != "" {
			sections[res.name] = strings.TrimSpace(res.text)
		}
	}
	return sections
}

func orderedRuntimeSections(sections map[string]string) []string {
	return []string{
		sections["policy"],
		sections["belief"],
		sections["magma"],
		sections["evolving"],
		sections["recent"],
	}
}

func (r *Runtime) laneTimeout() time.Duration {
	if r.Config.Timeout > 0 {
		return r.Config.Timeout
	}
	return defaultRuntimeTimeout
}

func (r *Runtime) retrievePolicyContext(ctx context.Context, req Request) (string, int, error) {
	records, err := r.PolicyProvider.PromptContext(ctx, policy.EvaluationRequest{
		TenantID: req.UserID, UserID: req.UserID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, Role: req.Role,
	})
	if err != nil {
		return "", 0, err
	}
	return policy.BuildPromptSection(records), len(records), nil
}

func (r *Runtime) retrieveBeliefContext(ctx context.Context, req Request) (string, int, error) {
	results, err := r.Belief.Retrieve(ctx, belief.RetrievalRequest{
		TenantID:              req.UserID,
		UserID:                req.UserID,
		ProjectID:             req.ProjectID,
		ObjectiveID:           req.ObjectiveID,
		SessionID:             req.SessionID,
		Role:                  req.Role,
		Query:                 req.UserInput,
		Limit:                 firstPositive(r.BeliefMaxBeliefs, 5),
		MinConfidence:         r.BeliefMinConfidence,
		IncludeContradictions: r.BeliefContradictions,
	})
	if err != nil {
		return "", 0, err
	}
	block := belief.BuildPromptSection(results, belief.PromptOptions{
		MaxBeliefs: firstPositive(r.BeliefMaxBeliefs, 5),
		MaxTokens:  firstPositive(r.BeliefPromptTokenBudget, 700),
	})
	return block.Text, len(block.Selected), nil
}

func (r *Runtime) retrieveMagmaContext(ctx context.Context, req Request) (string, int, error) {
	magmaCtx, err := r.Magma.RetrieveMagmaContext(ctx, MagmaRequest{
		Query:                req.UserInput,
		Tenant:               fmt.Sprintf("user:%d", req.UserID),
		MaxHops:              r.MagmaMaxHops,
		MaxNodes:             r.MagmaMaxNodes,
		ContextFormat:        firstNonEmptyRuntime(r.MagmaContextFormat, "structured"),
		IntentClassification: firstNonEmptyRuntime(r.MagmaIntentClassification, "hybrid"),
	})
	if err != nil {
		return "", 0, err
	}
	text := strings.TrimSpace(magmaCtx.Text)
	if text == "" {
		return "", magmaCtx.Items, nil
	}
	return text, magmaCtx.Items, nil
}

func (r *Runtime) retrieveEvolvingContext(ctx context.Context, req Request) (string, int, error) {
	retrieved, _, err := r.Evolving.SearchWithDiagnostics(ctx, req.UserInput)
	if err != nil {
		return "", 0, err
	}
	return r.Evolving.SynthesizeScored(ctx, req.UserInput, retrieved), len(retrieved), nil
}

func (r *Runtime) retrieveRecentContext(context.Context) (string, int, error) {
	text := r.Evolving.BuildExpRecentContext()
	return text, len(r.Evolving.GetRecentWindow()), nil
}

func (r *Runtime) RecordEpisode(ctx context.Context, record EpisodeRecord) (string, error) {
	if !r.Enabled() {
		return "", nil
	}
	var entryID string
	if r.RecordEvolving != nil {
		id, err := r.RecordEvolving(ctx, record)
		if err != nil {
			return "", err
		}
		entryID = id
	}
	if r.RecordBeliefs != nil {
		if err := r.RecordBeliefs(ctx, record, entryID); err != nil {
			return entryID, err
		}
	}
	return entryID, nil
}

func buildRuntimeContext(sections []string, maxTokens int) ContextBlock {
	parts := make([]string, 0, len(sections))
	seen := map[string]bool{}
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" || seen[section] {
			continue
		}
		seen[section] = true
		parts = append(parts, section)
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if text == "" {
		return ContextBlock{}
	}
	tokenEstimate := llm.EstimateTokens(text)
	if maxTokens <= 0 || tokenEstimate <= maxTokens {
		return ContextBlock{Text: text, TokenEstimate: tokenEstimate}
	}
	maxChars := maxTokens * 4
	if maxChars < 256 {
		maxChars = 256
	}
	if len(text) > maxChars {
		text = strings.TrimSpace(text[:maxChars]) + "\n\n[Additional memory context omitted due to prompt budget.]"
	}
	return ContextBlock{Text: text, TokenEstimate: llm.EstimateTokens(text), Truncated: true}
}

func maxRuntimeTokens(value int) int {
	return firstPositive(value, defaultRuntimeMemoryTokens)
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonEmptyRuntime(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
