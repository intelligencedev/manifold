package agentd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"manifold/internal/defaultprompt"
	persist "manifold/internal/persistence"
	"manifold/internal/specialists"
)

var errOrchestratorDelete = errors.New("cannot delete orchestrator")
var errTeamOrchestratorInUse = errors.New("cannot remove team orchestrator")

func (a *app) listSpecialistsForUser(ctx context.Context, userID int64) ([]persist.Specialist, error) {
	list, err := a.specStore.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	membership := a.teamMembershipsForUser(ctx, userID)
	out := make([]persist.Specialist, 0, len(list)+1)
	orchestrator := a.orchestratorSpecialist(ctx, userID)
	orchestrator.Teams = membership[orchestrator.Name]
	out = append(out, orchestrator)
	out = append(out, list...)
	for i := range out {
		if teams, ok := membership[out[i].Name]; ok {
			out[i].Teams = teams
		}
	}
	return out, nil
}

func (a *app) getSpecialistForUser(ctx context.Context, userID int64, name string) (persist.Specialist, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return persist.Specialist{}, false, nil
	}
	if name == specialists.OrchestratorName {
		sp := a.orchestratorSpecialist(ctx, userID)
		sp.Teams = a.teamMembershipsForUser(ctx, userID)[sp.Name]
		return sp, true, nil
	}
	sp, ok, err := a.specStore.GetByName(ctx, userID, name)
	if err != nil || !ok {
		return sp, ok, err
	}
	sp.Teams = a.teamMembershipsForUser(ctx, userID)[sp.Name]
	return sp, true, nil
}

func (a *app) createSpecialistForUser(ctx context.Context, userID int64, sp persist.Specialist) (persist.Specialist, int, error) {
	name := strings.TrimSpace(sp.Name)
	if name == "" {
		return persist.Specialist{}, 0, errors.New("name required")
	}
	sp = a.prepareSpecialistInput(sp)
	defaultPromptID, defaultVersionID, err := a.defaultPromptReference(ctx, userID)
	if err != nil {
		return persist.Specialist{}, 0, fmt.Errorf("resolve default prompt: %w", err)
	}
	if strings.TrimSpace(sp.PromptID) == "" && strings.TrimSpace(sp.PromptVersionID) == "" {
		sp.PromptID, sp.PromptVersionID = defaultPromptID, defaultVersionID
	}
	if strings.TrimSpace(sp.System) == "" && sp.PromptID == defaultPromptID && sp.PromptVersionID == defaultVersionID {
		sp.System = defaultprompt.Content
	}
	if name == specialists.OrchestratorName {
		if userID == systemUserID {
			if err := a.applyOrchestratorUpdate(ctx, sp); err != nil {
				return persist.Specialist{}, 0, err
			}
			updated, _, _ := a.getSpecialistForUser(ctx, userID, specialists.OrchestratorName)
			return updated, 200, nil
		}
		updated, err := a.saveUserOrchestratorOverlay(ctx, userID, sp)
		return updated, httpStatusCreated, err
	}
	saved, err := a.saveSpecialistForUser(ctx, userID, name, sp)
	return saved, httpStatusCreated, err
}

func (a *app) updateSpecialistForUser(ctx context.Context, userID int64, name string, sp persist.Specialist) (persist.Specialist, error) {
	sp = a.prepareSpecialistInput(sp)
	if name == specialists.OrchestratorName {
		if userID == systemUserID {
			if err := a.applyOrchestratorUpdate(ctx, sp); err != nil {
				return persist.Specialist{}, err
			}
			updated, _, _ := a.getSpecialistForUser(ctx, userID, specialists.OrchestratorName)
			return updated, nil
		}
		return a.saveUserOrchestratorOverlay(ctx, userID, sp)
	}
	return a.saveSpecialistForUser(ctx, userID, name, sp)
}

func (a *app) deleteSpecialistForUser(ctx context.Context, userID int64, name string) error {
	if name == specialists.OrchestratorName {
		return errOrchestratorDelete
	}
	if err := a.ensureSpecialistNotTeamOrchestrator(ctx, userID, name); err != nil {
		return err
	}
	if err := a.specStore.Delete(ctx, userID, name); err != nil {
		return err
	}
	if err := a.removeSpecialistFromTeams(ctx, userID, name); err != nil {
		return err
	}
	a.invalidateSpecialistsCache(ctx, userID)
	return nil
}

func (a *app) saveSpecialistForUser(ctx context.Context, userID int64, name string, sp persist.Specialist) (persist.Specialist, error) {
	sp.Name = name
	sp.UserID = userID
	saved, err := a.specStore.Upsert(ctx, userID, sp)
	if err != nil {
		return persist.Specialist{}, err
	}
	if err := a.applyTeamMemberships(ctx, userID, saved.Name, sp.Teams); err != nil {
		return persist.Specialist{}, err
	}
	saved.Teams = sp.Teams
	a.invalidateSpecialistsCache(ctx, userID)
	return saved, nil
}

func (a *app) saveUserOrchestratorOverlay(ctx context.Context, userID int64, sp persist.Specialist) (persist.Specialist, error) {
	sp.Name = specialists.OrchestratorName
	sp.UserID = userID
	if _, err := a.specStore.Upsert(ctx, userID, sp); err != nil {
		return persist.Specialist{}, err
	}
	a.invalidateSpecialistsCache(ctx, userID)
	updated, _, _ := a.getSpecialistForUser(ctx, userID, specialists.OrchestratorName)
	return updated, nil
}

