package agentd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"manifold/internal/auth"
	"manifold/internal/playground"
	"manifold/internal/playground/experiment"
	"manifold/internal/playground/provider"
	"manifold/internal/playground/worker"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/workspaces"
)

type playgroundSpecialistRunner struct {
	app      *app
	fallback worker.TaskRunner
}

func newPlaygroundSpecialistRunner(app *app, fallback worker.TaskRunner) *playgroundSpecialistRunner {
	return &playgroundSpecialistRunner{app: app, fallback: fallback}
}

func (r *playgroundSpecialistRunner) ValidateSpecialist(ctx context.Context, ownerID int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if r == nil || r.app == nil || r.app.specStore == nil {
		return playground.ErrSpecialistRunnerUnavailable
	}
	if strings.EqualFold(name, specialists.OrchestratorName) {
		return fmt.Errorf("%w: %s", playground.ErrSpecialistNotFound, name)
	}
	sp, ok, err := r.app.getSpecialistForUser(ctx, ownerID, name)
	if err != nil {
		return fmt.Errorf("validate playground specialist %q: %w", name, err)
	}
	if !ok {
		return fmt.Errorf("%w: %s", playground.ErrSpecialistNotFound, name)
	}
	if sp.Paused {
		return fmt.Errorf("%w: %s", playground.ErrSpecialistPaused, name)
	}
	return nil
}

func (r *playgroundSpecialistRunner) RunTask(ctx context.Context, task worker.Task, renderedPrompt string) (provider.Response, error) {
	execution := experiment.NormalizeExecution(task.Execution)
	if execution == nil {
		if r == nil || r.fallback == nil {
			return provider.Response{}, playground.ErrSpecialistRunnerUnavailable
		}
		return r.fallback.RunTask(ctx, task, renderedPrompt)
	}

	ownerID := playgroundTaskOwner(ctx, task.OwnerID)
	if err := r.ValidateSpecialist(ctx, ownerID, execution.SpecialistName); err != nil {
		return provider.Response{}, err
	}

	sessionID := playgroundTaskSessionID(task)
	runCtx, workspace, err := r.prepareTaskContext(ctx, ownerID, task, sessionID)
	if err != nil {
		return provider.Response{}, err
	}
	if workspace != nil {
		defer r.app.commitWorkspace(runCtx, workspace)
	}

	build := r.app.buildSpecialistChatEngine(runCtx, chatEngineBuildRequest{
		Name:           execution.SpecialistName,
		SessionID:      sessionID,
		ProjectID:      task.ProjectID,
		Owner:          ownerID,
		MemorySettings: defaultChatMemoryRunSettings(),
	})
	if build.Err != nil {
		return provider.Response{}, build.Err
	}
	build = sanitizeImageGenerationBuild(build)
	if build.Engine == nil {
		return provider.Response{}, fmt.Errorf("playground specialist %q did not produce an engine", execution.SpecialistName)
	}
	configureFleetCallbacks(r.app, build.Engine, fleetCallbackRequest{
		RunID:     task.RunID,
		SessionID: sessionID,
		ProjectID: task.ProjectID,
		UserID:    &ownerID,
	})

	runCtx, cancel, timeout := withMaybeTimeout(runCtx, r.app.cfg.AgentRunTimeoutSeconds)
	_ = timeout
	defer cancel()

	started := time.Now()
	output, err := build.Engine.Run(runCtx, renderedPrompt, nil)
	if err != nil {
		return provider.Response{}, err
	}
	return provider.Response{
		Output:       output,
		Tokens:       len(output) / 4,
		Latency:      time.Since(started),
		ProviderName: "specialist:" + execution.SpecialistName,
		Model:        build.Engine.Model,
	}, nil
}

func (r *playgroundSpecialistRunner) prepareTaskContext(ctx context.Context, ownerID int64, task worker.Task, sessionID string) (context.Context, *workspaces.Workspace, error) {
	runCtx := sandbox.WithSessionID(ctx, sessionID)
	projectID := strings.TrimSpace(task.ProjectID)
	if projectID == "" {
		return runCtx, nil, nil
	}
	if r == nil || r.app == nil || r.app.workspaceManager == nil {
		return runCtx, nil, fmt.Errorf("workspace manager unavailable for playground project %q", projectID)
	}
	workspace, err := r.app.workspaceManager.Checkout(runCtx, ownerID, projectID, sessionID)
	if err != nil {
		return runCtx, nil, err
	}
	if strings.TrimSpace(workspace.BaseDir) != "" {
		runCtx = sandbox.WithBaseDir(runCtx, workspace.BaseDir)
		runCtx = sandbox.WithProjectID(runCtx, projectID)
	}
	return runCtx, &workspace, nil
}

func playgroundTaskOwner(ctx context.Context, fallback int64) int64 {
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		return u.ID
	}
	return fallback
}

func playgroundTaskSessionID(task worker.Task) string {
	return fmt.Sprintf("playground:%s:%s:%s", task.RunID, task.Row.ID, task.Variant.ID)
}
