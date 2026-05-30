package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	persist "manifold/internal/persistence"
	"manifold/internal/projects"
	"manifold/internal/workspaces"
)

func (a *app) chatSessionDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, currentUser, ok := a.chatDetailAccess(w, r)
		if !ok {
			return
		}
		id, subresource, subresourceID, ok := parseChatSessionDetailPath(r)
		if !ok {
			http.NotFound(w, r)
			return
		}
		setChatDetailCORSHeaders(w, r, subresource)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch subresource {
		case "activities":
			a.handleChatActivities(w, r, userID, id)
		case "messages":
			a.handleChatMessages(w, r, userID, id, subresourceID)
		case "title":
			a.handleChatTitle(w, r, userID, id)
		default:
			a.handleChatSession(w, r, currentUser, userID, id)
		}
	}
}

func (a *app) chatDetailAccess(w http.ResponseWriter, r *http.Request) (*int64, *auth.User, bool) {
	if !a.cfg.Auth.Enabled {
		return nil, nil, true
	}
	u, ok := auth.CurrentUser(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, nil, false
	}
	userID, _, err := resolveChatAccess(r.Context(), a.authStore, u)
	if err != nil {
		log.Error().Err(err).Msg("resolve_chat_access")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil, nil, false
	}
	return userID, u, true
}

func parseChatSessionDetailPath(r *http.Request) (string, string, string, bool) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/chat/sessions/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	subresource := ""
	subresourceID := ""
	if len(parts) >= 2 {
		subresource = parts[1]
	}
	if len(parts) >= 3 {
		subresourceID = parts[2]
	}
	return parts[0], subresource, subresourceID, true
}

func setChatDetailCORSHeaders(w http.ResponseWriter, r *http.Request, subresource string) {
	switch subresource {
	case "messages":
		setChatCORSHeaders(w, r, "GET, DELETE, OPTIONS")
	case "activities":
		setChatCORSHeaders(w, r, "GET, OPTIONS")
	case "title":
		setChatCORSHeaders(w, r, "POST, OPTIONS")
	default:
		setChatCORSHeaders(w, r, "GET, PATCH, DELETE, OPTIONS")
	}
}

func (a *app) handleChatActivities(w http.ResponseWriter, r *http.Request, userID *int64, sessionID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.activityStore == nil {
		http.Error(w, "specialist activity unavailable", http.StatusServiceUnavailable)
		return
	}
	if _, err := a.chatStore.GetSession(r.Context(), userID, sessionID); err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "get_chat_session_for_activities")
		return
	}
	activities, err := a.activityStore.ListSessionActivities(r.Context(), userID, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("list_chat_activities")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeChatJSON(w, activities, "encode_chat_activities")
}

func (a *app) handleChatMessages(w http.ResponseWriter, r *http.Request, userID *int64, sessionID, messageID string) {
	if messageID != "" {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.deleteChatMessage(w, r, userID, sessionID, messageID)
		return
	}
	if r.Method == http.MethodDelete {
		a.deleteChatMessagesAfter(w, r, userID, sessionID)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := chatMessageLimit(r)
	msgs, err := a.chatStore.ListMessages(r.Context(), userID, sessionID, limit)
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "list_chat_messages")
		return
	}
	writeChatJSON(w, hydrateChatMessages(msgs), "encode_chat_messages")
}

