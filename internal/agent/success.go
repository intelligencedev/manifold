package agent

// SuccessCriterion is pluggable per workflow.
type SuccessCriterion interface {
	IsSatisfied(history []Interaction) bool
}

// NoErrorSuccess finishes when last N interactions have nil Err.
type NoErrorSuccess struct {
	Window int
}

func (s NoErrorSuccess) IsSatisfied(history []Interaction) bool {
	if len(history) < s.Window {
		return false
	}
	for _, h := range history[len(history)-s.Window:] {
		if h.Observation.Err != nil {
			return false
		}
	}
	return true
}
