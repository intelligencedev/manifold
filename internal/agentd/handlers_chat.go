package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/prompts"
	"manifold/internal/auth"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/warpp"
	"manifold/internal/workspaces"
)

type agentStreamTracer struct {
	w  io.Writer
	fl http.Flusher
	mu *sync.Mutex
}

func (t *agentStreamTracer) Trace(ev agent.AgentTrace) {
	if t == nil || t.w == nil || t.fl == nil {
		return
	}
	payload := map[string]any{
		"type":           ev.Type,
		"agent":          ev.Agent,
		"model":          ev.Model,
		"call_id":        ev.CallID,
		"parent_call_id": ev.ParentCallID,
		"depth":          ev.Depth,
		"role":           ev.Role,
		"content":        ev.Content,
		"title":          ev.Title,
		"args":           ev.Args,
		"data":           ev.Data,
		"tool_id":        ev.ToolID,
		"error":          ev.Error,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if t.mu != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
	}
	fmt.Fprintf(t.w, "data: %s\n\n", b)
	t.fl.Flush()
}

func (a *app) runsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")

		// Prefer ClickHouse-backed runs when available so the UI persists across restarts.
		if a.runMetrics != nil {
			runs, err := a.runMetrics.RecentRuns(r.Context(), 24*time.Hour, 200)
			if err != nil {
				log.Warn().Err(err).Msg("clickhouse runs query failed")
			} else if len(runs) > 0 {
				_ = json.NewEncoder(w).Encode(runs)
				return
			}
		}

		_ = json.NewEncoder(w).Encode(a.runs.list())
	}
}

