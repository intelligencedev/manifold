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
	"manifold/internal/auth"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
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
		"type":            ev.Type,
		"agent":           ev.Agent,
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
		subresourceID := ""
		if len(parts) >= 2 {
			subresource = parts[1]
		}
		if len(parts) >= 3 {
			subresourceID = parts[2]
		}
		switch subresource {
		case "messages":
			setChatCORSHeaders(w, r, "GET, DELETE, OPTIONS")
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
			if subresourceID != "" {
				if r.Method != http.MethodDelete {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				// Load messages to determine summary impact and related tool outputs.
				msgs, err := a.chatStore.ListMessages(r.Context(), userID, id, 0)
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
				msgIndex := -1
				var target persist.ChatMessage
				for i, m := range msgs {
					if m.ID == subresourceID {
						msgIndex = i
						target = m
						break
					}
				}
				if msgIndex == -1 {
					http.NotFound(w, r)
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

				// Delete target message first.
				if err := a.chatStore.DeleteMessage(r.Context(), userID, id, subresourceID); err != nil {
					if errors.Is(err, persist.ErrForbidden) {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
					if errors.Is(err, persist.ErrNotFound) {
						http.NotFound(w, r)
						return
					}
					log.Error().Err(err).Str("session", id).Msg("delete_chat_message")
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}

				// Remove related tool outputs if the assistant message contained tool calls.
				toolIDs := toolCallIDsFromMessage(target)
				if len(toolIDs) > 0 {
					toolSet := make(map[string]struct{}, len(toolIDs))
					for _, id := range toolIDs {
						toolSet[id] = struct{}{}
					}
					for _, m := range msgs {
						if m.Role != "tool" {
							continue
						}
						if toolID := toolIDFromMessage(m); toolID != "" {
							if _, ok := toolSet[toolID]; ok {
								_ = a.chatStore.DeleteMessage(r.Context(), userID, id, m.ID)
							}
						}
					}
				}

				// If the deleted message was part of the summarized range, clear summary.
				if sess.SummarizedCount > 0 && msgIndex < sess.SummarizedCount {
					if err := a.chatStore.UpdateSummary(r.Context(), userID, id, "", 0); err != nil {
						log.Error().Err(err).Str("session", id).Msg("reset_chat_summary")
					}
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}

			if r.Method == http.MethodDelete {
				afterID := strings.TrimSpace(r.URL.Query().Get("after"))
				if afterID == "" {
					http.Error(w, "missing after", http.StatusBadRequest)
					return
				}
				inclusive := false
				switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("inclusive"))) {
				case "true", "1", "yes", "y":
					inclusive = true
				}

				msgs, err := a.chatStore.ListMessages(r.Context(), userID, id, 0)
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
				msgIndex := -1
				var target persist.ChatMessage
				for i, m := range msgs {
					if m.ID == afterID {
						msgIndex = i
						target = m
						break
					}
				}
				if msgIndex == -1 {
					http.NotFound(w, r)
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

				if err := a.chatStore.DeleteMessagesAfter(r.Context(), userID, id, afterID, inclusive); err != nil {
					if errors.Is(err, persist.ErrForbidden) {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
					if errors.Is(err, persist.ErrNotFound) {
						http.NotFound(w, r)
						return
					}
					log.Error().Err(err).Str("session", id).Msg("delete_chat_messages_after")
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}

				// Remove related tool outputs if the target assistant message contained tool calls.
				if inclusive {
					toolIDs := toolCallIDsFromMessage(target)
					if len(toolIDs) > 0 {
						toolSet := make(map[string]struct{}, len(toolIDs))
						for _, id := range toolIDs {
							toolSet[id] = struct{}{}
						}
						for _, m := range msgs {
							if m.Role != "tool" {
								continue
							}
							if toolID := toolIDFromMessage(m); toolID != "" {
								if _, ok := toolSet[toolID]; ok {
									_ = a.chatStore.DeleteMessage(r.Context(), userID, id, m.ID)
								}
							}
						}
					}
				}

				remainingCount := msgIndex + 1
				if inclusive {
					remainingCount = msgIndex
				}
				if sess.SummarizedCount > remainingCount {
					if err := a.chatStore.UpdateSummary(r.Context(), userID, id, "", 0); err != nil {
						log.Error().Err(err).Str("session", id).Msg("reset_chat_summary")
					}
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}

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

func toolCallIDsFromMessage(msg persist.ChatMessage) []string {
	if msg.Role != "assistant" {
		return nil
	}
	trimmed := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var data struct {
		ToolCalls []llm.ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return nil
	}
	if len(data.ToolCalls) == 0 {
		return nil
	}
	ids := make([]string, 0, len(data.ToolCalls))
	for _, tc := range data.ToolCalls {
		id := strings.TrimSpace(tc.ID)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func toolIDFromMessage(msg persist.ChatMessage) string {
	if msg.Role != "tool" {
		return ""
	}
	trimmed := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(trimmed, "{") {
		return ""
	}
	var data struct {
		ToolID string `json:"tool_id"`
	}
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.ToolID)
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
		var req chatRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.normalize()
		r, checkedOutWorkspace, statusCode, err := a.prepareChatRunRequest(r, userID, req)
		if err != nil {
			switch statusCode {
			case http.StatusBadRequest:
				if errors.Is(err, workspaces.ErrInvalidProjectID) {
					http.Error(w, "invalid project_id", http.StatusBadRequest)
					return
				}
				if errors.Is(err, workspaces.ErrProjectNotFound) {
					http.Error(w, "project not found (project_id must match the project directory/ID)", http.StatusBadRequest)
					return
				}
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			case http.StatusInternalServerError:
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			default:
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
		}

		currentRun := a.runs.create(req.Prompt)

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

		target := resolveChatDispatchTarget(r.URL.Query())
		_, hasCustomTarget := a.describeChatTarget(target, req.SystemPrompt, specOwner)

		if a.cfg.OpenAI.APIKey == "" && !hasCustomTarget {
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
		if handled := a.handleChatTarget(w, r, target, req.Prompt, req.SessionID, req.SystemPrompt, history, userID, specOwner, a.agentRunOrchestratorDescriptor(r.Context(), specOwner, req, checkedOutWorkspace, memorySummaryResult)); handled {
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

		var req chatRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("decode prompt: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.normalize()
		r, checkedOutWorkspace, statusCode, err := a.prepareChatRunRequest(r, userID, req)
		if err != nil {
			switch statusCode {
			case http.StatusBadRequest:
				if errors.Is(err, workspaces.ErrInvalidProjectID) {
					http.Error(w, "invalid project_id", http.StatusBadRequest)
					return
				}
				if errors.Is(err, workspaces.ErrProjectNotFound) {
					http.Error(w, "project not found (project_id must match the project directory/ID)", http.StatusBadRequest)
					return
				}
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			case http.StatusInternalServerError:
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			default:
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
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

		specOwner := systemUserID
		if userID != nil {
			specOwner = *userID
		}

		target := resolveChatDispatchTarget(r.URL.Query())
		_, hasCustomTarget := a.describeChatTarget(target, req.SystemPrompt, specOwner)

		if a.cfg.OpenAI.APIKey == "" && !hasCustomTarget {
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
		if handled := a.handleChatTarget(w, r, target, req.Prompt, req.SessionID, req.SystemPrompt, history, userID, specOwner, a.promptOrchestratorDescriptor(r.Context(), specOwner, req, checkedOutWorkspace)); handled {
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
