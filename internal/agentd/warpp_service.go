package agentd

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/durable"
	llmpkg "manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/tools"
	"manifold/internal/warpp"
	"manifold/internal/warpp/toolnode"
)

const (
	warppDurableQueue       = "warpp"
	warppDurableRunTaskName = "warpp.run"
)

// warppExecutionRegistry returns the policy-aware tool registry, falling back
// to the base registry.
func (a *app) warppExecutionRegistry() tools.Registry {
	if a.toolRegistry != nil {
		return a.toolRegistry
	}
	return a.baseToolRegistry
}

// warppResolver builds the node-type resolver: builtins, tool adapters, then
// published workflows as subflow nodes.
func (a *app) warppResolver(ctx context.Context, userID int64) warpp.Resolver {
	return warpp.ChainResolvers(
		warpp.BuiltinResolver(),
		toolnode.Resolver(toolnode.Builtin()),
		a.warppSubflowResolver(ctx, userID),
	)
}

// warppRunners builds the runner set: builtins, tool adapters, the LLM node,
// and subflow nodes.
func (a *app) warppRunners(ctx context.Context, userID int64) map[string]warpp.NodeRunner {
	runners := warpp.BuiltinRunners()
	for k, v := range toolnode.Runners(a.warppExecutionRegistry(), toolnode.Builtin()) {
		runners[k] = v
	}
	runners["llm.generate"] = warpp.LLMRunner(a.warppChat)
	a.registerSubflowRunners(ctx, userID, runners)
	return runners
}

// warppChat is the ChatFunc backing the llm.generate node.
func (a *app) warppChat(ctx context.Context, instruction, input, model string) (string, error) {
	if a.llm == nil {
		return "", fmt.Errorf("no llm provider available")
	}
	var msgs []llmpkg.Message
	if strings.TrimSpace(instruction) != "" {
		msgs = append(msgs, llmpkg.Message{Role: "system", Content: instruction})
	}
	msgs = append(msgs, llmpkg.Message{Role: "user", Content: input})
	reply, err := a.llm.Chat(ctx, msgs, nil, strings.TrimSpace(model))
	if err != nil {
		return "", err
	}
	return reply.Content, nil
}

// newWarppEngine assembles an engine for a run, wiring event recording and
// durable step checkpoints.
func (a *app) newWarppEngine(ctx context.Context, userID int64, runID string, doc warpp.Document) *warpp.Engine {
	emit := func(ev warpp.Event) {
		if de, err := durable.RecordEvent(ctx, "warpp."+string(ev.Type), warppEventPayload(ev)); err == nil {
			ev.Sequence = de.Sequence
			ev.OccurredAt = de.OccurredAt
		}
		_ = a.warppState().appendRunEvent(userID, runID, ev)
	}
	step := func(sctx context.Context, key string, fn func(context.Context) (map[string]warpp.Value, error)) (map[string]warpp.Value, error) {
		return durable.Step[map[string]warpp.Value](sctx, key, fn)
	}
	return &warpp.Engine{
		Resolve:        a.warppResolver(ctx, userID),
		Runners:        a.warppRunners(ctx, userID),
		Emit:           emit,
		Step:           step,
		MaxConcurrency: doc.Settings.MaxConcurrency,
	}
}

// warppProjectContext scopes filesystem tools to a project for a run, using the
// same workspace manager chat uses so paths resolve identically.
func (a *app) warppProjectContext(ctx context.Context, userID int64, projectID, sessionID string) (context.Context, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ctx, nil
	}
	if a.workspaceManager != nil {
		ws, err := a.workspaceManager.Checkout(ctx, userID, projectID, sessionID)
		if err != nil {
			return ctx, err
		}
		if ws.BaseDir != "" {
			ctx = sandbox.WithBaseDir(ctx, ws.BaseDir)
			ctx = sandbox.WithProjectID(ctx, projectID)
		}
		return ctx, nil
	}
	return workflowToolContext(ctx, a.cfg, userID, projectID)
}

// executeWarppRun runs a workflow document to completion, streaming events into
// the runtime for the given runID.
func (a *app) executeWarppRun(ctx context.Context, userID int64, runID string, doc warpp.Document, input map[string]any) warpp.Result {
	if projectID := strings.TrimSpace(doc.ProjectID); projectID != "" {
		pctx, err := a.warppProjectContext(ctx, userID, projectID, "warpp-"+doc.ID)
		if err != nil {
			a.warppState().appendRunEvent(userID, runID, warpp.Event{Type: warpp.EventRunStarted, Status: warpp.StatusRunning, Message: "run started"})
			a.warppState().appendRunEvent(userID, runID, warpp.Event{
				Type:    warpp.EventRunFailed,
				Status:  warpp.StatusFailed,
				Error:   fmt.Sprintf("project %q: %v", projectID, err),
				Message: "run failed",
			})
			return warpp.Result{Status: warpp.StatusFailed, Err: err}
		}
		ctx = pctx
	}
	eng := a.newWarppEngine(ctx, userID, runID, doc)
	return eng.Execute(ctx, doc, input)
}
