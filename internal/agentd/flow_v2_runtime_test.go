package agentd

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"manifold/internal/flow"
	"manifold/internal/llm"
	"manifold/internal/tools"
)

type runtimeTestTool struct {
	name   string
	callFn func(ctx context.Context, raw json.RawMessage) (any, error)
}

func (t runtimeTestTool) Name() string { return t.name }

func (t runtimeTestTool) JSONSchema() map[string]any {
	return map[string]any{"type": "object"}
}

func (t runtimeTestTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	if t.callFn == nil {
		return map[string]any{"ok": true}, nil
	}
	return t.callFn(ctx, raw)
}

type runtimeStubRegistry struct {
	mu      sync.Mutex
	tools   map[string]runtimeTestTool
	ordered []string
}

func newRuntimeStubRegistry(ts ...runtimeTestTool) *runtimeStubRegistry {
	r := &runtimeStubRegistry{tools: make(map[string]runtimeTestTool, len(ts))}
	for _, tool := range ts {
		r.Register(tool)
	}
	return r
}

func (r *runtimeStubRegistry) Schemas() []llm.ToolSchema {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]llm.ToolSchema, 0, len(r.ordered))
	for _, name := range r.ordered {
		out = append(out, llm.ToolSchema{Name: name})
	}
	return out
}

func (r *runtimeStubRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
	r.mu.Lock()
	tool, ok := r.tools[name]
	r.mu.Unlock()
	if !ok {
		return nil, context.Canceled
	}
	result, err := tool.Call(ctx, raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (r *runtimeStubRegistry) Register(t tools.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt := runtimeTestTool{name: t.Name(), callFn: t.Call}
	if _, exists := r.tools[rt.name]; !exists {
		r.ordered = append(r.ordered, rt.name)
	}
	r.tools[rt.name] = rt
	sort.Strings(r.ordered)
}

func (r *runtimeStubRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
	out := r.ordered[:0]
	for _, existing := range r.ordered {
		if existing != name {
			out = append(out, existing)
		}
	}
	r.ordered = out
}

func TestExecuteFlowV2RunParallelFanOutAndConverge(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	started := map[string]time.Time{}
	finished := map[string]time.Time{}

	reg := newRuntimeStubRegistry(
		runtimeTestTool{name: "root", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			mu.Lock()
			started["root"] = time.Now()
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			finished["root"] = time.Now()
			mu.Unlock()
			return map[string]any{"branch": "ready"}, nil
		}},
		runtimeTestTool{name: "branch_b", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			mu.Lock()
			started["branch_b"] = time.Now()
			mu.Unlock()
			time.Sleep(120 * time.Millisecond)
			mu.Lock()
			finished["branch_b"] = time.Now()
			mu.Unlock()
			return map[string]any{"branch": "b"}, nil
		}},
		runtimeTestTool{name: "branch_c", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			mu.Lock()
			started["branch_c"] = time.Now()
			mu.Unlock()
			time.Sleep(120 * time.Millisecond)
			mu.Lock()
			finished["branch_c"] = time.Now()
			mu.Unlock()
			return map[string]any{"branch": "c"}, nil
		}},
		runtimeTestTool{name: "join", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			mu.Lock()
			started["join"] = time.Now()
			mu.Unlock()
			return map[string]any{"joined": true}, nil
		}},
	)

	a := &app{flowV2: newFlowV2Runtime(nil), baseToolRegistry: reg, toolRegistry: reg}
	wf := flow.Workflow{
		ID:       "wf_parallel",
		Name:     "Parallel",
		Trigger:  flow.Trigger{Type: flow.TriggerTypeManual},
		Settings: flow.WorkflowSettings{MaxConcurrency: 4},
		Nodes: []flow.Node{
			{ID: "root", Name: "Root", Kind: flow.NodeKindAction, Type: "tool", Tool: "root"},
			{ID: "branch_b", Name: "Branch B", Kind: flow.NodeKindAction, Type: "tool", Tool: "branch_b"},
			{ID: "branch_c", Name: "Branch C", Kind: flow.NodeKindAction, Type: "tool", Tool: "branch_c"},
			{ID: "join", Name: "Join", Kind: flow.NodeKindAction, Type: "tool", Tool: "join"},
		},
		Edges: []flow.Edge{
			{Source: flow.PortRef{NodeID: "root", Port: "result"}, Target: flow.PortRef{NodeID: "branch_b", Port: "input"}},
			{Source: flow.PortRef{NodeID: "root", Port: "result"}, Target: flow.PortRef{NodeID: "branch_c", Port: "input"}},
			{Source: flow.PortRef{NodeID: "branch_b", Port: "result"}, Target: flow.PortRef{NodeID: "join", Port: "input_b"}},
			{Source: flow.PortRef{NodeID: "branch_c", Port: "result"}, Target: flow.PortRef{NodeID: "join", Port: "input_c"}},
		},
	}
	plan, diags := flow.CompileWorkflow(wf)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}

	runID := a.flowV2.createRun(0, wf.ID, nil)
	a.executeFlowV2Run(context.Background(), 0, runID, wf, plan, nil)

	events, status, ok := a.flowV2.getRunEvents(0, runID)
	if !ok {
		t.Fatal("expected run events")
	}
	if status != "completed" {
		t.Fatalf("expected completed status, got %s with events=%+v", status, events)
	}

	mu.Lock()
	defer mu.Unlock()
	branchDelta := started["branch_b"].Sub(started["branch_c"])
	if branchDelta < 0 {
		branchDelta = -branchDelta
	}
	if branchDelta > 70*time.Millisecond {
		t.Fatalf("expected sibling branches to start in parallel, delta=%s", branchDelta)
	}
	latestBranchFinish := finished["branch_b"]
	if finished["branch_c"].After(latestBranchFinish) {
		latestBranchFinish = finished["branch_c"]
	}
	if started["join"].Before(latestBranchFinish) {
		t.Fatalf("expected join to wait for both branches, join=%s latest=%s", started["join"], latestBranchFinish)
	}

	assertMonotonicSequences(t, events)
}

