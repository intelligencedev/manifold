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
	RunRequest          chatRunRequest
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

	if req.ProjectID != "" {
		if _, err := workspaces.ValidateProjectID(req.ProjectID); err != nil {
			http.Error(w, "invalid project_id", http.StatusBadRequest)
			return nil, false
		}
	}

	sess, err := ensureChatSession(r.Context(), a.chatStore, userID, req.SessionID)
	if err != nil {
		if err == persist.ErrForbidden {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil, false
		}
		log.Error().Err(err).Str("session", req.SessionID).Msg("ensure_chat_session")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil, false
	}

	if sess.ProjectID != "" {
		if req.ProjectID != "" && req.ProjectID != sess.ProjectID {
			http.Error(w, "session is locked to a different project", http.StatusConflict)
			return nil, false
		}
		req.ProjectID = sess.ProjectID
	}

	sessionMemorySettings := chatMemorySettingsFromSession(sess)
	runMemorySettings := sessionMemorySettings
	memorySettingsChanged := false
	if req.EvolvingMemoryEnabled != nil {
		runMemorySettings.EvolvingMemoryEnabled = *req.EvolvingMemoryEnabled
		memorySettingsChanged = true
	}
	if req.BeliefMemoryEnabled != nil {
		runMemorySettings.BeliefMemoryEnabled = *req.BeliefMemoryEnabled
		memorySettingsChanged = true
	}
	if memorySettingsChanged && runMemorySettings != sessionMemorySettings {
		updated, err := a.chatStore.SetSessionMemorySettings(r.Context(), userID, req.SessionID, runMemorySettings.EvolvingMemoryEnabled, runMemorySettings.BeliefMemoryEnabled)
		if err != nil {
			if err == persist.ErrForbidden {
				http.Error(w, "forbidden", http.StatusForbidden)
				return nil, false
			}
			log.Error().Err(err).Str("session", req.SessionID).Msg("set_chat_session_memory_settings")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return nil, false
		}
		runMemorySettings = chatMemorySettingsFromSession(updated)
	}
	req.EvolvingMemoryEnabled = boolPtr(runMemorySettings.EvolvingMemoryEnabled)
	req.BeliefMemoryEnabled = boolPtr(runMemorySettings.BeliefMemoryEnabled)

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

	if sess.ProjectID == "" && req.ProjectID != "" {
		updated, err := a.chatStore.SetSessionProject(r.Context(), userID, req.SessionID, req.ProjectID)
		if err != nil {
			if err == persist.ErrForbidden {
				http.Error(w, "forbidden", http.StatusForbidden)
				return nil, false
			}
			log.Error().Err(err).Str("session", req.SessionID).Str("project_id", req.ProjectID).Msg("lock_chat_session_project")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return nil, false
		}
		req.ProjectID = updated.ProjectID
	}

	if req.Image {
		r = r.WithContext(llm.WithImagePrompt(r.Context(), llm.ImagePromptOptions{Size: req.ImageSize}))
	}

	return &preparedChatHandlerState{
		Request:             r,
		RunRequest:          req,
		UserID:              userID,
		CurrentUser:         currentUser,
		Owner:               chatRequestOwner(currentUser, userID),
		CheckedOutWorkspace: checkedOutWorkspace,
	}, true
}