func chatMessageLimit(r *http.Request) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func (a *app) deleteChatMessage(w http.ResponseWriter, r *http.Request, userID *int64, sessionID, messageID string) {
	msgs, msgIndex, target, ok := a.findChatMessage(w, r, userID, sessionID, messageID)
	if !ok {
		return
	}
	sess, err := a.chatStore.GetSession(r.Context(), userID, sessionID)
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "get_chat_session")
		return
	}
	relatedMessageIDs := relatedToolMessageIDs(msgs, target)
	resetSummary := sess.SummarizedCount > 0 && msgIndex < sess.SummarizedCount
	if atomicStore, ok := a.chatStore.(atomicChatTurnDeleteStore); ok {
		err := atomicStore.DeleteMessageWithRelated(r.Context(), userID, sessionID, messageID, relatedMessageIDs, resetSummary)
		if err != nil {
			writeChatDetailStoreError(w, r, err, sessionID, "delete_chat_message")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := a.chatStore.DeleteMessage(r.Context(), userID, sessionID, messageID); err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "delete_chat_message")
		return
	}
	a.deleteRelatedChatMessages(r.Context(), userID, sessionID, msgs, relatedMessageIDs)
	a.resetChatSummaryIfNeeded(r.Context(), userID, sessionID, resetSummary)
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) deleteChatMessagesAfter(w http.ResponseWriter, r *http.Request, userID *int64, sessionID string) {
	afterID := strings.TrimSpace(r.URL.Query().Get("after"))
	if afterID == "" {
		http.Error(w, "missing after", http.StatusBadRequest)
		return
	}
	inclusive := isTruthyQueryValue(r.URL.Query().Get("inclusive"))
	msgs, msgIndex, target, ok := a.findChatMessage(w, r, userID, sessionID, afterID)
	if !ok {
		return
	}
	sess, err := a.chatStore.GetSession(r.Context(), userID, sessionID)
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "get_chat_session")
		return
	}
	relatedMessageIDs := []string(nil)
	if inclusive {
		relatedMessageIDs = relatedToolMessageIDs(msgs, target)
	}
	remainingCount := msgIndex + 1
	if inclusive {
		remainingCount = msgIndex
	}
	resetSummary := sess.SummarizedCount > remainingCount
	if a.deleteChatMessagesAfterAtomic(w, r, userID, sessionID, afterID, inclusive, relatedMessageIDs, resetSummary) {
		return
	}
	if err := a.chatStore.DeleteMessagesAfter(r.Context(), userID, sessionID, afterID, inclusive); err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "delete_chat_messages_after")
		return
	}
	if inclusive {
		a.deleteRelatedChatMessages(r.Context(), userID, sessionID, msgs, relatedMessageIDs)
	}
	a.resetChatSummaryIfNeeded(r.Context(), userID, sessionID, resetSummary)
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) deleteChatMessagesAfterAtomic(
	w http.ResponseWriter,
	r *http.Request,
	userID *int64,
	sessionID string,
	afterID string,
	inclusive bool,
	relatedMessageIDs []string,
	resetSummary bool,
) bool {
	atomicStore, ok := a.chatStore.(atomicChatTurnDeleteStore)
	if !ok {
		return false
	}
	err := atomicStore.DeleteMessagesAfterWithRelated(r.Context(), persist.ChatDeleteAfterRequest{
		UserID:            userID,
		SessionID:         sessionID,
		MessageID:         afterID,
		Inclusive:         inclusive,
		RelatedMessageIDs: relatedMessageIDs,
		ResetSummary:      resetSummary,
	})
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "delete_chat_messages_after")
		return true
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}

func (a *app) findChatMessage(
	w http.ResponseWriter,
	r *http.Request,
	userID *int64,
	sessionID string,
	messageID string,
) ([]persist.ChatMessage, int, persist.ChatMessage, bool) {
	msgs, err := a.chatStore.ListMessages(r.Context(), userID, sessionID, 0)
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "list_chat_messages")
		return nil, -1, persist.ChatMessage{}, false
	}
	for i, m := range msgs {
		if m.ID == messageID {
			return msgs, i, m, true
		}
	}
	http.NotFound(w, r)
	return nil, -1, persist.ChatMessage{}, false
}

func (a *app) deleteRelatedChatMessages(
	ctx context.Context,
	userID *int64,
	sessionID string,
	msgs []persist.ChatMessage,
	relatedMessageIDs []string,
) {
	if len(relatedMessageIDs) == 0 {
		return
	}
	relatedSet := make(map[string]struct{}, len(relatedMessageIDs))
	for _, relatedID := range relatedMessageIDs {
		relatedSet[relatedID] = struct{}{}
	}
	for _, m := range msgs {
		if _, ok := relatedSet[m.ID]; ok {
			_ = a.chatStore.DeleteMessage(ctx, userID, sessionID, m.ID)
		}
	}
}

func (a *app) resetChatSummaryIfNeeded(ctx context.Context, userID *int64, sessionID string, reset bool) {
	if !reset {
		return
	}
	if err := a.chatStore.UpdateSummary(ctx, userID, sessionID, "", 0); err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("reset_chat_summary")
	}
}

func isTruthyQueryValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "y":
		return true
	default:
		return false
	}
}