func TestExecuteFlowV2RunMaxConcurrencyOneIsSerial(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	started := map[string]time.Time{}
	finished := map[string]time.Time{}

	reg := newRuntimeStubRegistry(
		runtimeTestTool{name: "root", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		}},
		runtimeTestTool{name: "left", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			mu.Lock()
			started["left"] = time.Now()
			mu.Unlock()
			time.Sleep(80 * time.Millisecond)
			mu.Lock()
			finished["left"] = time.Now()
			mu.Unlock()
			return map[string]any{"ok": true}, nil
		}},
		runtimeTestTool{name: "right", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			mu.Lock()
			started["right"] = time.Now()
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			finished["right"] = time.Now()
			mu.Unlock()
			return map[string]any{"ok": true}, nil
		}},
	)

	a := &app{flowV2: newFlowV2Runtime(nil), baseToolRegistry: reg, toolRegistry: reg}
	wf := flow.Workflow{
		ID:       "wf_serial",
		Name:     "Serial",
		Trigger:  flow.Trigger{Type: flow.TriggerTypeManual},
		Settings: flow.WorkflowSettings{MaxConcurrency: 1},
		Nodes: []flow.Node{
			{ID: "root", Name: "Root", Kind: flow.NodeKindAction, Type: "tool", Tool: "root"},
			{ID: "left", Name: "Left", Kind: flow.NodeKindAction, Type: "tool", Tool: "left"},
			{ID: "right", Name: "Right", Kind: flow.NodeKindAction, Type: "tool", Tool: "right"},
		},
		Edges: []flow.Edge{
			{Source: flow.PortRef{NodeID: "root", Port: "result"}, Target: flow.PortRef{NodeID: "left", Port: "input"}},
			{Source: flow.PortRef{NodeID: "root", Port: "result"}, Target: flow.PortRef{NodeID: "right", Port: "input"}},
		},
	}
	plan, _ := flow.CompileWorkflow(wf)
	runID := a.flowV2.createRun(0, wf.ID, nil)
	a.executeFlowV2Run(context.Background(), 0, runID, wf, plan, nil)

	mu.Lock()
	defer mu.Unlock()
	if started["right"].Before(finished["left"]) && started["left"].Before(finished["right"]) {
		t.Fatalf("expected serial execution at max_concurrency=1, got overlap left=%s/%s right=%s/%s", started["left"], finished["left"], started["right"], finished["right"])
	}
}

