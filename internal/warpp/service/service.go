// Package service owns WARP execution orchestration. It deliberately depends
// on callbacks for application-owned tools, LLM access, and workspace setup;
// the HTTP server remains only a composition root.
package service

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/durable"
	persist "manifold/internal/persistence"
	"manifold/internal/tools"
	"manifold/internal/warpp"
	"manifold/internal/warpp/runtime"
	"manifold/internal/warpp/toolnode"
)

// State is the persistence and in-memory run surface required by workflow
// execution. runtime.Runtime satisfies this interface.
type State interface {
	ListWorkflowSummaries(context.Context, int64) ([]runtime.WorkflowSummary, error)
	GetWorkflow(context.Context, int64, string) (warpp.Document, warpp.Canvas, bool, error)
	CreateRun(int64, string, map[string]any) string
	CreateRunWithID(int64, string, string, map[string]any) string
	AppendRunEvent(int64, string, warpp.Event) bool
}

// Deps are the application callbacks needed to execute a workflow.
type Deps struct {
	State             State
	CatalogRegistry   func() tools.Registry
	ExecutionRegistry func() tools.Registry
	Chat              warpp.ChatFunc
	ProjectContext    func(context.Context, int64, string, string) (context.Context, error)
	SystemUserID      int64
	SaveWorkflow      func(context.Context, int64, warpp.Document, warpp.Canvas) (bool, error)
}

// Service validates, resolves, and executes WARP workflows.
type Service struct {
	deps Deps
}

type workflowWriter interface {
	UpsertWorkflow(context.Context, int64, warpp.Document, warpp.Canvas) (persist.WarppWorkflowRecord, bool, error)
	DeleteWorkflow(context.Context, int64, string) (bool, error)
}

type runReader interface {
	GetRunEvents(int64, string) ([]warpp.Event, string, bool)
	SubscribeRun(int64, string) ([]warpp.Event, chan warpp.Event, bool, bool)
	UnsubscribeRun(string, chan warpp.Event)
}

// ListWorkflowSummaries exposes workflow state to thin HTTP adapters without
// leaking the runtime implementation back into agentd.
func (s *Service) ListWorkflowSummaries(ctx context.Context, userID int64) ([]runtime.WorkflowSummary, error) {
	if s == nil || s.deps.State == nil {
		return nil, fmt.Errorf("workflow service unavailable")
	}
	return s.deps.State.ListWorkflowSummaries(ctx, userID)
}

// GetWorkflow returns a saved workflow and its canvas.
func (s *Service) GetWorkflow(ctx context.Context, userID int64, workflowID string) (warpp.Document, warpp.Canvas, bool, error) {
	if s == nil || s.deps.State == nil {
		return warpp.Document{}, warpp.Canvas{}, false, fmt.Errorf("workflow service unavailable")
	}
	return s.deps.State.GetWorkflow(ctx, userID, workflowID)
}

// UpsertWorkflow stores a workflow definition.
func (s *Service) UpsertWorkflow(ctx context.Context, userID int64, doc warpp.Document, canvas warpp.Canvas) (persist.WarppWorkflowRecord, bool, error) {
	writer, ok := s.state().(workflowWriter)
	if !ok {
		return persist.WarppWorkflowRecord{}, false, fmt.Errorf("workflow store unavailable")
	}
	return writer.UpsertWorkflow(ctx, userID, doc, canvas)
}

// DeleteWorkflow removes a workflow definition.
func (s *Service) DeleteWorkflow(ctx context.Context, userID int64, workflowID string) (bool, error) {
	writer, ok := s.state().(workflowWriter)
	if !ok {
		return false, fmt.Errorf("workflow store unavailable")
	}
	return writer.DeleteWorkflow(ctx, userID, workflowID)
}

// CreateRun creates a live workflow run.
func (s *Service) CreateRun(userID int64, workflowID string, input map[string]any) string {
	if s == nil || s.deps.State == nil {
		return ""
	}
	return s.deps.State.CreateRun(userID, workflowID, input)
}

// CreateRunWithID creates a live workflow run with a durable task ID.
func (s *Service) CreateRunWithID(userID int64, workflowID, runID string, input map[string]any) string {
	if s == nil || s.deps.State == nil {
		return ""
	}
	return s.deps.State.CreateRunWithID(userID, workflowID, runID, input)
}

// AppendRunEvent records and publishes an event for a live workflow run.
func (s *Service) AppendRunEvent(userID int64, runID string, event warpp.Event) bool {
	if s == nil || s.deps.State == nil {
		return false
	}
	return s.deps.State.AppendRunEvent(userID, runID, event)
}

// GetRunEvents returns a run snapshot and state.
func (s *Service) GetRunEvents(userID int64, runID string) ([]warpp.Event, string, bool) {
	reader, ok := s.state().(runReader)
	if !ok {
		return nil, "", false
	}
	return reader.GetRunEvents(userID, runID)
}

