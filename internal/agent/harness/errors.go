package harness

import (
	"errors"
	"fmt"
)

var (
	ErrNilProvider                = errors.New("harness provider is nil")
	ErrValidationRetriesExhausted = errors.New("harness validation retries exhausted")
	ErrToolErrorsExhausted        = errors.New("harness tool error budget exhausted")
)

// RetryExhaustedError reports the last invalid model response after retry nudges.
type RetryExhaustedError struct {
	Attempts int
	Last     ValidationResult
}

func (e RetryExhaustedError) Error() string {
	reason := e.Last.Reason
	if reason == "" {
		reason = ValidationReasonInvalid
	}
	return fmt.Sprintf("%s after %d attempt(s): %s", ErrValidationRetriesExhausted, e.Attempts, reason)
}

func (e RetryExhaustedError) Unwrap() error {
	return ErrValidationRetriesExhausted
}

// ToolErrorsExhaustedError reports repeated tool failures that exceeded budget.
type ToolErrorsExhaustedError struct {
	ToolName string
	Count    int
	Last     ToolError
}

func (e ToolErrorsExhaustedError) Error() string {
	detail := e.Last.Message
	if detail == "" {
		detail = "tool execution failed"
	}
	return fmt.Sprintf("%s after %d consecutive failure(s) from %q: %s", ErrToolErrorsExhausted, e.Count, e.ToolName, detail)
}

func (e ToolErrorsExhaustedError) Unwrap() error {
	return ErrToolErrorsExhausted
}