func TestExecuteFlowV2RunFailFastCancelsSiblings(t *testing.T) {
	t.Parallel()

	cancelObserved := make(chan struct{}, 1)
	reg := newRuntimeStubRegistry(
		runtimeTestTool{name: "root", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		}},
		runtimeTestTool{name: "boom", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return nil, context.DeadlineExceeded
		}},
		runtimeTestTool{name: "slow", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
			select {
			case <-ctx.Done():
				cancelObserved <- struct{}{}
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
				return map[string]any{"ok": true}, nil
			}
		}},
	)

	a := &app{flowV2: newFlowV2Runtime(nil), baseToolRegistry: reg, toolRegistry: reg}
	wf := flow.Workflow{
		ID:       "wf_failfast",
		Name:     "Fail Fast",
		Trigger:  flow.Trigger{Type: flow.TriggerTypeManual},
		Settings: flow.WorkflowSettings{MaxConcurrency: 2},
		Nodes: []flow.Node{
			{ID: "root", Name: "Root", Kind: flow.NodeKindAction, Type: "tool", Tool: "root"},
			{ID: "boom", Name: "Boom", Kind: flow.NodeKindAction, Type: "tool", Tool: "boom", Execution: flow.NodeExecution{OnError: flow.ErrorStrategyFail}},
			{ID: "slow", Name: "Slow", Kind: flow.NodeKindAction, Type: "tool", Tool: "slow", Execution: flow.NodeExecution{OnError: flow.ErrorStrategyFail}},
		},
		Edges: []flow.Edge{
			{Source: flow.PortRef{NodeID: "root", Port: "result"}, Target: flow.PortRef{NodeID: "boom", Port: "input"}},
			{Source: flow.PortRef{NodeID: "root", Port: "result"}, Target: flow.PortRef{NodeID: "slow", Port: "input"}},
		},
	}
	plan, _ := flow.CompileWorkflow(wf)
	runID := a.flowV2.createRun(0, wf.ID, nil)
	a.executeFlowV2Run(context.Background(), 0, runID, wf, plan, nil)

	select {
	case <-cancelObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("expected sibling branch cancellation to be observed")
	}

	events, status, ok := a.flowV2.getRunEvents(0, runID)
	if !ok {
		t.Fatal("expected run events")
	}
	if status != "failed" {
		t.Fatalf("expected failed status, got %s", status)
	}
	if !hasRunEvent(events, flow.RunEventTypeRunFailed) {
		t.Fatalf("expected run_failed event, got %+v", events)
	}
	assertMonotonicSequences(t, events)
}

func TestExecuteFlowV2RunGuardSkip(t *testing.T) {
	t.Parallel()

	reg := newRuntimeStubRegistry(runtimeTestTool{name: "guarded", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	}})
	a := &app{flowV2: newFlowV2Runtime(nil), baseToolRegistry: reg, toolRegistry: reg}
	wf := flow.Workflow{
		ID:      "wf_guard",
		Name:    "Guard",
		Trigger: flow.Trigger{Type: flow.TriggerTypeManual},
		Nodes: []flow.Node{{
			ID:    "guarded",
			Name:  "Guarded",
			Kind:  flow.NodeKindAction,
			Type:  "tool",
			Tool:  "guarded",
			Guard: "false",
		}},
	}
	plan, _ := flow.CompileWorkflow(wf)
	runID := a.flowV2.createRun(0, wf.ID, nil)
	a.executeFlowV2Run(context.Background(), 0, runID, wf, plan, nil)

	events, status, ok := a.flowV2.getRunEvents(0, runID)
	if !ok {
		t.Fatal("expected run events")
	}
	if status != "completed" {
		t.Fatalf("expected completed status, got %s", status)
	}
	if !hasRunEvent(events, flow.RunEventTypeNodeSkipped) {
		t.Fatalf("expected node_skipped event, got %+v", events)
	}
	if hasRunEvent(events, flow.RunEventTypeNodeStarted) {
		t.Fatalf("did not expect node_started for skipped node, got %+v", events)
	}
}

