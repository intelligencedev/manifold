// Package client provides the A2A client implementation
package client

import (
	"manifold/internal/a2a/server"
)

// SendTaskStreamingResponse represents a streaming response from tasks/sendSubscribe
type SendTaskStreamingResponse struct {
	Task  *server.Task
	Done  bool
	Error error
}

// SendTaskResponse represents a response from tasks/send
type SendTaskResponse struct {
	Task  *server.Task
	Error error
}
