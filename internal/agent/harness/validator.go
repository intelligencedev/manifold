package harness

import (
	"sort"

	"manifold/internal/llm"
)

// ValidationReason describes why a model response was rejected.
type ValidationReason string

const (
	ValidationReasonInvalid              ValidationReason = "invalid"
	ValidationReasonUnknownTool          ValidationReason = "unknown_tool"
	ValidationReasonBareTextInWorkflow   ValidationReason = "bare_text_in_workflow"
	ValidationReasonRequiredStepMissing  ValidationReason = "required_step_missing"
	ValidationReasonPrerequisiteMissing  ValidationReason = "prerequisite_missing"
	ValidationReasonValidationNotApplied ValidationReason = "validation_not_applied"
)

// ValidationResult is returned for every model response the harness checks.
type ValidationResult struct {
	Valid        bool
	Reason       ValidationReason
	Nudge        string
	UnknownTools []string
	AllowedTools []string
	StepCheck    StepCheck
}

// ResponseValidator checks provider output before tool dispatch or final return.
type ResponseValidator struct {
	mode      Mode
	workflow  WorkflowConfig
	toolNames map[string]struct{}
}

// NewResponseValidator creates a validator for a run config and visible tool schemas.
func NewResponseValidator(cfg RunConfig, schemas []llm.ToolSchema) *ResponseValidator {
	cfg = normalizeRunConfig(cfg)
	names := make(map[string]struct{}, len(schemas))
	for _, schema := range schemas {
		name := normalizeName(schema.Name)
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}
	return &ResponseValidator{
		mode:      cfg.Mode,
		workflow:  cfg.Workflow,
		toolNames: names,
	}
}

// Validate returns a nudge when a response violates the active harness mode.
func (v *ResponseValidator) Validate(msg llm.Message, tracker *StepTracker) ValidationResult {
	if v == nil || v.mode == ModeLegacy {
		return ValidationResult{Valid: true, Reason: ValidationReasonValidationNotApplied}
	}

	if unknown := v.unknownTools(msg.ToolCalls); len(unknown) > 0 {
		allowed := v.allowedTools()
		return ValidationResult{
			Valid:        false,
			Reason:       ValidationReasonUnknownTool,
			Nudge:        unknownToolNudge(unknown, allowed),
			UnknownTools: unknown,
			AllowedTools: allowed,
		}
	}

	if v.mode == ModeWorkflow && len(msg.ToolCalls) == 0 {
		terminal := append([]string(nil), v.workflow.TerminalTools...)
		return ValidationResult{
			Valid:  false,
			Reason: ValidationReasonBareTextInWorkflow,
			Nudge:  workflowTextNudge(terminal),
		}
	}

	if len(msg.ToolCalls) > 0 {
		check := NewStepEnforcer(v.workflow).Check(msg.ToolCalls, tracker)
		if !check.OK {
			return ValidationResult{
				Valid:     false,
				Reason:    check.Reason,
				Nudge:     check.Nudge,
				StepCheck: check,
			}
		}
	}

	return ValidationResult{Valid: true}
}

func (v *ResponseValidator) unknownTools(calls []llm.ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	unknown := make([]string, 0)
	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		name := normalizeName(call.Name)
		if name == "" {
			continue
		}
		if _, ok := v.toolNames[name]; ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		unknown = append(unknown, name)
	}
	return unknown
}

func (v *ResponseValidator) allowedTools() []string {
	out := make([]string, 0, len(v.toolNames))
	for name := range v.toolNames {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
