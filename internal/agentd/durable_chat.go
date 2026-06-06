package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/inputrequest"
	agentmemory "manifold/internal/agent/memory"
	"manifold/internal/commandexec"
	"manifold/internal/durable"
	"manifold/internal/fleet"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

const (
	durableChatQueue             = "chat"
	durableChatRunTaskName       = "chat.run"
	durableChatWorkerConcurrency = 4
)

type durableChatTaskParams struct {
	Request  chatRunRequest           `json:"request"`
	Target   durableChatTargetPayload `json:"target,omitempty"`
	Endpoint string                   `json:"endpoint,omitempty"`
	Owner    int64                    `json:"owner,omitempty"`
}

type durableChatTargetPayload struct {
	Specialist string `json:"specialist,omitempty"`
	Team       string `json:"team,omitempty"`
}

type durableChatPreparedRun struct {
	exec       chatExecutionRequest
	streamOpts chatStreamOptions
	summary    *agentmemory.SummaryResult
}

type durableChatExecution struct {
	runCtx            context.Context
	task              durable.Task
	prepared          durableChatPreparedRun
	writer            *durableChatEventWriter
	collector         *chatTurnCollector
	activityCollector *chatActivityCollector
}

type durableChatTaskMetadata struct {
	sessionID          string
	userMessageID      string
	assistantMessageID string
}

type durableChatPreparedRequest struct {
	req    chatRunRequest
	owner  int64
	userID *int64
	runCtx context.Context
}

type durableRunCheckpointer struct{}

func (durableRunCheckpointer) Load(ctx context.Context, key string, target any) (bool, error) {
	tc, ok := durable.FromContext(ctx)
	if !ok || tc.Store == nil {
		return false, nil
	}
	raw, found, err := tc.Store.GetCheckpoint(ctx, tc.Task.ID, key)
	if err != nil || !found {
		return found, err
	}
	if len(raw) == 0 {
		return true, nil
	}
	return true, unmarshalDurableCheckpoint(raw, target)
}

func (durableRunCheckpointer) Save(ctx context.Context, key string, value any) error {
	tc, ok := durable.FromContext(ctx)
	if !ok || tc.Store == nil {
		return nil
	}
	raw, err := json.Marshal(durableCheckpointJSONSafe(value))
	if err != nil {
		return err
	}
	_, err = tc.Store.SaveCheckpoint(ctx, tc.Task.ID, key, raw)
	return err
}

type durableCheckpointMessageCompat struct {
	Role             string                        `json:"role"`
	Content          string                        `json:"content"`
	ToolID           string                        `json:"tool_id"`
	ToolCalls        []durableCheckpointToolCompat `json:"tool_calls"`
	Images           []llm.GeneratedImage          `json:"images"`
	Compaction       *llm.CompactionItem           `json:"compaction"`
	ThoughtSignature string                        `json:"thought_signature"`
}

type durableCheckpointToolCompat struct {
	Name             string          `json:"name"`
	Args             json.RawMessage `json:"args"`
	ID               string          `json:"id"`
	ThoughtSignature string          `json:"thought_signature"`
}

func unmarshalDurableCheckpoint(raw []byte, target any) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	switch out := target.(type) {
	case *llm.Message:
		applyDurableCheckpointCompatMessage(raw, out)
	case *[]llm.Message:
		applyDurableCheckpointCompatMessages(raw, out)
	}
	return nil
}

func applyDurableCheckpointCompatMessages(raw []byte, target *[]llm.Message) {
	if target == nil {
		return
	}
	var compat []durableCheckpointMessageCompat
	if err := json.Unmarshal(raw, &compat); err != nil || len(compat) != len(*target) {
		return
	}
	for i := range compat {
		applyDurableCheckpointCompat(&(*target)[i], compat[i])
	}
}

func applyDurableCheckpointCompatMessage(raw []byte, target *llm.Message) {
	if target == nil {
		return
	}
	var compat durableCheckpointMessageCompat
	if err := json.Unmarshal(raw, &compat); err != nil {
		return
	}
	applyDurableCheckpointCompat(target, compat)
}

