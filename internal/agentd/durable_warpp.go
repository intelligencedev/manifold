package agentd

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/durable"
	"manifold/internal/warpp"
)

func (a *app) registerDurableHandlers() {
	if a == nil || a.durableRegistry == nil {
		return
	}
	a.durableRegistry.Register(warppDurableQueue, warppDurableRunTaskName, a.runDurableWarppTask)
	a.durableRegistry.Register(durableChatQueue, durableChatRunTaskName, a.runDurableChatTask)
	a.durableRegistry.Register(durablePulseQueue, durablePulseRunTaskName, a.runDurablePulseTask)
}

func (a *app) runDurableWarppTask(ctx context.Context, params map[string]any) (map[string]any, error) {
	tc, ok := durable.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("durable task context unavailable")
	}
	userID := tc.Task.UserID
	runID := tc.Task.ID
	workflowID, _ := params["workflow_id"].(string)
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil, fmt.Errorf("workflow_id required")
	}
	input, _ := params["input"].(map[string]any)

	doc, _, found, err := a.warppState().getWorkflow(ctx, userID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("load workflow: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("workflow not found")
	}
	if pid, _ := params["project_id"].(string); strings.TrimSpace(pid) != "" {
		doc.ProjectID = strings.TrimSpace(pid)
	}
	diags := warpp.Validate(doc, a.warppResolver(ctx, userID))
	if warpp.HasErrors(diags) {
		return nil, fmt.Errorf("workflow validation failed: %s", warppDiagSummary(diags))
	}

	a.warppState().createRunWithID(userID, doc.ID, runID, input)
	res := a.executeWarppRun(ctx, userID, runID, doc, input)
	if res.Err != nil {
		return nil, res.Err
	}
	if res.Status == warpp.StatusFailed || res.Status == warpp.StatusCancelled {
		return nil, fmt.Errorf("workflow finished with status %s", res.Status)
	}
	return map[string]any{
		"ok":       true,
		"run_id":   runID,
		"status":   res.Status,
		"outputs":  res.Outputs,
		"workflow": doc.ID,
	}, nil
}
