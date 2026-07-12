package warpp

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type EventType string

const (
	EventRunStarted    EventType = "run_started"
	EventRunCompleted  EventType = "run_completed"
	EventRunFailed     EventType = "run_failed"
	EventRunCancelled  EventType = "run_cancelled"
	EventNodeStarted   EventType = "node_started"
	EventNodeCompleted EventType = "node_completed"
	EventNodeFailed    EventType = "node_failed"
	EventNodeSkipped   EventType = "node_skipped"
	EventNodeRetrying  EventType = "node_retrying"
)

const (
	StatusRunning            = "running"
	StatusCompleted          = "completed"
	StatusCompletedWithSkips = "completed_with_skips"
	StatusFailed             = "failed"
	StatusCancelled          = "cancelled"
)

const retryBaseDelay = 200 * time.Millisecond

// Event is a single run-progress event emitted by the engine.
type Event struct {
	RunID      string         `json:"run_id"`
	Sequence   int64          `json:"sequence"`
	Type       EventType      `json:"type"`
	NodePath   string         `json:"node_path,omitempty"`
	Status     string         `json:"status,omitempty"`
	Message    string         `json:"message,omitempty"`
	Outputs    map[string]any `json:"outputs,omitempty"`
	Error      string         `json:"error,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
}

// StepFunc wraps a node execution for durable checkpointing.
type StepFunc func(ctx context.Context, key string, fn func(context.Context) (map[string]Value, error)) (map[string]Value, error)

// Engine executes workflow documents. It is stateless and safe to share.
type Engine struct {
	Resolve        Resolver
	Runners        map[string]NodeRunner
	Emit           func(Event)
	Step           StepFunc
	MaxConcurrency int
}

// Result is the terminal outcome of a run.
type Result struct {
	Status  string
	Outputs map[string]any
	Err     error
}

func (e *Engine) maxConc() int {
	if e.MaxConcurrency <= 0 {
		return 4
	}
	return e.MaxConcurrency
}

func (e *Engine) emit(ev Event) {
	if e.Emit == nil {
		return
	}
	ev.OccurredAt = time.Now().UTC()
	e.Emit(ev)
}

// execScope holds live per-run node outputs. Parent/child scopes share one
// mutex so cross-scope (lexical) reads during Map fan-out are race-free.
type execScope struct {
	parent   *execScope
	mu       *sync.RWMutex
	nodeSet  map[string]bool
	outputs  map[string]map[string]Value
	terminal map[string]bool
	skipped  map[string]bool
}

func (s *execScope) lookup(ref PortRef) (Value, bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for cur := s; cur != nil; cur = cur.parent {
		if !cur.nodeSet[ref.Node] {
			continue
		}
		if ports, ok := cur.outputs[ref.Node]; ok {
			v, fired := ports[ref.Port]
			return v, fired, true
		}
		return Value{}, false, true // known but not fired (skipped/pending)
	}
	return Value{}, false, false
}

func (s *execScope) record(nodeID string, outputs map[string]Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outputs[nodeID] = outputs
	s.terminal[nodeID] = true
}

func (s *execScope) markSkipped(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipped[nodeID] = true
	s.terminal[nodeID] = true
}

// Execute runs the document and returns its terminal result.
func (e *Engine) Execute(ctx context.Context, doc Document, input map[string]any) Result {
	e.emit(Event{Type: EventRunStarted, Status: StatusRunning, Message: "run started"})

	inValues, err := buildInputValues(doc.Inputs, input)
	if err != nil {
		e.emit(Event{Type: EventRunFailed, Status: StatusFailed, Error: err.Error(), Message: "run failed"})
		return Result{Status: StatusFailed, Err: err}
	}

	mu := &sync.RWMutex{}
	root := &execScope{
		mu:       mu,
		nodeSet:  map[string]bool{ReservedInputNode: true},
		outputs:  map[string]map[string]Value{ReservedInputNode: inValues},
		terminal: map[string]bool{ReservedInputNode: true},
		skipped:  map[string]bool{},
	}
	for _, n := range doc.Nodes {
		root.nodeSet[n.ID] = true
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	anySkipped, runErr := e.runScope(runCtx, doc.Nodes, root, "", doc.Settings.DefaultPolicy, e.maxConc())

	switch {
	case runErr != nil:
		e.emit(Event{Type: EventRunFailed, Status: StatusFailed, Error: runErr.Error(), Message: "run failed"})
		return Result{Status: StatusFailed, Err: runErr}
	case ctx.Err() != nil:
		e.emit(Event{Type: EventRunCancelled, Status: StatusCancelled, Error: ctx.Err().Error(), Message: "run cancelled"})
		return Result{Status: StatusCancelled, Err: ctx.Err()}
	}

	outputs := e.collectOutputs(doc.Outputs, root)
	status := StatusCompleted
	if anySkipped {
		status = StatusCompletedWithSkips
	}
	e.emit(Event{Type: EventRunCompleted, Status: status, Outputs: outputs, Message: "run completed"})
	return Result{Status: status, Outputs: outputs}
}

func buildInputValues(specs []PortSpec, input map[string]any) (map[string]Value, error) {
	out := map[string]Value{}
	for _, p := range specs {
		t, err := ParseType(p.Type)
		if err != nil {
			return nil, fmt.Errorf("input %q has invalid type: %w", p.Name, err)
		}
		if raw, ok := input[p.Name]; ok {
			v, err := CoerceRaw(raw, t)
			if err != nil {
				return nil, fmt.Errorf("input %q: %w", p.Name, err)
			}
			out[p.Name] = v
			continue
		}
		if p.Default != nil {
			v, err := CoerceRaw(p.Default, t)
			if err != nil {
				return nil, fmt.Errorf("input %q default: %w", p.Name, err)
			}
			out[p.Name] = v
			continue
		}
		if p.Required {
			return nil, fmt.Errorf("input %q required", p.Name)
		}
	}
	return out, nil
}

func (e *Engine) collectOutputs(outputs map[string]Binding, scope *execScope) map[string]any {
	if len(outputs) == 0 {
		return nil
	}
	out := map[string]any{}
	for name, b := range outputs {
		if b.HasValue {
			out[name] = b.Value
			continue
		}
		ref, err := ParseRef(b.From)
		if err != nil {
			continue
		}
		if v, fired, _ := scope.lookup(ref); fired {
			out[name] = v.Data
		}
	}
	return out
}

type nodeOutcome struct {
	nodeID     string
	outputs    map[string]Value
	skipped    bool
	err        error
	mapSkipped bool
}

// runScope schedules the nodes in one scope as a topological wavefront.
func (e *Engine) runScope(ctx context.Context, nodes []Node, scope *execScope, pathPrefix string, defaults Policy, maxConc int) (bool, error) {
	scopeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	siblings := siblingIDs(nodes)
	byID := map[string]*Node{}
	index := map[string]int{}
	remaining := map[string]int{}
	dependents := map[string][]string{}
	for i := range nodes {
		n := &nodes[i]
		byID[n.ID] = n
		index[n.ID] = i
		deps := nodeDeps(n, func(id string) bool { return siblings[id] })
		remaining[n.ID] = len(deps)
		for _, d := range deps {
			dependents[d] = append(dependents[d], n.ID)
		}
	}

	var ready []string
	for i := range nodes {
		if remaining[nodes[i].ID] == 0 {
			ready = append(ready, nodes[i].ID)
		}
	}
	sortByIndex(ready, index)

	resultCh := make(chan nodeOutcome, len(nodes))
	active := 0
	anySkipped := false
	var fatal error

	launch := func() {
		for active < maxConc && len(ready) > 0 {
			id := ready[0]
			ready = ready[1:]
			n := byID[id]
			path := pathPrefix + id
			active++
			go func() { resultCh <- e.executeNode(scopeCtx, n, scope, path, defaults) }()
		}
	}
	launch()

	for active > 0 {
		oc := <-resultCh
		active--
		n := byID[oc.nodeID]
		path := pathPrefix + oc.nodeID
		switch {
		case oc.skipped:
			scope.markSkipped(oc.nodeID)
			anySkipped = true
			e.emit(Event{Type: EventNodeSkipped, NodePath: path, Status: "skipped", Message: "node skipped"})
		case oc.err != nil:
			e.emit(Event{Type: EventNodeFailed, NodePath: path, Status: "failed", Error: oc.err.Error(), Message: "node failed"})
			if effectivePolicy(n, defaults).OnError == "skip" {
				scope.markSkipped(oc.nodeID)
				anySkipped = true
			} else if fatal == nil {
				fatal = oc.err
				cancel()
			}
		default:
			scope.record(oc.nodeID, oc.outputs)
			if oc.mapSkipped {
				anySkipped = true
			}
			e.emit(Event{Type: EventNodeCompleted, NodePath: path, Status: "completed", Outputs: valuesToAny(oc.outputs), Message: "node completed"})
		}

		if fatal == nil {
			for _, dep := range dependents[oc.nodeID] {
				remaining[dep]--
				if remaining[dep] == 0 {
					ready = append(ready, dep)
				}
			}
			sortByIndex(ready, index)
			launch()
		}
	}
	return anySkipped, fatal
}

func sortByIndex(ids []string, index map[string]int) {
	sort.SliceStable(ids, func(a, b int) bool { return index[ids[a]] < index[ids[b]] })
}

func valuesToAny(vals map[string]Value) map[string]any {
	out := make(map[string]any, len(vals))
	for k, v := range vals {
		out[k] = v.Data
	}
	return out
}

// executeNode resolves inputs, applies the skip rule, and runs the node with
// its effective policy. It runs on its own goroutine.
func (e *Engine) executeNode(ctx context.Context, node *Node, scope *execScope, path string, defaults Policy) nodeOutcome {
	m, ok := e.Resolve(node.Type)
	if !ok {
		return nodeOutcome{nodeID: node.ID, err: fmt.Errorf("unknown node type %q", node.Type)}
	}
	in, skip, err := e.resolveInputs(node, m, scope)
	if err != nil {
		return nodeOutcome{nodeID: node.ID, err: err}
	}
	if skip {
		return nodeOutcome{nodeID: node.ID, skipped: true}
	}
	e.emit(Event{Type: EventNodeStarted, NodePath: path, Status: "running", Message: "node started"})

	if node.Type == "control.map" {
		out, mapSkipped, mErr := e.runMapNode(ctx, node, in, scope, path, defaults)
		return nodeOutcome{nodeID: node.ID, outputs: out, err: mErr, mapSkipped: mapSkipped}
	}

	runner, ok := e.Runners[node.Type]
	if !ok {
		return nodeOutcome{nodeID: node.ID, err: fmt.Errorf("no runner for type %q", node.Type)}
	}
	pol := effectivePolicy(node, defaults)
	rc := RunnerCtx{Path: path, Node: node, Manifest: m}
	exec := func(c context.Context) (map[string]Value, error) {
		return e.runWithRetries(c, node, rc, runner, in, pol, path)
	}
	var out map[string]Value
	if e.Step != nil {
		out, err = e.Step(ctx, "node:"+path, exec)
	} else {
		out, err = exec(ctx)
	}
	return nodeOutcome{nodeID: node.ID, outputs: out, err: err}
}

func (e *Engine) runWithRetries(ctx context.Context, node *Node, rc RunnerCtx, runner NodeRunner, in NodeInputs, pol Policy, path string) (map[string]Value, error) {
	attempts := 1 + pol.Retries.Max
	if attempts < 1 {
		attempts = 1
	}
	var out map[string]Value
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		out, err = e.runOnce(ctx, rc, runner, in, pol)
		if err == nil {
			return out, nil
		}
		if attempt < attempts {
			e.emit(Event{Type: EventNodeRetrying, NodePath: path, Status: "retrying",
				Message: fmt.Sprintf("retry %d/%d", attempt, attempts-1), Error: err.Error()})
			if !sleepBackoff(ctx, pol, attempt) {
				return nil, ctx.Err()
			}
		}
	}
	return nil, err
}

func (e *Engine) runOnce(ctx context.Context, rc RunnerCtx, runner NodeRunner, in NodeInputs, pol Policy) (map[string]Value, error) {
	if d := parseDuration(pol.Timeout); d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	return runner(ctx, rc, in)
}

func sleepBackoff(ctx context.Context, pol Policy, attempt int) bool {
	delay := retryBaseDelay
	if pol.Retries.Backoff == "exponential" {
		delay = retryBaseDelay << (attempt - 1)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func effectivePolicy(node *Node, defaults Policy) Policy {
	p := Policy{}
	if node.Policy != nil {
		p = *node.Policy
	}
	if p.Timeout == "" {
		p.Timeout = defaults.Timeout
	}
	if p.OnError == "" {
		p.OnError = defaults.OnError
	}
	if p.OnError == "" {
		p.OnError = "fail"
	}
	if p.Retries.Max == 0 && defaults.Retries.Max > 0 {
		p.Retries.Max = defaults.Retries.Max
	}
	if p.Retries.Backoff == "" {
		p.Retries.Backoff = defaults.Retries.Backoff
	}
	return p
}

func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// resolveInputs gathers a node's inputs and applies the skip rule (spec §7).
func (e *Engine) resolveInputs(node *Node, m Manifest, scope *execScope) (NodeInputs, bool, error) {
	in := NodeInputs{
		Values: map[string]Value{},
		List:   map[string][]Value{},
		Named:  map[string]map[string]Value{},
	}
	for _, p := range m.Inputs {
		binding, provided := node.Inputs[p.Name]
		portType, isDynamic := parsePortType(p.Type)

		switch p.Variadic {
		case "list":
			fired := []Value{}
			for _, b := range collectBindingsOrdered(binding) {
				v, ok, err := e.resolveOne(b, portType, isDynamic, scope)
				if err != nil {
					return NodeInputs{}, false, err
				}
				if ok {
					fired = append(fired, v)
				}
			}
			if p.Required && len(fired) == 0 {
				return NodeInputs{}, true, nil
			}
			in.List[p.Name] = fired
		case "named":
			out := map[string]Value{}
			skip := false
			for key, b := range binding.Named {
				v, ok, err := e.resolveOne(b, portType, isDynamic, scope)
				if err != nil {
					return NodeInputs{}, false, err
				}
				if !ok {
					skip = true
					break
				}
				out[key] = v
			}
			if skip {
				return NodeInputs{}, true, nil
			}
			in.Named[p.Name] = out
		default:
			if !provided || binding.One == nil {
				if p.Default != nil {
					dv, err := defaultValue(p, portType, isDynamic)
					if err != nil {
						return NodeInputs{}, false, err
					}
					in.Values[p.Name] = dv
				}
				continue
			}
			v, ok, err := e.resolveOne(*binding.One, portType, isDynamic, scope)
			if err != nil {
				return NodeInputs{}, false, err
			}
			if !ok {
				if p.Default != nil {
					dv, err := defaultValue(p, portType, isDynamic)
					if err != nil {
						return NodeInputs{}, false, err
					}
					in.Values[p.Name] = dv
					continue
				}
				if p.Required {
					return NodeInputs{}, true, nil
				}
				continue
			}
			in.Values[p.Name] = v
		}
	}
	return in, false, nil
}

// resolveOne resolves a single binding. ok=false means the wired source did
// not fire (the node should skip if the port is required).
func (e *Engine) resolveOne(b Binding, portType Type, isDynamic bool, scope *execScope) (Value, bool, error) {
	if b.HasValue {
		return literalValue(b.Value, portType, isDynamic), true, nil
	}
	ref, err := ParseRef(b.From)
	if err != nil {
		return Value{}, false, fmt.Errorf("invalid ref %q", b.From)
	}
	v, fired, _ := scope.lookup(ref)
	if !fired {
		return Value{}, false, nil
	}
	if isDynamic || portType.HasVar() {
		return v, true, nil
	}
	coerced, err := Coerce(v, portType)
	if err != nil {
		return Value{}, false, err
	}
	return coerced, true, nil
}

func literalValue(data any, portType Type, isDynamic bool) Value {
	if isDynamic || portType.HasVar() {
		return Value{Type: InferLiteral(data), Data: data}
	}
	if v, err := CoerceRaw(data, portType); err == nil {
		return v
	}
	return Value{Type: InferLiteral(data), Data: data}
}

func defaultValue(p PortSpec, portType Type, isDynamic bool) (Value, error) {
	return literalValue(p.Default, portType, isDynamic), nil
}

func collectBindingsOrdered(in Input) []Binding {
	if in.List != nil {
		return in.List
	}
	if in.One != nil {
		return []Binding{*in.One}
	}
	return nil
}