func applyDurableCheckpointCompat(target *llm.Message, compat durableCheckpointMessageCompat) {
	if target.Role == "" {
		target.Role = compat.Role
	}
	if target.Content == "" {
		target.Content = compat.Content
	}
	if target.ToolID == "" {
		target.ToolID = compat.ToolID
	}
	if len(target.ToolCalls) == 0 && len(compat.ToolCalls) > 0 {
		target.ToolCalls = make([]llm.ToolCall, 0, len(compat.ToolCalls))
		for _, call := range compat.ToolCalls {
			target.ToolCalls = append(target.ToolCalls, llm.ToolCall{
				Name:             call.Name,
				Args:             normalizeDurableCheckpointRawMessage(call.Args),
				ID:               call.ID,
				ThoughtSignature: call.ThoughtSignature,
			})
		}
	}
	if len(target.Images) == 0 && len(compat.Images) > 0 {
		target.Images = compat.Images
	}
	if target.Compaction == nil {
		target.Compaction = compat.Compaction
	}
	if target.ThoughtSignature == "" {
		target.ThoughtSignature = compat.ThoughtSignature
	}
}

func normalizeDurableCheckpointRawMessage(raw json.RawMessage) json.RawMessage {
	if json.Valid(raw) {
		return raw
	}
	return json.RawMessage(`{}`)
}

type durableChatEventWriter struct {
	ctx       context.Context
	taskID    string
	sessionID string
	mu        sync.Mutex
	nextKey   int64
	prior     map[string]struct{}
}

func newDurableChatEventWriter(ctx context.Context, taskID, sessionID string) *durableChatEventWriter {
	writer := &durableChatEventWriter{
		ctx:       ctx,
		taskID:    strings.TrimSpace(taskID),
		sessionID: strings.TrimSpace(sessionID),
		prior:     map[string]struct{}{},
	}
	if tc, ok := durable.FromContext(ctx); ok && tc.Store != nil {
		events, _, _, err := tc.Store.ListTaskEvents(ctx, tc.Task.UserID, tc.Task.ID, 0)
		if err == nil {
			for _, event := range events {
				if event.Sequence > writer.nextKey {
					writer.nextKey = event.Sequence
				}
				if eventPayload, ok := durableChatEventPayload(event); ok {
					if typ, _ := eventPayload["type"].(string); typ != "delta" {
						writer.prior[durableChatEventFingerprint(eventPayload)] = struct{}{}
					}
				}
			}
		}
	}
	return writer
}

