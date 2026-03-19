package agentd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/config"
	"manifold/internal/flow"
	"manifold/internal/sandbox"
	"manifold/internal/tools/warpptool"
)

const workflowToolDefaultTimeout = 5 * time.Minute

func (a *app) syncWarppTools(ctx context.Context) {
	if a == nil {
		return
	}
	reg := a.baseToolRegistry
	if reg == nil {
		reg = a.toolRegistry
	}
	if reg == nil {
		return
	}
	workflows, err := a.flowV2State().listWorkflowSummaries(ctx, systemUserID)
	if err != nil {
		log.Warn().Err(err).Msg("warpp_workflow_tool_sync_failed")
		return
	}
	a.warppToolMu.Lock()
	defer a.warppToolMu.Unlock()
	warpptool.UnregisterAll(reg, a.warppToolNames)
	a.warppToolNames = warpptool.SyncAll(reg, a, systemUserID, workflows)
	log.Info().Strs("tools", a.warppToolNames).Msg("warpp_workflow_tools_synced")
}

func (a *app) ExecuteWorkflowSync(ctx context.Context, userID int64, workflowID string, input map[string]any) (map[string]any, error) {
	if a == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil, fmt.Errorf("workflow id required")
	}
	runCtx := ctx
	cancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		runCtx, cancel = context.WithTimeout(ctx, workflowToolDefaultTimeout)
	}
	defer cancel()

	wf, _, found, err := a.flowV2State().getWorkflow(runCtx, userID, workflowID)
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
	if projectID := strings.TrimSpace(wf.ProjectID); projectID != "" {
		var projectErr error
		runCtx, projectErr = workflowToolContext(runCtx, a.cfg, userID, projectID)
		if projectErr != nil {
			return nil, projectErr
		}
	}
	runID := a.flowV2State().createRun(userID, wf.ID, input)
	a.executeFlowV2Run(runCtx, userID, runID, wf, plan, input)
	events, status, ok := a.flowV2State().getRunEvents(userID, runID)
	if !ok {
		return nil, fmt.Errorf("run result unavailable")
	}
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
	return result, nil
}

func workflowToolContext(ctx context.Context, cfg *config.Config, userID int64, projectID string) (context.Context, error) {
	if cfg == nil {
		return ctx, fmt.Errorf("config unavailable")
	}
	cleanProjectID := filepath.Clean(projectID)
	if cleanProjectID != projectID || strings.HasPrefix(cleanProjectID, "..") || strings.Contains(cleanProjectID, string(filepath.Separator)+"..") || filepath.IsAbs(cleanProjectID) {
		return ctx, fmt.Errorf("invalid project_id")
	}
	if _, ok := sandbox.ProjectIDFromContext(ctx); !ok {
		ctx = sandbox.WithProjectID(ctx, cleanProjectID)
	}
	if _, ok := sandbox.BaseDirFromContext(ctx); ok {
		return ctx, nil
	}
	baseRoot := filepath.Join(cfg.Workdir, "users", fmt.Sprint(userID), "projects")
	baseDir := filepath.Join(baseRoot, cleanProjectID)
	if !strings.HasPrefix(baseDir, baseRoot+string(filepath.Separator)) && baseDir != baseRoot {
		return ctx, fmt.Errorf("invalid project_id")
	}
	return sandbox.WithBaseDir(ctx, baseDir), nil
}

func flowDiagnosticSummary(diags []flow.Diagnostic) string {
	parts := make([]string, 0, len(diags))
	for _, diag := range diags {
		if diag.Severity != flow.DiagnosticSeverityError {
			continue
		}
		parts = append(parts, strings.TrimSpace(diag.Message))
	}
	if len(parts) == 0 {
		return "invalid workflow"
	}
	return strings.Join(parts, "; ")
}

func unwrapWorkflowPayload(output map[string]any) (any, bool) {
	if output == nil {
		return nil, false
	}
	if parsed, ok := output["json"].(map[string]any); ok {
		if payload, exists := parsed["payload"]; exists {
			return payload, true
		}
	}
	payload, ok := output["payload"]
	return payload, ok
}
