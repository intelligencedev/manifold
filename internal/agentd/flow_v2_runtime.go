package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"manifold/internal/flow"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/tools"
)

type flowV2RunRecord struct {
	ID         string
	UserID     int64
	WorkflowID string
	Status     string
	Input      map[string]any
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Sequence   int64
	Events     []flow.RunEvent
	Subs       map[chan flow.RunEvent]struct{}
}

type flowV2Runtime struct {
	mu    sync.RWMutex
	store persist.FlowV2WorkflowStore
	runs  map[string]*flowV2RunRecord
}

type flowNodeResult struct {
	nodeID  string
	output  map[string]any
	err     error
	skipped bool
}

func newFlowV2Runtime(store persist.FlowV2WorkflowStore) *flowV2Runtime {
	if store == nil {
		store = databases.NewPostgresFlowV2Store(nil)
	}
	return &flowV2Runtime{
		store: store,
		runs:  map[string]*flowV2RunRecord{},
	}
}

func (s *flowV2Runtime) listWorkflowSummaries(ctx context.Context, userID int64) ([]flow.WorkflowSummary, error) {
	records, err := s.store.ListWorkflows(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []flow.WorkflowSummary{}, nil
	}
	out := make([]flow.WorkflowSummary, 0, len(records))
	for _, rec := range records {
		out = append(out, flow.WorkflowSummary{
			ID:          rec.Workflow.ID,
			Name:        rec.Workflow.Name,
			Description: rec.Workflow.Description,
		})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].ID) < strings.ToLower(out[j-1].ID); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func (s *flowV2Runtime) getWorkflow(ctx context.Context, userID int64, workflowID string) (flow.Workflow, flow.WorkflowCanvas, bool, error) {
	rec, ok, err := s.store.GetWorkflow(ctx, userID, workflowID)
	if err != nil {
		return flow.Workflow{}, flow.WorkflowCanvas{}, false, err
	}
	if !ok {
		return flow.Workflow{}, flow.WorkflowCanvas{}, false, nil
	}
	return cloneWorkflow(rec.Workflow), cloneCanvas(rec.Canvas), true, nil
}

func (s *flowV2Runtime) upsertWorkflow(ctx context.Context, userID int64, wf flow.Workflow, canvas flow.WorkflowCanvas) (persist.FlowV2WorkflowRecord, bool, error) {
	return s.store.UpsertWorkflow(ctx, userID, persist.FlowV2WorkflowRecord{
		UserID:   userID,
		Workflow: cloneWorkflow(wf),
		Canvas:   cloneCanvas(canvas),
	})
}

