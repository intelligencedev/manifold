package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	agentmemory "manifold/internal/agent/memory"
	chatpkg "manifold/internal/agentd/chat"
	"manifold/internal/fleet"
	"manifold/internal/llm"
	"manifold/internal/sandbox"
	"manifold/internal/tools"
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

type chatEventWriter interface {
	write(payload any)
	writeText(text string)
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

func (s *chatSSEWriter) Write(payload any) { s.write(payload) }

func (s *chatSSEWriter) WriteText(text string) { s.writeText(text) }

type chatTurnCollector struct {
	baseDir      string
	projectID    string
	stream       chatEventWriter
	savedImages  []savedImage
	savedVideos  []savedVideo
	turnMessages []llm.Message
}

func newChatTurnCollector(baseDir, projectID string, stream chatEventWriter) *chatTurnCollector {
	return &chatTurnCollector{baseDir: baseDir, projectID: projectID, stream: stream}
}

func (c *chatTurnCollector) attach(eng *agent.Engine) {
	eng.OnTurnMessage = func(msg llm.Message) {
		c.turnMessages = append(c.turnMessages, msg)
	}
	eng.OnAssistant = func(msg llm.Message) {
		if len(msg.Images) > 0 {
			saved := saveGeneratedImages(c.baseDir, msg.Images, c.projectID)
			if len(saved) > 0 {
				c.savedImages = append(c.savedImages, saved...)
				if c.stream != nil {
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
		}
		if len(msg.Videos) > 0 {
			saved := saveGeneratedVideos(c.baseDir, msg.Videos, c.projectID)
			if len(saved) > 0 {
				c.savedVideos = append(c.savedVideos, saved...)
				if c.stream != nil {
					for _, video := range saved {
						payload := map[string]any{
							"type": "video",
							"name": video.Name,
							"mime": video.MIME,
						}
						if video.DataURL != "" {
							payload["data_url"] = video.DataURL
						}
						if video.URL != "" {
							payload["url"] = video.URL
						}
						if video.RelPath != "" {
							payload["rel_path"] = video.RelPath
						}
						if video.FullPath != "" {
							payload["file_path"] = video.FullPath
						}
						c.stream.write(payload)
					}
				}
			}
		}
	}
}

func (c *chatTurnCollector) resultText(result string) string {
	if len(c.savedImages) > 0 {
		result = appendImageSummary(result, c.savedImages)
	}
	if len(c.savedVideos) > 0 {
		result = appendVideoSummary(result, c.savedVideos)
	}
	return result
}

func buildChatJSONPayload(result string, images []savedImage, videos []savedVideo, ctx context.Context, includeMatrixMessages bool) map[string]any {
	payload := map[string]any{"result": result}
	if len(images) > 0 {
		payload["images"] = append([]savedImage(nil), images...)
	}
	if len(videos) > 0 {
		payload["videos"] = append([]savedVideo(nil), videos...)
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

func buildChatStreamFinalPayload(result string, ctx context.Context, includeMatrixMessages bool, durationMs ...int64) map[string]any {
	payload := map[string]any{"type": "final", "data": result}
	if len(durationMs) > 0 && durationMs[0] >= 0 {
		payload["durationMs"] = durationMs[0]
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

func configureCommonStreamCallbacks(eng *agent.Engine, stream chatEventWriter, emitThoughtSummary bool, emitSummaryEvents bool) {
	eng.OnDelta = func(d string) {
		stream.write(map[string]string{"type": "delta", "data": d})
	}
	eng.OnStreamRollback = func(chars int) {
		stream.write(map[string]any{"type": "delta_rollback", "count": chars})
	}
	eng.OnMemoryContext = func(block agentmemory.ContextBlock, diag agentmemory.Diagnostics) {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			return
		}
		stream.write(buildChatMemoryContextPayload(text, block, diag))
	}
	eng.OnContextMetrics = func(metrics agent.ContextMetrics) {
		stream.write(buildChatContextMetricsPayload(metrics))
	}
	if emitThoughtSummary {
		eng.OnThoughtSummary = func(summary string) {
			log.Debug().Int("summary_len", len(summary)).Msg("http_handler_thought_summary")
			stream.write(map[string]string{"type": "thought_summary", "data": summary})
		}
	} else {
		eng.OnThoughtSummary = nil
	}
	eng.OnToolStartWithTitle = func(name string, title string, args []byte, toolID string) {
		activityTitle := toolActivityTitle(name, title)
		payload := map[string]any{"type": "tool_start", "title": activityTitle, "tool_title": activityTitle, "tool_name": name, "tool_id": toolID, "args": string(args)}
		if name == "agent_call" || name == "ask_agent" {
			payload["agent"] = true
		}
		stream.write(payload)
	}
	eng.OnToolWithTitle = func(name string, title string, args []byte, result []byte, toolID string) {
		if name == "text_to_speech_chunk" {
			var meta map[string]any
			_ = json.Unmarshal(result, &meta)
			stream.write(map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]})
			return
		}
		activityTitle := toolActivityTitle(name, title)
		payload := map[string]any{"type": "tool_result", "title": activityTitle, "tool_title": activityTitle, "tool_name": name, "data": string(result), "tool_id": toolID}
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
			contextWindow, reserveTokens := summaryEventBudgetFromEngine(eng, tokenBudget)
			stream.write(map[string]any{
				"type":             "summary",
				"input_tokens":     inputTokens,
				"token_budget":     tokenBudget,
				"context_window":   contextWindow,
				"reserve_tokens":   reserveTokens,
				"message_count":    messageCount,
				"summarized_count": summarizedCount,
			})
		}
	} else {
		eng.OnSummaryTriggered = nil
	}
}

func summaryEventBudgetFromEngine(eng *agent.Engine, tokenBudget int) (int, int) {
	reserveTokens := 25_000
	contextWindow := tokenBudget
	if eng != nil {
		if eng.SummaryReserveBufferTokens > 0 {
			reserveTokens = eng.SummaryReserveBufferTokens
		}
		contextWindow = eng.ContextWindowTokens
		if contextWindow <= 0 && strings.TrimSpace(eng.Model) != "" {
			if size, _ := llm.ContextSize(eng.Model); size > 0 {
				contextWindow = size
			}
		}
	}
	if contextWindow <= 0 {
		contextWindow = tokenBudget + reserveTokens
	}
	if contextWindow < tokenBudget {
		contextWindow = tokenBudget + reserveTokens
	}
	if contextWindow > tokenBudget {
		reserveTokens = contextWindow - tokenBudget
	}
	return contextWindow, reserveTokens
}

type chatMemoryContextPayload struct {
	Type          string                           `json:"type"`
	Data          string                           `json:"data"`
	TokenEstimate int                              `json:"token_estimate,omitempty"`
	Truncated     bool                             `json:"truncated,omitempty"`
	DurationMs    int64                            `json:"duration_ms,omitempty"`
	Lanes         map[string]chatMemoryLanePayload `json:"lanes,omitempty"`
}

type chatMemoryLanePayload struct {
	Enabled    bool   `json:"enabled"`
	Returned   bool   `json:"returned"`
	TimedOut   bool   `json:"timed_out"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Items      int    `json:"items,omitempty"`
	Tokens     int    `json:"tokens,omitempty"`
}

func buildChatMemoryContextPayload(text string, block agentmemory.ContextBlock, diag agentmemory.Diagnostics) chatMemoryContextPayload {
	payload := chatMemoryContextPayload{
		Type:          "memory_context",
		Data:          text,
		TokenEstimate: block.TokenEstimate,
		Truncated:     block.Truncated,
		DurationMs:    diag.DurationMs,
	}
	if len(diag.Lanes) == 0 {
		return payload
	}
	payload.Lanes = make(map[string]chatMemoryLanePayload, len(diag.Lanes))
	for name, lane := range diag.Lanes {
		payload.Lanes[name] = chatMemoryLanePayload{
			Enabled:    lane.Enabled,
			Returned:   lane.Returned,
			TimedOut:   lane.TimedOut,
			Error:      lane.Error,
			DurationMs: lane.DurationMs,
			Items:      lane.Items,
			Tokens:     lane.Tokens,
		}
	}
	return payload
}

type chatContextMetricsPayload struct {
	Type             string                            `json:"type"`
	Phase            string                            `json:"phase"`
	InputTokens      int                               `json:"input_tokens"`
	ContextWindow    int                               `json:"context_window"`
	SummaryThreshold int                               `json:"summary_threshold"`
	ReserveTokens    int                               `json:"reserve_tokens"`
	MessageCount     int                               `json:"message_count"`
	SummarizedCount  int                               `json:"summarized_count,omitempty"`
	WillSummarize    bool                              `json:"will_summarize"`
	Segments         []chatContextMetricSegmentPayload `json:"segments,omitempty"`
}

type chatContextMetricSegmentPayload struct {
	Kind   string `json:"kind"`
	Tokens int    `json:"tokens"`
}

func buildChatContextMetricsPayload(metrics agent.ContextMetrics) chatContextMetricsPayload {
	payload := chatContextMetricsPayload{
		Type:             "context_metrics",
		Phase:            metrics.Phase,
		InputTokens:      metrics.InputTokens,
		ContextWindow:    metrics.ContextWindow,
		SummaryThreshold: metrics.SummaryThreshold,
		ReserveTokens:    metrics.ReserveTokens,
		MessageCount:     metrics.MessageCount,
		SummarizedCount:  metrics.SummarizedCount,
		WillSummarize:    metrics.WillSummarize,
	}
	if len(metrics.Segments) == 0 {
		return payload
	}
	payload.Segments = make([]chatContextMetricSegmentPayload, 0, len(metrics.Segments))
	for _, segment := range metrics.Segments {
		if segment.Tokens <= 0 || strings.TrimSpace(segment.Kind) == "" {
			continue
		}
		payload.Segments = append(payload.Segments, chatContextMetricSegmentPayload{
			Kind:   segment.Kind,
			Tokens: segment.Tokens,
		})
	}
	return payload
}

type fleetCallbackRequest = chatpkg.FleetCallbackRequest

func configureFleetCallbacks(app *app, eng *agent.Engine, req fleetCallbackRequest) {
	if app == nil {
		return
	}
	configureFleetCallbacksWithBus(app.fleetBus, eng, req)
}

func configureFleetCallbacksWithBus(bus *fleet.Bus, eng *agent.Engine, req fleetCallbackRequest) {
	chatpkg.AttachFleetCallbacks(bus, eng, req)
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
	if !build.ImageGeneration && !build.VideoGeneration {
		return ctx
	}
	if build.VideoGeneration {
		if _, ok := llm.VideoPromptFromContext(ctx); ok {
			return ctx
		}
		return llm.WithVideoPrompt(ctx, llm.VideoPromptOptions{})
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

type chatExecutionRequest struct {
	RunContext          context.Context
	Engine              *agent.Engine
	RunRequest          chatRunRequest
	History             []llm.Message
	RunID               string
	UserID              *int64
	CheckedOutWorkspace *workspaces.Workspace
}

func (a *app) executeStreamChat(w http.ResponseWriter, r *http.Request, exec chatExecutionRequest, opts chatStreamOptions) {
	runCtx := exec.RunContext
	eng := exec.Engine
	req := exec.RunRequest
	history := exec.History
	runID := exec.RunID
	userID := exec.UserID
	checkedOutWorkspace := exec.CheckedOutWorkspace
	if req.EphemeralSession {
		defer cleanupEphemeralChatSession(a.chatStore, userID, req.SessionID)
	}
	stream, err := newChatSSEWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	activityCollector := a.newStreamActivityCollector(req, runID, userID)
	defer a.flushStreamActivities(r.Context(), req, userID, activityCollector)
	attachChatEngineRuntime(a.chatDeps(), chatEngineAttachConfig{
		Engine:             eng,
		StreamWriter:       stream,
		EmitThoughtSummary: opts.EmitThoughtSummary,
		EmitSummaryEvents:  opts.EmitSummaryEvents,
		Tracer:             opts.Tracer,
		TracerMutex:        &stream.mu,
		Activity: func(ev agent.AgentTrace) {
			if activityCollector != nil {
				activityCollector.Handle(ev)
			}
		},
		Capture: llmRequestCaptureConfig{
			SessionID:           req.SessionID,
			UserID:              userID,
			RunID:               runID,
			MessageID:           req.AssistantMessageID,
			ParentUserMessageID: req.UserMessageID,
			SpecialistID:        streamAgentName(eng),
		},
		Fleet: fleetCallbackRequest{
			RunID:       runID,
			SessionID:   req.SessionID,
			ProjectID:   req.ProjectID,
			ObjectiveID: req.ObjectiveID,
			UserID:      userID,
		},
	})
	a.publishChatRunEvent(fleet.EventRunStarted, runID, req, userID, req.Prompt)
	writeInitialSummaryEvent(stream, opts.InitialSummary)

	ctx, cancel, dur := a.streamExecutionContext(streamExecutionContextRequest{
		RunContext: runCtx,
		Request:    req,
		Engine:     eng,
		Stream:     stream,
		Options:    opts,
		RunID:      runID,
		UserID:     userID,
	})
	defer cancel()
	logChatRunTimeout(opts.Endpoint, true, dur)

	stopKeepalive := startStreamKeepalive(ctx, stream, opts.KeepAlive)
	defer stopKeepalive()

	collector := newChatTurnCollector(sandbox.ResolveBaseDir(ctx, a.cfg.Workdir), req.ProjectID, stream)
	collector.attach(eng)

	assistantStartedAt := time.Now()
	result, err := eng.RunStream(ctx, req.Prompt, history)
	durationMs := time.Since(assistantStartedAt).Milliseconds()
	if err != nil {
		a.handleStreamChatError(ctx, r, stream, checkedOutWorkspace, streamChatErrorRequest{
			RunID:   runID,
			Request: req,
			UserID:  userID,
			Options: opts,
			Err:     err,
		})
		return
	}
	a.finishStreamChatSuccess(stream, collector, eng, streamChatSuccessRequest{
		Context:    ctx,
		StoreCtx:   r.Context(),
		RunID:      runID,
		Request:    req,
		UserID:     userID,
		Options:    opts,
		Result:     result,
		DurationMs: durationMs,
		Workspace:  checkedOutWorkspace,
	})
}

func (a *app) executeJSONChat(w http.ResponseWriter, r *http.Request, exec chatExecutionRequest, opts chatJSONOptions) {
	payload, err := a.executeInternalJSONChat(r.Context(), exec, opts)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func (a *app) executeInternalJSONChat(storeCtx context.Context, exec chatExecutionRequest, opts chatJSONOptions) (map[string]any, error) {
	runCtx := exec.RunContext
	eng := exec.Engine
	req := exec.RunRequest
	history := exec.History
	runID := exec.RunID
	userID := exec.UserID
	checkedOutWorkspace := exec.CheckedOutWorkspace
	if req.EphemeralSession {
		defer cleanupEphemeralChatSession(a.chatStore, userID, req.SessionID)
	}
	attachChatEngineRuntime(a.chatDeps(), chatEngineAttachConfig{
		Engine: eng,
		Capture: llmRequestCaptureConfig{
			SessionID:           req.SessionID,
			UserID:              userID,
			RunID:               runID,
			MessageID:           req.AssistantMessageID,
			ParentUserMessageID: req.UserMessageID,
			SpecialistID:        streamAgentName(eng),
		},
		Fleet: fleetCallbackRequest{
			RunID:       runID,
			SessionID:   req.SessionID,
			ProjectID:   req.ProjectID,
			ObjectiveID: req.ObjectiveID,
			UserID:      userID,
		},
	})
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

	assistantStartedAt := time.Now()
	result, err := eng.Run(ctx, req.Prompt, history)
	durationMs := time.Since(assistantStartedAt).Milliseconds()
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
	payload := buildChatJSONPayload(result, collector.savedImages, collector.savedVideos, ctx, opts.IncludeMatrixMessages)
	a.runs.updateStatus(runID, "completed", 0)
	if a.fleetBus != nil {
		a.fleetBus.Publish(fleet.Event{Kind: fleet.EventRunFinished, RunID: runID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, UserID: derefInputUserID(userID), Message: result})
	}
	if err := storeChatTurnWithHistory(storeCtx, a.chatStore, chatTurnHistoryRecord{
		UserID:             userID,
		SessionID:          req.SessionID,
		UserMessageID:      req.UserMessageID,
		UserContent:        req.Prompt,
		TurnMessages:       collector.turnMessages,
		FinalContent:       result,
		AssistantMessageID: req.AssistantMessageID,
		DurationMs:         &durationMs,
		Model:              chatStoreModel(eng, opts.StoreModel),
	}); err != nil {
		log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
	}
	a.commitWorkspace(ctx, checkedOutWorkspace)
	return payload, nil
}

func toolActivityTitle(name string, title string) string {
	if strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	return tools.HumanizeToolName(name)
}