// SubscribeRun subscribes to a live workflow run.
func (s *Service) SubscribeRun(userID int64, runID string) ([]warpp.Event, chan warpp.Event, bool, bool) {
	reader, ok := s.state().(runReader)
	if !ok {
		return nil, nil, false, false
	}
	return reader.SubscribeRun(userID, runID)
}

// UnsubscribeRun removes a run event subscriber.
func (s *Service) UnsubscribeRun(runID string, ch chan warpp.Event) {
	if reader, ok := s.state().(runReader); ok {
		reader.UnsubscribeRun(runID, ch)
	}
}

func (s *Service) state() State {
	if s == nil {
		return nil
	}
	return s.deps.State
}

// WithDeps returns an execution-configured view that preserves this service's
// package-owned workflow state.
func (s *Service) WithDeps(deps Deps) *Service {
	deps.State = s.state()
	return New(deps)
}

// New constructs a workflow service from composition-root dependencies.
func New(deps Deps) *Service {
	return &Service{deps: deps}
}

// BaseResolver returns the bounded resolver used for built-in manifests and
// workflow manifest derivation.
func BaseResolver() warpp.Resolver {
	return warpp.ChainResolvers(warpp.BuiltinResolver(), toolnode.Resolver(toolnode.Builtin()))
}

// Resolver returns a resolver that includes saved workflows as subflows.
func (s *Service) Resolver(ctx context.Context, userID int64) warpp.Resolver {
	base := BaseResolver()
	var catalog tools.Registry
	if s != nil && s.deps.CatalogRegistry != nil {
		catalog = s.deps.CatalogRegistry()
	}
	dynamic := toolnode.DynamicResolver(catalog, toolnode.CuratedToolNames())
	subflows := func(nodeType string) (warpp.Manifest, bool) {
		if !strings.HasPrefix(nodeType, "flow.") || s == nil || s.deps.State == nil {
			return warpp.Manifest{}, false
		}
		id := strings.TrimPrefix(nodeType, "flow.")
		doc, _, found, err := s.deps.State.GetWorkflow(ctx, userID, id)
		if err != nil || !found {
			return warpp.Manifest{}, false
		}
		manifest, _ := warpp.WorkflowManifest(doc, base)
		return manifest, true
	}
	return warpp.ChainResolvers(base, dynamic, subflows)
}

// Runners returns all built-in, tool, LLM, and subflow runners for a user.
func (s *Service) Runners(ctx context.Context, userID int64) map[string]warpp.NodeRunner {
	runners := warpp.BuiltinRunners()
	if s == nil {
		return runners
	}
	registry := tools.Registry(nil)
	if s.deps.ExecutionRegistry != nil {
		registry = s.deps.ExecutionRegistry()
	}
	for key, runner := range toolnode.Runners(registry, toolnode.Builtin()) {
		runners[key] = runner
	}
	var catalog tools.Registry
	if s.deps.CatalogRegistry != nil {
		catalog = s.deps.CatalogRegistry()
	}
	for key, runner := range toolnode.DynamicRunners(catalog, registry, toolnode.CuratedToolNames()) {
		runners[key] = runner
	}
	if s.deps.Chat != nil {
		runners["llm.generate"] = warpp.LLMRunner(s.deps.Chat)
	}
	s.registerSubflowRunners(ctx, userID, runners)
	return runners
}

// NewEngine assembles an execution engine with durable step and event hooks.
func (s *Service) NewEngine(ctx context.Context, userID int64, runID string, doc warpp.Document) *warpp.Engine {
	if s == nil {
		return nil
	}
	emit := func(ev warpp.Event) {
		if recorded, err := durable.RecordEvent(ctx, "warpp."+string(ev.Type), runtime.EventPayload(ev)); err == nil {
			ev.Sequence = recorded.Sequence
			ev.OccurredAt = recorded.OccurredAt
		}
		if s.deps.State != nil {
			_ = s.deps.State.AppendRunEvent(userID, runID, ev)
		}
	}
	step := func(stepCtx context.Context, key string, fn func(context.Context) (map[string]warpp.Value, error)) (map[string]warpp.Value, error) {
		return durable.Step[map[string]warpp.Value](stepCtx, key, fn)
	}
	return &warpp.Engine{
		Resolve:        s.Resolver(ctx, userID),
		Runners:        s.Runners(ctx, userID),
		Emit:           emit,
		Step:           step,
		MaxConcurrency: doc.Settings.MaxConcurrency,
	}
}

