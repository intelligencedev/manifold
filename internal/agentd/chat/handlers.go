package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	persist "manifold/internal/persistence"
)

// ErrUnauthorized tells a handler adapter that the request has no usable
// authenticated identity. Other access-resolution errors are treated as
// server failures.
var ErrUnauthorized = errors.New("unauthorized")

// SessionAccess contains the identity resolved by the composition root.
type SessionAccess struct {
	UserID      *int64
	CurrentUser *auth.User
	IsAdmin     bool
}

// SessionHandlerDeps contains only the application callbacks needed by the
// session collection endpoint. Policy overlays and temporary-project setup
// remain injectable because they belong to agentd's application lifecycle.
type SessionHandlerDeps struct {
	AuthEnabled bool
	Store       persist.ChatStore

	ResolveAccess func(context.Context, *http.Request) (SessionAccess, error)
	SetCORS       func(http.ResponseWriter, *http.Request, string)

	OverlaySessions        func(context.Context, *int64, []persist.ChatSession) []persist.ChatSession
	OverlaySession         func(context.Context, *int64, persist.ChatSession) persist.ChatSession
	EnsureTemporaryProject func(*http.Request, *int64, int64, persist.ChatSession) (persist.ChatSession, error)
	RequestOwner           func(*auth.User, *int64) int64
}

// SessionsHandler returns the collection handler for chat sessions.
func SessionsHandler(deps SessionHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := resolveSessionAccess(w, r, deps)
		if !ok {
			return
		}
		if deps.SetCORS != nil {
			deps.SetCORS(w, r, "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if deps.Store == nil {
			http.Error(w, "chat sessions unavailable", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			sessions, err := deps.Store.ListSessions(r.Context(), access.UserID)
			if err != nil {
				log.Error().Err(err).Msg("list_chat_sessions")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if deps.OverlaySessions != nil {
				sessions = deps.OverlaySessions(r.Context(), access.UserID, sessions)
			}
			writeSessionsJSON(w, http.StatusOK, sessions)

		case http.MethodPost:
			defer r.Body.Close()
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			sess, err := deps.Store.CreateSession(r.Context(), access.UserID, body.Name)
			if err != nil {
				if errors.Is(err, persist.ErrForbidden) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				log.Error().Err(err).Msg("create_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if deps.EnsureTemporaryProject != nil {
				owner := int64(0)
				if deps.RequestOwner != nil {
					owner = deps.RequestOwner(access.CurrentUser, access.UserID)
				}
				sess, err = deps.EnsureTemporaryProject(r, access.UserID, owner, sess)
				if err != nil {
					log.Error().Err(err).Str("session", sess.ID).Msg("ensure_temporary_chat_project")
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
			}
			if deps.OverlaySession != nil {
				sess = deps.OverlaySession(r.Context(), access.UserID, sess)
			}
			writeSessionsJSON(w, http.StatusCreated, sess)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func resolveSessionAccess(w http.ResponseWriter, r *http.Request, deps SessionHandlerDeps) (SessionAccess, bool) {
	if !deps.AuthEnabled {
		return SessionAccess{IsAdmin: true}, true
	}
	if deps.ResolveAccess == nil {
		writeUnauthorized(w)
		return SessionAccess{}, false
	}
	access, err := deps.ResolveAccess(r.Context(), r)
	if err == nil {
		return access, true
	}
	if errors.Is(err, ErrUnauthorized) {
		writeUnauthorized(w)
		return SessionAccess{}, false
	}
	log.Error().Err(err).Msg("resolve_chat_access")
	http.Error(w, "internal server error", http.StatusInternalServerError)
	return SessionAccess{}, false
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="sio"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func writeSessionsJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Error().Err(err).Msg("encode_chat_sessions")
	}
}
