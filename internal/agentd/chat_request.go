package agentd

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

type chatRunRequest struct {
	Prompt       string `json:"prompt"`
	SessionID    string `json:"session_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	RoomID       string `json:"room_id,omitempty"`
	BotID        string `json:"bot_id,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	Image        bool   `json:"image,omitempty"`
	ImageSize    string `json:"image_size,omitempty"`
}

type chatDispatchTarget struct {
	SpecialistName string
	TeamName       string
}

func (req *chatRunRequest) normalize() {
	req.SessionID = strings.TrimSpace(req.SessionID)
	if req.SessionID == "" {
		req.SessionID = "default"
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.RoomID = strings.TrimSpace(req.RoomID)
	req.BotID = strings.TrimSpace(req.BotID)
	req.SystemPrompt = strings.TrimSpace(req.SystemPrompt)
	req.ImageSize = strings.TrimSpace(req.ImageSize)
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
	if req.RoomID != "" {
		ctx = sandbox.WithRoomID(ctx, req.RoomID)
		ctx = sandbox.WithMatrixOutbox(ctx, sandbox.NewMatrixOutbox())
	}
	if req.BotID != "" {
		ctx = sandbox.WithBotID(ctx, req.BotID)
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
