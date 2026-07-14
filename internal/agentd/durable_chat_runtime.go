package agentd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/inputrequest"
	"manifold/internal/agent/memory"
	chatpkg "manifold/internal/agentd/chat"
	"manifold/internal/commandexec"
	"manifold/internal/durable"
	"manifold/internal/fleet"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

type durableChatSpawnRequest struct {
	Request  chatRunRequest
	Target   chatDispatchTarget
	Endpoint string
	Owner    int64
	UserID   *int64
}

func (a *app) spawnDurableChatRun(ctx context.Context, req durableChatSpawnRequest) (durable.Task, error) {
	return chatpkg.SpawnDurableRun(ctx, chatpkg.DurableSpawnDeps{
		Client:             a.durableClient,
		Store:              a.durableStore,
		Registry:           a.durableRegistry,
		Queue:              durableChatQueue,
		TaskName:           durableChatRunTaskName,
		SystemUserID:       systemUserID,
		PersistUserMessage: a.persistDurableChatUserMessage,
		RecordRun: func(task durable.Task, request chatpkg.RunRequest) {
			a.runs.createWithID(task.ID, request.Prompt, task.CreatedAt)
		},
	}, chatpkg.DurableSpawnRequest{Request: req.Request, Target: req.Target, Endpoint: req.Endpoint, Owner: req.Owner, UserID: req.UserID})
}

func (a *app) persistDurableChatUserMessage(ctx context.Context, userID *int64, req chatRunRequest) error {
	if strings.TrimSpace(req.Prompt) == "" || strings.TrimSpace(req.UserMessageID) == "" {
		return nil
	}
	return a.chatStore.AppendMessagesOnce(ctx, userID, req.SessionID, []persist.ChatMessage{{
		ID:        strings.TrimSpace(req.UserMessageID),
		SessionID: req.SessionID,
		Role:      "user",
		Content:   req.Prompt,
		CreatedAt: time.Now().UTC(),
	}}, previewSnippet(req.Prompt), "")
}

func (a *app) durableChatAvailable() bool {
	if a == nil || a.durableClient == nil || a.durableStore == nil || a.durableRegistry == nil {
		return false
	}
	_, ok := a.durableRegistry.Get(durableChatQueue, durableChatRunTaskName)
	return ok
}

func (a *app) ensureChatRunMessageIDs(req *chatRunRequest) {
	if strings.TrimSpace(req.UserMessageID) == "" {
		req.UserMessageID = uuid.NewString()
	}
	if strings.TrimSpace(req.AssistantMessageID) == "" {
		req.AssistantMessageID = uuid.NewString()
	}
}

func durableChatRequestParams(req chatRunRequest) (map[string]any, error) {
	return chatpkg.DurableRequestParams(req)
}

func durableChatIdempotencyKey(req chatRunRequest) string {
	return chatpkg.DurableIdempotencyKey(req.SessionID, req.Prompt, req.AssistantMessageID, req.UserMessageID)
}

func (a *app) runDurableChatTask(ctx context.Context, params map[string]any) (map[string]any, error) {
	return chatpkg.RunDurableTask(ctx, params, func(ctx context.Context, params map[string]any) (chatpkg.DurableExecution, error) {
		exec, err := a.newDurableChatExecution(ctx, params)
		if err != nil {
			return chatpkg.DurableExecution{}, err
		}
		return chatpkg.DurableExecution{
			Context: exec.runCtx,
			Engine:  exec.prepared.exec.Engine,
			Prompt:  exec.prepared.exec.RunRequest.Prompt,
			History: exec.prepared.exec.History,
			Flush: func() {
				a.flushStreamActivities(context.WithoutCancel(ctx), exec.prepared.exec.RunRequest, exec.prepared.exec.UserID, exec.activityCollector)
			},
			Start:          func() { a.startDurableChatExecution(exec) },
			StartHeartbeat: func() func() { return startDurableChatHeartbeat(exec.runCtx) },
			HandleError:    func(err error) (map[string]any, error) { return a.handleDurableChatRunError(exec, err) },
			Complete: func(result string, durationMs int64) map[string]any {
				return a.completeDurableChatRun(context.WithoutCancel(ctx), exec, result, durationMs)
			},
		}, nil
	})
}

