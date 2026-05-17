package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/inputrequest"
	agentmemory "manifold/internal/agent/memory"
	"manifold/internal/fleet"
	"manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

type chatStreamOptions struct {
	Endpoint              string
	IncludeMatrixMessages bool
	KeepAlive             bool
	EmitThoughtSummary    bool
	EmitSummaryEvents     bool
	StructuredErrors      bool
	InheritImagePrompt    bool
	TimeoutSeconds        int
	StoreModel            string
	InitialSummary        *agentmemory.SummaryResult
	Tracer                *agentStreamTracer
}

type chatJSONOptions struct {
	Endpoint              string
	IncludeMatrixMessages bool
	InheritImagePrompt    bool
	TimeoutSeconds        int
	StoreModel            string
}

const defaultImagePromptSize = "1K"

type chatSSEWriter struct {
	w  io.Writer
	fl http.Flusher
	mu sync.Mutex
}

func newChatSSEWriter(w http.ResponseWriter) (*chatSSEWriter, error) {
	fl, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}
	return &chatSSEWriter{w: w, fl: fl}, nil
}

func (s *chatSSEWriter) write(payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.w, "data: %s\n\n", b)
	s.fl.Flush()
}

func (s *chatSSEWriter) writeText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprint(s.w, text)
	s.fl.Flush()
}

type chatTurnCollector struct {
	baseDir      string
	projectID    string
	stream       *chatSSEWriter
	savedImages  []savedImage
	turnMessages []llm.Message
}

func newChatTurnCollector(baseDir, projectID string, stream *chatSSEWriter) *chatTurnCollector {
	return &chatTurnCollector{baseDir: baseDir, projectID: projectID, stream: stream}
}

func (c *chatTurnCollector) attach(eng *agent.Engine) {
	eng.OnTurnMessage = func(msg llm.Message) {
		c.turnMessages = append(c.turnMessages, msg)
	}
	eng.OnAssistant = func(msg llm.Message) {
		if len(msg.Images) == 0 {
			return
		}
		saved := saveGeneratedImages(c.baseDir, msg.Images, c.projectID)
		if len(saved) == 0 {
			return
		}
		c.savedImages = append(c.savedImages, saved...)
		if c.stream == nil {
			return
		}
		for _, img := range saved {
			payload := map[string]any{
				"type":     "image",
				"name":     img.Name,
				"mime":     img.MIME,
				"data_url": img.DataURL,
			}
			if img.URL != "" {
				payload["url"] = img.URL
			}
			if img.RelPath != "" {
				payload["rel_path"] = img.RelPath
			}
			if img.FullPath != "" {
				payload["file_path"] = img.FullPath
			}
			c.stream.write(payload)
		}
	}
}

func (c *chatTurnCollector) resultText(result string) string {
	if len(c.savedImages) == 0 {
		return result
	}
	return appendImageSummary(result, c.savedImages)
}

func buildChatJSONPayload(result string, images []savedImage, ctx context.Context, includeMatrixMessages bool) map[string]any {
	payload := map[string]any{"result": result}
	if len(images) > 0 {
		payload["images"] = append([]savedImage(nil), images...)
	}
	if includeMatrixMessages {
		if outbox, ok := sandbox.MatrixOutboxFromContext(ctx); ok {
			if messages := outbox.Messages(); len(messages) > 0 {
				payload["matrix_messages"] = messages
			}
		}
	}
	return payload
}

func buildChatStreamFinalPayload(result string, ctx context.Context, includeMatrixMessages bool) map[string]any {
	payload := map[string]any{"type": "final", "data": result}
	if includeMatrixMessages {
		if outbox, ok := sandbox.MatrixOutboxFromContext(ctx); ok {
			if messages := outbox.Messages(); len(messages) > 0 {
				payload["matrix_messages"] = messages
			}
		}
	}
	return payload
}

