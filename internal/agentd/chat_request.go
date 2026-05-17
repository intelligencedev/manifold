package agentd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

type chatRunRequest struct {
	Prompt             string `json:"prompt"`
	SessionID          string `json:"session_id,omitempty"`
	AssistantMessageID string `json:"assistant_message_id,omitempty"`
	EphemeralSession   bool   `json:"ephemeral_session,omitempty"`
	ProjectID          string `json:"project_id,omitempty"`
	ObjectiveID        string `json:"objective_id,omitempty"`
	RoomID             string `json:"room_id,omitempty"`
	RouteTarget        string `json:"route_target,omitempty"`
	SystemPrompt       string `json:"system_prompt,omitempty"`
	Image              bool   `json:"image,omitempty"`
	ImageSize          string `json:"image_size,omitempty"`
}

func (req *chatRunRequest) UnmarshalJSON(data []byte) error {
	type rawChatRunRequest struct {
		Prompt             string `json:"prompt"`
		SessionID          string `json:"session_id,omitempty"`
		AssistantMessageID string `json:"assistant_message_id,omitempty"`
		EphemeralSession   bool   `json:"ephemeral_session,omitempty"`
		ProjectID          string `json:"project_id,omitempty"`
		ObjectiveID        string `json:"objective_id,omitempty"`
		RoomID             string `json:"room_id,omitempty"`
		RouteTarget        string `json:"route_target,omitempty"`
		BotID              string `json:"bot_id,omitempty"`
		SystemPrompt       string `json:"system_prompt,omitempty"`
		Image              bool   `json:"image,omitempty"`
		ImageSize          string `json:"image_size,omitempty"`
	}
	var decoded rawChatRunRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	req.Prompt = decoded.Prompt
	req.SessionID = decoded.SessionID
	req.AssistantMessageID = decoded.AssistantMessageID
	req.EphemeralSession = decoded.EphemeralSession
	req.ProjectID = decoded.ProjectID
	req.ObjectiveID = decoded.ObjectiveID
	req.RoomID = decoded.RoomID
	req.RouteTarget = decoded.RouteTarget
	if req.RouteTarget == "" {
		req.RouteTarget = decoded.BotID
	}
	req.SystemPrompt = decoded.SystemPrompt
	req.Image = decoded.Image
	req.ImageSize = decoded.ImageSize
	return nil
}

type chatDispatchTarget struct {
	SpecialistName string
	TeamName       string
}

func (req *chatRunRequest) normalize() {
	req.SessionID = normalizeClientChatSessionID(req.SessionID)
	req.AssistantMessageID = strings.TrimSpace(req.AssistantMessageID)
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.ObjectiveID = strings.TrimSpace(req.ObjectiveID)
	req.RoomID = strings.TrimSpace(req.RoomID)
	req.RouteTarget = strings.TrimSpace(req.RouteTarget)
	req.SystemPrompt = strings.TrimSpace(req.SystemPrompt)
	req.ImageSize = strings.TrimSpace(req.ImageSize)
}

func normalizeClientChatSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "default"
	}
	if _, err := uuid.Parse(id); err == nil {
		return id
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()
}

func resolveChatDispatchTarget(query url.Values) chatDispatchTarget {
	teamName := strings.TrimSpace(query.Get("team"))
	if teamName == "" {
		teamName = strings.TrimSpace(query.Get("group"))
	}
	return chatDispatchTarget{
		SpecialistName: strings.TrimSpace(query.Get("specialist")),
		TeamName:       teamName,
	}
}

func (a *app) prepareChatRunRequest(r *http.Request, userID *int64, req chatRunRequest) (*http.Request, *workspaces.Workspace, int, error) {
	ctx := sandbox.WithSessionID(r.Context(), req.SessionID)
	if req.ObjectiveID != "" {
		ctx = sandbox.WithObjectiveID(ctx, req.ObjectiveID)
	}
	if req.RoomID != "" {
		ctx = sandbox.WithRoomID(ctx, req.RoomID)
		ctx = sandbox.WithMatrixOutbox(ctx, sandbox.NewMatrixOutbox())
	}
	if req.RouteTarget != "" {
		ctx = sandbox.WithRouteTarget(ctx, req.RouteTarget)
	}

	if a.cfg.Auth.Enabled {
		cookieName := a.cfg.Auth.CookieName
		if cookieName == "" {
			cookieName = "sio_session"
		}
		if c, err := r.Cookie(cookieName); err == nil && c != nil && c.Value != "" {
			ctx = sandbox.WithAuthCookie(ctx, cookieName+"="+c.Value)
		}
	}

	r = r.WithContext(ctx)
	if req.ProjectID == "" {
		return r, nil, 0, nil
	}

	var resolvedUserID int64
	if userID != nil {
		resolvedUserID = *userID
	}

	ws, err := a.workspaceManager.Checkout(r.Context(), resolvedUserID, req.ProjectID, req.SessionID)
	if err != nil {
		switch {
		case errors.Is(err, workspaces.ErrInvalidProjectID):
			return r, nil, http.StatusBadRequest, err
		case errors.Is(err, workspaces.ErrProjectNotFound):
			log.Error().Err(err).Str("project_id", req.ProjectID).Msg("project_dir_missing")
			return r, nil, http.StatusBadRequest, err
		default:
			log.Error().Err(err).Str("project_id", req.ProjectID).Msg("workspace_checkout_failed")
			return r, nil, http.StatusInternalServerError, err
		}
	}
	if ws.BaseDir == "" {
		return r, nil, 0, nil
	}

	ctx = sandbox.WithBaseDir(r.Context(), ws.BaseDir)
	ctx = sandbox.WithProjectID(ctx, req.ProjectID)
	r = r.WithContext(ctx)
	return r, &ws, 0, nil
}