func (a *app) handleChatTitle(w http.ResponseWriter, r *http.Request, userID *int64, sessionID string) {
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
	sess, err := a.chatStore.GetSession(r.Context(), userID, sessionID)
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "get_chat_session")
		return
	}
	if !isDefaultSessionName(sess.Name) {
		writeChatJSON(w, sess, "encode_chat_session")
		return
	}
	title, genErr := a.generateChatTitle(r.Context(), prompt)
	if genErr != nil {
		log.Warn().Err(genErr).Str("session", sessionID).Msg("chat_title_fallback")
	}
	updated, err := a.chatStore.RenameSession(r.Context(), userID, sessionID, title)
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "rename_chat_session")
		return
	}
	writeChatJSON(w, updated, "encode_chat_session")
}

func (a *app) handleChatSession(
	w http.ResponseWriter,
	r *http.Request,
	currentUser *auth.User,
	userID *int64,
	sessionID string,
) {
	switch r.Method {
	case http.MethodGet:
		sess, err := a.chatStore.GetSession(r.Context(), userID, sessionID)
		if err != nil {
			writeChatDetailStoreError(w, r, err, sessionID, "get_chat_session")
			return
		}
		writeChatJSON(w, sess, "encode_chat_session")
	case http.MethodPatch:
		a.patchChatSession(w, r, currentUser, userID, sessionID)
	case http.MethodDelete:
		sess, err := a.chatStore.GetSession(r.Context(), userID, sessionID)
		if err != nil {
			writeChatDetailStoreError(w, r, err, sessionID, "get_chat_session")
			return
		}
		deleteProject, err := a.shouldDeleteTemporaryChatProject(r.Context(), chatRequestOwner(currentUser, userID), sess)
		if err != nil {
			log.Error().Err(err).Str("session", sessionID).Str("project_id", sess.ProjectID).Msg("check_temporary_chat_project")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if err := a.chatStore.DeleteSession(r.Context(), userID, sessionID); err != nil {
			writeChatDetailStoreError(w, r, err, sessionID, "delete_chat_session")
			return
		}
		if deleteProject {
			err = a.projectsService.DeleteProject(r.Context(), chatRequestOwner(currentUser, userID), strings.TrimSpace(sess.ProjectID))
		}
		if err != nil {
			log.Error().Err(err).Str("session", sessionID).Str("project_id", sess.ProjectID).Msg("delete_temporary_chat_project")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) shouldDeleteTemporaryChatProject(ctx context.Context, owner int64, sess persist.ChatSession) (bool, error) {
	projectID := strings.TrimSpace(sess.ProjectID)
	if projectID == "" || a.projectsService == nil {
		return false, nil
	}
	return a.isTemporaryProject(ctx, owner, projectID)
}

func (a *app) isTemporaryProject(ctx context.Context, owner int64, projectID string) (bool, error) {
	temporaryProjects, err := a.projectsService.ListProjectsByKindWithUsage(ctx, owner, projects.ProjectKindTemporary, false)
	if err != nil {
		return false, err
	}
	for _, project := range temporaryProjects {
		if project.ID == projectID {
			return true, nil
		}
	}
	return false, nil
}

type patchChatSessionRequest struct {
	Name                        *string `json:"name"`
	ProjectID                   *string `json:"projectId"`
	LegacyProjectID             *string `json:"project_id"`
	EvolvingMemoryEnabled       *bool   `json:"evolvingMemoryEnabled"`
	LegacyEvolvingMemoryEnabled *bool   `json:"evolving_memory_enabled"`
	BeliefMemoryEnabled         *bool   `json:"beliefMemoryEnabled"`
	LegacyBeliefMemoryEnabled   *bool   `json:"belief_memory_enabled"`
}

func (b patchChatSessionRequest) normalized() (*string, *bool, *bool) {
	projectID := b.ProjectID
	if projectID == nil {
		projectID = b.LegacyProjectID
	}
	evolvingMemoryEnabled := b.EvolvingMemoryEnabled
	if evolvingMemoryEnabled == nil {
		evolvingMemoryEnabled = b.LegacyEvolvingMemoryEnabled
	}
	beliefMemoryEnabled := b.BeliefMemoryEnabled
	if beliefMemoryEnabled == nil {
		beliefMemoryEnabled = b.LegacyBeliefMemoryEnabled
	}
	return projectID, evolvingMemoryEnabled, beliefMemoryEnabled
}

func (a *app) patchChatSession(
	w http.ResponseWriter,
	r *http.Request,
	currentUser *auth.User,
	userID *int64,
	sessionID string,
) {
	defer r.Body.Close()
	var body patchChatSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	projectID, evolvingMemoryEnabled, beliefMemoryEnabled := body.normalized()
	if body.Name == nil && projectID == nil && evolvingMemoryEnabled == nil && beliefMemoryEnabled == nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sess, ok := a.applyChatSessionPatch(w, r, currentUser, userID, sessionID, body.Name, projectID)
	if !ok {
		return
	}
	if evolvingMemoryEnabled != nil || beliefMemoryEnabled != nil {
		var err error
		sess, err = a.patchChatSessionMemorySettings(r.Context(), userID, sessionID, sess, evolvingMemoryEnabled, beliefMemoryEnabled)
		if err != nil {
			writeChatDetailStoreError(w, r, err, sessionID, "set_chat_session_memory_settings")
			return
		}
	}
	writeChatJSON(w, sess, "encode_chat_session")
}

func (a *app) applyChatSessionPatch(
	w http.ResponseWriter,
	r *http.Request,
	currentUser *auth.User,
	userID *int64,
	sessionID string,
	name *string,
	projectID *string,
) (persist.ChatSession, bool) {
	var sess persist.ChatSession
	var err error
	if name != nil {
		sess, err = a.chatStore.RenameSession(r.Context(), userID, sessionID, *name)
		if err != nil {
			writeChatDetailStoreError(w, r, err, sessionID, "rename_chat_session")
			return persist.ChatSession{}, false
		}
	}
	if projectID == nil {
		return sess, true
	}
	cleanProjectID, err := a.validateChatSessionProject(r.Context(), chatRequestOwner(currentUser, userID), *projectID)
	if err != nil {
		writeChatProjectError(w, err, sessionID, *projectID)
		return persist.ChatSession{}, false
	}
	sess, err = a.chatStore.SetSessionProject(r.Context(), userID, sessionID, cleanProjectID)
	if err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "set_chat_session_project")
		return persist.ChatSession{}, false
	}
	return sess, true
}

func (a *app) patchChatSessionMemorySettings(
	ctx context.Context,
	userID *int64,
	sessionID string,
	sess persist.ChatSession,
	evolvingMemoryEnabled *bool,
	beliefMemoryEnabled *bool,
) (persist.ChatSession, error) {
	if sess.ID == "" {
		var err error
		sess, err = a.chatStore.GetSession(ctx, userID, sessionID)
		if err != nil {
			return persist.ChatSession{}, err
		}
	}
	nextSettings := chatMemorySettingsFromSession(sess)
	if evolvingMemoryEnabled != nil {
		nextSettings.EvolvingMemoryEnabled = *evolvingMemoryEnabled
	}
	if beliefMemoryEnabled != nil {
		nextSettings.BeliefMemoryEnabled = *beliefMemoryEnabled
	}
	return a.chatStore.SetSessionMemorySettings(
		ctx,
		userID,
		sessionID,
		nextSettings.EvolvingMemoryEnabled,
		nextSettings.BeliefMemoryEnabled,
	)
}

func writeChatProjectError(w http.ResponseWriter, err error, sessionID, projectID string) {
	switch {
	case errors.Is(err, workspaces.ErrInvalidProjectID):
		http.Error(w, "invalid project_id", http.StatusBadRequest)
	case errors.Is(err, workspaces.ErrProjectNotFound):
		http.Error(w, "project not found (project_id must match the project directory/ID)", http.StatusBadRequest)
	default:
		log.Error().Err(err).Str("session", sessionID).Str("project_id", projectID).Msg("validate_chat_session_project")
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeChatDetailStoreError(w http.ResponseWriter, r *http.Request, err error, sessionID, msg string) {
	if errors.Is(err, persist.ErrForbidden) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if errors.Is(err, persist.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	log.Error().Err(err).Str("session", sessionID).Msg(msg)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func writeChatJSON(w http.ResponseWriter, value any, logMsg string) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Error().Err(err).Msg(logMsg)
	}
}

func (a *app) validateChatSessionProject(ctx context.Context, owner int64, projectID string) (string, error) {
	cleanProjectID, err := workspaces.ValidateProjectID(strings.TrimSpace(projectID))
	if err != nil || cleanProjectID == "" {
		return cleanProjectID, err
	}
	if a.projectsService == nil {
		return cleanProjectID, nil
	}
	projects, err := a.projectsService.ListProjects(ctx, owner)
	if err != nil {
		return "", err
	}
	for _, project := range projects {
		if project.ID == cleanProjectID {
			return cleanProjectID, nil
		}
	}
	return "", workspaces.ErrProjectNotFound
}

// hydrateChatMessages post-processes persisted messages for client display.
// It strips JSON wrappers used to preserve tool calls and attaches tool names/args