func (a *app) newDurableChatExecution(ctx context.Context, params map[string]any) (durableChatExecution, error) {
	tc, ok := durable.FromContext(ctx)
	if !ok {
		return durableChatExecution{}, fmt.Errorf("durable task context unavailable")
	}
	parsed, err := durableChatTaskParamsFromMap(params)
	if err != nil {
		return durableChatExecution{}, err
	}
	prepared, err := a.prepareDurableChatRun(ctx, tc.Task, parsed)
	if err != nil {
		return durableChatExecution{}, err
	}
	writer := newDurableChatEventWriter(ctx, tc.Task.ID, prepared.exec.RunRequest.SessionID)
	activityCollector := a.newStreamActivityCollector(prepared.exec.RunRequest, tc.Task.ID, prepared.exec.UserID)
	a.configureDurableChatEngine(tc.Task, prepared, writer, activityCollector)
	runCtx := a.durableChatRunContext(tc.Task.ID, prepared, writer)
	collector := newChatTurnCollector(sandbox.ResolveBaseDir(runCtx, a.cfg.Workdir), prepared.exec.RunRequest.ProjectID, writer)
	collector.attach(prepared.exec.Engine)
	return durableChatExecution{runCtx: runCtx, task: tc.Task, prepared: prepared, writer: writer, collector: collector, activityCollector: activityCollector}, nil
}

func (a *app) configureDurableChatEngine(task durable.Task, prepared durableChatPreparedRun, writer *durableChatEventWriter, activityCollector *chatActivityCollector) {
	var onTrace func(agent.AgentTrace)
	if activityCollector != nil {
		onTrace = activityCollector.Handle
	}
	attachChatEngineRuntime(a.chatDeps(), chatEngineAttachConfig{
		Engine:             prepared.exec.Engine,
		StreamWriter:       writer,
		EmitThoughtSummary: prepared.streamOpts.EmitThoughtSummary,
		EmitSummaryEvents:  prepared.streamOpts.EmitSummaryEvents,
		Tracer:             durableAgentTracer{writer: writer, onTrace: onTrace},
		Checkpointer:       durableRunCheckpointer{},
		UserID:             task.UserID,
		SetUserID:          true,
		Capture: llmRequestCaptureConfig{
			SessionID:           prepared.exec.RunRequest.SessionID,
			UserID:              prepared.exec.UserID,
			RunID:               task.ID,
			MessageID:           prepared.exec.RunRequest.AssistantMessageID,
			ParentUserMessageID: prepared.exec.RunRequest.UserMessageID,
			SpecialistID:        streamAgentName(prepared.exec.Engine),
		},
		Fleet: fleetCallbackRequest{
			RunID:       task.ID,
			SessionID:   prepared.exec.RunRequest.SessionID,
			ProjectID:   prepared.exec.RunRequest.ProjectID,
			ObjectiveID: prepared.exec.RunRequest.ObjectiveID,
			UserID:      prepared.exec.UserID,
		},
	})
}

func (a *app) durableChatRunContext(runID string, prepared durableChatPreparedRun, writer *durableChatEventWriter) context.Context {
	runCtx := prepared.exec.RunContext
	runCtx = applyChatImagePrompt(runCtx, runCtx, prepared.exec.RunRequest, prepared.streamOpts.InheritImagePrompt)
	runCtx = inputRequestContext(runCtx, durableInputRequester{
		writer:  writer,
		session: prepared.exec.RunRequest.SessionID,
		runID:   runID,
		userID:  prepared.exec.UserID,
		bus:     a.fleetBus,
	}, inputrequest.RunMetadata{
		Agent: streamAgentName(prepared.exec.Engine),
		Model: prepared.exec.Engine.Model,
		Depth: prepared.exec.Engine.AgentDepth,
	})
	runCtx = commandexec.WithCommandSessionScope(runCtx, commandPolicySessionScope(prepared.exec.UserID, prepared.exec.RunRequest.SessionID))
	return commandexec.WithApprovalController(runCtx, commandPolicyApprovalController{app: a})
}

func (a *app) startDurableChatExecution(exec durableChatExecution) {
	req := exec.prepared.exec.RunRequest
	a.publishChatRunEvent(fleet.EventRunStarted, exec.task.ID, req, exec.prepared.exec.UserID, req.Prompt)
	exec.writer.write(map[string]any{"type": "run_started", "run_id": exec.task.ID, "session_id": exec.prepared.exec.RunRequest.SessionID})
	writeInitialSummaryEvent(exec.writer, exec.prepared.summary)
}

func (exec durableChatExecution) run() (string, error) {
	req := exec.prepared.exec.RunRequest
	return exec.prepared.exec.Engine.RunStream(exec.runCtx, req.Prompt, exec.prepared.exec.History)
}

func (a *app) handleDurableChatRunError(exec durableChatExecution, err error) (map[string]any, error) {
	if errors.Is(err, durable.ErrSuspended) {
		a.runs.updateStatus(exec.task.ID, "waiting", 0)
		return nil, err
	}
	logChatRunError(err)
	exec.writer.write(map[string]any{"type": "error", "data": "(error) " + err.Error(), "recoverable": true})
	a.runs.updateStatus(exec.task.ID, "failed", 0)
	req := exec.prepared.exec.RunRequest
	a.publishChatRunEvent(fleet.EventRunFailed, exec.task.ID, req, exec.prepared.exec.UserID, err.Error())
	a.commitWorkspace(exec.runCtx, exec.prepared.exec.CheckedOutWorkspace)
	return nil, err
}

