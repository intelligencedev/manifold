package agentd

import (
	"encoding/json"
	"fmt"
	"strings"
)

func evalMultiFlowExpression(expr string, runInput map[string]any, outputs map[string]map[string]any) (any, bool, error) {
	lines := strings.Split(expr, "\n")
	if len(lines) <= 1 {
		return nil, false, nil
	}
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		value, err := evalFlowExpression(trimmed, runInput, outputs)
		if err != nil {
			return nil, true, err
		}
		parts = append(parts, fmt.Sprintf("%v", value))
	}
	return strings.Join(parts, "\n"), true, nil
}

func evalRunInputExpression(after string, runInput map[string]any) (any, error) {
	path := strings.TrimPrefix(after, ".")
	if path == "" {
		return cloneMap(runInput), nil
	}
	value, ok := selectFlowPath(runInput, path)
	if !ok {
		return nil, fmt.Errorf("path not found: $run.input.%s", path)
	}
	return value, nil
}

func evalNodeOutputExpression(expr, rest string, outputs map[string]map[string]any) (any, error) {
	firstDot := strings.Index(rest, ".")
	if firstDot <= 0 {
		return nil, fmt.Errorf("invalid node expression: %s", expr)
	}
	nodeID := rest[:firstDot]
	rem := rest[firstDot+1:]
	out := outputs[nodeID]
	if out == nil {
		return nil, fmt.Errorf("node output unavailable: %s", nodeID)
	}
	if rem == "output" {
		return cloneMap(out), nil
	}
	if !strings.HasPrefix(rem, "output.") {
		return nil, fmt.Errorf("invalid node expression: %s", expr)
	}
	path := strings.TrimPrefix(rem, "output.")
	value, ok := selectFlowPath(out, path)
	if !ok {
		return nil, fmt.Errorf("path not found: $node.%s.output.%s", nodeID, path)
	}
	return value, nil
}

func normalizeFlowExpression(expr string) string {
	norm := strings.TrimSpace(expr)
	if after, ok := strings.CutPrefix(norm, "="); ok {
		norm = strings.TrimSpace(after)
	}
	if strings.HasPrefix(norm, "{{") && strings.HasSuffix(norm, "}}") && len(norm) >= 4 {
		norm = strings.TrimSpace(norm[2 : len(norm)-2])
	}
	return norm
}

func evalFlowExpression(expr string, runInput map[string]any, outputs map[string]map[string]any) (any, error) {
	if strings.Count(expr, "={{") > 1 {
		if value, ok, err := evalMultiFlowExpression(expr, runInput, outputs); ok || err != nil {
			return value, err
		}
	}

	norm := normalizeFlowExpression(expr)
	if after, ok := strings.CutPrefix(norm, "$run.input"); ok {
		return evalRunInputExpression(after, runInput)
	}
	if after, ok := strings.CutPrefix(norm, "$node."); ok {
		return evalNodeOutputExpression(expr, after, outputs)
	}

	var value any
	if err := json.Unmarshal([]byte(norm), &value); err == nil {
		return value, nil
	}
	return nil, fmt.Errorf("unsupported expression: %s", expr)
}
