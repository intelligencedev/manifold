package agentd

import (
	"context"
	"net/http"
	"strings"

	fleetapi "manifold/internal/agentd/fleetapi"
	"manifold/internal/fleet"
	persist "manifold/internal/persistence"
)

type fleetStateResponse struct {
	Runs                  []AgentRun                    `json:"runs"`
	Specialists           []persist.Specialist          `json:"specialists"`
	Teams                 []persist.SpecialistTeam      `json:"teams"`
	OpenInputRequests     []pendingInputRequestSnapshot `json:"open_input_requests"`
	ActiveDelegationEdges []fleet.ActiveEdge            `json:"active_delegation_edges"`
	RecentEvents          []fleet.Event                 `json:"recent_events,omitempty"`
}

func (a *app) fleetStateHandler() http.HandlerFunc {
	return fleetapi.StateHandler(fleetapi.Deps{
		RequireUserID: a.requireUserID,
		BuildState: func(ctx context.Context, userID int64) any {
			uidPtr := &userID
			runs := a.runs.list()
			specialists, _ := a.listSpecialistsForUser(ctx, userID)
			teams, _ := a.listTeamsForUser(ctx, userID)
			recent := a.fleetBus.Recent(userID)
			return fleetStateResponse{
				Runs:                  runs,
				Specialists:           specialists,
				Teams:                 teams,
				OpenInputRequests:     a.activeInputRequestBroker().list(uidPtr),
				ActiveDelegationEdges: fleet.ActiveEdges(recent),
				RecentEvents:          recent,
			}
		},
	})
}

func (a *app) fleetEventsHandler() http.HandlerFunc {
	return fleetapi.EventsHandler(fleetapi.Deps{RequireUserID: a.requireUserID, Subscribe: a.fleetBus.Subscribe})
}

type runTimelineResponse struct {
	RunID  string `json:"run_id"`
	Events []any  `json:"events"`
	Status string `json:"status,omitempty"`
}

func (a *app) runTimelineHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		runID := strings.TrimPrefix(r.URL.Path, "/api/runs/")
		runID = strings.TrimSuffix(runID, "/timeline")
		runID = strings.Trim(runID, "/")
		if runID == "" || strings.Contains(runID, "/") {
			http.NotFound(w, r)
			return
		}
		status := ""
		if run, ok := a.runs.get(runID); ok {
			status = run.Status
		}
		if strings.HasPrefix(runID, "warpprun_") {
			events, warppStatus, ok := a.warppState().GetRunEvents(userID, runID)
			if !ok {
				http.NotFound(w, r)
				return
			}
			resp := runTimelineResponse{RunID: runID, Status: warppStatus, Events: make([]any, 0, len(events))}
			for _, ev := range events {
				resp.Events = append(resp.Events, ev)
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
		uidPtr := &userID
		sessions, _ := a.chatStore.ListSessions(r.Context(), uidPtr)
		resp := runTimelineResponse{RunID: runID, Status: status, Events: []any{}}
		for _, session := range sessions {
			activities, err := a.activityStore.ListSessionActivities(r.Context(), uidPtr, session.ID)
			if err != nil {
				continue
			}
			for _, activity := range activities {
				if activity.RunID == runID {
					resp.Events = append(resp.Events, activity)
				}
			}
		}
		for _, ev := range a.fleetBus.Recent(userID) {
			if ev.RunID == runID {
				resp.Events = append(resp.Events, ev)
			}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