func TestExecuteFlowV2RunUnknownPlannedNodeFailsWithoutHang(t *testing.T) {
	t.Parallel()

	reg := newRuntimeStubRegistry(runtimeTestTool{name: "root", callFn: func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	}})
	a := &app{flowV2: newFlowV2Runtime(nil), baseToolRegistry: reg, toolRegistry: reg}
	wf := flow.Workflow{
		ID:      "wf_unknown_node",
		Name:    "Unknown Node",
		Trigger: flow.Trigger{Type: flow.TriggerTypeManual},
		Nodes: []flow.Node{{
			ID:   "root",
			Name: "Root",
			Kind: flow.NodeKindAction,
			Type: "tool",
			Tool: "root",
		}},
	}
	edgeToMissing := flow.Edge{
		Source: flow.PortRef{NodeID: "root", Port: "output"},
		Target: flow.PortRef{NodeID: "missing", Port: "input"},
	}
	plan := &flow.Plan{
		WorkflowID: wf.ID,
		NodeOrder:  []string{"root", "missing"},
		Incoming: map[string][]flow.Edge{
			"root":    {},
			"missing": {edgeToMissing},
		},
		Outgoing: map[string][]flow.Edge{
			"root":    {edgeToMissing},
			"missing": {},
		},
		Indegree: map[string]int{
			"root":    0,
			"missing": 1,
		},
	}

	runID := a.flowV2.createRun(0, wf.ID, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.executeFlowV2Run(context.Background(), 0, runID, wf, plan, nil)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("executeFlowV2Run hung on unknown planned node")
	}

	events, status, ok := a.flowV2.getRunEvents(0, runID)
	if !ok {
		t.Fatal("expected run events")
	}
	if status != "failed" {
		t.Fatalf("expected failed status, got %s with events=%+v", status, events)
	}
	if !hasRunEvent(events, flow.RunEventTypeRunFailed) {
		t.Fatalf("expected run_failed event, got %+v", events)
	}
	assertMonotonicSequences(t, events)
	if got := events[len(events)-1].Error; got != "execution plan referenced unknown node missing" {
		t.Fatalf("expected unknown node error, got %q", got)
	}
	completedIndex := -1
	failedIndex := -1
	for idx, event := range events {
		switch event.Type {
		case flow.RunEventTypeNodeCompleted:
			if event.NodeID == "root" && completedIndex < 0 {
				completedIndex = idx
			}
		case flow.RunEventTypeRunFailed:
			if failedIndex < 0 {
				failedIndex = idx
			}
		}
	}
	if completedIndex < 0 || failedIndex < 0 || completedIndex >= failedIndex {
		t.Fatalf("expected root node to complete before run failure, got %+v", events)
	}
}

func TestEvalFlowExpressionSingleLineMultiExpressionReturnsError(t *testing.T) {
	t.Parallel()

	_, err := evalFlowExpression(
		"={{ $run.input.first }} ={{ $run.input.second }}",
		map[string]any{"first": "alpha", "second": "beta"},
		nil,
	)
	if err == nil {
		t.Fatal("expected unsupported single-line multi-expression error")
	}
	if !strings.Contains(err.Error(), "$run.input.first") {
		t.Fatalf("expected single-line multi-expression to fail fast, got %v", err)
	}
}

func assertMonotonicSequences(t *testing.T, events []flow.RunEvent) {
	t.Helper()
	for idx := 1; idx < len(events); idx++ {
		if events[idx].Sequence <= events[idx-1].Sequence {
			t.Fatalf("expected monotonic sequence at idx=%d: prev=%d curr=%d", idx, events[idx-1].Sequence, events[idx].Sequence)
		}
	}
}
