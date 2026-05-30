package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"manifold/internal/durable"
	"manifold/internal/flow"
	"manifold/internal/tools"
)

func (a *app) executeFlowV2Run(ctx context.Context, userID int64, runID string, wf flow.Workflow, plan *flow.Plan, input map[string]any) {
	newFlowV2RunExecutor(flowV2RunExecutorOptions{
		app:    a,
		ctx:    ctx,
		userID: userID,
		runID:  runID,
		wf:     wf,
		plan:   plan,
		input:  input,
	}).run()
}

type flowV2RunExecutorOptions struct {
	app    *app
	ctx    context.Context
	userID int64
	runID  string
	wf     flow.Workflow
	plan   *flow.Plan
	input  map[string]any
}

type flowV2RunExecutor struct {
	app            *app
	ctx            context.Context
	userID         int64
	runID          string
	wf             flow.Workflow
	plan           *flow.Plan
	input          map[string]any
	emit           func(flow.RunEvent)
	nodeByID       map[string]flow.Node
	reg            tools.Registry
	toolSet        map[string]bool
	defaultExec    flow.NodeExecution
	maxConcurrency int
	nodeIndex      map[string]int
	remaining      map[string]int
	nodeOutputs    map[string]map[string]any
	launched       map[string]bool
	stateMu        sync.RWMutex
	fatalErr       error
	processed      int
	resultCh       chan flowNodeResult
	runCtx         context.Context
	cancelRun      context.CancelFunc
	readyQueue     []string
}

func newFlowV2RunExecutor(opts flowV2RunExecutorOptions) *flowV2RunExecutor {
	runCtx, cancelRun := context.WithCancel(opts.ctx)
	exec := &flowV2RunExecutor{
		app:            opts.app,
		ctx:            opts.ctx,
		userID:         opts.userID,
		runID:          opts.runID,
		wf:             opts.wf,
		plan:           opts.plan,
		input:          opts.input,
		nodeByID:       flowNodeByID(opts.wf),
		reg:            opts.app.flowV2ExecutionRegistry(),
		defaultExec:    opts.wf.Settings.DefaultExecution,
		maxConcurrency: flowMaxConcurrency(opts.wf),
		nodeIndex:      flowNodeIndex(opts.plan),
		remaining:      flowRemainingNodes(opts.wf, opts.plan),
		nodeOutputs:    make(map[string]map[string]any, len(opts.wf.Nodes)),
		launched:       make(map[string]bool, len(opts.wf.Nodes)),
		resultCh:       make(chan flowNodeResult, len(opts.wf.Nodes)),
		runCtx:         runCtx,
		cancelRun:      cancelRun,
		readyQueue:     make([]string, 0, len(opts.wf.Nodes)),
	}
	exec.toolSet = flowToolSet(exec.reg)
	exec.emit = exec.emitRunEvent
	return exec
}

func (e *flowV2RunExecutor) run() {
	defer e.cancelRun()
	e.emit(flow.RunEvent{Type: flow.RunEventTypeRunStarted, Status: "running", Message: "run started"})
	active := e.launchInitialReady()
	for active > 0 {
		res := <-e.resultCh
		active--
		e.processed++
		e.handleNodeResult(res)
		if e.fatalErr == nil {
			e.pushReady(e.downstreamReady(res.nodeID))
			active = e.launchReady(active)
		}
	}
	e.finish()
}

func (e *flowV2RunExecutor) emitRunEvent(ev flow.RunEvent) {
	if durableEvent, err := durable.RecordEvent(e.ctx, "flow."+string(ev.Type), flowEventPayload(ev)); err == nil {
		ev.Sequence = durableEvent.Sequence
		ev.OccurredAt = durableEvent.OccurredAt
	}
	_ = e.app.flowV2State().appendRunEvent(e.userID, e.runID, ev)
}

func (e *flowV2RunExecutor) launchInitialReady() int {
	initialReady := make([]string, 0, len(e.wf.Nodes))
	for _, node := range e.wf.Nodes {
		if e.remaining[node.ID] == 0 {
			initialReady = append(initialReady, node.ID)
		}
	}
	e.pushReady(initialReady)
	return e.launchReady(0)
}

func (e *flowV2RunExecutor) launchNode(nodeID string) bool {
	node, ok := e.nodeByID[nodeID]
	if !ok {
		e.fail(fmt.Errorf("execution plan referenced unknown node %s", nodeID))
		return false
	}
	e.launched[nodeID] = true
	go e.executeNode(node)
	return true
}

