package agentd

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"

	chatpkg "manifold/internal/agentd/chat"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

type chatRunRequest = chatpkg.RunRequest
type chatDispatchTarget = chatpkg.DispatchTarget

func normalizeClientChatSessionID(id string) string { return chatpkg.NormalizeSessionID(id) }

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
	workspaceRef := req.ProjectID
	ws, err := a.workspaceManager.Checkout(r.Context(), resolvedUserID, workspaceRef, req.SessionID)
	if err != nil {
		switch {
		case errors.Is(err, workspaces.ErrInvalidProjectID):
			return r, nil, http.StatusBadRequest, err
		case errors.Is(err, workspaces.ErrProjectNotFound):
			log.Error().Err(err).Str(workspaceRefFieldSnake(), workspaceRef).Msg("workspace_dir_missing")
			return r, nil, http.StatusBadRequest, err
		default:
			log.Error().Err(err).Str(workspaceRefFieldSnake(), workspaceRef).Msg("workspace_checkout_failed")
			return r, nil, http.StatusInternalServerError, err
		}
	}
	if ws.BaseDir == "" {
		return r, nil, 0, nil
	}
	ctx = sandbox.WithBaseDir(r.Context(), ws.BaseDir)
	ctx = sandbox.WithProjectID(ctx, workspaceRef)
	return r.WithContext(ctx), &ws, 0, nil
}