func (a *app) completeDurableChatRun(storeCtx context.Context, exec durableChatExecution, result string, durationMs int64) map[string]any {
	result = exec.collector.resultText(result)
	req := exec.prepared.exec.RunRequest
	exec.writer.write(buildChatStreamFinalPayload(result, exec.runCtx, exec.prepared.streamOpts.IncludeMatrixMessages, durationMs))
	a.runs.updateStatus(exec.task.ID, "completed", 0)
	a.publishChatRunEvent(fleet.EventRunFinished, exec.task.ID, req, exec.prepared.exec.UserID, result)
	a.storeStreamChatTurn(storeCtx, exec.collector, exec.prepared.exec.Engine, streamChatSuccessRequest{
		Context:    exec.runCtx,
		StoreCtx:   storeCtx,
		RunID:      exec.task.ID,
		Request:    req,
		UserID:     exec.prepared.exec.UserID,
		Options:    exec.prepared.streamOpts,
		Result:     result,
		DurationMs: durationMs,
		Workspace:  exec.prepared.exec.CheckedOutWorkspace,
	}, result)
	a.commitWorkspace(exec.runCtx, exec.prepared.exec.CheckedOutWorkspace)
	return durableChatResultPayload(exec.task.ID, req, result, durationMs)
}

func durableChatResultPayload(runID string, req chatRunRequest, result string, durationMs int64) map[string]any {
	return chatpkg.DurableResultPayload(runID, req.SessionID, req.UserMessageID, req.AssistantMessageID, result, durationMs)
}

func durableChatTaskParamsFromMap(params map[string]any) (durableChatTaskParams, error) {
	var out durableChatTaskParams
	if err := chatpkg.DecodeDurableParams(params, &out); err != nil {
		return durableChatTaskParams{}, err
	}
	out.Request.Normalize()
	return out, nil
}

func (a *app) prepareDurableChatRun(ctx context.Context, task durable.Task, params durableChatTaskParams) (durableChatPreparedRun, error) {
	preparedReq := a.prepareDurableChatRequest(ctx, task, params)
	checkedOutWorkspace, err := a.checkoutDurableChatWorkspace(preparedReq.runCtx, task.UserID, preparedReq.req)
	if err != nil {
		return durableChatPreparedRun{}, err
	}
	preparedReq.runCtx = withCheckedOutWorkspaceContext(preparedReq.runCtx, preparedReq.req, checkedOutWorkspace)
	memorySettings := chatMemorySettingsFromRunRequest(preparedReq.req)
	descriptor := a.durableChatDescriptor(params, preparedReq, memorySettings, checkedOutWorkspace)
	build, runCtx, err := durableChatBuild(preparedReq, descriptor, memorySettings)
	if err != nil {
		return durableChatPreparedRun{}, err
	}
	history, summary, err := a.loadDurableChatHistory(ctx, preparedReq, &build)
	if err != nil {
		return durableChatPreparedRun{}, err
	}
	streamOpts := descriptor.Stream
	if streamOpts.StoreModel == "" {
		streamOpts.StoreModel = build.ModelLabel
	}
	return durableChatPreparedRun{
		exec: chatExecutionRequest{
			RunContext:          runCtx,
			Engine:              build.Engine,
			RunRequest:          preparedReq.req,
			History:             history,
			RunID:               task.ID,
			UserID:              preparedReq.userID,
			CheckedOutWorkspace: checkedOutWorkspace,
		},
		streamOpts: streamOpts,
		summary:    summary,
	}, nil
}

func (a *app) prepareDurableChatRequest(ctx context.Context, task durable.Task, params durableChatTaskParams) durableChatPreparedRequest {
	req := params.Request
	a.ensureChatRunMessageIDs(&req)
	owner := params.Owner
	if owner == 0 {
		owner = task.UserID
	}
	if strings.TrimSpace(req.ObjectiveID) == "" {
		req.ObjectiveID = a.resolveChatObjectiveID(ctx, owner, req)
	}
	runCtx := llm.WithUserID(ctx, owner)
	runCtx = sandbox.WithSessionID(runCtx, req.SessionID)
	if strings.TrimSpace(req.ObjectiveID) != "" {
		runCtx = sandbox.WithObjectiveID(runCtx, req.ObjectiveID)
	}
	return durableChatPreparedRequest{req: req, owner: owner, userID: a.chatUserIDPtr(task.UserID), runCtx: runCtx}
}