func (e *flowV2RunExecutor) executeNode(node flow.Node) {
	if checkpointed, found, err := loadDurableNodeCheckpoint(e.runCtx, node.ID); err != nil {
		e.resultCh <- flowNodeResult{nodeID: node.ID, err: err}
		return
	} else if found {
		e.resultCh <- flowNodeResult{nodeID: node.ID, output: checkpointed}
		return
	}
	outputsSnapshot := e.outputsSnapshot()
	if ok, err := e.nodeGuardAllows(node, outputsSnapshot); err != nil {
		e.resultCh <- flowNodeResult{nodeID: node.ID, err: err}
		return
	} else if !ok {
		e.resultCh <- flowNodeResult{nodeID: node.ID, skipped: true}
		return
	}
	e.emit(flow.RunEvent{Type: flow.RunEventTypeNodeStarted, NodeID: node.ID, Status: "running", Message: "node started"})
	resolvedInputs, err := resolveNodeInputs(node, e.plan.Incoming[node.ID], outputsSnapshot, e.input)
	if err != nil {
		e.resultCh <- flowNodeResult{nodeID: node.ID, err: err}
		return
	}
	output, err := durable.Step[map[string]any](e.runCtx, "node:"+node.ID, func(stepCtx context.Context) (map[string]any, error) {
		return e.app.executeFlowV2NodeWithRetries(stepCtx, node, resolvedInputs, e.reg, e.toolSet, e.defaultExec, e.emit)
	})
	e.resultCh <- flowNodeResult{nodeID: node.ID, output: output, err: err}
}

func (e *flowV2RunExecutor) outputsSnapshot() map[string]map[string]any {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	return cloneNodeOutputs(e.nodeOutputs)
}

func (e *flowV2RunExecutor) nodeGuardAllows(node flow.Node, outputsSnapshot map[string]map[string]any) (bool, error) {
	guard := strings.TrimSpace(node.Guard)
	if guard == "" {
		return true, nil
	}
	guardValue, err := evalFlowExpression(guard, e.input, outputsSnapshot)
	if err != nil {
		return false, fmt.Errorf("node %s guard: %w", node.ID, err)
	}
	guardOK, ok := asBool(guardValue)
	return !ok || guardOK, nil
}

func (e *flowV2RunExecutor) pushReady(nodeIDs []string) {
	slices.SortFunc(nodeIDs, func(left, right string) int { return e.nodeIndex[left] - e.nodeIndex[right] })
	for _, nodeID := range nodeIDs {
		e.stateMu.RLock()
		aborted := e.fatalErr != nil
		e.stateMu.RUnlock()
		if !e.launched[nodeID] && !aborted && e.runCtx.Err() == nil {
			e.readyQueue = append(e.readyQueue, nodeID)
		}
	}
}

func (e *flowV2RunExecutor) launchReady(active int) int {
	for active < e.maxConcurrency && len(e.readyQueue) > 0 {
		next := e.readyQueue[0]
		e.readyQueue = e.readyQueue[1:]
		if e.launchNode(next) {
			active++
		}
	}
	return active
}

func (e *flowV2RunExecutor) handleNodeResult(res flowNodeResult) {
	node := e.nodeByID[res.nodeID]
	switch {
	case res.skipped:
		e.emit(flow.RunEvent{Type: flow.RunEventTypeNodeSkipped, NodeID: res.nodeID, Status: "skipped", Message: "node skipped"})
	case res.err != nil:
		e.handleNodeError(res, node)
	default:
		e.recordNodeOutput(res)
	}
}

func (e *flowV2RunExecutor) handleNodeError(res flowNodeResult, node flow.Node) {
	message := "node failed"
	if strings.Contains(res.err.Error(), "input ") || strings.Contains(res.err.Error(), "path not found") {
		message = "node input resolution failed"
	}
	e.emit(flow.RunEvent{Type: flow.RunEventTypeNodeFailed, NodeID: res.nodeID, Status: "failed", Error: res.err.Error(), Message: message})
	if effectiveOnError(node, e.defaultExec) != flow.ErrorStrategyContinue && e.fatalErr == nil {
		e.fail(res.err)
	}
}

func (e *flowV2RunExecutor) recordNodeOutput(res flowNodeResult) {
	clonedOutput := cloneMap(res.output)
	e.stateMu.Lock()
	e.nodeOutputs[res.nodeID] = clonedOutput
	e.stateMu.Unlock()
	e.emit(flow.RunEvent{Type: flow.RunEventTypeNodeCompleted, NodeID: res.nodeID, Status: "completed", Output: cloneMap(clonedOutput), Message: "node completed"})
}

func (e *flowV2RunExecutor) downstreamReady(nodeID string) []string {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	ready := make([]string, 0, len(e.plan.Outgoing[nodeID]))
	for _, edge := range e.plan.Outgoing[nodeID] {
		targetID := edge.Target.NodeID
		e.remaining[targetID]--
		if e.remaining[targetID] == 0 && !e.launched[targetID] {
			ready = append(ready, targetID)
		}
	}
	return ready
}

func (e *flowV2RunExecutor) fail(err error) {
	e.stateMu.Lock()
	if e.fatalErr == nil {
		e.fatalErr = err
		e.cancelRun()
	}
	e.stateMu.Unlock()
}

