package agentd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"manifold/internal/durable"
	"manifold/internal/flow"
)

const (
	durableFlowQueue       = "flow_v2"
	durableFlowRunTaskName = "flow_v2.run"
)

func (a *app) registerDurableHandlers() {
	if a == nil || a.durableRegistry == nil {
		return
	}
	a.durableRegistry.Register(durableFlowQueue, durableFlowRunTaskName, a.runDurableFlowV2Task)
	a.durableRegistry.Register(durableChatQueue, durableChatRunTaskName, a.runDurableChatTask)
}

func (a *app) runDurableFlowV2Task(ctx context.Context, params map[string]any) (map[string]any, error) {
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
	projectID, _ := params["project_id"].(string)

	wf, _, found, err := a.flowV2State().getWorkflow(ctx, userID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("load workflow: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("workflow not found")
	}
	plan, diags := flow.CompileWorkflow(wf)
	if hasFlowV2Errors(diags) || plan == nil {
		return nil, fmt.Errorf("workflow validation failed: %s", flowDiagnosticSummary(diags))
	}
	if strings.TrimSpace(projectID) != "" {
		var projectErr error
		ctx, projectErr = workflowToolContext(ctx, a.cfg, userID, strings.TrimSpace(projectID))
		if projectErr != nil {
			return nil, projectErr
		}
	}

	a.flowV2State().createRunWithID(userID, wf.ID, runID, input)
	a.executeFlowV2Run(ctx, userID, runID, wf, plan, input)
	events, status, ok := a.flowV2State().getRunEvents(userID, runID)
	if !ok {
		return nil, fmt.Errorf("run result unavailable")
	}
	return workflowResultFromEvents(runID, status, wf, plan, events)
}

func workflowResultFromEvents(runID, status string, wf flow.Workflow, plan *flow.Plan, events []flow.RunEvent) (map[string]any, error) {
	outputs := make(map[string]map[string]any)
	var runErr string
	for _, event := range events {
		switch event.Type {
		case flow.RunEventTypeNodeCompleted:
			if event.Output != nil {
				outputs[event.NodeID] = cloneMap(event.Output)
			}
		case flow.RunEventTypeRunFailed, flow.RunEventTypeRunCancelled:
			if strings.TrimSpace(event.Error) != "" {
				runErr = event.Error
			} else if strings.TrimSpace(event.Message) != "" {
				runErr = event.Message
			}
		}
	}
	if status != "completed" {
		if runErr == "" {
			runErr = fmt.Sprintf("workflow finished with status %s", status)
		}
		return nil, errors.New(runErr)
	}
	result := map[string]any{
		"ok":            true,
		"run_id":        runID,
		"status":        status,
		"workflow_id":   wf.ID,
		"workflow_name": wf.Name,
		"outputs":       outputs,
	}
	if plan != nil {
		for idx := len(plan.NodeOrder) - 1; idx >= 0; idx-- {
			nodeID := plan.NodeOrder[idx]
			output, exists := outputs[nodeID]
			if !exists {
				continue
			}
			finalOutput := cloneMap(output)
			result["final_node_id"] = nodeID
			result["final_output"] = finalOutput
			if payload, ok := unwrapWorkflowPayload(finalOutput); ok {
				result["payload"] = payload
			}
			if inputs, ok := finalOutput["inputs"]; ok {
				result["inputs"] = inputs
			}
			break
		}
	}
	return result, nil
}