func (a *app) checkoutDurableChatWorkspace(ctx context.Context, taskUserID int64, req chatRunRequest) (*workspaces.Workspace, error) {
	if strings.TrimSpace(req.ProjectID) == "" || a.workspaceManager == nil {
		return nil, nil
	}
	ws, err := a.workspaceManager.Checkout(ctx, taskUserID, req.ProjectID, req.SessionID)
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

func withCheckedOutWorkspaceContext(ctx context.Context, req chatRunRequest, ws *workspaces.Workspace) context.Context {
	return chatpkg.WithCheckedOutWorkspaceContext(ctx, req.ProjectID, ws)
}

func (a *app) durableChatDescriptor(params durableChatTaskParams, prepared durableChatPreparedRequest, memorySettings chatMemoryRunSettings, workspace *workspaces.Workspace) chatTargetDescriptor {
	target := chatDispatchTarget{SpecialistName: params.Target.Specialist, TeamName: params.Target.Team}
	descriptor, ok := a.describeChatTarget(chatTargetDescribeRequest{
		Target:               target,
		SessionID:            prepared.req.SessionID,
		ProjectID:            prepared.req.ProjectID,
		ObjectiveID:          prepared.req.ObjectiveID,
		SystemPromptOverride: prepared.req.SystemPrompt,
		Owner:                prepared.owner,
		MemorySettings:       memorySettings,
	})
	if ok {
		if descriptor.RunContext == nil {
			descriptor.RunContext = prepared.runCtx
		}
		return descriptor
	}
	if strings.TrimSpace(params.Endpoint) == "/api/prompt" {
		descriptor = a.promptOrchestratorDescriptor(prepared.runCtx, prepared.owner, prepared.req, workspace)
	} else {
		descriptor = a.agentRunOrchestratorDescriptor(prepared.runCtx, prepared.owner, prepared.req, workspace)
	}
	if descriptor.RunContext == nil {
		descriptor.RunContext = prepared.runCtx
	}
	return descriptor
}

func durableChatBuild(prepared durableChatPreparedRequest, descriptor chatTargetDescriptor, memorySettings chatMemoryRunSettings) (chatEngineBuildResult, context.Context, error) {
	if descriptor.RunContext == nil {
		descriptor.RunContext = prepared.runCtx
	}
	build := descriptor.Build(descriptor.RunContext)
	if build.Err != nil {
		return chatEngineBuildResult{}, nil, build.Err
	}
	build = sanitizeImageGenerationBuild(build)
	runCtx := chatTargetRunContext(&http.Request{URL: &url.URL{}, Method: http.MethodPost}, dispatchOptionsFromDescriptor(descriptor, chatTargetDispatchRequest{
		Prompt:             prepared.req.Prompt,
		SessionID:          prepared.req.SessionID,
		UserMessageID:      prepared.req.UserMessageID,
		AssistantMessageID: prepared.req.AssistantMessageID,
		ProjectID:          prepared.req.ProjectID,
		ObjectiveID:        prepared.req.ObjectiveID,
		EphemeralSession:   prepared.req.EphemeralSession,
		UserID:             prepared.userID,
		MemorySettings:     memorySettings,
	}), build)
	return build, runCtx, nil
}

func (a *app) loadDurableChatHistory(ctx context.Context, prepared durableChatPreparedRequest, build *chatEngineBuildResult) ([]llm.Message, *memory.SummaryResult, error) {
	history, summary, err := a.prepareChatHistoryForBuild(ctx, prepared.userID, prepared.req.SessionID, build)
	if err != nil {
		if err == persist.ErrForbidden {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("load chat history: %w", err)
	}
	return trimCurrentDurableUserMessage(history, prepared.req.Prompt), summary, nil
}

func trimCurrentDurableUserMessage(history []llm.Message, prompt string) []llm.Message {
	if len(history) == 0 || strings.TrimSpace(prompt) == "" {
		return history
	}
	idx := len(history) - 1
	last := history[idx]
	if last.Role != "user" || strings.TrimSpace(last.Content) != strings.TrimSpace(prompt) {
		return history
	}
	out := make([]llm.Message, idx)
	copy(out, history[:idx])
	return out
}

func (a *app) chatUserIDPtr(userID int64) *int64 {
	if a != nil && a.cfg != nil && a.cfg.Auth.Enabled {
		out := userID
		return &out
	}
	return nil
}

func startDurableChatHeartbeat(ctx context.Context) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				if err := durable.Heartbeat(ctx, 5*time.Minute); err != nil {
					log.Warn().Err(err).Msg("durable_chat_heartbeat_failed")
				}
			}
		}
	}()
	return func() { close(stop) }
}
