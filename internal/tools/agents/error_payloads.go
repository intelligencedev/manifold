package agents

import (
	"context"
	"errors"
	"strings"
)

func delegatedRunErrorPayload(err error) map[string]any {
	payload := map[string]any{
		"ok":    false,
		"error": err.Error(),
	}
	switch {
	case errors.Is(err, context.Canceled):
		payload["error"] = "delegated run cancelled"
		payload["error_code"] = "delegated_run_cancelled"
		payload["cancelled"] = true
	case delegatedRunTimedOut(err):
		payload["error_code"] = "delegated_run_timeout"
		payload["timed_out"] = true
	case delegatedRunMaxSteps(err.Error()):
		payload["error_code"] = "delegated_run_max_steps"
		payload["max_steps_exceeded"] = true
	}
	return payload
}

func delegatedRunStatusPayload(status int, body string) map[string]any {
	payload := map[string]any{
		"ok":     false,
		"status": status,
		"error":  body,
	}
	lower := strings.ToLower(strings.TrimSpace(body))
	switch {
	case strings.Contains(lower, "context canceled"), strings.Contains(lower, "context cancelled"):
		payload["error"] = "delegated run cancelled"
		payload["error_code"] = "delegated_run_cancelled"
		payload["cancelled"] = true
	case strings.Contains(lower, "context deadline exceeded"), strings.Contains(lower, "client.timeout exceeded"):
		payload["error_code"] = "delegated_run_timeout"
		payload["timed_out"] = true
	case delegatedRunMaxSteps(lower):
		payload["error_code"] = "delegated_run_max_steps"
		payload["max_steps_exceeded"] = true
	}
	return payload
}

func delegatedRunTimedOut(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "client.timeout exceeded")
}

func delegatedRunMaxSteps(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "exceeded max steps") || strings.Contains(lower, "max steps") && strings.Contains(lower, "without final response")
}
