package durable

import "errors"

var (
	ErrNotFound        = errors.New("durable task not found")
	ErrTaskNotFound    = ErrNotFound
	ErrTaskExists      = errors.New("durable task already exists")
	ErrNoRunnableTasks = errors.New("no runnable durable tasks")
	ErrHandlerNotFound = errors.New("durable handler not found")
	ErrSuspended       = errors.New("durable task suspended")
	ErrCancelled       = errors.New("durable task cancelled")
	ErrDeadlock        = errors.New("durable child wait would deadlock")
)