func (a *app) prepareSpecialistInput(sp persist.Specialist) persist.Specialist {
	if strings.TrimSpace(sp.Provider) == "" {
		sp.Provider = a.cfg.LLMClient.Provider
	}
	return sp
}

func (a *app) listTeamsForUser(ctx context.Context, userID int64) ([]persist.SpecialistTeam, error) {
	return a.teamStore.List(ctx, userID)
}

func (a *app) getTeamForUser(ctx context.Context, userID int64, name string) (persist.SpecialistTeam, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return persist.SpecialistTeam{}, false, nil
	}
	return a.teamStore.GetByName(ctx, userID, name)
}

func (a *app) createTeamForUser(ctx context.Context, userID int64, team persist.SpecialistTeam) (persist.SpecialistTeam, error) {
	team.Name = strings.TrimSpace(team.Name)
	if team.Name == "" {
		return persist.SpecialistTeam{}, errors.New("name required")
	}
	team.UserID = userID
	team.OrchestratorName = strings.TrimSpace(team.OrchestratorName)
	if _, _, err := a.resolveTeamOrchestratorSpecialist(ctx, userID, team); err != nil {
		return persist.SpecialistTeam{}, err
	}
	return a.teamStore.Upsert(ctx, userID, team)
}

func (a *app) updateTeamForUser(ctx context.Context, userID int64, name string, team persist.SpecialistTeam) (persist.SpecialistTeam, error) {
	team.Name = strings.TrimSpace(name)
	team.UserID = userID
	team.OrchestratorName = strings.TrimSpace(team.OrchestratorName)
	if _, _, err := a.resolveTeamOrchestratorSpecialist(ctx, userID, team); err != nil {
		return persist.SpecialistTeam{}, err
	}
	return a.teamStore.Upsert(ctx, userID, team)
}

func (a *app) deleteTeamForUser(ctx context.Context, userID int64, name string) error {
	return a.teamStore.Delete(ctx, userID, name)
}

func (a *app) addSpecialistToTeamForUser(ctx context.Context, userID int64, teamName, specialistName string) error {
	return a.teamStore.AddMember(ctx, userID, teamName, specialistName)
}

func (a *app) removeSpecialistFromTeamForUser(ctx context.Context, userID int64, teamName, specialistName string) error {
	if a.teamStore == nil {
		return nil
	}
	team, ok, err := a.teamStore.GetByName(ctx, userID, teamName)
	if err != nil {
		return err
	}
	if ok && strings.EqualFold(strings.TrimSpace(team.OrchestratorName), strings.TrimSpace(specialistName)) {
		return errTeamOrchestratorInUse
	}
	return a.teamStore.RemoveMember(ctx, userID, teamName, specialistName)
}

func (a *app) resolveTeamOrchestratorSpecialist(ctx context.Context, userID int64, team persist.SpecialistTeam) (persist.Specialist, int, error) {
	name := strings.TrimSpace(team.OrchestratorName)
	if name == "" {
		return persist.Specialist{}, httpStatusBadRequest, errors.New("team orchestrator is required")
	}
	if strings.EqualFold(name, specialists.OrchestratorName) {
		return persist.Specialist{}, httpStatusBadRequest, errors.New("team orchestrator must be a persisted specialist, not the reserved orchestrator")
	}
	if !teamHasMember(team.Members, name) {
		return persist.Specialist{}, httpStatusBadRequest, errors.New("team orchestrator must be a team member")
	}
	if a.specStore == nil {
		return persist.Specialist{}, httpStatusInternalServerError, errors.New("specialists unavailable")
	}
	sp, ok, err := a.specStore.GetByName(ctx, userID, name)
	if err != nil {
		return persist.Specialist{}, httpStatusInternalServerError, fmt.Errorf("failed to load team orchestrator specialist: %w", err)
	}
	if !ok {
		return persist.Specialist{}, httpStatusBadRequest, fmt.Errorf("team orchestrator specialist not found: %s", name)
	}
	if sp.Paused {
		return persist.Specialist{}, httpStatusBadRequest, fmt.Errorf("team orchestrator specialist is paused: %s", name)
	}
	return sp, 0, nil
}

func (a *app) ensureSpecialistNotTeamOrchestrator(ctx context.Context, userID int64, specialistName string) error {
	if a.teamStore == nil {
		return nil
	}
	teams, err := a.teamStore.List(ctx, userID)
	if err != nil {
		return err
	}
	for _, team := range teams {
		if strings.EqualFold(strings.TrimSpace(team.OrchestratorName), strings.TrimSpace(specialistName)) {
			return errTeamOrchestratorInUse
		}
	}
	return nil
}

func teamHasMember(members []string, name string) bool {
	name = strings.TrimSpace(name)
	for _, member := range members {
		if strings.EqualFold(strings.TrimSpace(member), name) {
			return true
		}
	}
	return false
}

func parseTeamMemberPath(path string) (teamName, specialistName string, ok bool) {
	parts := strings.SplitN(path, "/members/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	teamName = strings.TrimSpace(parts[0])
	specialistName = strings.TrimSpace(parts[1])
	if teamName == "" || specialistName == "" {
		return "", "", false
	}
	return teamName, specialistName, true
}

const httpStatusCreated = 201
const httpStatusBadRequest = 400
const httpStatusInternalServerError = 500
