package agentd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/warpp"
)

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
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		currentRun := a.runs.create(req.Prompt)
		if req.SessionID == "" {
			req.SessionID = "default"
		}

		if strings.TrimSpace(req.ProjectID) != "" {
			var uid int64
			if userID != nil {
				uid = *userID
			}
			base := filepath.Join(a.cfg.Workdir, "users", fmt.Sprint(uid), "projects", req.ProjectID)
			r = r.WithContext(sandbox.WithBaseDir(r.Context(), base))
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
		history, err := a.chatMemory.BuildContext(r.Context(), userID, req.SessionID)
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
		if specialistName != "" && !strings.EqualFold(specialistName, "orchestrator") {
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

		eng := a.engine
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
			ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
			defer cancel()
			if dur > 0 {
				log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Bool("stream", true).Msg("using configured stream timeout")
			} else {
				log.Debug().Str("endpoint", "/agent/run").Bool("stream", true).Msg("no timeout configured; running until completion")
			}

			eng.OnDelta = func(d string) {
				payload := map[string]string{"type": "delta", "data": d}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnToolStart = func(name string, args []byte, toolID string) {
				payload := map[string]any{"type": "tool_start", "title": "Tool: " + name, "tool_id": toolID, "args": string(args)}
				// Hint UI to group agent-related tool calls under a team panel
				if name == "agent_call" || name == "ask_agent" {
					payload["agent"] = true
				}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnTool = func(name string, args []byte, result []byte) {
				if name == "text_to_speech_chunk" {
					var meta map[string]any
					_ = json.Unmarshal(result, &meta)
					metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
					b, _ := json.Marshal(metaPayload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
					return
				}
				payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result)}
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

			res, err := eng.RunStream(ctx, req.Prompt, history)
			if err != nil {
				log.Error().Err(err).Msg("agent run error")
				if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
					fmt.Fprintf(w, "data: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "data: %q\n\n", "(error)")
				}
				fl.Flush()
				a.runs.updateStatus(currentRun.ID, "failed", 0)
				return
			}
			payload := map[string]string{"type": "final", "data": res}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			a.runs.updateStatus(currentRun.ID, "completed", 0)
			if err := storeChatTurn(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, res, eng.Model); err != nil {
				log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_stream")
			}
			return
		}

		seconds := a.cfg.AgentRunTimeoutSeconds
		ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
		defer cancel()
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Bool("stream", false).Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/agent/run").Bool("stream", false).Msg("no timeout configured; running until completion")
		}
		result, err := eng.Run(ctx, req.Prompt, history)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			a.runs.updateStatus(currentRun.ID, "failed", 0)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
		a.runs.updateStatus(currentRun.ID, "completed", 0)
		if err := storeChatTurn(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, result, eng.Model); err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
		}
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
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("decode prompt: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.SessionID == "" {
			req.SessionID = "default"
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
		history, err := a.chatMemory.BuildContext(r.Context(), userID, req.SessionID)
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

		eng := a.engine
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
			ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
			defer cancel()
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
			eng.OnTool = func(name string, args []byte, result []byte) {
				if name == "text_to_speech_chunk" {
					var meta map[string]any
					_ = json.Unmarshal(result, &meta)
					metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
					b, _ := json.Marshal(metaPayload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
					return
				}
				payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result)}
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

			prun := a.runs.create(req.Prompt)
			res, err := eng.RunStream(ctx, req.Prompt, history)
			if err != nil {
				log.Error().Err(err).Msg("agent run error")
				if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
					fmt.Fprintf(w, "data: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "data: %q\n\n", "(error)")
				}
				fl.Flush()
				a.runs.updateStatus(prun.ID, "failed", 0)
				return
			}
			payload := map[string]string{"type": "final", "data": res}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			a.runs.updateStatus(prun.ID, "completed", 0)
			if err := storeChatTurn(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, res, eng.Model); err != nil {
				log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_stream")
			}
			return
		}

		seconds := a.cfg.AgentRunTimeoutSeconds
		ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
		defer cancel()
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/api/prompt").Bool("stream", false).Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/api/prompt").Bool("stream", false).Msg("no timeout configured; running until completion")
		}
		prun := a.runs.create(req.Prompt)
		result, err := eng.Run(ctx, req.Prompt, history)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			a.runs.updateStatus(prun.ID, "failed", 0)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
		a.runs.updateStatus(prun.ID, "completed", 0)
		if err := storeChatTurn(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, result, eng.Model); err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
		}
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

	if r.Header.Get("Accept") == "text/event-stream" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return true
		}
		seconds := a.cfg.StreamRunTimeoutSeconds
		if seconds <= 0 {
			seconds = a.cfg.AgentRunTimeoutSeconds
		}
		ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
		defer cancel()
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("specialist", name).Bool("stream", true).Msg("using configured stream timeout")
		} else {
			log.Debug().Str("endpoint", "/agent/run").Str("specialist", name).Bool("stream", true).Msg("no timeout configured; running until completion")
		}
		prun := a.runs.create(prompt)
		handler := &specialistStreamHandler{w: w, fl: fl}
		if err := sp.Stream(ctx, prompt, history, handler); err != nil {
			log.Error().Err(err).Str("specialist", name).Msg("specialist_stream_error")
			handler.SendError(err.Error())
			a.runs.updateStatus(prun.ID, "failed", 0)
			return true
		}
		final := handler.Final()
		handler.SendFinal(final)
		a.runs.updateStatus(prun.ID, "completed", 0)
		if err := storeChatTurn(r.Context(), a.chatStore, userID, sessionID, prompt, final, modelLabel); err != nil {
			log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_specialist_stream")
		}
		return true
	}

	seconds := a.cfg.AgentRunTimeoutSeconds
	ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
	defer cancel()
	if dur > 0 {
		log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("specialist", name).Msg("using configured agent timeout")
	} else {
		log.Debug().Str("endpoint", "/agent/run").Str("specialist", name).Msg("no timeout configured; running until completion")
	}
	prun := a.runs.create(prompt)
	out, err := sp.Inference(ctx, prompt, history)
	if err != nil {
		log.Error().Err(err).Str("specialist", name).Msg("specialist_inference_error")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		a.runs.updateStatus(prun.ID, "failed", 0)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": out})
	a.runs.updateStatus(prun.ID, "completed", 0)
	if err := storeChatTurn(r.Context(), a.chatStore, userID, sessionID, prompt, out, modelLabel); err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_specialist")
	}
	return true
}

type specialistStreamHandler struct {
	w   http.ResponseWriter
	fl  http.Flusher
	buf strings.Builder
}

func (h *specialistStreamHandler) OnDelta(content string) {
	if content == "" {
		return
	}
	h.buf.WriteString(content)
	payload := map[string]string{"type": "delta", "data": content}
	h.write(payload)
}

func (h *specialistStreamHandler) OnToolCall(tc llm.ToolCall) {
	// Specialists are streamed as plain responses; tool calls are not surfaced.
}

func (h *specialistStreamHandler) Final() string {
	return h.buf.String()
}

func (h *specialistStreamHandler) SendFinal(text string) {
	h.write(map[string]string{"type": "final", "data": text})
}

func (h *specialistStreamHandler) SendError(msg string) {
	h.write(map[string]string{"type": "error", "data": msg})
}

func (h *specialistStreamHandler) write(payload map[string]string) {
	b, _ := json.Marshal(payload)
	fmt.Fprintf(h.w, "data: %s\n\n", b)
	h.fl.Flush()
}