func configureCommonStreamCallbacks(eng *agent.Engine, stream *chatSSEWriter, emitThoughtSummary bool, emitSummaryEvents bool) {
	eng.OnDelta = func(d string) {
		stream.write(map[string]string{"type": "delta", "data": d})
	}
	if emitThoughtSummary {
		eng.OnThoughtSummary = func(summary string) {
			log.Debug().Int("summary_len", len(summary)).Msg("http_handler_thought_summary")
			stream.write(map[string]string{"type": "thought_summary", "data": summary})
		}
	} else {
		eng.OnThoughtSummary = nil
	}
	eng.OnToolStart = func(name string, args []byte, toolID string) {
		payload := map[string]any{"type": "tool_start", "title": "Tool: " + name, "tool_id": toolID, "args": string(args)}
		if name == "agent_call" || name == "ask_agent" {
			payload["agent"] = true
		}
		stream.write(payload)
	}
	eng.OnTool = func(name string, args []byte, result []byte, toolID string) {
		if name == "text_to_speech_chunk" {
			var meta map[string]any
			_ = json.Unmarshal(result, &meta)
			stream.write(map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]})
			return
		}
		payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result), "tool_id": toolID}
		if name == "agent_call" || name == "ask_agent" {
			payload["agent"] = true
		}
		stream.write(payload)
		if name == "text_to_speech" {
			var resp map[string]any
			if err := json.Unmarshal(result, &resp); err == nil {
				if fp, ok := resp["file_path"].(string); ok && fp != "" {
					trimmed := fp
					for _, prefix := range []string{"./", "/"} {
						trimmed = trimPrefixOnce(trimmed, prefix)
					}
					stream.write(map[string]any{"type": "tts_audio", "file_path": fp, "url": "/audio/" + trimmed})
				}
			}
		}
	}
	if emitSummaryEvents {
		eng.OnSummaryTriggered = func(inputTokens, tokenBudget, messageCount, summarizedCount int) {
			stream.write(map[string]any{
				"type":             "summary",
				"input_tokens":     inputTokens,
				"token_budget":     tokenBudget,
				"message_count":    messageCount,
				"summarized_count": summarizedCount,
			})
		}
	} else {
		eng.OnSummaryTriggered = nil
	}
}

func configureFleetCallbacks(app *app, eng *agent.Engine, runID, sessionID, projectID, objectiveID string, userID *int64) {
	if app == nil || app.fleetBus == nil || eng == nil {
		return
	}
	uid := systemUserID
	if userID != nil {
		uid = *userID
	}
	prevToolStart := eng.OnToolStart
	prevTool := eng.OnTool
	prevTracer := eng.AgentTracer
	eng.OnToolStart = func(name string, args []byte, toolID string) {
		if prevToolStart != nil {
			prevToolStart(name, args, toolID)
		}
		app.fleetBus.Publish(fleet.Event{Kind: fleet.EventToolStart, RunID: runID, SessionID: sessionID, ProjectID: projectID, ObjectiveID: objectiveID, ToolID: toolID, UserID: uid, Title: name, Data: map[string]any{"args": string(args)}})
	}
	eng.OnTool = func(name string, args []byte, result []byte, toolID string) {
		if prevTool != nil {
			prevTool(name, args, result, toolID)
		}
		app.fleetBus.Publish(fleet.Event{Kind: fleet.EventToolResult, RunID: runID, SessionID: sessionID, ProjectID: projectID, ObjectiveID: objectiveID, ToolID: toolID, UserID: uid, Title: name, Data: map[string]any{"args": string(args), "result": string(result)}})
	}
	eng.AgentTracer = fleetAgentTracer{bus: app.fleetBus, next: prevTracer, runID: runID, sessionID: sessionID, projectID: projectID, objectiveID: objectiveID, userID: uid}
}

type fleetAgentTracer struct {
	bus         *fleet.Bus
	next        agent.AgentTracer
	runID       string
	sessionID   string
	projectID   string
	objectiveID string
	userID      int64
}

func (t fleetAgentTracer) Trace(ev agent.AgentTrace) {
	if t.next != nil {
		t.next.Trace(ev)
	}
	if t.bus == nil {
		return
	}
	kind := fleet.EventDelegation
	if ev.Type == "agent_error" {
		kind = fleet.EventError
	} else if ev.Type == "agent_final" {
		kind = fleet.EventRunFinished
	}
	t.bus.Publish(fleet.Event{Kind: kind, RunID: t.runID, SessionID: t.sessionID, ProjectID: t.projectID, ObjectiveID: t.objectiveID, Specialist: ev.Agent, Agent: ev.Agent, CallID: ev.CallID, ParentCallID: ev.ParentCallID, ToolID: ev.ToolID, Depth: ev.Depth, UserID: t.userID, Title: ev.Title, Message: ev.Content, Data: map[string]any{"type": ev.Type, "team": ev.Team, "args": ev.Args, "data": ev.Data, "error": ev.Error, "thought_summary": ev.ThoughtSummary}})
}

func trimPrefixOnce(value, prefix string) string {
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return value
}

func logChatRunTimeout(endpoint string, stream bool, dur time.Duration) {
	if dur > 0 {
		log.Debug().Dur("timeout", dur).Str("endpoint", endpoint).Bool("stream", stream).Msg("using configured agent timeout")
		return
	}
	log.Debug().Str("endpoint", endpoint).Bool("stream", stream).Msg("no timeout configured; running until completion")
}

