package fleet

type ActiveEdge struct {
	ParentCallID string `json:"parent_call_id"`
	CallID       string `json:"call_id"`
	Agent        string `json:"agent,omitempty"`
	Depth        int    `json:"depth,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

func ActiveEdges(events []Event) []ActiveEdge {
	byCall := map[string]ActiveEdge{}
	finished := map[string]bool{}
	for _, ev := range events {
		if ev.CallID == "" || ev.ParentCallID == "" {
			continue
		}
		switch ev.Kind {
		case EventDelegation:
			byCall[ev.CallID] = ActiveEdge{
				ParentCallID: ev.ParentCallID,
				CallID:       ev.CallID,
				Agent:        ev.Agent,
				Depth:        ev.Depth,
				RunID:        ev.RunID,
				SessionID:    ev.SessionID,
			}
		case EventRunFinished, EventRunFailed:
			finished[ev.CallID] = true
		}
	}
	out := make([]ActiveEdge, 0, len(byCall))
	for callID, edge := range byCall {
		if finished[callID] {
			continue
		}
		out = append(out, edge)
	}
	return out
}
