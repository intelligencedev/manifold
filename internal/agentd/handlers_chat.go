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
	"manifold/internal/auth"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

type agentStreamTracer struct {
	w       io.Writer
	fl      http.Flusher
	mu      *sync.Mutex
	onTrace func(agent.AgentTrace)
}

func (t *agentStreamTracer) Trace(ev agent.AgentTrace) {
	if t == nil || t.w == nil || t.fl == nil {
		return
	}
	if t.onTrace != nil {
		t.onTrace(ev)
	}
	payload := map[string]any{
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

		window, err := parseWindowParam(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		limit := parseLimitParam(r, 200)

		// Prefer ClickHouse-backed runs when available so the UI persists across restarts.
		if a.runMetrics != nil {
			runs, err := a.runMetrics.RecentRuns(r.Context(), runsWindowOrDefault(window), limit)
			if err != nil {
				log.Warn().Err(err).Msg("clickhouse runs query failed")
			} else if len(runs) > 0 {
				_ = json.NewEncoder(w).Encode(runs)
				return
			}
		}

		runs := filterAgentRunsByWindow(a.runs.list(), time.Now(), window)
		_ = json.NewEncoder(w).Encode(limitAgentRuns(runs, limit))
	}
}

func runsWindowOrDefault(window time.Duration) time.Duration {
	if window > 0 {
		return window
	}
	return 24 * time.Hour
}

func filterAgentRunsByWindow(runs []AgentRun, now time.Time, window time.Duration) []AgentRun {
	if window <= 0 {
		return runs
	}
	cutoff := now.Add(-window)
	filtered := make([]AgentRun, 0, len(runs))
	for _, run := range runs {
		createdAt, err := time.Parse(time.RFC3339, run.CreatedAt)
		if err != nil {
			continue
		}
		if !createdAt.Before(cutoff) && !createdAt.After(now) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func limitAgentRuns(runs []AgentRun, limit int) []AgentRun {
	if limit > 0 && len(runs) > limit {
		return runs[:limit]
	}
	return runs
}

func (a *app) chatSessionsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			userID      *int64
			currentUser *auth.User
			isAdmin     bool
		)
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			currentUser = u
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
			sessions = a.overlayCommandPolicySessionStates(r.Context(), userID, sessions)
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
			sess, err = a.ensureTemporaryChatProject(r, userID, chatRequestOwner(currentUser, userID), sess)
			if err != nil {
				log.Error().Err(err).Str("session", sess.ID).Msg("ensure_temporary_chat_project")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			sess = a.overlayCommandPolicySessionState(r.Context(), userID, sess)
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

func (a *app) agentRunHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := prepareChatTransport(w, r, chatTransportOptions{})
		if !ok {
			return
		}
		state, ok := a.prepareChatHandlerState(w, r, req)
		if !ok {
			return
		}
		r = state.Request
		req = state.RunRequest
		specOwner := state.Owner
		req.ObjectiveID = a.resolveChatObjectiveID(r.Context(), specOwner, req)
		r = r.WithContext(sandbox.WithObjectiveID(r.Context(), req.ObjectiveID))
		memorySettings := chatMemorySettingsFromRunRequest(req)

		target := resolveChatDispatchTarget(r.URL.Query())
		_, hasCustomTarget := a.describeChatTarget(chatTargetDescribeRequest{
			Target:               target,
			SessionID:            req.SessionID,
			ProjectID:            req.ProjectID,
			ObjectiveID:          req.ObjectiveID,
			SystemPromptOverride: req.SystemPrompt,
			Owner:                specOwner,
			MemorySettings:       memorySettings,
		})

		if a.cfg.OpenAI.APIKey == "" && !hasCustomTarget {
			a.handleDevMockChat(w, r, req.Prompt)
			return
		}
		if a.tryHandleDurableChatCompatibility(w, r, req, state, target, "/agent/run") {
			return
		}
		if handled := a.handleChatTarget(w, r, chatTargetHandleRequest{
			Target:               target,
			Prompt:               req.Prompt,
			SessionID:            req.SessionID,
			UserMessageID:        req.UserMessageID,
			AssistantMessageID:   req.AssistantMessageID,
			ProjectID:            req.ProjectID,
			ObjectiveID:          req.ObjectiveID,
			EphemeralSession:     req.EphemeralSession,
			SystemPromptOverride: req.SystemPrompt,
			UserID:               state.UserID,
			Owner:                specOwner,
			Fallback:             a.agentRunOrchestratorDescriptor(r.Context(), specOwner, req, state.CheckedOutWorkspace),
			MemorySettings:       memorySettings,
		}); handled {
			return
		}
	}
}

// commitWorkspace commits workspace changes back to storage.
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
		req, ok := prepareChatTransport(w, r, chatTransportOptions{
			EnablePromptCORS: true,
			MaxBodyBytes:     64 * 1024,
			DecodeErrorLabel: "decode prompt",
		})
		if !ok {
			return
		}
		state, ok := a.prepareChatHandlerState(w, r, req)
		if !ok {
			return
		}
		r = state.Request
		req = state.RunRequest
		specOwner := state.Owner
		req.ObjectiveID = a.resolveChatObjectiveID(r.Context(), specOwner, req)
		r = r.WithContext(sandbox.WithObjectiveID(r.Context(), req.ObjectiveID))
		memorySettings := chatMemorySettingsFromRunRequest(req)

		target := resolveChatDispatchTarget(r.URL.Query())
		_, hasCustomTarget := a.describeChatTarget(chatTargetDescribeRequest{
			Target:               target,
			SessionID:            req.SessionID,
			ProjectID:            req.ProjectID,
			ObjectiveID:          req.ObjectiveID,
			SystemPromptOverride: req.SystemPrompt,
			Owner:                specOwner,
			MemorySettings:       memorySettings,
		})

		if a.cfg.OpenAI.APIKey == "" && !hasCustomTarget {
			a.handleDevMockChat(w, r, req.Prompt)
			return
		}
		if a.tryHandleDurableChatCompatibility(w, r, req, state, target, "/api/prompt") {
			return
		}
		if handled := a.handleChatTarget(w, r, chatTargetHandleRequest{
			Target:               target,
			Prompt:               req.Prompt,
			SessionID:            req.SessionID,
			UserMessageID:        req.UserMessageID,
			AssistantMessageID:   req.AssistantMessageID,
			ProjectID:            req.ProjectID,
			ObjectiveID:          req.ObjectiveID,
			EphemeralSession:     req.EphemeralSession,
			SystemPromptOverride: req.SystemPrompt,
			UserID:               state.UserID,
			Owner:                specOwner,
			Fallback:             a.promptOrchestratorDescriptor(r.Context(), specOwner, req, state.CheckedOutWorkspace),
			MemorySettings:       memorySettings,
		}); handled {
			return
		}
	}
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
