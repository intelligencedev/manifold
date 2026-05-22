package harness

import (
	"bytes"
	"encoding/json"
	"reflect"

	"manifold/internal/llm"
)

// CompletedStep records a successful tool call outside model-visible history.
type CompletedStep struct {
	ToolName   string
	ToolCallID string
	Args       json.RawMessage
	argsObject map[string]any
}

// StepTracker keeps authoritative workflow progress outside prompt context.
type StepTracker struct {
	completed []CompletedStep
}

// NewStepTracker creates an empty workflow progress tracker.
func NewStepTracker() *StepTracker {
	return &StepTracker{}
}

// RecordSuccess marks a tool call as successfully completed.
func (s *StepTracker) RecordSuccess(call llm.ToolCall) {
	if s == nil {
		return
	}
	args := append(json.RawMessage(nil), call.Args...)
	step := CompletedStep{
		ToolName:   normalizeName(call.Name),
		ToolCallID: call.ID,
		Args:       args,
		argsObject: decodeObject(args),
	}
	if step.ToolName == "" {
		return
	}
	s.completed = append(s.completed, step)
}

// Completed returns a copy of completed step records.
func (s *StepTracker) Completed() []CompletedStep {
	if s == nil {
		return nil
	}
	out := make([]CompletedStep, len(s.completed))
	copy(out, s.completed)
	return out
}

// HasSuccessful reports whether a tool has succeeded at least once.
func (s *StepTracker) HasSuccessful(tool string) bool {
	tool = normalizeName(tool)
	if s == nil || tool == "" {
		return false
	}
	for _, step := range s.completed {
		if step.ToolName == tool {
			return true
		}
	}
	return false
}

func (s *StepTracker) hasSuccessfulWithArg(tool, arg string, value any) bool {
	tool = normalizeName(tool)
	if s == nil || tool == "" || arg == "" {
		return false
	}
	for _, step := range s.completed {
		if step.ToolName != tool {
			continue
		}
		got, ok := step.argsObject[arg]
		if ok && reflect.DeepEqual(got, value) {
			return true
		}
	}
	return false
}

func (s *StepTracker) missing(required []string) []string {
	missing := make([]string, 0, len(required))
	for _, step := range required {
		if !s.HasSuccessful(step) {
			missing = append(missing, step)
		}
	}
	return missing
}

// StepCheck is the result of pre-dispatch workflow enforcement.
type StepCheck struct {
	OK           bool
	Reason       ValidationReason
	Nudge        string
	ToolName     string
	MissingSteps []string
	Prerequisite Prerequisite
}

// StepEnforcer validates terminal and prerequisite rules against pre-batch state.
type StepEnforcer struct {
	workflow WorkflowConfig
}

// NewStepEnforcer builds an enforcer for a workflow config.
func NewStepEnforcer(workflow WorkflowConfig) StepEnforcer {
	return StepEnforcer{workflow: workflow.normalized()}
}

// Check evaluates a whole batch against tracker state before any call is run.
func (e StepEnforcer) Check(calls []llm.ToolCall, tracker *StepTracker) StepCheck {
	if tracker == nil {
		tracker = NewStepTracker()
	}
	for _, call := range calls {
		tool := normalizeName(call.Name)
		if e.workflow.isTerminalTool(tool) {
			missing := tracker.missing(e.workflow.RequiredSteps)
			if len(missing) > 0 {
				return StepCheck{
					OK:           false,
					Reason:       ValidationReasonRequiredStepMissing,
					Nudge:        requiredStepsNudge(tool, missing),
					ToolName:     tool,
					MissingSteps: missing,
				}
			}
		}

		currentArgs := decodeObject(call.Args)
		for _, prereq := range e.workflow.ToolPrerequisites[tool] {
			if prereq.MatchArg == "" {
				if tracker.HasSuccessful(prereq.Tool) {
					continue
				}
			} else if value, ok := currentArgs[prereq.MatchArg]; ok && tracker.hasSuccessfulWithArg(prereq.Tool, prereq.MatchArg, value) {
				continue
			}
			return StepCheck{
				OK:           false,
				Reason:       ValidationReasonPrerequisiteMissing,
				Nudge:        prerequisiteNudge(tool, prereq),
				ToolName:     tool,
				Prerequisite: prereq,
			}
		}
	}
	return StepCheck{OK: true}
}

func decodeObject(raw json.RawMessage) map[string]any {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