func applyChatImagePrompt(ctx, runCtx context.Context, req chatRunRequest, inherit bool) context.Context {
	if inherit {
		if opts, ok := llm.ImagePromptFromContext(runCtx); ok {
			return llm.WithImagePrompt(ctx, opts)
		}
	}
	if req.Image {
		return llm.WithImagePrompt(ctx, llm.ImagePromptOptions{Size: req.ImageSize})
	}
	return ctx
}

func applyBuildImagePrompt(ctx context.Context, build chatEngineBuildResult) context.Context {
	if !build.ImageGeneration {
		return ctx
	}
	if _, ok := llm.ImagePromptFromContext(ctx); ok {
		return ctx
	}
	return llm.WithImagePrompt(ctx, llm.ImagePromptOptions{Size: defaultImagePromptSize})
}

func chatStoreModel(eng *agent.Engine, override string) string {
	if override != "" {
		return override
	}
	if eng != nil {
		return eng.Model
	}
	return ""
}

func (a *app) executeStreamChat(w http.ResponseWriter, r *http.Request, runCtx context.Context, eng *agent.Engine, req chatRunRequest, history []llm.Message, runID string, userID *int64, checkedOutWorkspace *workspaces.Workspace, opts chatStreamOptions) {
	if req.EphemeralSession {
		defer cleanupEphemeralChatSession(a.chatStore, userID, req.SessionID)
	}
	stream, err := newChatSSEWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var activityCollector *chatActivityCollector
	if a.activityStore != nil && !req.EphemeralSession {
		activityCollector = newChatActivityCollector(req.SessionID, runID, userID, req.AssistantMessageID)
		defer func() {
			if activityCollector == nil {
				return
			}
			activities := activityCollector.Snapshot()
			if len(activities) == 0 {
				return
			}
			if err := a.activityStore.UpsertSessionActivities(r.Context(), userID, req.SessionID, activities); err != nil {
				log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_activities")
			}
		}()
	}
	if opts.Tracer != nil {
		if opts.Tracer.mu == nil {
			opts.Tracer.mu = &stream.mu
		}
		if activityCollector != nil {
			opts.Tracer.onTrace = activityCollector.Handle
		}
		eng.AgentTracer = opts.Tracer
	}
	configureCommonStreamCallbacks(eng, stream, opts.EmitThoughtSummary, opts.EmitSummaryEvents)
	configureFleetCallbacks(a, eng, runID, req.SessionID, req.ProjectID, req.ObjectiveID, userID)
	if a.fleetBus != nil {
		a.fleetBus.Publish(fleet.Event{Kind: fleet.EventRunStarted, RunID: runID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, UserID: derefInputUserID(userID), Message: req.Prompt})
	}
	if opts.InitialSummary != nil && opts.InitialSummary.Triggered {
		stream.write(map[string]any{
			"type":             "summary",
			"input_tokens":     opts.InitialSummary.EstimatedTokens,
			"token_budget":     opts.InitialSummary.TokenBudget,
			"message_count":    opts.InitialSummary.MessageCount,
			"summarized_count": opts.InitialSummary.SummarizedCount,
		})
	}

	seconds := opts.TimeoutSeconds
	if seconds <= 0 {
		seconds = a.cfg.StreamRunTimeoutSeconds
		if seconds <= 0 {
			seconds = a.cfg.AgentRunTimeoutSeconds
		}
	}
	ctx, cancel, dur := withMaybeTimeout(runCtx, seconds)
	defer cancel()
	ctx = applyChatImagePrompt(ctx, runCtx, req, opts.InheritImagePrompt)
	agentName := eng.AgentRole
	if agentName == "" {
		agentName = "orchestrator"
	}
	ctx = inputRequestContext(ctx, newStreamInputRequester(a.activeInputRequestBroker(), stream, req.SessionID, runID, userID, a.fleetBus), inputrequest.RunMetadata{
		Agent: agentName,
		Model: eng.Model,
		Depth: eng.AgentDepth,
	})
	logChatRunTimeout(opts.Endpoint, true, dur)

	if opts.KeepAlive {
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
		defer close(stopKeepalive)
	}

	collector := newChatTurnCollector(sandbox.ResolveBaseDir(ctx, a.cfg.Workdir), req.ProjectID, stream)
	collector.attach(eng)

	result, err := eng.RunStream(ctx, req.Prompt, history)
	if err != nil {
		logStreamContextDone(err, r, opts.Endpoint, req.SessionID, req.ProjectID, "")
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Warn().Err(err).Msg("agent run cancelled")
		} else {
			log.Error().Err(err).Msg("agent run error")
		}
		if opts.StructuredErrors {
			stream.write(map[string]string{"type": "error", "data": "(error) " + err.Error()})
		} else if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
			stream.writeText(fmt.Sprintf("data: %s\n\n", b))
		} else {
			stream.writeText(fmt.Sprintf("data: %q\n\n", "(error)"))
		}
		a.runs.updateStatus(runID, "failed", 0)
		if a.fleetBus != nil {
			a.fleetBus.Publish(fleet.Event{Kind: fleet.EventRunFailed, RunID: runID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, UserID: derefInputUserID(userID), Message: err.Error()})
		}
		a.commitWorkspace(ctx, checkedOutWorkspace)
		return
	}
	result = collector.resultText(result)
	stream.write(buildChatStreamFinalPayload(result, ctx, opts.IncludeMatrixMessages))
	a.runs.updateStatus(runID, "completed", 0)
	if a.fleetBus != nil {
		a.fleetBus.Publish(fleet.Event{Kind: fleet.EventRunFinished, RunID: runID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, UserID: derefInputUserID(userID), Message: result})
	}
	if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, collector.turnMessages, result, req.AssistantMessageID, chatStoreModel(eng, opts.StoreModel)); err != nil {
		log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_stream")
	}
	a.commitWorkspace(ctx, checkedOutWorkspace)
}

