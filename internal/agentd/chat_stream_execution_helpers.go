package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/inputrequest"
	agentmemory "manifold/internal/agent/memory"
	"manifold/internal/fleet"
	"manifold/internal/workspaces"
)

type streamChatErrorRequest struct {
	RunID   string
	Request chatRunRequest
	UserID  *int64
	Options chatStreamOptions
	Err     error
}

type streamChatSuccessRequest struct {
	Context   context.Context
	StoreCtx  context.Context
	RunID     string
	Request   chatRunRequest
	UserID    *int64
	Options   chatStreamOptions
	Result    string
	Workspace *workspaces.Workspace
}

type streamExecutionContextRequest struct {
	RunContext context.Context
	Request    chatRunRequest
	Engine     *agent.Engine
	Stream     *chatSSEWriter
	Options    chatStreamOptions
	RunID      string
	UserID     *int64
}

func (a *app) newStreamActivityCollector(req chatRunRequest, runID string, userID *int64) *chatActivityCollector {
	if a.activityStore == nil || req.EphemeralSession {
		return nil
	}
	return newChatActivityCollector(req.SessionID, runID, userID, req.AssistantMessageID)
}

func (a *app) flushStreamActivities(ctx context.Context, req chatRunRequest, userID *int64, collector *chatActivityCollector) {
	if collector == nil {
		return
	}
	activities := collector.Snapshot()
	if len(activities) == 0 {
		return
	}
	if err := a.activityStore.UpsertSessionActivities(ctx, userID, req.SessionID, activities); err != nil {
		log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_activities")
	}
}

func (a *app) configureStreamExecutionCallbacks(eng *agent.Engine, stream *chatSSEWriter, opts chatStreamOptions, collector *chatActivityCollector, fleetReq fleetCallbackRequest) {
	if opts.Tracer != nil {
		if opts.Tracer.mu == nil {
			opts.Tracer.mu = &stream.mu
		}
		if collector != nil {
			opts.Tracer.onTrace = collector.Handle
		}
		eng.AgentTracer = opts.Tracer
	}
	configureCommonStreamCallbacks(eng, stream, opts.EmitThoughtSummary, opts.EmitSummaryEvents)
	configureFleetCallbacks(a, eng, fleetReq)
}

func (a *app) publishChatRunEvent(kind fleet.EventKind, runID string, req chatRunRequest, userID *int64, message string) {
	if a.fleetBus == nil {
		return
	}
	a.fleetBus.Publish(fleet.Event{
		Kind:        kind,
		RunID:       runID,
		SessionID:   req.SessionID,
		ProjectID:   req.ProjectID,
		ObjectiveID: req.ObjectiveID,
		UserID:      derefInputUserID(userID),
		Message:     message,
	})
}

func writeInitialSummaryEvent(stream *chatSSEWriter, summary *agentmemory.SummaryResult) {
	if summary == nil || !summary.Triggered {
		return
	}
	stream.write(map[string]any{
		"type":             "summary",
		"input_tokens":     summary.EstimatedTokens,
		"token_budget":     summary.TokenBudget,
		"message_count":    summary.MessageCount,
		"summarized_count": summary.SummarizedCount,
	})
}

func (a *app) streamExecutionContext(req streamExecutionContextRequest) (context.Context, context.CancelFunc, time.Duration) {
	ctx, cancel, dur := withMaybeTimeout(req.RunContext, a.streamTimeoutSeconds(req.Options))
	ctx = applyChatImagePrompt(ctx, req.RunContext, req.Request, req.Options.InheritImagePrompt)
	ctx = inputRequestContext(ctx, newStreamInputRequester(a.activeInputRequestBroker(), req.Stream, req.Request.SessionID, req.RunID, req.UserID, a.fleetBus), inputrequest.RunMetadata{
		Agent: streamAgentName(req.Engine),
		Model: req.Engine.Model,
		Depth: req.Engine.AgentDepth,
	})
	return ctx, cancel, dur
}

func (a *app) streamTimeoutSeconds(opts chatStreamOptions) int {
	if opts.TimeoutSeconds > 0 {
		return opts.TimeoutSeconds
	}
	if a.cfg.StreamRunTimeoutSeconds > 0 {
		return a.cfg.StreamRunTimeoutSeconds
	}
	return a.cfg.AgentRunTimeoutSeconds
}

func streamAgentName(eng *agent.Engine) string {
	if eng.AgentRole != "" {
		return eng.AgentRole
	}
	return "orchestrator"
}

func startStreamKeepalive(ctx context.Context, stream *chatSSEWriter, enabled bool) func() {
	if !enabled {
		return func() {}
	}
	stopKeepalive := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopKeepalive:
				return
			case <-ticker.C:
				stream.writeText(": keepalive\n\n")
			}
		}
	}()
	return func() { close(stopKeepalive) }
}

func (a *app) handleStreamChatError(ctx context.Context, r *http.Request, stream *chatSSEWriter, workspace *workspaces.Workspace, req streamChatErrorRequest) {
	logStreamContextDone(req.Err, r, req.Options.Endpoint, req.Request.SessionID, req.Request.ProjectID, "")
	logChatRunError(req.Err)
	writeStreamChatError(stream, req.Err, req.Options.StructuredErrors)
	a.runs.updateStatus(req.RunID, "failed", 0)
	a.publishChatRunEvent(fleet.EventRunFailed, req.RunID, req.Request, req.UserID, req.Err.Error())
	a.commitWorkspace(ctx, workspace)
}

func logChatRunError(err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		log.Warn().Err(err).Msg("agent run cancelled")
		return
	}
	log.Error().Err(err).Msg("agent run error")
}

func writeStreamChatError(stream *chatSSEWriter, err error, structured bool) {
	if structured {
		stream.write(map[string]string{"type": "error", "data": "(error) " + err.Error()})
		return
	}
	if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
		stream.writeText(fmt.Sprintf("data: %s\n\n", b))
		return
	}
	stream.writeText(fmt.Sprintf("data: %q\n\n", "(error)"))
}

func (a *app) finishStreamChatSuccess(stream *chatSSEWriter, collector *chatTurnCollector, eng *agent.Engine, req streamChatSuccessRequest) {
	result := collector.resultText(req.Result)
	stream.write(buildChatStreamFinalPayload(result, req.Context, req.Options.IncludeMatrixMessages))
	a.runs.updateStatus(req.RunID, "completed", 0)
	a.publishChatRunEvent(fleet.EventRunFinished, req.RunID, req.Request, req.UserID, result)
	a.storeStreamChatTurn(req.StoreCtx, collector, eng, req, result)
	a.commitWorkspace(req.Context, req.Workspace)
}

func (a *app) storeStreamChatTurn(ctx context.Context, collector *chatTurnCollector, eng *agent.Engine, req streamChatSuccessRequest, result string) {
	if err := storeChatTurnWithHistory(ctx, a.chatStore, chatTurnHistoryRecord{
		UserID:             req.UserID,
		SessionID:          req.Request.SessionID,
		UserContent:        req.Request.Prompt,
		TurnMessages:       collector.turnMessages,
		FinalContent:       result,
		AssistantMessageID: req.Request.AssistantMessageID,
		Model:              chatStoreModel(eng, req.Options.StoreModel),
	}); err != nil {
		log.Error().Err(err).Str("session", req.Request.SessionID).Msg("store_chat_turn_stream")
	}
}