func (a *app) chatSessionsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			userID  *int64
			isAdmin bool
		)
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id, admin, err := resolveChatAccess(r.Context(), a.authStore, u)
			if err != nil {
				log.Error().Err(err).Msg("resolve_chat_access")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			userID, isAdmin = id, admin
		} else {
			isAdmin = true
		}
		_ = isAdmin
		setChatCORSHeaders(w, r, "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.Method {
		case http.MethodGet:
			sessions, err := a.chatStore.ListSessions(r.Context(), userID)
			if err != nil {
				log.Error().Err(err).Msg("list_chat_sessions")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(sessions); err != nil {
				log.Error().Err(err).Msg("encode_chat_sessions")
			}
		case http.MethodPost:
			defer r.Body.Close()
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			sess, err := a.chatStore.CreateSession(r.Context(), userID, body.Name)
			if err != nil {
				if errors.Is(err, persist.ErrForbidden) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				log.Error().Err(err).Msg("create_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(sess); err != nil {
				log.Error().Err(err).Msg("encode_chat_session")
			}
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) chatSessionDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			userID  *int64
			isAdmin bool
		)
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id, admin, err := resolveChatAccess(r.Context(), a.authStore, u)
			if err != nil {
				log.Error().Err(err).Msg("resolve_chat_access")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			userID, isAdmin = id, admin
		} else {
			isAdmin = true
		}
		_ = isAdmin
		rest := strings.TrimPrefix(r.URL.Path, "/api/chat/sessions/")
		rest = strings.Trim(rest, "/")
		if rest == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rest, "/")
		id := parts[0]
		subresource := ""
		if len(parts) == 2 {
			subresource = parts[1]
		}
		switch subresource {
		case "messages":
			setChatCORSHeaders(w, r, "GET, OPTIONS")
		case "title":
			setChatCORSHeaders(w, r, "POST, OPTIONS")
		default:
			setChatCORSHeaders(w, r, "GET, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if subresource == "messages" {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			limit := 0
			if raw := r.URL.Query().Get("limit"); raw != "" {
				if v, err := strconv.Atoi(raw); err == nil && v > 0 {
					limit = v
				}
			}
			msgs, err := a.chatStore.ListMessages(r.Context(), userID, id, limit)
			if err != nil {
				if errors.Is(err, persist.ErrForbidden) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if errors.Is(err, persist.ErrNotFound) {
					http.NotFound(w, r)
					return
				}
				log.Error().Err(err).Str("session", id).Msg("list_chat_messages")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			msgs = hydrateChatMessages(msgs)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(msgs); err != nil {
				log.Error().Err(err).Msg("encode_chat_messages")
			}
			return
		}
		if subresource == "title" {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			defer r.Body.Close()
			var body struct {
				Prompt string `json:"prompt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			prompt := strings.TrimSpace(body.Prompt)
			if prompt == "" {
				http.Error(w, "prompt required", http.StatusBadRequest)
				return
			}
			sess, err := a.chatStore.GetSession(r.Context(), userID, id)
			if err != nil {
				if errors.Is(err, persist.ErrForbidden) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if errors.Is(err, persist.ErrNotFound) {
					http.NotFound(w, r)
					return
				}
				log.Error().Err(err).Str("session", id).Msg("get_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !isDefaultSessionName(sess.Name) {
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(sess); err != nil {
					log.Error().Err(err).Msg("encode_chat_session")
				}
				return
			}
			title, genErr := a.generateChatTitle(r.Context(), prompt)
			if genErr != nil {
				log.Warn().Err(genErr).Str("session", id).Msg("chat_title_fallback")
			}
			updated, err := a.chatStore.RenameSession(r.Context(), userID, id, title)
			if err != nil {
				if errors.Is(err, persist.ErrForbidden) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if errors.Is(err, persist.ErrNotFound) {
					http.NotFound(w, r)
					return
				}
				log.Error().Err(err).Str("session", id).Msg("rename_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(updated); err != nil {
				log.Error().Err(err).Msg("encode_chat_session")
			}
			return
		}
		switch r.Method {
		case http.MethodGet:
			sess, err := a.chatStore.GetSession(r.Context(), userID, id)
			if err != nil {
				if errors.Is(err, persist.ErrForbidden) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if errors.Is(err, persist.ErrNotFound) {
					http.NotFound(w, r)
					return
				}
				log.Error().Err(err).Str("session", id).Msg("get_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(sess); err != nil {
				log.Error().Err(err).Msg("encode_chat_session")
			}
		case http.MethodPatch:
			defer r.Body.Close()
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			sess, err := a.chatStore.RenameSession(r.Context(), userID, id, body.Name)
			if err != nil {
				if errors.Is(err, persist.ErrForbidden) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if errors.Is(err, persist.ErrNotFound) {
					http.NotFound(w, r)
					return
				}
				log.Error().Err(err).Str("session", id).Msg("rename_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(sess); err != nil {
				log.Error().Err(err).Msg("encode_chat_session")
			}
		case http.MethodDelete:
			if err := a.chatStore.DeleteSession(r.Context(), userID, id); err != nil {
				if errors.Is(err, persist.ErrForbidden) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if errors.Is(err, persist.ErrNotFound) {
					http.NotFound(w, r)
					return
				}
				log.Error().Err(err).Str("session", id).Msg("delete_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// hydrateChatMessages post-processes persisted messages for client display.
// It strips JSON wrappers used to preserve tool calls and attaches tool names/args
// to tool-role messages so the UI can render the tool pane correctly after reload.
func hydrateChatMessages(raw []persist.ChatMessage) []persist.ChatMessage {
	out := make([]persist.ChatMessage, 0, len(raw))

	type toolMeta struct {
		name string
		args string
	}

	metaByID := make(map[string]toolMeta)

	for _, msg := range raw {
		m := msg
		trimmed := strings.TrimSpace(m.Content)

		if m.Role == "assistant" && strings.HasPrefix(trimmed, "{") {
			var data struct {
				Content   string         `json:"content"`
				ToolCalls []llm.ToolCall `json:"tool_calls"`
			}
			if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
				if data.Content != "" {
					m.Content = data.Content
				}
				for _, tc := range data.ToolCalls {
					args := strings.TrimSpace(string(tc.Args))
					metaByID[tc.ID] = toolMeta{name: tc.Name, args: args}
				}
				// Assistant messages that only carried tool_calls should not render in the chat pane.
				if strings.TrimSpace(data.Content) == "" && len(data.ToolCalls) > 0 {
					continue
				}
			}
		} else if m.Role == "tool" && strings.HasPrefix(trimmed, "{") {
			var data struct {
				Content string `json:"content"`
				ToolID  string `json:"tool_id"`
			}
			if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
				if data.Content != "" {
					m.Content = data.Content
				}
				if data.ToolID != "" {
					m.ToolID = data.ToolID
					if meta, ok := metaByID[data.ToolID]; ok {
						m.Title = meta.name
						m.ToolArgs = meta.args
					}
				}
			}
		}

		out = append(out, m)
	}

	return out
}

func (a *app) agentRunHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			userID      *int64
			currentUser *auth.User
		)
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			currentUser = u
			id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
			if err != nil {
				log.Error().Err(err).Msg("resolve_chat_access")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			userID = id
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Prompt    string `json:"prompt"`
			SessionID string `json:"session_id,omitempty"`
			ProjectID string `json:"project_id,omitempty"`
			Image     bool   `json:"image,omitempty"`
			ImageSize string `json:"image_size,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Track workspace for commit after agent run completes.
		var checkedOutWorkspace *workspaces.Workspace

		// If a project_id was provided, checkout a workspace using the workspace manager.
		// This abstracts the path computation and validation, preparing for future
		// ephemeral workspace support.
		if pid := strings.TrimSpace(req.ProjectID); pid != "" {
			var uid int64
			if userID != nil {
				uid = *userID
			}
			ws, err := a.workspaceManager.Checkout(r.Context(), uid, pid, req.SessionID)
			if err != nil {
				if errors.Is(err, workspaces.ErrInvalidProjectID) {
					http.Error(w, "invalid project_id", http.StatusBadRequest)
					return
				}
				if errors.Is(err, workspaces.ErrProjectNotFound) {
					log.Error().Err(err).Str("project_id", pid).Msg("project_dir_missing")
					http.Error(w, "project not found (project_id must match the project directory/ID)", http.StatusBadRequest)
					return
				}
				log.Error().Err(err).Str("project_id", pid).Msg("workspace_checkout_failed")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if ws.BaseDir != "" {
				r = r.WithContext(sandbox.WithBaseDir(r.Context(), ws.BaseDir))
				r = r.WithContext(sandbox.WithProjectID(r.Context(), pid))
				checkedOutWorkspace = &ws
			}
		}

		currentRun := a.runs.create(req.Prompt)
		if req.SessionID == "" {
			req.SessionID = "default"
		}

		// Attach session ID to context so tools like ask_agent can inherit it.
		r = r.WithContext(sandbox.WithSessionID(r.Context(), req.SessionID))

		if _, err := ensureChatSession(r.Context(), a.chatStore, userID, req.SessionID); err != nil {
			if errors.Is(err, persist.ErrForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Str("session", req.SessionID).Msg("ensure_chat_session")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// Check if the main LLM provider supports compaction (OpenAI Responses API).
		// Non-OpenAI providers cannot use encrypted compaction summaries.
		targetSupportsCompaction := providerSupportsCompaction(a.llm)
		history, memorySummaryResult, err := a.chatMemory.BuildContextForProvider(r.Context(), userID, req.SessionID, targetSupportsCompaction)
		if err != nil {
			if errors.Is(err, persist.ErrForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Str("session", req.SessionID).Msg("load_chat_history")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if req.Image {
			r = r.WithContext(llm.WithImagePrompt(r.Context(), llm.ImagePromptOptions{Size: req.ImageSize}))
		}
		if req.Image {
			r = r.WithContext(llm.WithImagePrompt(r.Context(), llm.ImagePromptOptions{Size: req.ImageSize}))
		}

		specOwner := systemUserID
		if currentUser != nil {
			specOwner = currentUser.ID
		} else if userID != nil {
			specOwner = *userID
		}

		if r.URL.Query().Get("warpp") == "true" {
			seconds := a.cfg.WorkflowTimeoutSeconds
			if seconds <= 0 {
				seconds = a.cfg.AgentRunTimeoutSeconds
			}
			ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
			defer cancel()

			if dur > 0 {
				log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("mode", "warpp").Msg("using configured workflow timeout")
			} else {
				log.Debug().Str("endpoint", "/agent/run").Str("mode", "warpp").Msg("no timeout configured; running until completion")
			}
			runner, err := a.warppRunnerForUser(ctx, specOwner)
			if err != nil {
				log.Error().Err(err).Msg("warpp_runner_for_user")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			intent := runner.DetectIntent(ctx, req.Prompt)
			wf, err := runner.Workflows.Get(intent)
			if err != nil {
				http.Error(w, "workflow not found", http.StatusNotFound)
				return
			}
			attrs := warpp.Attrs{"utter": req.Prompt}
			wfStar, _, attrs, err := runner.Personalize(ctx, wf, attrs)
			if err != nil {
				log.Error().Err(err).Msg("personalize")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			allow := map[string]bool{}
			for _, s := range wfStar.Steps {
				if s.Tool != nil {
					allow[s.Tool.Name] = true
				}
			}
			final, err := runner.Execute(ctx, wfStar, allow, attrs, nil)
			if err != nil {
				log.Error().Err(err).Msg("warpp")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"result": final})
			return
		}

		specialistName := strings.TrimSpace(r.URL.Query().Get("specialist"))
		if specialistName != "" && !strings.EqualFold(specialistName, specialists.OrchestratorName) {
			if handled := a.handleSpecialistChat(w, r, specialistName, req.Prompt, req.SessionID, history, userID, specOwner); handled {
				return
			}
		}

		if a.cfg.OpenAI.APIKey == "" {
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fl, _ := w.(http.Flusher)
				if b, err := json.Marshal("(dev) mock response: " + req.Prompt); err == nil {
					fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock response")
				}
				fl.Flush()
				a.runs.updateStatus(currentRun.ID, "completed", 0)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock response: " + req.Prompt})
			a.runs.updateStatus(currentRun.ID, "completed", 0)
			return
		}

		eng := a.cloneEngineForUser(r.Context(), specOwner, req.SessionID)
		if eng == nil {
			http.Error(w, "agent unavailable", http.StatusServiceUnavailable)
			return
		}
		ctx := llm.WithUserID(r.Context(), specOwner)

		// Inject project-specific skills into the system prompt if a workspace is active.
		if checkedOutWorkspace != nil && checkedOutWorkspace.BaseDir != "" {
			skillsSection := prompts.RenderSkillsForProject(checkedOutWorkspace.BaseDir)
			log.Debug().
				Str("baseDir", checkedOutWorkspace.BaseDir).
				Str("projectID", checkedOutWorkspace.ProjectID).
				Bool("hasSkills", skillsSection != "").
				Msg("skills_injection_check")
			if skillsSection != "" {
				eng.System = eng.System + "\n\n" + skillsSection
			}
		}

		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}

			// Serialize all writes to the SSE stream. Tool calls and delegated agent
			// tracing can write concurrently.
			var streamMu sync.Mutex
			writeSSE := func(payload any) {
				b, err := json.Marshal(payload)
				if err != nil {
					return
				}
				streamMu.Lock()
				defer streamMu.Unlock()
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			writeSSEText := func(text string) {
				streamMu.Lock()
				defer streamMu.Unlock()
				fmt.Fprint(w, text)
				fl.Flush()
			}

			// If memory manager triggered summarization during BuildContext, emit event
			if memorySummaryResult != nil && memorySummaryResult.Triggered {
				payload := map[string]any{
					"type":             "summary",
					"input_tokens":     memorySummaryResult.EstimatedTokens,
					"token_budget":     memorySummaryResult.TokenBudget,
					"message_count":    memorySummaryResult.MessageCount,
					"summarized_count": memorySummaryResult.SummarizedCount,
				}
				log.Debug().
					Int("input_tokens", memorySummaryResult.EstimatedTokens).
					Int("token_budget", memorySummaryResult.TokenBudget).
					Int("message_count", memorySummaryResult.MessageCount).
					Int("summarized_count", memorySummaryResult.SummarizedCount).
					Msg("emitting_summary_sse_event")
				writeSSE(payload)
			}

			tracer := &agentStreamTracer{w: w, fl: fl, mu: &streamMu}

			seconds := a.cfg.StreamRunTimeoutSeconds
			if seconds <= 0 {
				seconds = a.cfg.AgentRunTimeoutSeconds
			}
			ctx, cancel, dur := withMaybeTimeout(ctx, seconds)
			defer cancel()

			// Keep-alive pings prevent some browsers/proxies (notably Safari/WebKit)
			// from closing idle SSE connections during long-running tool execution.
			// This does not affect the agent logic; it's a transport-level heartbeat.
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
						// SSE comment line (ignored by clients) + blank line delimiter.
						writeSSEText(": keepalive\n\n")
					}
				}
			}()
			defer close(stopKeepalive)

			if req.Image {
				ctx = llm.WithImagePrompt(ctx, llm.ImagePromptOptions{Size: req.ImageSize})
			}
			if dur > 0 {
				log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Bool("stream", true).Msg("using configured stream timeout")
			} else {
				log.Debug().Str("endpoint", "/agent/run").Bool("stream", true).Msg("no timeout configured; running until completion")
			}
			baseDir := sandbox.ResolveBaseDir(ctx, a.cfg.Workdir)
			var savedImages []savedImage
			eng.AgentTracer = tracer
			eng.AgentTracer = tracer

			eng.OnDelta = func(d string) {
				payload := map[string]string{"type": "delta", "data": d}
				writeSSE(payload)
			}
			eng.OnThoughtSummary = func(summary string) {
				log.Debug().Int("summary_len", len(summary)).Msg("http_handler_thought_summary")
				payload := map[string]string{"type": "thought_summary", "data": summary}
				writeSSE(payload)
			}
			eng.OnToolStart = func(name string, args []byte, toolID string) {
				payload := map[string]any{"type": "tool_start", "title": "Tool: " + name, "tool_id": toolID, "args": string(args)}
				// Hint UI to group agent-related tool calls under a team panel
				if name == "agent_call" || name == "ask_agent" {
					payload["agent"] = true
				}
				writeSSE(payload)
			}
			eng.OnTool = func(name string, args []byte, result []byte, toolID string) {
				if name == "text_to_speech_chunk" {
					var meta map[string]any
					_ = json.Unmarshal(result, &meta)
					metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
					writeSSE(metaPayload)
					return
				}
				payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result), "tool_id": toolID}
				if name == "agent_call" || name == "ask_agent" {
					payload["agent"] = true
				}
				writeSSE(payload)

				if name == "text_to_speech" {
					var resp map[string]any
					if err := json.Unmarshal(result, &resp); err == nil {
						if fp, ok := resp["file_path"].(string); ok && fp != "" {
							trimmed := strings.TrimPrefix(fp, "./")
							trimmed = strings.TrimPrefix(trimmed, "/")
							url := "/audio/" + trimmed
							ap := map[string]any{"type": "tts_audio", "file_path": fp, "url": url}
							writeSSE(ap)
						}
					}
				}
			}
			var turnMessages []llm.Message
			eng.OnTurnMessage = func(msg llm.Message) {
				turnMessages = append(turnMessages, msg)
			}
			eng.OnAssistant = func(msg llm.Message) {
				if len(msg.Images) == 0 {
					return
				}
				saved := saveGeneratedImages(baseDir, msg.Images, req.ProjectID)
				if len(saved) == 0 {
					return
				}
				savedImages = append(savedImages, saved...)
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
					writeSSE(payload)
				}
			}
			eng.OnSummaryTriggered = func(inputTokens, tokenBudget, messageCount, summarizedCount int) {
				payload := map[string]any{
					"type":             "summary",
					"input_tokens":     inputTokens,
					"token_budget":     tokenBudget,
					"message_count":    messageCount,
					"summarized_count": summarizedCount,
				}
				writeSSE(payload)
			}

			res, err := eng.RunStream(ctx, req.Prompt, history)
			if err != nil {
				logStreamContextDone(err, r, "/agent/run", req.SessionID, req.ProjectID, "")
				log.Error().Err(err).Msg("agent run error")
				writeSSE(map[string]string{"type": "error", "data": "(error) " + err.Error()})
				a.runs.updateStatus(currentRun.ID, "failed", 0)
				// Commit workspace changes even on error so partial work is preserved
				a.commitWorkspace(ctx, checkedOutWorkspace)
				return
			}
			if len(savedImages) > 0 {
				res = appendImageSummary(res, savedImages)
			}
			payload := map[string]string{"type": "final", "data": res}
			writeSSE(payload)
			a.runs.updateStatus(currentRun.ID, "completed", 0)
			if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, turnMessages, res, eng.Model); err != nil {
				log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_stream")
			}
			// Commit workspace changes to S3 after successful run
			a.commitWorkspace(ctx, checkedOutWorkspace)
			return
		}

		seconds := a.cfg.AgentRunTimeoutSeconds
		ctx, cancel, dur := withMaybeTimeout(ctx, seconds)
		defer cancel()

		if req.Image {
			ctx = llm.WithImagePrompt(ctx, llm.ImagePromptOptions{Size: req.ImageSize})
		}
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Bool("stream", false).Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/agent/run").Bool("stream", false).Msg("no timeout configured; running until completion")
		}
		baseDir := sandbox.ResolveBaseDir(ctx, a.cfg.Workdir)
		var savedImages []savedImage
		var turnMessages []llm.Message
		eng.OnAssistant = func(msg llm.Message) {
			if len(msg.Images) == 0 {
				return
			}
			saved := saveGeneratedImages(baseDir, msg.Images, req.ProjectID)
			if len(saved) == 0 {
				return
			}
			savedImages = append(savedImages, saved...)
		}
		eng.OnTurnMessage = func(msg llm.Message) {
			turnMessages = append(turnMessages, msg)
		}
		result, err := eng.Run(ctx, req.Prompt, history)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			a.runs.updateStatus(currentRun.ID, "failed", 0)
			// Commit workspace changes even on error so partial work is preserved
			a.commitWorkspace(ctx, checkedOutWorkspace)
			return
		}
		if len(savedImages) > 0 {
			result = appendImageSummary(result, savedImages)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
		a.runs.updateStatus(currentRun.ID, "completed", 0)
		if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, turnMessages, result, eng.Model); err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
		}
		// Commit workspace changes to S3 after successful run
		a.commitWorkspace(ctx, checkedOutWorkspace)
	}
}