func (a *app) executeJSONChat(w http.ResponseWriter, r *http.Request, runCtx context.Context, eng *agent.Engine, req chatRunRequest, history []llm.Message, runID string, userID *int64, checkedOutWorkspace *workspaces.Workspace, opts chatJSONOptions) {
	payload, err := a.executeInternalJSONChat(r.Context(), runCtx, eng, req, history, runID, userID, checkedOutWorkspace, opts)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func (a *app) executeInternalJSONChat(storeCtx, runCtx context.Context, eng *agent.Engine, req chatRunRequest, history []llm.Message, runID string, userID *int64, checkedOutWorkspace *workspaces.Workspace, opts chatJSONOptions) (map[string]any, error) {
	if req.EphemeralSession {
		defer cleanupEphemeralChatSession(a.chatStore, userID, req.SessionID)
	}
	configureFleetCallbacks(a, eng, runID, req.SessionID, req.ProjectID, req.ObjectiveID, userID)
	if a.fleetBus != nil {
		a.fleetBus.Publish(fleet.Event{Kind: fleet.EventRunStarted, RunID: runID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, UserID: derefInputUserID(userID), Message: req.Prompt})
	}
	seconds := opts.TimeoutSeconds
	if seconds <= 0 {
		seconds = a.cfg.AgentRunTimeoutSeconds
	}
	ctx, cancel, dur := withMaybeTimeout(runCtx, seconds)
	defer cancel()
	ctx = applyChatImagePrompt(ctx, runCtx, req, opts.InheritImagePrompt)
	logChatRunTimeout(opts.Endpoint, false, dur)

	collector := newChatTurnCollector(sandbox.ResolveBaseDir(ctx, a.cfg.Workdir), req.ProjectID, nil)
	collector.attach(eng)

	result, err := eng.Run(ctx, req.Prompt, history)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Warn().Err(err).Msg("agent run cancelled")
		} else {
			log.Error().Err(err).Msg("agent run error")
		}
		a.runs.updateStatus(runID, "failed", 0)
		if a.fleetBus != nil {
			a.fleetBus.Publish(fleet.Event{Kind: fleet.EventRunFailed, RunID: runID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, UserID: derefInputUserID(userID), Message: err.Error()})
		}
		a.commitWorkspace(ctx, checkedOutWorkspace)
		return nil, err
	}
	result = collector.resultText(result)
	payload := buildChatJSONPayload(result, collector.savedImages, ctx, opts.IncludeMatrixMessages)
	a.runs.updateStatus(runID, "completed", 0)
	if a.fleetBus != nil {
		a.fleetBus.Publish(fleet.Event{Kind: fleet.EventRunFinished, RunID: runID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, UserID: derefInputUserID(userID), Message: result})
	}
	if err := storeChatTurnWithHistory(storeCtx, a.chatStore, userID, req.SessionID, req.Prompt, collector.turnMessages, result, req.AssistantMessageID, chatStoreModel(eng, opts.StoreModel)); err != nil {
		log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
	}
	a.commitWorkspace(ctx, checkedOutWorkspace)
	return payload, nil
}
