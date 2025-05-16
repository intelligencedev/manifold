package client

import (
	"manifold/internal/a2a/rpc"
)

type SendTaskStreamingResponse struct {
	Task  *rpc.Task
	Done  bool
	Error error
}

type SendTaskResponse struct {
	Task  *rpc.Task
	Error error
}
