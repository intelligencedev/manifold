package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	agentmemory "manifold/internal/agent/memory"
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
	StoreModel            string
	InitialSummary        *agentmemory.SummaryResult
	Tracer                *agentStreamTracer
}

type chatJSONOptions struct {
	Endpoint              string
	IncludeMatrixMessages bool
	InheritImagePrompt    bool
	StoreModel            string
}

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

func buildChatJSONPayload(result string, ctx context.Context, includeMatrixMessages bool) map[string]any {
	payload := map[string]any{"result": result}
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
	stream, err := newChatSSEWriter(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if opts.Tracer != nil {
		if opts.Tracer.mu == nil {
			opts.Tracer.mu = &stream.mu
		}
		eng.AgentTracer = opts.Tracer
	}
	configureCommonStreamCallbacks(eng, stream, opts.EmitThoughtSummary, opts.EmitSummaryEvents)
	if opts.InitialSummary != nil && opts.InitialSummary.Triggered {
		stream.write(map[string]any{
			"type":             "summary",
			"input_tokens":     opts.InitialSummary.EstimatedTokens,
			"token_budget":     opts.InitialSummary.TokenBudget,
			"message_count":    opts.InitialSummary.MessageCount,
			"summarized_count": opts.InitialSummary.SummarizedCount,
		})
	}

	seconds := a.cfg.StreamRunTimeoutSeconds
	if seconds <= 0 {
		seconds = a.cfg.AgentRunTimeoutSeconds
	}
	ctx, cancel, dur := withMaybeTimeout(runCtx, seconds)
	defer cancel()
	ctx = applyChatImagePrompt(ctx, runCtx, req, opts.InheritImagePrompt)
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
		log.Error().Err(err).Msg("agent run error")
		if opts.StructuredErrors {
			stream.write(map[string]string{"type": "error", "data": "(error) " + err.Error()})
		} else if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
			stream.writeText(fmt.Sprintf("data: %s\n\n", b))
		} else {
			stream.writeText(fmt.Sprintf("data: %q\n\n", "(error)"))
		}
		a.runs.updateStatus(runID, "failed", 0)
		a.commitWorkspace(ctx, checkedOutWorkspace)
		return
	}
	result = collector.resultText(result)
	stream.write(buildChatStreamFinalPayload(result, ctx, opts.IncludeMatrixMessages))
	a.runs.updateStatus(runID, "completed", 0)
	if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, collector.turnMessages, result, chatStoreModel(eng, opts.StoreModel)); err != nil {
		log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_stream")
	}
	a.commitWorkspace(ctx, checkedOutWorkspace)
}

func (a *app) executeJSONChat(w http.ResponseWriter, r *http.Request, runCtx context.Context, eng *agent.Engine, req chatRunRequest, history []llm.Message, runID string, userID *int64, checkedOutWorkspace *workspaces.Workspace, opts chatJSONOptions) {
	ctx, cancel, dur := withMaybeTimeout(runCtx, a.cfg.AgentRunTimeoutSeconds)
	defer cancel()
	ctx = applyChatImagePrompt(ctx, runCtx, req, opts.InheritImagePrompt)
	logChatRunTimeout(opts.Endpoint, false, dur)

	collector := newChatTurnCollector(sandbox.ResolveBaseDir(ctx, a.cfg.Workdir), req.ProjectID, nil)
	collector.attach(eng)

	result, err := eng.Run(ctx, req.Prompt, history)
	if err != nil {
		log.Error().Err(err).Msg("agent run error")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		a.runs.updateStatus(runID, "failed", 0)
		a.commitWorkspace(ctx, checkedOutWorkspace)
		return
	}
	result = collector.resultText(result)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildChatJSONPayload(result, ctx, opts.IncludeMatrixMessages))
	a.runs.updateStatus(runID, "completed", 0)
	if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, collector.turnMessages, result, chatStoreModel(eng, opts.StoreModel)); err != nil {
		log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
	}
	a.commitWorkspace(ctx, checkedOutWorkspace)
}
