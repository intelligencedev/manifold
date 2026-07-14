package agentd

import (
	"context"

	agentmemory "manifold/internal/agent/memory"
	"manifold/internal/durable"
)

const (
	durableChatQueue             = "chat"
	durableChatRunTaskName       = "chat.run"
	durableChatWorkerConcurrency = 4
)

type durableChatTaskParams struct {
	Request  chatRunRequest           `json:"request"`
	Target   durableChatTargetPayload `json:"target,omitempty"`
	Endpoint string                   `json:"endpoint,omitempty"`
	Owner    int64                    `json:"owner,omitempty"`
}

type durableChatTargetPayload struct {
	Specialist string `json:"specialist,omitempty"`
	Team       string `json:"team,omitempty"`
}

type durableChatPreparedRun struct {
	exec       chatExecutionRequest
	streamOpts chatStreamOptions
	summary    *agentmemory.SummaryResult
}

type durableChatExecution struct {
	runCtx            context.Context
	task              durable.Task
	prepared          durableChatPreparedRun
	writer            *durableChatEventWriter
	collector         *chatTurnCollector
	activityCollector *chatActivityCollector
}

type durableChatTaskMetadata struct {
	sessionID          string
	userMessageID      string
	assistantMessageID string
}

type durableChatPreparedRequest struct {
	req    chatRunRequest
	owner  int64
	userID *int64
	runCtx context.Context
}
