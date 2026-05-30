package agentd

import (
	"net/http"
	"strings"

	"manifold/internal/auth"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
	"manifold/internal/projects"
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

type chatAuthState struct {
	UserID      *int64
	CurrentUser *auth.User
}

func (a *app) prepareChatHandlerState(w http.ResponseWriter, r *http.Request, req chatRunRequest) (*preparedChatHandlerState, bool) {
	authState, ok := a.prepareChatAuthState(w, r)
	if !ok {
		return nil, false
	}

	if !validateChatProjectID(w, req.ProjectID) {
		return nil, false
	}

	sess, ok := a.prepareChatSession(w, r, req, authState.UserID, chatRequestOwner(authState.CurrentUser, authState.UserID))
	if !ok {
		return nil, false
	}

	req, ok = applySessionProject(w, req, sess.ProjectID)
	if !ok {
		return nil, false
	}

	req, ok = a.prepareChatMemorySettings(w, r, req, sess, authState.UserID)
	if !ok {
		return nil, false
	}

	r, checkedOutWorkspace, ok := a.prepareCheckedOutChatRequest(w, r, req, authState.UserID)
	if !ok {
		return nil, false
	}

	req, ok = a.lockChatSessionProject(w, r, req, sess.ProjectID, authState.UserID)
	if !ok {
		return nil, false
	}

	return &preparedChatHandlerState{
		Request:             withChatImagePrompt(r, req),
		RunRequest:          req,
		UserID:              authState.UserID,
		CurrentUser:         authState.CurrentUser,
		Owner:               chatRequestOwner(authState.CurrentUser, authState.UserID),
		CheckedOutWorkspace: checkedOutWorkspace,
	}, true
}

func (a *app) prepareChatAuthState(w http.ResponseWriter, r *http.Request) (chatAuthState, bool) {
	if !a.cfg.Auth.Enabled {
		return chatAuthState{}, true
	}
	u, ok := auth.CurrentUser(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return chatAuthState{}, false
	}
	id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
	if err != nil {
		log.Error().Err(err).Msg("resolve_chat_access")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return chatAuthState{}, false
	}
	return chatAuthState{UserID: id, CurrentUser: u}, true
}

func validateChatProjectID(w http.ResponseWriter, projectID string) bool {
	if projectID == "" {
		return true
	}
	if _, err := workspaces.ValidateProjectID(projectID); err != nil {
		http.Error(w, "invalid project_id", http.StatusBadRequest)
		return false
	}
	return true
}

func (a *app) prepareChatSession(w http.ResponseWriter, r *http.Request, req chatRunRequest, userID *int64, owner int64) (persist.ChatSession, bool) {
	sess, err := ensureChatSession(r.Context(), a.chatStore, userID, req.SessionID)
	if err != nil {
		writeChatStoreError(w, err, "ensure_chat_session", req.SessionID)
		return persist.ChatSession{}, false
	}
	if sess.ProjectID == "" && req.ProjectID == "" {
		sess, err = a.ensureTemporaryChatProject(r, userID, owner, sess)
		if err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("ensure_temporary_chat_project")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return persist.ChatSession{}, false
		}
	}
	return sess, true
}

func (a *app) ensureTemporaryChatProject(r *http.Request, userID *int64, owner int64, sess persist.ChatSession) (persist.ChatSession, error) {
	if a.projectsService == nil {
		return sess, nil
	}
	name := "Temporary Chat"
	if trimmed := strings.TrimSpace(sess.Name); trimmed != "" && !isDefaultSessionName(trimmed) {
		name = "Temporary " + trimmed
	}
	project, err := a.projectsService.CreateProjectKind(r.Context(), owner, name, projects.ProjectKindTemporary)
	if err != nil {
		return persist.ChatSession{}, err
	}
	return a.chatStore.SetSessionProject(r.Context(), userID, sess.ID, project.ID)
}

func applySessionProject(w http.ResponseWriter, req chatRunRequest, sessionProjectID string) (chatRunRequest, bool) {
	if sessionProjectID == "" {
		return req, true
	}
	if req.ProjectID != "" && req.ProjectID != sessionProjectID {
		http.Error(w, "session is locked to a different project", http.StatusConflict)
		return req, false
	}
	req.ProjectID = sessionProjectID
	return req, true
}

func (a *app) prepareChatMemorySettings(w http.ResponseWriter, r *http.Request, req chatRunRequest, sess persist.ChatSession, userID *int64) (chatRunRequest, bool) {
	sessionSettings := chatMemorySettingsFromSession(sess)
	runSettings, changed := requestedChatMemorySettings(req, sessionSettings)
	if changed && runSettings != sessionSettings {
		updated, err := a.chatStore.SetSessionMemorySettings(r.Context(), userID, req.SessionID, runSettings.EvolvingMemoryEnabled, runSettings.BeliefMemoryEnabled)
		if err != nil {
			writeChatStoreError(w, err, "set_chat_session_memory_settings", req.SessionID)
			return req, false
		}
		runSettings = chatMemorySettingsFromSession(updated)
	}
	req.EvolvingMemoryEnabled = boolPtr(runSettings.EvolvingMemoryEnabled)
	req.BeliefMemoryEnabled = boolPtr(runSettings.BeliefMemoryEnabled)
	return req, true
}

func requestedChatMemorySettings(req chatRunRequest, settings chatMemoryRunSettings) (chatMemoryRunSettings, bool) {
	changed := false
	if req.EvolvingMemoryEnabled != nil {
		settings.EvolvingMemoryEnabled = *req.EvolvingMemoryEnabled
		changed = true
	}
	if req.BeliefMemoryEnabled != nil {
		settings.BeliefMemoryEnabled = *req.BeliefMemoryEnabled
		changed = true
	}
	return settings, changed
}

func (a *app) prepareCheckedOutChatRequest(w http.ResponseWriter, r *http.Request, req chatRunRequest, userID *int64) (*http.Request, *workspaces.Workspace, bool) {
	r, checkedOutWorkspace, statusCode, err := a.prepareChatRunRequest(r, userID, req)
	if err != nil {
		writePrepareChatRunError(w, statusCode, err)
		return nil, nil, false
	}
	return r, checkedOutWorkspace, true
}

func writePrepareChatRunError(w http.ResponseWriter, statusCode int, err error) {
	if statusCode != http.StatusBadRequest {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	switch {
	case err == workspaces.ErrInvalidProjectID:
		http.Error(w, "invalid project_id", http.StatusBadRequest)
	case err == workspaces.ErrProjectNotFound:
		http.Error(w, "project not found (project_id must match the project directory/ID)", http.StatusBadRequest)
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
	}
}

func (a *app) lockChatSessionProject(w http.ResponseWriter, r *http.Request, req chatRunRequest, sessionProjectID string, userID *int64) (chatRunRequest, bool) {
	if sessionProjectID != "" || req.ProjectID == "" {
		return req, true
	}
	updated, err := a.chatStore.SetSessionProject(r.Context(), userID, req.SessionID, req.ProjectID)
	if err != nil {
		writeChatStoreError(w, err, "lock_chat_session_project", req.SessionID)
		return req, false
	}
	req.ProjectID = updated.ProjectID
	return req, true
}

func writeChatStoreError(w http.ResponseWriter, err error, operation, sessionID string) {
	if err == persist.ErrForbidden {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	log.Error().Err(err).Str("session", sessionID).Msg(operation)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func withChatImagePrompt(r *http.Request, req chatRunRequest) *http.Request {
	if !req.Image {
		return r
	}
	return r.WithContext(llm.WithImagePrompt(r.Context(), llm.ImagePromptOptions{Size: req.ImageSize}))
}
