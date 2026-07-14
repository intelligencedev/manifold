package chat

import (
	"manifold/internal/agent"
	"manifold/internal/fleet"
)

type FleetCallbackRequest struct {
	RunID       string
	SessionID   string
	ProjectID   string
	ObjectiveID string
	UserID      *int64
}

func AttachFleetCallbacks(bus *fleet.Bus, eng *agent.Engine, req FleetCallbackRequest) {
	if bus == nil || eng == nil {
		return
	}
	uid := int64(0)
	if req.UserID != nil {
		uid = *req.UserID
	}
	prevToolStart := eng.OnToolStart
	prevTool := eng.OnTool
	prevTracer := eng.AgentTracer
	eng.OnToolStart = func(name string, args []byte, toolID string) {
		if prevToolStart != nil {
			prevToolStart(name, args, toolID)
		}
		bus.Publish(fleet.Event{Kind: fleet.EventToolStart, RunID: req.RunID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, ToolID: toolID, UserID: uid, Title: name, Data: map[string]any{"args": string(args)}})
	}
	eng.OnTool = func(name string, args []byte, result []byte, toolID string) {
		if prevTool != nil {
			prevTool(name, args, result, toolID)
		}
		bus.Publish(fleet.Event{Kind: fleet.EventToolResult, RunID: req.RunID, SessionID: req.SessionID, ProjectID: req.ProjectID, ObjectiveID: req.ObjectiveID, ToolID: toolID, UserID: uid, Title: name, Data: map[string]any{"args": string(args), "result": string(result)}})
	}
	eng.AgentTracer = fleetAgentTracer{bus: bus, next: prevTracer, runID: req.RunID, sessionID: req.SessionID, projectID: req.ProjectID, objectiveID: req.ObjectiveID, userID: uid}
}

type fleetAgentTracer struct {
	bus         *fleet.Bus
	next        agent.AgentTracer
	runID       string
	sessionID   string
	projectID   string
	objectiveID string
	userID      int64
}

func (t fleetAgentTracer) Trace(ev agent.AgentTrace) {
	if t.next != nil {
		t.next.Trace(ev)
	}
	if t.bus == nil {
		return
	}
	kind := fleet.EventDelegation
	if ev.Type == "agent_error" {
		kind = fleet.EventError
	} else if ev.Type == "agent_final" {
		kind = fleet.EventRunFinished
	}
	t.bus.Publish(fleet.Event{Kind: kind, RunID: t.runID, SessionID: t.sessionID, ProjectID: t.projectID, ObjectiveID: t.objectiveID, Specialist: ev.Agent, Agent: ev.Agent, CallID: ev.CallID, ParentCallID: ev.ParentCallID, ToolID: ev.ToolID, Depth: ev.Depth, UserID: t.userID, Title: ev.Title, Message: ev.Content, Data: map[string]any{"type": ev.Type, "team": ev.Team, "args": ev.Args, "data": ev.Data, "error": ev.Error, "thought_summary": ev.ThoughtSummary}})
}