// Execute runs a workflow after applying its project context, if configured.
func (s *Service) Execute(ctx context.Context, userID int64, runID string, doc warpp.Document, input map[string]any) warpp.Result {
	if s == nil {
		return warpp.Result{Status: warpp.StatusFailed, Err: fmt.Errorf("workflow service unavailable")}
	}
	if projectID := strings.TrimSpace(doc.ProjectID); projectID != "" && s.deps.ProjectContext != nil {
		projectCtx, err := s.deps.ProjectContext(ctx, userID, projectID, "warpp-"+doc.ID)
		if err != nil {
			if s.deps.State != nil {
				s.deps.State.AppendRunEvent(userID, runID, warpp.Event{Type: warpp.EventRunStarted, Status: warpp.StatusRunning, Message: "run started"})
				s.deps.State.AppendRunEvent(userID, runID, warpp.Event{Type: warpp.EventRunFailed, Status: warpp.StatusFailed, Error: fmt.Sprintf("project %q: %v", projectID, err), Message: "run failed"})
			}
			return warpp.Result{Status: warpp.StatusFailed, Err: err}
		}
		ctx = projectCtx
	}
	engine := s.NewEngine(ctx, userID, runID, doc)
	if engine == nil {
		return warpp.Result{Status: warpp.StatusFailed, Err: fmt.Errorf("workflow engine unavailable")}
	}
	return engine.Execute(ctx, doc, input)
}

// RunSync validates and executes a document, returning its run id.
func (s *Service) RunSync(ctx context.Context, userID int64, doc warpp.Document, input map[string]any) (warpp.Result, string, error) {
	if s == nil || s.deps.State == nil {
		return warpp.Result{Status: warpp.StatusFailed}, "", fmt.Errorf("workflow service unavailable")
	}
	diagnostics := warpp.Validate(doc, s.Resolver(ctx, userID))
	if warpp.HasErrors(diagnostics) {
		return warpp.Result{Status: warpp.StatusFailed}, "", fmt.Errorf("workflow validation failed: %s", DiagnosticSummary(diagnostics))
	}
	runID := s.deps.State.CreateRun(userID, doc.ID, input)
	result := s.Execute(ctx, userID, runID, doc, input)
	return result, runID, result.Err
}

// ExecuteSync loads and executes a saved workflow, returning its public result.
func (s *Service) ExecuteSync(ctx context.Context, userID int64, workflowID string, input map[string]any) (map[string]any, error) {
	if s == nil || s.deps.State == nil {
		return nil, fmt.Errorf("workflow service unavailable")
	}
	doc, _, found, err := s.deps.State.GetWorkflow(ctx, userID, strings.TrimSpace(workflowID))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("workflow not found")
	}
	result, runID, err := s.RunSync(ctx, userID, doc, input)
	if err != nil {
		return nil, err
	}
	if result.Status == warpp.StatusFailed || result.Status == warpp.StatusCancelled {
		return nil, fmt.Errorf("workflow finished with status %s", result.Status)
	}
	return map[string]any{"ok": true, "run_id": runID, "status": result.Status, "outputs": result.Outputs, "workflow": doc.ID}, nil
}

// RunDurableTask executes the durable task payload used by the warpp queue.
func (s *Service) RunDurableTask(ctx context.Context, params map[string]any) (map[string]any, error) {
	if s == nil || s.deps.State == nil {
		return nil, fmt.Errorf("workflow service unavailable")
	}
	taskContext, ok := durable.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("durable task context unavailable")
	}
	workflowID, _ := params["workflow_id"].(string)
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil, fmt.Errorf("workflow_id required")
	}
	input, _ := params["input"].(map[string]any)
	doc, _, found, err := s.deps.State.GetWorkflow(ctx, taskContext.Task.UserID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("load workflow: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("workflow not found")
	}
	if projectID, _ := params["project_id"].(string); strings.TrimSpace(projectID) != "" {
		doc.ProjectID = strings.TrimSpace(projectID)
	}
	diagnostics := warpp.Validate(doc, s.Resolver(ctx, taskContext.Task.UserID))
	if warpp.HasErrors(diagnostics) {
		return nil, fmt.Errorf("workflow validation failed: %s", DiagnosticSummary(diagnostics))
	}
	s.deps.State.CreateRunWithID(taskContext.Task.UserID, doc.ID, taskContext.Task.ID, input)
	result := s.Execute(ctx, taskContext.Task.UserID, taskContext.Task.ID, doc, input)
	if result.Err != nil {
		return nil, result.Err
	}
	if result.Status == warpp.StatusFailed || result.Status == warpp.StatusCancelled {
		return nil, fmt.Errorf("workflow finished with status %s", result.Status)
	}
	return map[string]any{"ok": true, "run_id": taskContext.Task.ID, "status": result.Status, "outputs": result.Outputs, "workflow": doc.ID}, nil
}

// DiagnosticSummary returns the human-readable error portion of diagnostics.
func DiagnosticSummary(diagnostics []warpp.Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == warpp.SeverityError {
			parts = append(parts, diagnostic.Message)
		}
	}
	if len(parts) == 0 {
		return "invalid workflow"
	}
	return strings.Join(parts, "; ")
}