func (e *flowV2RunExecutor) finish() {
	switch {
	case e.fatalErr != nil:
		e.emit(flow.RunEvent{Type: flow.RunEventTypeRunFailed, Status: "failed", Error: e.fatalErr.Error(), Message: "run failed"})
	case e.ctx.Err() != nil:
		e.emit(flow.RunEvent{Type: flow.RunEventTypeRunFailed, Status: "failed", Error: e.ctx.Err().Error(), Message: "run cancelled"})
	case e.processed != len(e.wf.Nodes):
		errText := fmt.Sprintf("workflow terminated early: processed %d of %d nodes", e.processed, len(e.wf.Nodes))
		e.emit(flow.RunEvent{Type: flow.RunEventTypeRunFailed, Status: "failed", Error: errText, Message: "run failed"})
	default:
		e.emit(flow.RunEvent{Type: flow.RunEventTypeRunCompleted, Status: "completed", Message: "run completed"})
	}
}

func flowNodeByID(wf flow.Workflow) map[string]flow.Node {
	nodeByID := make(map[string]flow.Node, len(wf.Nodes))
	for _, n := range wf.Nodes {
		nodeByID[n.ID] = n
	}
	return nodeByID
}

func flowToolSet(reg tools.Registry) map[string]bool {
	toolSet := map[string]bool{}
	if reg != nil {
		for _, schema := range reg.Schemas() {
			toolSet[schema.Name] = true
		}
	}
	return toolSet
}

func flowNodeIndex(plan *flow.Plan) map[string]int {
	nodeIndex := make(map[string]int, len(plan.NodeOrder))
	for idx, nodeID := range plan.NodeOrder {
		nodeIndex[nodeID] = idx
	}
	return nodeIndex
}

func flowRemainingNodes(wf flow.Workflow, plan *flow.Plan) map[string]int {
	remaining := make(map[string]int, len(wf.Nodes))
	for _, node := range wf.Nodes {
		remaining[node.ID] = len(plan.Incoming[node.ID])
	}
	if len(plan.Indegree) > 0 {
		maps.Copy(remaining, plan.Indegree)
	}
	return remaining
}

func flowMaxConcurrency(wf flow.Workflow) int {
	if wf.Settings.MaxConcurrency <= 0 {
		return 4
	}
	return wf.Settings.MaxConcurrency
}

func (a *app) executeFlowV2NodeWithRetries(
	ctx context.Context,
	node flow.Node,
	inputs map[string]any,
	reg tools.Registry,
	toolSet map[string]bool,
	defaults flow.NodeExecution,
	emit func(flow.RunEvent),
) (map[string]any, error) {
	attempts := effectiveRetries(node, defaults)
	var output map[string]any
	var runErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		output, runErr = a.executeFlowV2Node(ctx, node, inputs, reg, toolSet, defaults)
		if runErr == nil {
			return output, nil
		}
		if attempt < attempts {
			emit(flow.RunEvent{
				Type:    flow.RunEventTypeNodeRetrying,
				NodeID:  node.ID,
				Status:  "retrying",
				Message: fmt.Sprintf("retry %d/%d", attempt, attempts-1),
				Error:   runErr.Error(),
			})
			if !sleepFlowRetry(ctx, node, defaults, attempt) {
				return nil, context.Canceled
			}
		}
	}
	return nil, runErr
}

func (a *app) executeFlowV2Node(ctx context.Context, node flow.Node, inputs map[string]any, reg tools.Registry, toolSet map[string]bool, defaults flow.NodeExecution) (map[string]any, error) {
	execCfg := effectiveNodeExecution(node, defaults)
	cctx := ctx
	if d := parseFlowDuration(execCfg.Timeout); d > 0 {
		var cancel context.CancelFunc
		cctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}

	switch node.Type {
	case "tool":
		if strings.TrimSpace(node.Tool) == "" {
			return nil, fmt.Errorf("tool node %q missing tool name", node.ID)
		}
		if reg == nil {
			return nil, fmt.Errorf("tool registry unavailable")
		}
		if !toolSet[node.Tool] {
			return nil, fmt.Errorf("tool not found: %s", node.Tool)
		}
		raw, _ := json.Marshal(inputs)
		payload, err := reg.Dispatch(cctx, node.Tool, raw)
		if err != nil {
			return nil, err
		}
		out := map[string]any{
			"inputs":  cloneMap(inputs),
			"payload": string(payload),
		}
		var parsed any
		if err := json.Unmarshal(payload, &parsed); err == nil {
			out["json"] = parsed
			if m, ok := parsed.(map[string]any); ok {
				if em, ok := m["error"].(string); ok && strings.TrimSpace(em) != "" {
					if okv, hasOK := m["ok"].(bool); !hasOK || !okv {
						return nil, fmt.Errorf("tool %s returned error: %s", node.Tool, em)
					}
				}
				for k, v := range m {
					if _, exists := out[k]; !exists {
						out[k] = v
					}
				}
			}
		}
		return out, nil
	case "if":
		cond, ok := asBool(inputs["condition"])
		if !ok {
			cond = false
		}
		return map[string]any{
			"result": cond,
			"inputs": cloneMap(inputs),
		}, nil
	default:
		// Generic passthrough for action/data nodes whose execution
		// semantics are not yet implemented in runtime.
		return map[string]any{
			"inputs": cloneMap(inputs),
		}, nil
	}
}
