package warpptool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"manifold/internal/flow"
	"manifold/internal/tools"
)

// ToolPrefix keeps backward compatibility with the previous WARPP workflow tool names.
const ToolPrefix = "warpp_"

// WorkflowRunner executes a saved workflow synchronously for tool callers.
type WorkflowRunner interface {
	ExecuteWorkflowSync(ctx context.Context, userID int64, workflowID string, input map[string]any) (map[string]any, error)
}

type workflowTool struct {
	name         string
	workflowID   string
	workflowName string
	description  string
	userID       int64
	runner       WorkflowRunner
}

func (t *workflowTool) Name() string { return t.name }

func (t *workflowTool) JSONSchema() map[string]any {
	desc := strings.TrimSpace(t.description)
	if desc == "" {
		label := strings.TrimSpace(t.workflowName)
		if label == "" {
			label = t.workflowID
		}
		desc = fmt.Sprintf("Run saved workflow '%s' with a natural language query.", label)
	}
	return map[string]any{
		"name":        t.name,
		"description": desc,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural language request passed to the workflow as $run.input.query.",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (t *workflowTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("invalid args: %v", err)}, nil
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return map[string]any{"ok": false, "error": "query required"}, nil
	}
	if t.runner == nil {
		return map[string]any{"ok": false, "error": "workflow runner unavailable"}, nil
	}
	result, err := t.runner.ExecuteWorkflowSync(ctx, t.userID, t.workflowID, map[string]any{"query": query})
	if err != nil {
		return map[string]any{
			"ok":            false,
			"error":         err.Error(),
			"workflow_id":   t.workflowID,
			"workflow_name": t.workflowName,
		}, nil
	}
	if result == nil {
		result = map[string]any{}
	}
	result["ok"] = true
	result["workflow_id"] = t.workflowID
	result["workflow_name"] = t.workflowName
	return result, nil
}

// SyncAll registers one tool per workflow and returns the registered tool names.
func SyncAll(reg tools.Registry, runner WorkflowRunner, userID int64, workflows []flow.WorkflowSummary) []string {
	if reg == nil || runner == nil || len(workflows) == 0 {
		return nil
	}
	used := make(map[string]int, len(workflows))
	registered := make([]string, 0, len(workflows))
	for _, wf := range workflows {
		workflowID := strings.TrimSpace(wf.ID)
		if workflowID == "" {
			continue
		}
		base := ToolPrefix + sanitize(workflowID)
		name := base
		if count := used[base]; count > 0 {
			suffix := sanitize(workflowID)
			if suffix == "workflow" {
				suffix = fmt.Sprintf("%d", count+1)
			}
			name = base + "_" + suffix
		}
		used[base]++
		reg.Register(&workflowTool{
			name:         name,
			workflowID:   workflowID,
			workflowName: workflowID,
			description:  wf.Description,
			userID:       userID,
			runner:       runner,
		})
		registered = append(registered, name)
	}
	return registered
}

// UnregisterAll removes previously registered workflow tools by name.
func UnregisterAll(reg tools.Registry, names []string) {
	if reg == nil {
		return
	}
	for _, name := range names {
		reg.Unregister(name)
	}
}

func sanitize(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "workflow"
	}
	var b strings.Builder
	b.Grow(len(value))
	lastUnderscore := false
	for _, r := range value {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "workflow"
	}
	return out
}
