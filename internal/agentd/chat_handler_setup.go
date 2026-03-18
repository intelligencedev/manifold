package agentd

import (
	"net/http"

	"manifold/internal/auth"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/workspaces"

	"github.com/rs/zerolog/log"
)

type preparedChatHandlerState struct {
	Request             *http.Request
	UserID              *int64
	CurrentUser         *auth.User
	Owner               int64
	CheckedOutWorkspace *workspaces.Workspace
}

func chatRequestOwner(currentUser *auth.User, userID *int64) int64 {
	if currentUser != nil {
		return currentUser.ID
	}
	if userID != nil {
		return *userID
	}
	return systemUserID
}

func (a *app) prepareChatHandlerState(w http.ResponseWriter, r *http.Request, req chatRunRequest) (*preparedChatHandlerState, bool) {
	var (
		userID      *int64
		currentUser *auth.User
	)

	if a.cfg.Auth.Enabled {
		u, ok := auth.CurrentUser(r.Context())
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return nil, false
		}
		currentUser = u
		id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
		if err != nil {
			log.Error().Err(err).Msg("resolve_chat_access")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return nil, false
		}
		userID = id
	}

	r, checkedOutWorkspace, statusCode, err := a.prepareChatRunRequest(r, userID, req)
	if err != nil {
		switch statusCode {
		case http.StatusBadRequest:
			switch {
			case err == workspaces.ErrInvalidProjectID:
				http.Error(w, "invalid project_id", http.StatusBadRequest)
			case err == workspaces.ErrProjectNotFound:
				http.Error(w, "project not found (project_id must match the project directory/ID)", http.StatusBadRequest)
			default:
				http.Error(w, "bad request", http.StatusBadRequest)
			}
		case http.StatusInternalServerError:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return nil, false
	}

	if _, err := ensureChatSession(r.Context(), a.chatStore, userID, req.SessionID); err != nil {
		if err == persist.ErrForbidden {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil, false
		}
		log.Error().Err(err).Str("session", req.SessionID).Msg("ensure_chat_session")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil, false
	}

	if req.Image {
		r = r.WithContext(llm.WithImagePrompt(r.Context(), llm.ImagePromptOptions{Size: req.ImageSize}))
	}

	return &preparedChatHandlerState{
		Request:             r,
		UserID:              userID,
		CurrentUser:         currentUser,
		Owner:               chatRequestOwner(currentUser, userID),
		CheckedOutWorkspace: checkedOutWorkspace,
	}, true
}