func (s *flowV2Runtime) deleteWorkflow(ctx context.Context, userID int64, workflowID string) (bool, error) {
	_, found, err := s.store.GetWorkflow(ctx, userID, workflowID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if err := s.store.DeleteWorkflow(ctx, userID, workflowID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *flowV2Runtime) createRun(userID int64, workflowID string, input map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID := fmt.Sprintf("flowrun_%d", time.Now().UnixNano())
	now := time.Now().UTC()
	s.runs[runID] = &flowV2RunRecord{
		ID:         runID,
		UserID:     userID,
		WorkflowID: workflowID,
		Status:     "running",
		Input:      cloneMap(input),
		CreatedAt:  now,
		UpdatedAt:  now,
		Events:     make([]flow.RunEvent, 0, 32),
		Subs:       map[chan flow.RunEvent]struct{}{},
	}
	return runID
}

func (s *flowV2Runtime) appendRunEvent(userID int64, runID string, event flow.RunEvent) bool {
	s.mu.Lock()
	run, ok := s.runs[runID]
	if !ok || run.UserID != userID {
		s.mu.Unlock()
		return false
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	run.Sequence++
	event.Sequence = run.Sequence
	event.RunID = runID
	if event.Output != nil {
		event.Output = cloneMap(event.Output)
	}
	run.Events = append(run.Events, event)
	run.UpdatedAt = event.OccurredAt
	switch event.Type {
	case flow.RunEventTypeRunCompleted:
		run.Status = "completed"
	case flow.RunEventTypeRunFailed:
		run.Status = "failed"
		if strings.TrimSpace(event.Error) != "" {
			run.Error = event.Error
		}
	case flow.RunEventTypeRunCancelled:
		run.Status = "cancelled"
	}
	subs := make([]chan flow.RunEvent, 0, len(run.Subs))
	for ch := range run.Subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
	return true
}

func (s *flowV2Runtime) getRunEvents(userID int64, runID string) ([]flow.RunEvent, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[runID]
	if !ok || run.UserID != userID {
		return nil, "", false
	}
	out := make([]flow.RunEvent, len(run.Events))
	copy(out, run.Events)
	return out, run.Status, true
}

func (s *flowV2Runtime) subscribeRun(userID int64, runID string) ([]flow.RunEvent, chan flow.RunEvent, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.UserID != userID {
		return nil, nil, false, false
	}
	snapshot := make([]flow.RunEvent, len(run.Events))
	copy(snapshot, run.Events)
	done := run.Status != "running"
	if done {
		return snapshot, nil, true, true
	}
	ch := make(chan flow.RunEvent, 64)
	run.Subs[ch] = struct{}{}
	return snapshot, ch, false, true
}

func (s *flowV2Runtime) unsubscribeRun(runID string, ch chan flow.RunEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.Subs == nil {
		return
	}
	delete(run.Subs, ch)
}

func (a *app) executeFlowV2Run(ctx context.Context, userID int64, runID string, wf flow.Workflow, plan *flow.Plan, input map[string]any) {
	emit := func(ev flow.RunEvent) {
		_ = a.flowV2State().appendRunEvent(userID, runID, ev)
	}
	emit(flow.RunEvent{
		Type:    flow.RunEventTypeRunStarted,
		Status:  "running",
		Message: "run started",
	})

	nodeByID := make(map[string]flow.Node, len(wf.Nodes))
	for _, n := range wf.Nodes {
		nodeByID[n.ID] = n
	}

	reg := a.flowV2ExecutionRegistry()
	toolSet := map[string]bool{}
	if reg != nil {
		for _, schema := range reg.Schemas() {
			toolSet[schema.Name] = true
		}
	}

	defaultExec := wf.Settings.DefaultExecution
	maxConcurrency := wf.Settings.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}

	nodeIndex := make(map[string]int, len(plan.NodeOrder))
	for idx, nodeID := range plan.NodeOrder {
		nodeIndex[nodeID] = idx
	}

	remaining := make(map[string]int, len(wf.Nodes))
	for _, node := range wf.Nodes {
		remaining[node.ID] = len(plan.Incoming[node.ID])
	}
	if len(plan.Indegree) > 0 {
		for nodeID, degree := range plan.Indegree {
			remaining[nodeID] = degree
		}
	}

	nodeOutputs := make(map[string]map[string]any, len(wf.Nodes))
	launched := make(map[string]bool, len(wf.Nodes))
	var stateMu sync.RWMutex
	var fatalErr error
	var processed int
	resultCh := make(chan flowNodeResult, len(wf.Nodes))
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	launchNode := func(nodeID string) {
		node, ok := nodeByID[nodeID]
		if !ok {
			stateMu.Lock()
			if fatalErr == nil {
				fatalErr = fmt.Errorf("execution plan referenced unknown node %s", nodeID)
				cancelRun()
			}
			stateMu.Unlock()
			return
		}
		launched[nodeID] = true
		go func(node flow.Node) {
			if guard := strings.TrimSpace(node.Guard); guard != "" {
				stateMu.RLock()
				guardValue, guardErr := evalFlowExpression(guard, input, nodeOutputs)
				stateMu.RUnlock()
				if guardErr != nil {
					resultCh <- flowNodeResult{nodeID: node.ID, err: fmt.Errorf("node %s guard: %w", node.ID, guardErr)}
					return
				}
				if guardOK, ok := asBool(guardValue); ok && !guardOK {
					resultCh <- flowNodeResult{nodeID: node.ID, skipped: true}
					return
				}
			}

			emit(flow.RunEvent{
				Type:    flow.RunEventTypeNodeStarted,
				NodeID:  node.ID,
				Status:  "running",
				Message: "node started",
			})

			stateMu.RLock()
			resolvedInputs, err := resolveNodeInputs(node, plan.Incoming[node.ID], nodeOutputs, input)
			stateMu.RUnlock()
			if err != nil {
				resultCh <- flowNodeResult{nodeID: node.ID, err: err}
				return
			}

			output, err := a.executeFlowV2NodeWithRetries(runCtx, node, resolvedInputs, reg, toolSet, defaultExec, emit)
			resultCh <- flowNodeResult{nodeID: node.ID, output: output, err: err}
		}(node)
	}

	readyQueue := make([]string, 0, len(wf.Nodes))
	pushReady := func(nodeIDs []string) {
		slices.SortFunc(nodeIDs, func(left, right string) int {
			if nodeIndex[left] < nodeIndex[right] {
				return -1
			}
			if nodeIndex[left] > nodeIndex[right] {
				return 1
			}
			return 0
		})
		for _, nodeID := range nodeIDs {
			stateMu.RLock()
			aborted := fatalErr != nil
			stateMu.RUnlock()
			if launched[nodeID] || aborted || runCtx.Err() != nil {
				continue
			}
			readyQueue = append(readyQueue, nodeID)
		}
	}
	launchReady := func(active *int) {
		for *active < maxConcurrency && len(readyQueue) > 0 {
			next := readyQueue[0]
			readyQueue = readyQueue[1:]
			launchNode(next)
			*active = *active + 1
		}
	}

	initialReady := make([]string, 0, len(wf.Nodes))
	for _, node := range wf.Nodes {
		if remaining[node.ID] == 0 {
			initialReady = append(initialReady, node.ID)
		}
	}
	pushReady(initialReady)
	active := 0
	launchReady(&active)

	for active > 0 {
		res := <-resultCh
		active--
		processed++
		node := nodeByID[res.nodeID]

		switch {
		case res.skipped:
			emit(flow.RunEvent{
				Type:    flow.RunEventTypeNodeSkipped,
				NodeID:  res.nodeID,
				Status:  "skipped",
				Message: "node skipped",
			})
		case res.err != nil:
			message := "node failed"
			if strings.Contains(res.err.Error(), "input ") || strings.Contains(res.err.Error(), "path not found") {
				message = "node input resolution failed"
			}
			emit(flow.RunEvent{
				Type:    flow.RunEventTypeNodeFailed,
				NodeID:  res.nodeID,
				Status:  "failed",
				Error:   res.err.Error(),
				Message: message,
			})
			if effectiveOnError(node, defaultExec) != flow.ErrorStrategyContinue && fatalErr == nil {
				fatalErr = res.err
				cancelRun()
			}
		default:
			clonedOutput := cloneMap(res.output)
			stateMu.Lock()
			nodeOutputs[res.nodeID] = clonedOutput
			stateMu.Unlock()
			emit(flow.RunEvent{
				Type:    flow.RunEventTypeNodeCompleted,
				NodeID:  res.nodeID,
				Status:  "completed",
				Output:  cloneMap(clonedOutput),
				Message: "node completed",
			})
		}

		if fatalErr != nil {
			continue
		}

		stateMu.Lock()
		ready := make([]string, 0, len(plan.Outgoing[res.nodeID]))
		for _, edge := range plan.Outgoing[res.nodeID] {
			targetID := edge.Target.NodeID
			remaining[targetID]--
			if remaining[targetID] == 0 && !launched[targetID] {
				ready = append(ready, targetID)
			}
		}
		stateMu.Unlock()
		pushReady(ready)
		launchReady(&active)
	}

	if fatalErr != nil {
		emit(flow.RunEvent{
			Type:    flow.RunEventTypeRunFailed,
			Status:  "failed",
			Error:   fatalErr.Error(),
			Message: "run failed",
		})
		return
	}
	if ctx.Err() != nil {
		emit(flow.RunEvent{
			Type:    flow.RunEventTypeRunFailed,
			Status:  "failed",
			Error:   ctx.Err().Error(),
			Message: "run cancelled",
		})
		return
	}
	if processed != len(wf.Nodes) {
		emit(flow.RunEvent{
			Type:    flow.RunEventTypeRunFailed,
			Status:  "failed",
			Error:   fmt.Sprintf("workflow terminated early: processed %d of %d nodes", processed, len(wf.Nodes)),
			Message: "run failed",
		})
		return
	}
	emit(flow.RunEvent{
		Type:    flow.RunEventTypeRunCompleted,
		Status:  "completed",
		Message: "run completed",
	})
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
		cond, _ := asBool(inputs["condition"])
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

func resolveNodeInputs(node flow.Node, incoming []flow.Edge, outputs map[string]map[string]any, runInput map[string]any) (map[string]any, error) {
	resolved := map[string]any{}
	for _, edge := range incoming {
		src := outputs[edge.Source.NodeID]
		if src == nil {
			continue
		}
		if len(edge.Mapping) == 0 {
			targetKey := strings.TrimSpace(edge.Target.Port)
			if targetKey == "" {
				targetKey = strings.TrimSpace(edge.Source.Port)
			}
			if targetKey == "" {
				continue
			}
			val, ok := selectFlowPath(src, strings.TrimSpace(edge.Source.Port))
			if !ok {
				val = src
			}
			setFlowPath(resolved, targetKey, val)
			continue
		}
		for _, m := range edge.Mapping {
			from := strings.TrimSpace(m.From)
			to := strings.TrimSpace(m.To)
			if from == "" || to == "" {
				continue
			}
			val, ok := selectFlowPath(src, from)
			if !ok {
				continue
			}
			setFlowPath(resolved, to, val)
		}
	}

	for key, binding := range node.Inputs {
		if expr := strings.TrimSpace(binding.Expression); expr != "" {
			v, err := evalFlowExpression(expr, runInput, outputs)
			if err != nil {
				return nil, fmt.Errorf("node %s input %s: %w", node.ID, key, err)
			}
			resolved[key] = v
			continue
		}
		if binding.Literal != nil {
			resolved[key] = binding.Literal
		}
	}
	return resolved, nil
}

func evalFlowExpression(expr string, runInput map[string]any, outputs map[string]map[string]any) (any, error) {
	// Multi-expression: multiple ={{ ... }} blocks separated by newlines.
	// Evaluate each line independently and concatenate results with newlines.
	if strings.Count(expr, "={{") > 1 {
		lines := strings.Split(expr, "\n")
		var parts []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			v, err := evalFlowExpression(trimmed, runInput, outputs)
			if err != nil {
				return nil, err
			}
			parts = append(parts, fmt.Sprintf("%v", v))
		}
		return strings.Join(parts, "\n"), nil
	}

	norm := normalizeFlowExpression(expr)
	if strings.HasPrefix(norm, "$run.input") {
		path := strings.TrimPrefix(norm, "$run.input")
		path = strings.TrimPrefix(path, ".")
		if path == "" {
			return cloneMap(runInput), nil
		}
		v, ok := selectFlowPath(runInput, path)
		if !ok {
			return nil, fmt.Errorf("path not found: $run.input.%s", path)
		}
		return v, nil
	}
	if strings.HasPrefix(norm, "$node.") {
		rest := strings.TrimPrefix(norm, "$node.")
		firstDot := strings.Index(rest, ".")
		if firstDot <= 0 {
			return nil, fmt.Errorf("invalid node expression: %s", expr)
		}
		nodeID := rest[:firstDot]
		rem := rest[firstDot+1:]
		if rem == "output" {
			out := outputs[nodeID]
			if out == nil {
				return nil, fmt.Errorf("node output unavailable: %s", nodeID)
			}
			return cloneMap(out), nil
		}
		if !strings.HasPrefix(rem, "output.") {
			return nil, fmt.Errorf("invalid node expression: %s", expr)
		}
		out := outputs[nodeID]
		if out == nil {
			return nil, fmt.Errorf("node output unavailable: %s", nodeID)
		}
		path := strings.TrimPrefix(rem, "output.")
		v, ok := selectFlowPath(out, path)
		if !ok {
			return nil, fmt.Errorf("path not found: $node.%s.output.%s", nodeID, path)
		}
		return v, nil
	}

	var v any
	if err := json.Unmarshal([]byte(norm), &v); err == nil {
		return v, nil
	}
	return nil, fmt.Errorf("unsupported expression: %s", expr)
}

func normalizeFlowExpression(expr string) string {
	norm := strings.TrimSpace(expr)
	if strings.HasPrefix(norm, "=") {
		norm = strings.TrimSpace(strings.TrimPrefix(norm, "="))
	}
	if strings.HasPrefix(norm, "{{") && strings.HasSuffix(norm, "}}") && len(norm) >= 4 {
		norm = strings.TrimSpace(norm[2 : len(norm)-2])
	}
	return norm
}

func effectiveNodeExecution(node flow.Node, defaults flow.NodeExecution) flow.NodeExecution {
	out := node.Execution
	if strings.TrimSpace(out.Timeout) == "" {
		out.Timeout = defaults.Timeout
	}
	if out.Retries.Max <= 0 && defaults.Retries.Max > 0 {
		out.Retries.Max = defaults.Retries.Max
	}
	if out.Retries.Backoff == "" {
		out.Retries.Backoff = defaults.Retries.Backoff
	}
	if out.OnError == "" {
		out.OnError = defaults.OnError
	}
	if out.OnError == "" {
		out.OnError = flow.ErrorStrategyFail
	}
	return out
}

func effectiveOnError(node flow.Node, defaults flow.NodeExecution) flow.ErrorStrategy {
	return effectiveNodeExecution(node, defaults).OnError
}

func effectiveRetries(node flow.Node, defaults flow.NodeExecution) int {
	max := effectiveNodeExecution(node, defaults).Retries.Max
	if max < 0 {
		max = 0
	}
	return 1 + max
}

func sleepFlowRetry(ctx context.Context, node flow.Node, defaults flow.NodeExecution, attempt int) bool {
	execCfg := effectiveNodeExecution(node, defaults)
	backoff := execCfg.Retries.Backoff
	if backoff == "" {
		backoff = flow.BackoffFixed
	}
	delay := 200 * time.Millisecond
	switch backoff {
	case flow.BackoffExponential:
		delay = delay * time.Duration(1<<(attempt-1))
	case flow.BackoffFixed:
		// base delay
	default:
		return true
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

func setFlowPath(root map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := root
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i == len(parts)-1 {
			cur[part] = value
			return
		}
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
}

func selectFlowPath(root any, path string) (any, bool) {
	if path == "" {
		if root == nil {
			return nil, false
		}
		return root, true
	}
	parts := strings.Split(path, ".")
	cur := root
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch node := cur.(type) {
		case map[string]any:
			val, ok := node[part]
			if !ok {
				return nil, false
			}
			cur = val
		case map[string]string:
			val, ok := node[part]
			if !ok {
				return nil, false
			}
			cur = val
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		case []map[string]any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		case []string:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		case string:
			var decoded any
			if json.Unmarshal([]byte(node), &decoded) != nil {
				return nil, false
			}
			switch inner := decoded.(type) {
			case map[string]any:
				v, ok := inner[part]
				if !ok {
					return nil, false
				}
				cur = v
			case []any:
				idx, err := strconv.Atoi(part)
				if err != nil || idx < 0 || idx >= len(inner) {
					return nil, false
				}
				cur = inner[idx]
			default:
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if cur == nil {
		return nil, false
	}
	return cur, true
}

func asBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "y":
			return true, true
		case "false", "0", "no", "n":
			return false, true
		default:
			return false, false
		}
	case float64:
		return t != 0, true
	case int:
		return t != 0, true
	default:
		return false, false
	}
}

func cloneWorkflow(wf flow.Workflow) flow.Workflow {
	var out flow.Workflow
	b, _ := json.Marshal(wf)
	_ = json.Unmarshal(b, &out)
	return out
}

func cloneCanvas(c flow.WorkflowCanvas) flow.WorkflowCanvas {
	var out flow.WorkflowCanvas
	b, _ := json.Marshal(c)
	_ = json.Unmarshal(b, &out)
	return out
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	var out map[string]any
	b, _ := json.Marshal(m)
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func parseFlowDuration(s string) time.Duration {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func (a *app) flowV2ExecutionRegistry() tools.Registry {
	// Flow v2 should execute against the same full catalog surfaced by /api/flows/v2/tools.
	if a.baseToolRegistry != nil {
		return a.baseToolRegistry
	}
	return a.toolRegistry
}