func (w *durableChatEventWriter) write(payload any) {
	if w == nil {
		return
	}
	eventPayload := durableChatPayloadMap(durableChatJSONSafe(payload))
	if strings.TrimSpace(w.sessionID) != "" {
		eventPayload["session_id"] = w.sessionID
	}
	if strings.TrimSpace(w.taskID) != "" {
		eventPayload["run_id"] = w.taskID
	}
	fingerprint := durableChatEventFingerprint(eventPayload)
	if typ, _ := eventPayload["type"].(string); typ != "delta" {
		w.mu.Lock()
		if _, ok := w.prior[fingerprint]; ok {
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
	}
	w.mu.Lock()
	w.nextKey++
	eventKey := fmt.Sprintf("event:%012d", w.nextKey)
	w.mu.Unlock()
	name := "chat.event"
	if typ, _ := eventPayload["type"].(string); strings.TrimSpace(typ) != "" {
		name = "chat." + strings.TrimSpace(typ)
	}
	if _, err := durable.RecordEventOnce(w.ctx, eventKey, name, map[string]any{"event": durableChatJSONSafe(eventPayload)}); err != nil {
		log.Warn().Err(err).Str("task_id", w.taskID).Str("event", name).Msg("durable_chat_record_event_failed")
	}
}

func (w *durableChatEventWriter) writeText(string) {}

type durableAgentTracer struct {
	writer  *durableChatEventWriter
	onTrace func(agent.AgentTrace)
}

func (t durableAgentTracer) Trace(ev agent.AgentTrace) {
	if t.onTrace != nil {
		t.onTrace(ev)
	}
	if t.writer == nil {
		return
	}
	t.writer.write(map[string]any{
		"type":            ev.Type,
		"agent":           ev.Agent,
		"team":            ev.Team,
		"model":           ev.Model,
		"call_id":         ev.CallID,
		"parent_call_id":  ev.ParentCallID,
		"depth":           ev.Depth,
		"role":            ev.Role,
		"content":         ev.Content,
		"title":           ev.Title,
		"args":            ev.Args,
		"data":            ev.Data,
		"tool_id":         ev.ToolID,
		"error":           ev.Error,
		"thought_summary": ev.ThoughtSummary,
	})
}

type durableInputRequester struct {
	writer  *durableChatEventWriter
	session string
	runID   string
	userID  *int64
	bus     *fleet.Bus
}

func (r durableInputRequester) RequestInfo(ctx context.Context, req inputrequest.Request) (inputrequest.Response, error) {
	if strings.TrimSpace(req.ToolID) != "" {
		req.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(r.runID+":input:"+strings.TrimSpace(req.ToolID))).String()
	}
	if strings.TrimSpace(req.ID) == "" {
		return inputrequest.Response{}, errors.New("input request id is required")
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	if r.writer != nil {
		r.writer.write(inputRequestEventPayload(req, r.session, r.runID))
	}
	if r.bus != nil {
		r.bus.Publish(fleet.Event{Kind: fleet.EventInputRequest, RunID: r.runID, SessionID: r.session, CallID: req.CallID, ParentCallID: req.ParentCallID, ToolID: req.ToolID, Agent: req.Agent, Depth: req.Depth, UserID: derefInputUserID(r.userID), Data: map[string]any{"request_id": req.ID, "question": req.Question, "reason": req.Reason}})
	}
	resp, err := durable.AwaitEvent[inputrequest.Response](ctx, durableChatInputAnswerEvent(req.ID), 0)
	if err != nil {
		return inputrequest.Response{}, err
	}
	if strings.TrimSpace(resp.RequestID) == "" {
		resp.RequestID = req.ID
	}
	if resp.RespondedAt.IsZero() {
		resp.RespondedAt = time.Now().UTC()
	}
	return resp, nil
}

func durableChatInputAnswerEvent(requestID string) string {
	return "chat.input_answer." + strings.TrimSpace(requestID)
}

func (a *app) chatRunsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !a.durableChatAvailable() {
			http.Error(w, "durable chat backend unavailable", http.StatusServiceUnavailable)
			return
		}
		req, ok := prepareChatTransport(w, r, chatTransportOptions{})
		if !ok {
			return
		}
		state, ok := a.prepareChatHandlerState(w, r, req)
		if !ok {
			return
		}
		req = state.RunRequest
		a.ensureChatRunMessageIDs(&req)
		req.ObjectiveID = a.resolveChatObjectiveID(state.Request.Context(), state.Owner, req)
		target := resolveChatDispatchTarget(r.URL.Query())
		task, err := a.spawnDurableChatRun(state.Request.Context(), durableChatSpawnRequest{
			Request:  req,
			Target:   target,
			Endpoint: "/agent/run",
			Owner:    state.Owner,
			UserID:   state.UserID,
		})
		if err != nil {
			writeDurableError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"run_id":               task.ID,
			"session_id":           req.SessionID,
			"user_message_id":      req.UserMessageID,
			"assistant_message_id": req.AssistantMessageID,
			"status":               task.Status,
		})
	}
}

