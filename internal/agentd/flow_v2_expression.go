package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"manifold/internal/flow"
	"manifold/internal/tools"
)

func resolveNodeInputs(node flow.Node, incoming []flow.Edge, outputs map[string]map[string]any, runInput map[string]any) (map[string]any, error) {
	resolved := map[string]any{}
	for _, edge := range incoming {
		src := outputs[edge.Source.NodeID]
		if src == nil {
			continue
		}
		if len(edge.Mapping) == 0 {
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

func cloneNodeOutputs(outputs map[string]map[string]any) map[string]map[string]any {
	cloned := make(map[string]map[string]any, len(outputs))
	for nodeID, output := range outputs {
		cloned[nodeID] = cloneMap(output)
	}
	return cloned
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
