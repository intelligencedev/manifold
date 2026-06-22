package agent

import (
	"errors"
	"fmt"
)

var ErrMaxStepsExceeded = errors.New("agent exceeded max steps without final response")

type MaxStepsExceededError struct {
	MaxSteps int
}

func (e MaxStepsExceededError) Error() string {
	if e.MaxSteps > 0 {
		return fmt.Sprintf("agent exceeded max steps (%d) without final response", e.MaxSteps)
	}
	return ErrMaxStepsExceeded.Error()
}

func (e MaxStepsExceededError) Unwrap() error {
	return ErrMaxStepsExceeded
}
