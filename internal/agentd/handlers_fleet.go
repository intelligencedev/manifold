package agentd

import (
	"encoding/json"
	"net/http"
	"strings"

	"manifold/internal/fleet"
	"manifold/internal/flow"
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
		uidPtr := &userID
		runs := a.runs.list()
		specialists, _ := a.listSpecialistsForUser(r.Context(), userID)
		teams, _ := a.listTeamsForUser(r.Context(), userID)
		recent := a.fleetBus.Recent(userID)
		writeJSON(w, http.StatusOK, fleetStateResponse{
			Runs:                  runs,
			Specialists:           specialists,
			Teams:                 teams,
			OpenInputRequests:     a.activeInputRequestBroker().list(uidPtr),
			ActiveDelegationEdges: fleet.ActiveEdges(recent),
			RecentEvents:          recent,
		})
	}
}

func (a *app) fleetEventsHandler() http.HandlerFunc {
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
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		snapshot, ch := a.fleetBus.Subscribe(r.Context(), userID)
		writeFleetEvents(w, fl, snapshot)
		for ev := range ch {
			writeFleetEvents(w, fl, []fleet.Event{ev})
		}
	}
}

func writeFleetEvents(w http.ResponseWriter, fl http.Flusher, events []fleet.Event) {
	for _, ev := range events {
		payload, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		fl.Flush()
	}
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
		if strings.HasPrefix(runID, "flowrun_") {
			events, flowStatus, ok := a.flowV2State().getRunEvents(userID, runID)
			if !ok {
				http.NotFound(w, r)
				return
			}
			resp := runTimelineResponse{RunID: runID, Status: flowStatus, Events: make([]any, 0, len(events))}
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

func normalizeFlowEvents(events []flow.RunEvent) []any {
	out := make([]any, 0, len(events))
	for _, ev := range events {
		out = append(out, ev)
	}
	return out
}