// commitWorkspace commits workspace changes back to durable storage (e.g., S3).
// For ephemeral workspaces, this syncs any files created or modified by the agent.
// For legacy workspaces, this is a no-op since changes are already on disk.
func (a *app) commitWorkspace(ctx context.Context, ws *workspaces.Workspace) {
	if ws == nil {
		return
	}
	if err := a.workspaceManager.Commit(ctx, *ws); err != nil {
		log.Error().
			Err(err).
			Str("project_id", ws.ProjectID).
			Str("session_id", ws.SessionID).
			Msg("workspace_commit_failed")
	}
}

func (a *app) promptHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var userID *int64
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
			if err != nil {
				log.Error().Err(err).Msg("resolve_chat_access")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			userID = id
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()

		var req struct {
			Prompt    string `json:"prompt"`
			SessionID string `json:"session_id,omitempty"`
			ProjectID string `json:"project_id,omitempty"`
			Image     bool   `json:"image,omitempty"`
			ImageSize string `json:"image_size,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("decode prompt: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.SessionID == "" {
			req.SessionID = "default"
		}

		// Attach session ID to context so tools like ask_agent can inherit it.
		r = r.WithContext(sandbox.WithSessionID(r.Context(), req.SessionID))

		// Track workspace for commit after agent run completes.
		var checkedOutWorkspace *workspaces.Workspace

		// If a project_id was provided, checkout a workspace using the workspace manager.
		if pid := strings.TrimSpace(req.ProjectID); pid != "" {
			var uid int64
			if userID != nil {
				uid = *userID
			}
			ws, err := a.workspaceManager.Checkout(r.Context(), uid, pid, req.SessionID)
			if err != nil {
				if errors.Is(err, workspaces.ErrInvalidProjectID) {
					http.Error(w, "invalid project_id", http.StatusBadRequest)
					return
				}
				if errors.Is(err, workspaces.ErrProjectNotFound) {
					log.Error().Err(err).Str("project_id", pid).Msg("project_dir_missing")
					http.Error(w, "project not found (project_id must match the project directory/ID)", http.StatusBadRequest)
					return
				}
				log.Error().Err(err).Str("project_id", pid).Msg("workspace_checkout_failed")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if ws.BaseDir != "" {
				r = r.WithContext(sandbox.WithBaseDir(r.Context(), ws.BaseDir))
				r = r.WithContext(sandbox.WithProjectID(r.Context(), pid))
				checkedOutWorkspace = &ws
			}
		}

		if _, err := ensureChatSession(r.Context(), a.chatStore, userID, req.SessionID); err != nil {
			if errors.Is(err, persist.ErrForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Str("session", req.SessionID).Msg("ensure_chat_session")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// Check if the main LLM provider supports compaction (OpenAI Responses API).
		// Non-OpenAI providers cannot use encrypted compaction summaries.
		targetSupportsCompaction := providerSupportsCompaction(a.llm)
		history, _, err := a.chatMemory.BuildContextForProvider(r.Context(), userID, req.SessionID, targetSupportsCompaction)
		if err != nil {
			if errors.Is(err, persist.ErrForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Str("session", req.SessionID).Msg("load_chat_history")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if a.cfg.OpenAI.APIKey == "" {
			prun := a.runs.create(req.Prompt)
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fl, _ := w.(http.Flusher)
				if b, err := json.Marshal("(dev) mock response: " + req.Prompt); err == nil {
					fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock response")
				}
				fl.Flush()
				a.runs.updateStatus(prun.ID, "completed", 0)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock response: " + req.Prompt})
			a.runs.updateStatus(prun.ID, "completed", 0)
			return
		}

		// Determine user ID for per-user orchestrator config
		var orchUserID int64 = systemUserID
		if userID != nil {
			orchUserID = *userID
		}
		eng := a.cloneEngineForUser(r.Context(), orchUserID, req.SessionID)
		if eng == nil {
			http.Error(w, "agent unavailable", http.StatusServiceUnavailable)
			return
		}
		ctx := llm.WithUserID(r.Context(), orchUserID)

		// Inject project-specific skills into the system prompt if a workspace is active.
		if checkedOutWorkspace != nil && checkedOutWorkspace.BaseDir != "" {
			if skillsSection := prompts.RenderSkillsForProject(checkedOutWorkspace.BaseDir); skillsSection != "" {
				eng.System = eng.System + "\n\n" + skillsSection
			}
		}

		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}

			seconds := a.cfg.StreamRunTimeoutSeconds
			if seconds <= 0 {
				seconds = a.cfg.AgentRunTimeoutSeconds
			}
			ctx, cancel, dur := withMaybeTimeout(ctx, seconds)
			defer cancel()

			if req.Image {
				ctx = llm.WithImagePrompt(ctx, llm.ImagePromptOptions{Size: req.ImageSize})
			}
			baseDir := sandbox.ResolveBaseDir(ctx, a.cfg.Workdir)
			var savedImages []savedImage
			if dur > 0 {
				log.Debug().Dur("timeout", dur).Str("endpoint", "/api/prompt").Bool("stream", true).Msg("using configured stream timeout")
			} else {
				log.Debug().Str("endpoint", "/api/prompt").Bool("stream", true).Msg("no timeout configured; running until completion")
			}

			eng.OnDelta = func(d string) {
				payload := map[string]string{"type": "delta", "data": d}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnToolStart = func(name string, args []byte, toolID string) {
				payload := map[string]any{"type": "tool_start", "title": "Tool: " + name, "tool_id": toolID, "args": string(args)}
				if name == "agent_call" || name == "ask_agent" {
					payload["agent"] = true
				}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnTool = func(name string, args []byte, result []byte, toolID string) {
				if name == "text_to_speech_chunk" {
					var meta map[string]any
					_ = json.Unmarshal(result, &meta)
					metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
					b, _ := json.Marshal(metaPayload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
					return
				}
				payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result), "tool_id": toolID}
				if name == "agent_call" || name == "ask_agent" {
					payload["agent"] = true
				}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
				if name == "text_to_speech" {
					var resp map[string]any
					if err := json.Unmarshal(result, &resp); err == nil {
						if fp, ok := resp["file_path"].(string); ok && fp != "" {
							trimmed := strings.TrimPrefix(fp, "./")
							trimmed = strings.TrimPrefix(trimmed, "/")
							url := "/audio/" + trimmed
							ap := map[string]any{"type": "tts_audio", "file_path": fp, "url": url}
							if bb, err2 := json.Marshal(ap); err2 == nil {
								fmt.Fprintf(w, "data: %s\n\n", bb)
								fl.Flush()
							}
						}
					}
				}
			}
			var turnMessages []llm.Message
			eng.OnTurnMessage = func(msg llm.Message) {
				turnMessages = append(turnMessages, msg)
			}
			eng.OnAssistant = func(msg llm.Message) {
				if len(msg.Images) == 0 {
					return
				}
				saved := saveGeneratedImages(baseDir, msg.Images, req.ProjectID)
				if len(saved) == 0 {
					return
				}
				savedImages = append(savedImages, saved...)
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
					b, _ := json.Marshal(payload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
				}
			}

			prun := a.runs.create(req.Prompt)
			res, err := eng.RunStream(ctx, req.Prompt, history)
			if err != nil {
				logStreamContextDone(err, r, "/api/prompt", req.SessionID, req.ProjectID, "")
				log.Error().Err(err).Msg("agent run error")
				if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
					fmt.Fprintf(w, "data: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "data: %q\n\n", "(error)")
				}
				fl.Flush()
				a.runs.updateStatus(prun.ID, "failed", 0)
				// Commit workspace changes even on error so partial work is preserved
				a.commitWorkspace(ctx, checkedOutWorkspace)
				return
			}
			if len(savedImages) > 0 {
				res = appendImageSummary(res, savedImages)
			}
			payload := map[string]string{"type": "final", "data": res}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			a.runs.updateStatus(prun.ID, "completed", 0)
			if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, turnMessages, res, eng.Model); err != nil {
				log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_stream")
			}
			// Commit workspace changes to S3 after successful run
			a.commitWorkspace(ctx, checkedOutWorkspace)
			return
		}

		seconds := a.cfg.AgentRunTimeoutSeconds
		ctx, cancel, dur := withMaybeTimeout(ctx, seconds)
		defer cancel()

		if req.Image {
			ctx = llm.WithImagePrompt(ctx, llm.ImagePromptOptions{Size: req.ImageSize})
		}
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/api/prompt").Bool("stream", false).Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/api/prompt").Bool("stream", false).Msg("no timeout configured; running until completion")
		}
		prun := a.runs.create(req.Prompt)
		baseDir := sandbox.ResolveBaseDir(ctx, a.cfg.Workdir)
		var savedImages []savedImage
		var turnMessages []llm.Message
		eng.OnTurnMessage = func(msg llm.Message) {
			turnMessages = append(turnMessages, msg)
		}
		eng.OnAssistant = func(msg llm.Message) {
			if len(msg.Images) == 0 {
				return
			}
			saved := saveGeneratedImages(baseDir, msg.Images, req.ProjectID)
			if len(saved) == 0 {
				return
			}
			savedImages = append(savedImages, saved...)
		}
		result, err := eng.Run(ctx, req.Prompt, history)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			a.runs.updateStatus(prun.ID, "failed", 0)
			// Commit workspace changes even on error so partial work is preserved
			a.commitWorkspace(ctx, checkedOutWorkspace)
			return
		}
		if len(savedImages) > 0 {
			result = appendImageSummary(result, savedImages)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
		a.runs.updateStatus(prun.ID, "completed", 0)
		if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, turnMessages, result, eng.Model); err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
		}
		// Commit workspace changes to S3 after successful run
		a.commitWorkspace(ctx, checkedOutWorkspace)
	}
}

func (a *app) handleSpecialistChat(w http.ResponseWriter, r *http.Request, name, prompt, sessionID string, history []llm.Message, userID *int64, owner int64) bool {
	reg, err := a.specialistsRegistryForUser(r.Context(), owner)
	if err != nil {
		log.Error().Err(err).Str("specialist", name).Msg("load_specialist_registry")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return true
	}
	sp, ok := reg.Get(name)
	if !ok || sp == nil {
		http.Error(w, "specialist not found", http.StatusNotFound)
		return true
	}
	modelLabel := strings.TrimSpace(sp.Model)
	if modelLabel != "" {
		modelLabel = fmt.Sprintf("%s:%s", name, modelLabel)
	} else {
		modelLabel = name
	}

	prov := sp.Provider()
	if prov == nil {
		http.Error(w, "specialist not configured", http.StatusInternalServerError)
		return true
	}
	toolReg := sp.ToolsRegistry()
	if toolReg == nil || !sp.EnableTools {
		toolReg = tools.NewRegistry()
	}

	skillsSection := ""
	if baseDir, ok := sandbox.BaseDirFromContext(r.Context()); ok {
		skillsSection = prompts.RenderSkillsForProject(baseDir)
	}

	buildEngine := func() *agent.Engine {
		systemPrompt := prompts.EnsureMemoryInstructions(sp.System)
		if skillsSection != "" {
			systemPrompt = systemPrompt + "\n\n" + skillsSection
		}
		eng := &agent.Engine{
			LLM:                          prov,
			Tools:                        toolReg,
			MaxSteps:                     a.cfg.MaxSteps,
			System:                       systemPrompt,
			Model:                        sp.Model,
			SummaryEnabled:               a.cfg.SummaryEnabled,
			SummaryReserveBufferTokens:   a.cfg.SummaryReserveBufferTokens,
			SummaryMinKeepLastMessages:   a.cfg.SummaryMinKeepLastMessages,
			SummaryMaxSummaryChunkTokens: a.cfg.SummaryMaxSummaryChunkTokens,
		}
		eng.AttachTokenizer(prov, nil)
		return eng
	}

	if r.Header.Get("Accept") == "text/event-stream" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return true
		}

		// Serialize all writes to the SSE stream. Specialist engines can execute tools
		// concurrently, and tracing callbacks may also write.
		var streamMu sync.Mutex
		writeSSE := func(payload any) {
			b, err := json.Marshal(payload)
			if err != nil {
				return
			}
			streamMu.Lock()
			defer streamMu.Unlock()
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
		writeSSEText := func(text string) {
			streamMu.Lock()
			defer streamMu.Unlock()
			fmt.Fprint(w, text)
			fl.Flush()
		}

		tracer := &agentStreamTracer{w: w, fl: fl, mu: &streamMu}
		seconds := a.cfg.StreamRunTimeoutSeconds
		if seconds <= 0 {
			seconds = a.cfg.AgentRunTimeoutSeconds
		}
		ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
		defer cancel()

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
					writeSSEText(": keepalive\n\n")
				}
			}
		}()
		defer close(stopKeepalive)

		if opts, ok := llm.ImagePromptFromContext(r.Context()); ok {
			ctx = llm.WithImagePrompt(ctx, opts)
		}
		baseDir := sandbox.ResolveBaseDir(ctx, a.cfg.Workdir)
		var savedImages []savedImage
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("specialist", name).Bool("stream", true).Msg("using configured stream timeout")
		} else {
			log.Debug().Str("endpoint", "/agent/run").Str("specialist", name).Bool("stream", true).Msg("no timeout configured; running until completion")
		}
		prun := a.runs.create(prompt)
		eng := buildEngine()
		eng.AgentTracer = tracer
		eng.OnDelta = func(d string) {
			if d == "" {
				return
			}
			payload := map[string]string{"type": "delta", "data": d}
			writeSSE(payload)
		}
		eng.OnThoughtSummary = func(summary string) {
			if summary == "" {
				return
			}
			payload := map[string]string{"type": "thought_summary", "data": summary}
			writeSSE(payload)
		}
		eng.OnToolStart = func(toolName string, args []byte, toolID string) {
			payload := map[string]any{"type": "tool_start", "title": "Tool: " + toolName, "tool_id": toolID, "args": string(args)}
			if toolName == "agent_call" || toolName == "ask_agent" {
				payload["agent"] = true
			}
			writeSSE(payload)
		}
		eng.OnTool = func(toolName string, args []byte, result []byte, toolID string) {
			if toolName == "text_to_speech_chunk" {
				var meta map[string]any
				_ = json.Unmarshal(result, &meta)
				metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
				writeSSE(metaPayload)
				return
			}
			payload := map[string]any{"type": "tool_result", "title": "Tool: " + toolName, "data": string(result), "tool_id": toolID}
			if toolName == "agent_call" || toolName == "ask_agent" {
				payload["agent"] = true
			}
			writeSSE(payload)
			if toolName == "text_to_speech" {
				var resp map[string]any
				if err := json.Unmarshal(result, &resp); err == nil {
					if fp, ok := resp["file_path"].(string); ok && fp != "" {
						trimmed := strings.TrimPrefix(fp, "./")
						trimmed = strings.TrimPrefix(trimmed, "/")
						url := "/audio/" + trimmed
						ap := map[string]any{"type": "tts_audio", "file_path": fp, "url": url}
						writeSSE(ap)
					}
				}
			}
		}
		var turnMessages []llm.Message
		eng.OnTurnMessage = func(msg llm.Message) {
			turnMessages = append(turnMessages, msg)
		}
		eng.OnAssistant = func(msg llm.Message) {
			if len(msg.Images) == 0 {
				return
			}
			saved := saveGeneratedImages(baseDir, msg.Images, "")
			if len(saved) == 0 {
				return
			}
			savedImages = append(savedImages, saved...)
			for _, img := range saved {
				payload := map[string]any{
					"type":     "image",
					"name":     img.Name,
					"mime":     img.MIME,
					"data_url": img.DataURL,
				}
				if img.RelPath != "" {
					payload["rel_path"] = img.RelPath
				}
				if img.FullPath != "" {
					payload["file_path"] = img.FullPath
				}
				writeSSE(payload)
			}
		}
		eng.OnSummaryTriggered = func(inputTokens, tokenBudget, messageCount, summarizedCount int) {
			payload := map[string]any{
				"type":             "summary",
				"input_tokens":     inputTokens,
				"token_budget":     tokenBudget,
				"message_count":    messageCount,
				"summarized_count": summarizedCount,
			}
			writeSSE(payload)
		}

		res, err := eng.RunStream(ctx, prompt, history)
		if err != nil {
			logStreamContextDone(err, r, "/agent/run", sessionID, "", name)
			log.Error().Err(err).Str("specialist", name).Msg("specialist_stream_error")
			writeSSE(map[string]string{"type": "error", "data": "(error) " + err.Error()})
			a.runs.updateStatus(prun.ID, "failed", 0)
			return true
		}
		if len(savedImages) > 0 {
			res = appendImageSummary(res, savedImages)
		}
		payload := map[string]string{"type": "final", "data": res}
		writeSSE(payload)
		a.runs.updateStatus(prun.ID, "completed", 0)
		if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, sessionID, prompt, turnMessages, res, modelLabel); err != nil {
			log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_specialist_stream")
		}
		return true
	}

	seconds := a.cfg.AgentRunTimeoutSeconds
	ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
	defer cancel()

	if opts, ok := llm.ImagePromptFromContext(r.Context()); ok {
		ctx = llm.WithImagePrompt(ctx, opts)
	}
	if dur > 0 {
		log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("specialist", name).Msg("using configured agent timeout")
	} else {
		log.Debug().Str("endpoint", "/agent/run").Str("specialist", name).Msg("no timeout configured; running until completion")
	}
	prun := a.runs.create(prompt)
	eng := buildEngine()
	baseDir := sandbox.ResolveBaseDir(ctx, a.cfg.Workdir)
	var savedImages []savedImage
	var turnMessages []llm.Message
	eng.OnTurnMessage = func(msg llm.Message) {
		turnMessages = append(turnMessages, msg)
	}
	eng.OnAssistant = func(msg llm.Message) {
		if len(msg.Images) == 0 {
			return
		}
		saved := saveGeneratedImages(baseDir, msg.Images, "")
		if len(saved) == 0 {
			return
		}
		savedImages = append(savedImages, saved...)
	}
	out, err := eng.Run(ctx, prompt, history)
	if err != nil {
		log.Error().Err(err).Str("specialist", name).Msg("specialist_inference_error")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		a.runs.updateStatus(prun.ID, "failed", 0)
		return true
	}
	if len(savedImages) > 0 {
		out = appendImageSummary(out, savedImages)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": out})
	a.runs.updateStatus(prun.ID, "completed", 0)
	if err := storeChatTurnWithHistory(r.Context(), a.chatStore, userID, sessionID, prompt, turnMessages, out, modelLabel); err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_specialist")
	}
	return true
}

func logStreamContextDone(err error, r *http.Request, endpoint, sessionID, projectID, specialist string) {
	if err == nil {
		return
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return
	}
	event := log.Warn().
		Err(err).
		Str("endpoint", endpoint).
		Bool("stream", true).
		Str("remote_addr", r.RemoteAddr).
		Bool("deadline_exceeded", errors.Is(err, context.DeadlineExceeded))
	if ua := r.UserAgent(); ua != "" {
		event = event.Str("user_agent", ua)
	}
	if sessionID != "" {
		event = event.Str("session_id", sessionID)
	}
	if projectID != "" {
		event = event.Str("project_id", projectID)
	}
	if specialist != "" {
		event = event.Str("specialist", specialist)
	}
	event.Msg("request_context_done")
}