func (a *app) chatRunDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !a.durableChatAvailable() {
			http.Error(w, "durable chat backend unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		runID, suffix, requestID, ok := parseChatRunPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch suffix {
		case "events":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			after, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("after")), 10, 64)
			a.serveChatRunEvents(w, r, userID, runID, after)
		case "cancel":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := a.cancelDurableTask(r.Context(), userID, runID); err != nil {
				writeDurableError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "status": durable.TaskStatusCancelled})
		case "resume":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			task, retried, err := a.resumeOrAttachDurableChatRun(r.Context(), userID, runID)
			if err != nil {
				writeDurableError(w, err)
				return
			}
			sequences, err := a.durableChatSequenceSummary(r.Context(), userID, runID)
			if err != nil {
				writeDurableError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"run_id":              runID,
				"status":              task.Status,
				"retried":             retried,
				"last_sequence":       sequences.lastSequence,
				"last_retry_sequence": sequences.lastRetrySequence,
			})
		case "input":
			if r.Method != http.MethodPost || requestID == "" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			a.answerDurableChatInput(w, r, userID, runID, requestID)
		default:
			http.NotFound(w, r)
		}
	}
}

func parseChatRunPath(path string) (string, string, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(path, "/api/chat/runs/"), "/")
	if rest == "" || rest == path {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return "", "", "", false
	}
	runID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(runID) == "" {
		return "", "", "", false
	}
	switch parts[1] {
	case "events", "cancel", "resume":
		return runID, parts[1], "", len(parts) == 2
	case "input":
		if len(parts) != 4 || parts[3] != "answer" {
			return "", "", "", false
		}
		requestID, err := url.PathUnescape(parts[2])
		if err != nil || strings.TrimSpace(requestID) == "" {
			return "", "", "", false
		}
		return runID, "input", requestID, true
	default:
		return "", "", "", false
	}
}

