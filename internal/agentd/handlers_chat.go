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
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	specialiststool "manifold/internal/tools/specialists"
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
		setChatCORSHeaders(w, r, "GET, PATCH, DELETE, OPTIONS")
		if len(parts) == 2 && parts[1] == "messages" {
			setChatCORSHeaders(w, r, "GET, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if len(parts) == 2 && parts[1] == "messages" {
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
		userSpecReg, err := a.specialistsRegistryForUser(r.Context(), specOwner)
		if err != nil {
			log.Error().Err(err).Msg("load_specialists_registry")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if spName := strings.TrimSpace(r.URL.Query().Get("specialist")); spName != "" && strings.ToLower(spName) != "orchestrator" {
			if aSpec, ok := userSpecReg.Get(spName); ok {
				seconds := a.cfg.AgentRunTimeoutSeconds
				ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
				defer cancel()
				ctx = specialiststool.WithRegistry(ctx, userSpecReg)
				if dur > 0 {
					log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("mode", "specialist_override").Msg("using configured agent timeout")
				} else {
					log.Debug().Str("endpoint", "/agent/run").Str("mode", "specialist_override").Msg("no timeout configured; running until completion")
				}
				if r.Header.Get("Accept") == "text/event-stream" {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-cache")
					fl, ok := w.(http.Flusher)
					if !ok {
						http.Error(w, "streaming not supported", http.StatusInternalServerError)
						return
					}
					out, err := aSpec.Inference(ctx, req.Prompt, history)
					if err != nil {
						log.Error().Err(err).Msg("specialist override error")
						if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
							fmt.Fprintf(w, "data: %s\n\n", b)
						} else {
							fmt.Fprintf(w, "data: %q\n\n", "(error)")
						}
						fl.Flush()
						a.runs.updateStatus(currentRun.ID, "failed", 0)
						return
					}
					payload := map[string]string{"type": "final", "data": out}
					b, _ := json.Marshal(payload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
					a.runs.updateStatus(currentRun.ID, "completed", 0)
					if err := storeChatTurn(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, out, aSpec.Model); err != nil {
						log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_specialist_stream")
					}
					return
				}
				out, err := aSpec.Inference(ctx, req.Prompt, history)
				if err != nil {
					log.Error().Err(err).Msg("specialist override error")
					http.Error(w, "internal server error", http.StatusInternalServerError)
					a.runs.updateStatus(currentRun.ID, "failed", 0)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"result": out})
				a.runs.updateStatus(currentRun.ID, "completed", 0)
				if err := storeChatTurn(r.Context(), a.chatStore, userID, req.SessionID, req.Prompt, out, aSpec.Model); err != nil {
					log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_specialist")
				}
				return
			}
		}

		if name := specialists.Route(a.cfg.SpecialistRoutes, req.Prompt); name != "" {
			log.Info().Str("route", name).Msg("pre-dispatch specialist route matched")
			aSpec, ok := a.specRegistry.Get(name)
			if !ok {
				log.Error().Str("route", name).Msg("specialist not found for route")
			} else {
				seconds := a.cfg.AgentRunTimeoutSeconds
				ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
				defer cancel()
				if dur > 0 {
					log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("mode", "specialist_pre_dispatch").Msg("using configured agent timeout")
				} else {
					log.Debug().Str("endpoint", "/agent/run").Str("mode", "specialist_pre_dispatch").Msg("no timeout configured; running until completion")
				}
				out, err := aSpec.Inference(ctx, req.Prompt, nil)
				if err != nil {
					log.Error().Err(err).Msg("specialist pre-dispatch")
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"result": out})
				return
			}
		}

		if r.URL.Query().Get("warpp") == "true" {
			seconds := a.cfg.WorkflowTimeoutSeconds
			if seconds <= 0 {
				seconds = a.cfg.AgentRunTimeoutSeconds
			}
			ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
			defer cancel()
			ctx = specialiststool.WithRegistry(ctx, userSpecReg)
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
			ctx = specialiststool.WithRegistry(ctx, userSpecReg)
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
