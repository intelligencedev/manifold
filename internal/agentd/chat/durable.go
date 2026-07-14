package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/agent"
	"manifold/internal/durable"
	"manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

// DurableExecution is the protocol-neutral execution shell for one durable
// chat task. The composition root supplies lifecycle callbacks for its own
// persistence and event concerns; this package owns the run sequencing.
type DurableExecution struct {
	Context        context.Context
	Engine         *agent.Engine
	Prompt         string
	History        []llm.Message
	Flush          func()
	Start          func()
	StartHeartbeat func() func()
	HandleError    func(error) (map[string]any, error)
	Complete       func(string, int64) map[string]any
}

// RunDurableTask runs the shared durable chat lifecycle. In particular, the
// stream entry point is owned here so durable execution cannot drift from the
// chat service boundary.
func RunDurableTask(ctx context.Context, params map[string]any, build func(context.Context, map[string]any) (DurableExecution, error)) (map[string]any, error) {
	exec, err := build(ctx, params)
	if err != nil {
		return nil, err
	}
	if exec.Flush != nil {
		defer exec.Flush()
	}
	if exec.Start != nil {
		exec.Start()
	}
	stopHeartbeat := func() {}
	if exec.StartHeartbeat != nil {
		stopHeartbeat = exec.StartHeartbeat()
	}
	defer stopHeartbeat()
	if exec.Engine == nil {
		err := errServiceUnavailable
		if exec.HandleError != nil {
			return exec.HandleError(err)
		}
		return nil, err
	}
	startedAt := time.Now()
	result, err := exec.Engine.RunStream(exec.Context, exec.Prompt, exec.History)
	durationMs := time.Since(startedAt).Milliseconds()
	if err != nil {
		if exec.HandleError != nil {
			return exec.HandleError(err)
		}
		return nil, err
	}
	if exec.Complete == nil {
		return map[string]any{"ok": true, "result": result, "durationMs": durationMs}, nil
	}
	return exec.Complete(result, durationMs), nil
}

// DurableSpawnRequest describes a live chat request to enqueue on the durable
// chat worker. It deliberately contains no HTTP server types.
type DurableSpawnRequest struct {
	Request  RunRequest
	Target   DispatchTarget
	Endpoint string
	Owner    int64
	UserID   *int64
}

// DurableSpawnDeps are the infrastructure hooks needed to enqueue a chat
// task. Persistence of the user message and transient run bookkeeping remain
// composition-root callbacks because they are HTTP application concerns.
type DurableSpawnDeps struct {
	Client             *durable.Client
	Store              durable.Store
	Registry           *durable.Registry
	Queue              string
	TaskName           string
	SystemUserID       int64
	PersistUserMessage func(context.Context, *int64, RunRequest) error
	RecordRun          func(durable.Task, RunRequest)
}

// SpawnDurableRun persists the inbound message, enqueues the chat task, and
// returns the authoritative durable task record.
func SpawnDurableRun(ctx context.Context, deps DurableSpawnDeps, request DurableSpawnRequest) (durable.Task, error) {
	if deps.Client == nil || deps.Store == nil || deps.Registry == nil {
		return durable.Task{}, durable.ErrNotFound
	}
	if _, ok := deps.Registry.Get(deps.Queue, deps.TaskName); !ok {
		return durable.Task{}, durable.ErrNotFound
	}
	requestMap, err := DurableRequestParams(request.Request)
	if err != nil {
		return durable.Task{}, err
	}
	userID := deps.SystemUserID
	if request.UserID != nil {
		userID = *request.UserID
	}
	if deps.PersistUserMessage != nil {
		if err := deps.PersistUserMessage(ctx, request.UserID, request.Request); err != nil {
			return durable.Task{}, err
		}
	}
	spawn, err := deps.Client.Spawn(ctx, durable.SpawnRequest{
		Queue:  deps.Queue,
		Name:   deps.TaskName,
		UserID: userID,
		Params: map[string]any{"request": requestMap, "target": map[string]any{
			"specialist": request.Target.SpecialistName,
			"team":       request.Target.TeamName,
		}, "endpoint": strings.TrimSpace(request.Endpoint), "owner": request.Owner},
		IdempotencyKey: DurableIdempotencyKey(request.Request.SessionID, request.Request.Prompt, request.Request.AssistantMessageID, request.Request.UserMessageID),
		RetryPolicy:    durable.RetryPolicy{MaxAttempts: 1, Backoff: "none"},
	})
	if err != nil {
		return durable.Task{}, err
	}
	task, found, err := deps.Store.GetTask(ctx, userID, spawn.TaskID)
	if err != nil {
		return durable.Task{}, err
	}
	if !found {
		return durable.Task{}, durable.ErrTaskNotFound
	}
	if deps.RecordRun != nil {
		deps.RecordRun(task, request.Request)
	}
	return task, nil
}

// DurableRequestParams converts a JSON-marshalable chat request into durable
// task parameters without coupling the chat package to an HTTP request type.
func DurableRequestParams(request any) (map[string]any, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	return params, nil
}

// DecodeDurableParams decodes task parameters into a caller-owned request
// type, preserving the durable queue's JSON representation.
func DecodeDurableParams(params map[string]any, target any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

// DurableIdempotencyKey returns the stable idempotency key for a chat run.
func DurableIdempotencyKey(sessionID, prompt, assistantMessageID, userMessageID string) string {
	base := strings.TrimSpace(assistantMessageID)
	if base == "" {
		base = strings.TrimSpace(userMessageID)
	}
	if base == "" {
		base = uuid.NewSHA1(uuid.NameSpaceOID, []byte(sessionID+":"+prompt)).String()
	}
	return "chat:" + sessionID + ":" + base
}

// DurableResultPayload creates the stable result shape returned by chat.run.
func DurableResultPayload(runID, sessionID, userMessageID, assistantMessageID, result string, durationMs int64) map[string]any {
	return map[string]any{
		"ok":                   true,
		"run_id":               runID,
		"session_id":           sessionID,
		"user_message_id":      userMessageID,
		"assistant_message_id": assistantMessageID,
		"result":               result,
		"durationMs":           durationMs,
	}
}

// WithCheckedOutWorkspaceContext scopes a durable run to its checked-out
// project workspace.
func WithCheckedOutWorkspaceContext(ctx context.Context, projectID string, workspace *workspaces.Workspace) context.Context {
	if workspace == nil || strings.TrimSpace(workspace.BaseDir) == "" {
		return ctx
	}
	ctx = sandbox.WithBaseDir(ctx, workspace.BaseDir)
	if strings.TrimSpace(projectID) != "" {
		ctx = sandbox.WithProjectID(ctx, projectID)
	}
	return ctx
}
