package agentd

import (
	"net/http"

	teamsapi "manifold/internal/agentd/teamsapi"
)

func (a *app) teamsHandlerDeps() teamsapi.Deps {
	return teamsapi.Deps{
		RequireUserID: a.requireUserID,
		AuthEnabled:   func() bool { return a.cfg != nil && a.cfg.Auth.Enabled },
		List:          a.listTeamsForUser,
		Create:        a.createTeamForUser,
		Get:           a.getTeamForUser,
		Update:        a.updateTeamForUser,
		Delete:        a.deleteTeamForUser,
		AddMember:     a.addSpecialistToTeamForUser,
		RemoveMember:  a.removeSpecialistFromTeamForUser,
	}
}

func (a *app) teamsHandler() http.HandlerFunc {
	return teamsapi.CollectionHandler(a.teamsHandlerDeps())
}

func (a *app) teamDetailHandler() http.HandlerFunc {
	return teamsapi.DetailHandler(a.teamsHandlerDeps())
}