func (a *app) answerDurableChatInput(w http.ResponseWriter, r *http.Request, userID int64, runID, requestID string) {
	task, found, err := a.durableStore.GetTask(r.Context(), userID, runID)
	if err != nil {
		writeDurableError(w, err)
		return
	}
	if !found {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	var body struct {
		Answer    string   `json:"answer"`
		ChoiceIDs []string `json:"choice_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	choiceIDs := make([]string, 0, len(body.ChoiceIDs))
	for _, id := range body.ChoiceIDs {
		if id = strings.TrimSpace(id); id != "" {
			choiceIDs = append(choiceIDs, id)
		}
	}
	payload := map[string]any{
		"request_id":   requestID,
		"answer":       strings.TrimSpace(body.Answer),
		"choice_ids":   choiceIDs,
		"responded_at": time.Now().UTC(),
	}
	if _, err := a.durableClient.EmitEvent(r.Context(), userID, durableChatQueue, durableChatInputAnswerEvent(requestID), payload); err != nil {
		writeDurableError(w, err)
		return
	}
	if a.fleetBus != nil {
		metadata := durableChatTaskMetadataFromTask(task)
		a.fleetBus.Publish(fleet.Event{Kind: fleet.EventInputAnswered, RunID: runID, SessionID: metadata.sessionID, UserID: userID, Data: map[string]any{"request_id": requestID, "answer": strings.TrimSpace(body.Answer), "choice_ids": choiceIDs}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "request_id": requestID})
}

type durableChatSpawnRequest struct {
	Request  chatRunRequest
	Target   chatDispatchTarget
	Endpoint string
	Owner    int64
	UserID   *int64
}

func (a *app) spawnDurableChatRun(ctx context.Context, req durableChatSpawnRequest) (durable.Task, error) {
	if !a.durableChatAvailable() {
		return durable.Task{}, durable.ErrNotFound
	}
	requestMap, err := durableChatRequestParams(req.Request)
	if err != nil {
		return durable.Task{}, err
	}
	userID := systemUserID
	if req.UserID != nil {
		userID = *req.UserID
	}
	params := map[string]any{
		"request": requestMap,
		"target": map[string]any{
			"specialist": req.Target.SpecialistName,
			"team":       req.Target.TeamName,
		},
		"endpoint": strings.TrimSpace(req.Endpoint),
		"owner":    req.Owner,
	}
	if err := a.persistDurableChatUserMessage(ctx, req.UserID, req.Request); err != nil {
		return durable.Task{}, err
	}
	spawn, err := a.durableClient.Spawn(ctx, durable.SpawnRequest{
		Queue:          durableChatQueue,
		Name:           durableChatRunTaskName,
		UserID:         userID,
		Params:         params,
		IdempotencyKey: durableChatIdempotencyKey(req.Request),
		RetryPolicy: durable.RetryPolicy{
			MaxAttempts: 1,
			Backoff:     "none",
		},
	})
	if err != nil {
		return durable.Task{}, err
	}
	task, found, err := a.durableStore.GetTask(ctx, userID, spawn.TaskID)
	if err != nil {
		return durable.Task{}, err
	}
	if !found {
		return durable.Task{}, durable.ErrTaskNotFound
	}
	a.runs.createWithID(task.ID, req.Request.Prompt, task.CreatedAt)
	return task, nil
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
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func durableChatIdempotencyKey(req chatRunRequest) string {
	base := strings.TrimSpace(req.AssistantMessageID)
	if base == "" {
		base = strings.TrimSpace(req.UserMessageID)
	}
	if base == "" {
		base = uuid.NewSHA1(uuid.NameSpaceOID, []byte(req.SessionID+":"+req.Prompt)).String()
	}
	return "chat:" + req.SessionID + ":" + base
}

func (a *app) runDurableChatTask(ctx context.Context, params map[string]any) (map[string]any, error) {
	exec, err := a.newDurableChatExecution(ctx, params)
	if err != nil {
		return nil, err
	}
	defer a.flushStreamActivities(context.WithoutCancel(ctx), exec.prepared.exec.RunRequest, exec.prepared.exec.UserID, exec.activityCollector)

	a.startDurableChatExecution(exec)
	stopHeartbeat := startDurableChatHeartbeat(exec.runCtx)
	defer stopHeartbeat()

	result, err := exec.run()
	if err != nil {
		return a.handleDurableChatRunError(exec, err)
	}
	return a.completeDurableChatRun(context.WithoutCancel(ctx), exec, result), nil
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
	prepared.exec.Engine.AgentTracer = durableAgentTracer{writer: writer, onTrace: onTrace}
	configureCommonStreamCallbacks(prepared.exec.Engine, writer, prepared.streamOpts.EmitThoughtSummary, prepared.streamOpts.EmitSummaryEvents)
	configureFleetCallbacks(a, prepared.exec.Engine, fleetCallbackRequest{
		RunID:       task.ID,
		SessionID:   prepared.exec.RunRequest.SessionID,
		ProjectID:   prepared.exec.RunRequest.ProjectID,
		ObjectiveID: prepared.exec.RunRequest.ObjectiveID,
		UserID:      prepared.exec.UserID,
	})
	prepared.exec.Engine.Checkpointer = durableRunCheckpointer{}
	prepared.exec.Engine.UserID = task.UserID
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

func (a *app) completeDurableChatRun(storeCtx context.Context, exec durableChatExecution, result string) map[string]any {
	result = exec.collector.resultText(result)
	req := exec.prepared.exec.RunRequest
	exec.writer.write(buildChatStreamFinalPayload(result, exec.runCtx, exec.prepared.streamOpts.IncludeMatrixMessages))
	a.runs.updateStatus(exec.task.ID, "completed", 0)
	a.publishChatRunEvent(fleet.EventRunFinished, exec.task.ID, req, exec.prepared.exec.UserID, result)
	a.storeStreamChatTurn(storeCtx, exec.collector, exec.prepared.exec.Engine, streamChatSuccessRequest{
		Context:   exec.runCtx,
		StoreCtx:  storeCtx,
		RunID:     exec.task.ID,
		Request:   req,
		UserID:    exec.prepared.exec.UserID,
		Options:   exec.prepared.streamOpts,
		Result:    result,
		Workspace: exec.prepared.exec.CheckedOutWorkspace,
	}, result)
	a.commitWorkspace(exec.runCtx, exec.prepared.exec.CheckedOutWorkspace)
	return durableChatResultPayload(exec.task.ID, req, result)
}

func durableChatResultPayload(runID string, req chatRunRequest, result string) map[string]any {
	return map[string]any{
		"ok":                   true,
		"run_id":               runID,
		"session_id":           req.SessionID,
		"user_message_id":      req.UserMessageID,
		"assistant_message_id": req.AssistantMessageID,
		"result":               result,
	}
}

func durableChatTaskParamsFromMap(params map[string]any) (durableChatTaskParams, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return durableChatTaskParams{}, err
	}
	var out durableChatTaskParams
	if err := json.Unmarshal(raw, &out); err != nil {
		return durableChatTaskParams{}, err
	}
	out.Request.normalize()
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
	history, summary, err := a.loadDurableChatHistory(ctx, preparedReq, build)
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
	if ws == nil || strings.TrimSpace(ws.BaseDir) == "" {
		return ctx
	}
	ctx = sandbox.WithBaseDir(ctx, ws.BaseDir)
	if strings.TrimSpace(req.ProjectID) != "" {
		ctx = sandbox.WithProjectID(ctx, req.ProjectID)
	}
	return ctx
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

func (a *app) loadDurableChatHistory(ctx context.Context, prepared durableChatPreparedRequest, build chatEngineBuildResult) ([]llm.Message, *agentmemory.SummaryResult, error) {
	if build.ImageGeneration {
		return nil, nil, nil
	}
	history, summary, err := a.chatMemory.BuildContextForProvider(ctx, prepared.userID, prepared.req.SessionID, build.Engine.LLM, build.Engine.Model, agentmemory.SummaryPolicy{
		TargetContextWindowTokens:    build.Engine.ContextWindowTokens,
		PlainTextContextWindowTokens: a.cfg.Summary.PlainTextContextWindowTokens,
	})
	if err != nil {
		if err == persist.ErrForbidden {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("load chat history: %w", err)
	}
	build.Engine.SkipInitialSummarization = summary != nil && summary.Triggered
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

func (a *app) serveChatRunEvents(w http.ResponseWriter, r *http.Request, userID int64, runID string, after int64) {
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		seq := after
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			events, status, found, err := a.durableClient.ListEvents(r.Context(), userID, runID, seq)
			if err != nil {
				writeDurableSSE(w, fl, map[string]any{"type": "error", "data": err.Error(), "run_id": runID})
				return
			}
			if !found {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			for _, event := range events {
				if event.Sequence > seq {
					seq = event.Sequence
				}
				writeDurableSSE(w, fl, durableChatStreamPayload(runID, event))
			}
			if durableTaskStatusTerminal(status) {
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
		}
	}
	events, status, found, err := a.durableClient.ListEvents(r.Context(), userID, runID, after)
	if err != nil {
		writeDurableError(w, err)
		return
	}
	if !found {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	payloads := make([]map[string]any, 0, len(events))
	for _, event := range events {
		payloads = append(payloads, durableChatStreamPayload(runID, event))
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "status": status, "events": payloads})
}

func durableChatStreamPayload(runID string, event durable.Event) map[string]any {
	payload, ok := durableChatEventPayload(event)
	if !ok {
		payload = durableChatPayloadMap(event.Payload)
	}
	payload["sequence"] = event.Sequence
	payload["run_id"] = runID
	return payload
}

func durableChatEventPayload(event durable.Event) (map[string]any, bool) {
	raw, ok := event.Payload["event"]
	if !ok {
		return nil, false
	}
	return durableChatPayloadMap(raw), true
}

func durableChatPayloadMap(payload any) map[string]any {
	raw, err := json.Marshal(payload)
	if err != nil {
		return map[string]any{"type": "event"}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"type": "event", "data": string(raw)}
	}
	if out == nil {
		out = map[string]any{"type": "event"}
	}
	return out
}

func durableChatJSONSafe(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case json.RawMessage:
		return durableChatSafeRawMessage(v)
	case []json.RawMessage:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatSafeRawMessage(item))
		}
		return out
	case []llm.Message:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatJSONSafe(item))
		}
		return out
	case llm.Message:
		return map[string]any{
			"role":              v.Role,
			"content":           v.Content,
			"tool_id":           v.ToolID,
			"tool_calls":        durableChatJSONSafe(v.ToolCalls),
			"images":            v.Images,
			"compaction":        v.Compaction,
			"thought_signature": v.ThoughtSignature,
		}
	case []llm.ToolCall:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatJSONSafe(item))
		}
		return out
	case llm.ToolCall:
		return map[string]any{
			"name":              v.Name,
			"args":              durableChatSafeRawMessage(v.Args),
			"id":                v.ID,
			"thought_signature": v.ThoughtSignature,
		}
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = durableChatJSONSafe(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatJSONSafe(item))
		}
		return out
	default:
		return value
	}
}

func durableCheckpointJSONSafe(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case json.RawMessage:
		return durableChatSafeRawMessage(v)
	case []json.RawMessage:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableChatSafeRawMessage(item))
		}
		return out
	case []llm.Message:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableCheckpointJSONSafe(item))
		}
		return out
	case llm.Message:
		return map[string]any{
			"Role":             v.Role,
			"Content":          v.Content,
			"ToolID":           v.ToolID,
			"ToolCalls":        durableCheckpointJSONSafe(v.ToolCalls),
			"Images":           v.Images,
			"Compaction":       v.Compaction,
			"ThoughtSignature": v.ThoughtSignature,
		}
	case []llm.ToolCall:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableCheckpointJSONSafe(item))
		}
		return out
	case llm.ToolCall:
		return map[string]any{
			"Name":             v.Name,
			"Args":             durableChatSafeRawMessage(v.Args),
			"ID":               v.ID,
			"ThoughtSignature": v.ThoughtSignature,
		}
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = durableCheckpointJSONSafe(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, durableCheckpointJSONSafe(item))
		}
		return out
	default:
		return value
	}
}

func durableChatSafeRawMessage(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return map[string]any{}
	}
	if json.Valid([]byte(trimmed)) {
		var out any
		if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
			return out
		}
	}
	return trimmed
}

func durableChatEventFingerprint(payload map[string]any) string {
	clone := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "sequence" {
			continue
		}
		clone[key] = value
	}
	if typ, _ := clone["type"].(string); typ == "input_request" {
		delete(clone, "created_at")
	}
	raw, _ := json.Marshal(clone)
	return string(raw)
}

func (a *app) handleChatSessionRuns(w http.ResponseWriter, r *http.Request, userID *int64, sessionID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.durableChatAvailable() {
		http.Error(w, "durable chat backend unavailable", http.StatusServiceUnavailable)
		return
	}
	if _, err := a.chatStore.GetSession(r.Context(), userID, sessionID); err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "get_chat_session_for_runs")
		return
	}
	durableUserID := systemUserID
	if userID != nil {
		durableUserID = *userID
	}
	tasks, err := a.durableClient.ListTasks(r.Context(), durableUserID, durable.TaskListFilter{Queue: durableChatQueue, Name: durableChatRunTaskName, Limit: 200})
	if err != nil {
		writeDurableError(w, err)
		return
	}
	activeOnly := isTruthyQueryValue(r.URL.Query().Get("active"))
	runs := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		metadata := durableChatTaskMetadataFromTask(task)
		if metadata.sessionID != sessionID {
			continue
		}
		if activeOnly && !durableChatRecoverableStatus(task.Status) {
			continue
		}
		sequences, err := a.durableChatSequenceSummary(r.Context(), durableUserID, task.ID)
		if err != nil {
			writeDurableError(w, err)
			return
		}
		runs = append(runs, map[string]any{
			"run_id":               task.ID,
			"status":               task.Status,
			"session_id":           metadata.sessionID,
			"user_message_id":      metadata.userMessageID,
			"assistant_message_id": metadata.assistantMessageID,
			"created_at":           task.CreatedAt,
			"updated_at":           task.UpdatedAt,
			"error":                task.Error,
			"last_sequence":        sequences.lastSequence,
			"last_retry_sequence":  sequences.lastRetrySequence,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

type durableChatSequenceSummary struct {
	lastSequence      int64
	lastRetrySequence int64
}

func (a *app) durableChatSequenceSummary(ctx context.Context, userID int64, runID string) (durableChatSequenceSummary, error) {
	events, _, found, err := a.durableClient.ListEvents(ctx, userID, runID, 0)
	if err != nil {
		return durableChatSequenceSummary{}, err
	}
	if !found {
		return durableChatSequenceSummary{}, durable.ErrTaskNotFound
	}
	var summary durableChatSequenceSummary
	for _, event := range events {
		if event.Sequence > summary.lastSequence {
			summary.lastSequence = event.Sequence
		}
		if event.Name == "task_retried" && event.Sequence > summary.lastRetrySequence {
			summary.lastRetrySequence = event.Sequence
		}
	}
	return summary, nil
}

func (a *app) resumeOrAttachDurableChatRun(ctx context.Context, userID int64, runID string) (durable.Task, bool, error) {
	if a == nil || a.durableStore == nil || a.durableClient == nil {
		return durable.Task{}, false, durable.ErrNotFound
	}
	task, found, err := a.durableStore.GetTask(ctx, userID, runID)
	if err != nil {
		return durable.Task{}, false, err
	}
	if !found {
		return durable.Task{}, false, durable.ErrTaskNotFound
	}
	switch task.Status {
	case durable.TaskStatusFailed, durable.TaskStatusCancelled:
		retried, err := a.durableClient.Retry(ctx, userID, runID, false)
		return retried, true, err
	default:
		return task, false, nil
	}
}

func durableChatRecoverableStatus(status durable.TaskStatus) bool {
	return status == durable.TaskStatusQueued || status == durable.TaskStatusRunning || status == durable.TaskStatusWaiting || status == durable.TaskStatusFailed
}

func durableChatTaskMetadataFromTask(task durable.Task) durableChatTaskMetadata {
	params, err := durableChatTaskParamsFromMap(task.Params)
	if err != nil {
		return durableChatTaskMetadata{}
	}
	return durableChatTaskMetadata{
		sessionID:          params.Request.SessionID,
		userMessageID:      params.Request.UserMessageID,
		assistantMessageID: params.Request.AssistantMessageID,
	}
}

func (a *app) tryHandleDurableChatCompatibility(w http.ResponseWriter, r *http.Request, req chatRunRequest, state *preparedChatHandlerState, target chatDispatchTarget, endpoint string) bool {
	if !a.durableChatAvailable() || state == nil {
		return false
	}
	a.ensureChatRunMessageIDs(&req)
	task, err := a.spawnDurableChatRun(context.WithoutCancel(state.Request.Context()), durableChatSpawnRequest{
		Request:  req,
		Target:   target,
		Endpoint: endpoint,
		Owner:    state.Owner,
		UserID:   state.UserID,
	})
	if err != nil {
		writeDurableError(w, err)
		return true
	}
	if r.Header.Get("Accept") == "text/event-stream" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		userID := systemUserID
		if state.UserID != nil {
			userID = *state.UserID
		}
		a.serveChatRunEvents(w, r, userID, task.ID, 0)
		return true
	}
	userID := systemUserID
	if state.UserID != nil {
		userID = *state.UserID
	}
	snapshot, err := a.durableClient.AwaitResult(r.Context(), userID, task.ID, 250*time.Millisecond)
	if err != nil {
		writeDurableError(w, err)
		return true
	}
	if snapshot.State != durable.TaskStatusCompleted {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": snapshot.Error, "run_id": task.ID, "status": snapshot.State})
		return true
	}
	var result map[string]any
	if err := snapshot.DecodeResult(&result); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "run_id": task.ID})
		return true
	}
	writeJSON(w, http.StatusOK, result)
	return true
}
