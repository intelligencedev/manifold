package agent

func (e *Engine) stepAllowed(step int) bool {
	return e.MaxSteps <= 0 || step < e.MaxSteps
}
