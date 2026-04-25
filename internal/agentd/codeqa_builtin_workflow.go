package agentd

import (
	"context"
	"fmt"

	"manifold/internal/flow"
	codeqatool "manifold/internal/tools/codeqa"
)

const builtinCodeQAPipelineWorkflowID = "builtin/codeqa_pipeline_v1"

func (a *app) ensureBuiltinCodeQAWorkflow(ctx context.Context) error {
	if a == nil || a.flowV2 == nil {
		return nil
	}
	wf, canvas := builtinCodeQAWorkflow()
	if _, _, err := a.flowV2.upsertWorkflow(ctx, systemUserID, wf, canvas); err != nil {
		return fmt.Errorf("upsert builtin codeqa workflow: %w", err)
	}
	return nil
}

func builtinCodeQAWorkflow() (flow.Workflow, flow.WorkflowCanvas) {
	return flow.Workflow{
		ID:          builtinCodeQAPipelineWorkflowID,
		Name:        "CodeQA Pipeline v1",
		Description: "Run the CodeQA optimizer on an explicit set of target files and return the evaluated candidate.",
		Keywords:    []string{"codeqa", "quality", "optimize", "review"},
		Trigger:     flow.Trigger{Type: flow.TriggerTypeManual},
		Nodes: []flow.Node{{
			ID:   "optimize",
			Name: "Optimize Target Files",
			Kind: flow.NodeKindAction,
			Type: "tool",
			Tool: codeqatool.ToolNameOptimize,
			Inputs: map[string]flow.InputBinding{
				"repository_path":  {Expression: "$run.input.repository_path"},
				"project_id":       {Expression: "$run.input.project_id"},
				"objective":        {Expression: "$run.input.objective"},
				"target_paths":     {Expression: "$run.input.target_paths"},
				"max_iterations":   {Expression: "$run.input.max_iterations"},
				"accept_threshold": {Expression: "$run.input.accept_threshold"},
				"min_confidence":   {Expression: "$run.input.min_confidence"},
			},
			PublishResult: true,
			Execution: flow.NodeExecution{
				OnError: flow.ErrorStrategyFail,
			},
		}},
	}, flow.WorkflowCanvas{Nodes: map[string]flow.CanvasNode{"optimize": {X: 120, Y: 120}}}
}
