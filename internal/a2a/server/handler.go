// Package server provides the A2A server implementation
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"manifold/internal/a2a/rpc"
	"manifold/internal/a2a/sse"
)

// Server implements the A2A protocol server
type Server struct {
	rpc   *rpc.Router
	store TaskStore
	auth  Authenticator
}

// NewServer creates a new A2A server with the given task store and authenticator
func NewServer(store TaskStore, auth Authenticator) *Server {
	s := &Server{
		rpc:   rpc.NewRouter(),
		store: store,
		auth:  auth,
	}

	// Register A2A methods
	s.rpc.Register("tasks/send", s.handleSend)
	s.rpc.Register("tasks/sendSubscribe", s.handleSendSubscribe)
	s.rpc.Register("tasks/get", s.handleGet)
	s.rpc.Register("tasks/cancel", s.handleCancel)
	s.rpc.Register("tasks/pushNotification/set", s.handlePushSet)
	s.rpc.Register("tasks/pushNotification/get", s.handlePushGet)
	s.rpc.Register("tasks/resubscribe", s.handleResubscribe)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.rpc.ServeHTTP(w, r)
}

// SendRequest is the request format for the tasks/send method
type SendRequest struct {
	Message Message `json:"message"`
}

// SendResponse is the response format for the tasks/send method
type SendResponse struct {
	Task Task `json:"task"`
}

// handleSend implements the tasks/send method
func (s *Server) handleSend(ctx context.Context, params json.RawMessage) (interface{}, *rpc.JSONRPCError) {
	var req SendRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Invalid request parameters",
		}
	}

	// Create a new task
	task := Task{
		ID:        uuid.New().String(),
		Status:    TaskStatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Messages:  []Message{req.Message},
	}

	// Store the task
	createdTask, err := s.store.Create(ctx, task)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.InternalErrorCode,
			Message: "Failed to create task",
		}
	}

	// Start a goroutine to process the task
	go func() {
		// Update task to running
		s.store.UpdateStatus(context.Background(), createdTask.ID, TaskStatusRunning)

		// Simulate task processing (this would be replaced with real processing)
		time.Sleep(2 * time.Second)

		// Update task to completed
		s.store.UpdateStatus(context.Background(), createdTask.ID, TaskStatusCompleted)
	}()

	return SendResponse{Task: *createdTask}, nil
}

// GetRequest is the request format for the tasks/get method
type GetRequest struct {
	TaskID string `json:"taskId"`
}

// GetResponse is the response format for the tasks/get method
type GetResponse struct {
	Task Task `json:"task"`
}

// handleGet implements the tasks/get method
func (s *Server) handleGet(ctx context.Context, params json.RawMessage) (interface{}, *rpc.JSONRPCError) {
	var req GetRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Invalid request parameters",
		}
	}

	// Get the task from the store
	task, err := s.store.Get(ctx, req.TaskID)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.TaskNotFoundErrorCode,
			Message: "Task not found",
		}
	}

	return GetResponse{Task: *task}, nil
}

// CancelRequest is the request format for the tasks/cancel method
type CancelRequest struct {
	TaskID string `json:"taskId"`
}

// CancelResponse is the response format for the tasks/cancel method
type CancelResponse struct {
	Task Task `json:"task"`
}

// handleCancel implements the tasks/cancel method
func (s *Server) handleCancel(ctx context.Context, params json.RawMessage) (interface{}, *rpc.JSONRPCError) {
	var req CancelRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Invalid request parameters",
		}
	}

	// Cancel the task in the store
	task, err := s.store.Cancel(ctx, req.TaskID)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.TaskNotFoundErrorCode,
			Message: "Task not found",
		}
	}

	return CancelResponse{Task: *task}, nil
}

// PushSetRequest is the request format for the tasks/pushNotification/set method
type PushSetRequest struct {
	TaskID string                 `json:"taskId"`
	Config PushNotificationConfig `json:"config"`
}

// PushSetResponse is the response format for the tasks/pushNotification/set method
type PushSetResponse struct {
	Success bool `json:"success"`
}

// handlePushSet implements the tasks/pushNotification/set method
func (s *Server) handlePushSet(ctx context.Context, params json.RawMessage) (interface{}, *rpc.JSONRPCError) {
	var req PushSetRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Invalid request parameters",
		}
	}

	// Set the push notification config in the store
	err := s.store.SetPushConfig(ctx, req.TaskID, &req.Config)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.TaskNotFoundErrorCode,
			Message: "Task not found",
		}
	}

	return PushSetResponse{Success: true}, nil
}

// PushGetRequest is the request format for the tasks/pushNotification/get method
type PushGetRequest struct {
	TaskID string `json:"taskId"`
}

// PushGetResponse is the response format for the tasks/pushNotification/get method
type PushGetResponse struct {
	Config PushNotificationConfig `json:"config"`
}

// handlePushGet implements the tasks/pushNotification/get method
func (s *Server) handlePushGet(ctx context.Context, params json.RawMessage) (interface{}, *rpc.JSONRPCError) {
	var req PushGetRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Invalid request parameters",
		}
	}

	// Get the push notification config from the store
	config, err := s.store.GetPushConfig(ctx, req.TaskID)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.TaskNotFoundErrorCode,
			Message: "Task not found",
		}
	}

	return PushGetResponse{Config: *config}, nil
}

// handleSendSubscribe implements the tasks/sendSubscribe method for streaming responses
func (s *Server) handleSendSubscribe(ctx context.Context, params json.RawMessage) (interface{}, *rpc.JSONRPCError) {
	// This method needs special handling because it's streaming
	// It will be called by the RPC router but should never return a normal response
	// Instead, it should write directly to the HTTP response using SSE

	// Extract the HTTP request and response writer from the context
	httpReq, ok := ctx.Value(rpc.RequestCtxKey).(*http.Request)
	if !ok {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.InternalErrorCode,
			Message: "Failed to get HTTP request from context",
		}
	}

	// Parse the SendSubscribe request
	var req SendRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Invalid request parameters",
		}
	}

	// Extract the JSON-RPC request ID
	bodyData, err := io.ReadAll(httpReq.Body)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Failed to read request body",
		}
	}
	// Reset the body for future reads
	httpReq.Body = io.NopCloser(bytes.NewBuffer(bodyData))

	var jsonrpcReq rpc.JSONRPCRequest
	if err := json.Unmarshal(bodyData, &jsonrpcReq); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Failed to parse JSON-RPC request",
		}
	}

	// Get the response writer from context
	respWriter := ctx.Value(rpc.ResponseWriterCtxKey).(http.ResponseWriter)

	// Create an SSE writer
	sseWriter := sse.NewSSEWriter(respWriter)

	// Create the task
	task := Task{
		ID:        uuid.New().String(),
		Status:    TaskStatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Messages:  []Message{req.Message},
	}

	// Store the task
	createdTask, err := s.store.Create(ctx, task)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.InternalErrorCode,
			Message: "Failed to create task",
		}
	}

	// Send the initial task response
	initialResponse := rpc.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      jsonrpcReq.ID,
		Result: map[string]interface{}{
			"task": createdTask,
		},
	}

	if err := sseWriter.Send(initialResponse); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.InternalErrorCode,
			Message: "Failed to send SSE response",
		}
	}

	// Start a goroutine to process the task and send updates
	go func() {
		// Update task to running
		s.store.UpdateStatus(context.Background(), createdTask.ID, TaskStatusRunning)

		// Simulate task processing (should be replaced with actual processing)
		time.Sleep(1 * time.Second)

		// Send a progress update
		progressResponse := rpc.JSONRPCResponse{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"status":   "running",
				"progress": 0.5,
			},
		}
		sseWriter.Send(progressResponse)

		// Simulate more processing
		time.Sleep(1 * time.Second)

		// Create a response message
		responseMsg := Message{
			ID:        uuid.New().String(),
			Role:      "assistant",
			CreatedAt: time.Now().UTC(),
			Parts: []Part{
				TextPart{
					Type: "text",
					Text: "This is a response to your request",
				},
			},
		}

		// Add the response message to the task
		task.Messages = append(task.Messages, responseMsg)

		// Update task to completed
		s.store.UpdateStatus(context.Background(), createdTask.ID, TaskStatusCompleted)

		// Send the final update
		completedTask, _ := s.store.Get(context.Background(), createdTask.ID)
		finalResponse := rpc.JSONRPCResponse{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"task":   completedTask,
				"status": "completed",
			},
		}
		sseWriter.Send(finalResponse)

		// Close the SSE stream
		sseWriter.Close()
	}()

	// Return a special marker to indicate this is handled via streaming
	// This value is never serialized because the response is already being written
	return "STREAMING", nil
}

// handleResubscribe implements the tasks/resubscribe method
func (s *Server) handleResubscribe(ctx context.Context, params json.RawMessage) (interface{}, *rpc.JSONRPCError) {
	// Extract the HTTP request and response writer from the context
	httpReq, ok := ctx.Value(rpc.RequestCtxKey).(*http.Request)
	if !ok {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.InternalErrorCode,
			Message: "Failed to get HTTP request from context",
		}
	}

	// Parse the ResubscribeRequest
	var req struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Invalid request parameters",
		}
	}

	// Extract the JSON-RPC request ID
	bodyData, err := io.ReadAll(httpReq.Body)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Failed to read request body",
		}
	}
	// Reset the body for future reads
	httpReq.Body = io.NopCloser(bytes.NewBuffer(bodyData))

	var jsonrpcReq rpc.JSONRPCRequest
	if err := json.Unmarshal(bodyData, &jsonrpcReq); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.ParseErrorCode,
			Message: "Failed to parse JSON-RPC request",
		}
	}

	// Get the task
	task, err := s.store.Get(ctx, req.TaskID)
	if err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.TaskNotFoundErrorCode,
			Message: "Task not found",
		}
	}

	// Get the response writer from context
	respWriter := ctx.Value(rpc.ResponseWriterCtxKey).(http.ResponseWriter)

	// Create an SSE writer
	sseWriter := sse.NewSSEWriter(respWriter)

	// Send the current task state
	initialResponse := rpc.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      jsonrpcReq.ID,
		Result: map[string]interface{}{
			"task": task,
		},
	}

	if err := sseWriter.Send(initialResponse); err != nil {
		return nil, &rpc.JSONRPCError{
			Code:    rpc.InternalErrorCode,
			Message: "Failed to send SSE response",
		}
	}

	// If the task is already completed or canceled, we can close the stream
	if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed || task.Status == TaskStatusCanceled {
		sseWriter.Close()
		return "STREAMING", nil
	}

	// Otherwise, start a goroutine to send updates
	go func() {
		// Keep checking the task status
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// Get the latest task state
			latestTask, err := s.store.Get(context.Background(), req.TaskID)
			if err != nil {
				// Task not found, close the stream
				sseWriter.Close()
				return
			}

			// Send the latest state
			updateResponse := rpc.JSONRPCResponse{
				JSONRPC: "2.0",
				Result: map[string]interface{}{
					"task": latestTask,
				},
			}
			sseWriter.Send(updateResponse)

			// If the task is done, close the stream
			if latestTask.Status == TaskStatusCompleted || latestTask.Status == TaskStatusFailed || latestTask.Status == TaskStatusCanceled {
				sseWriter.Close()
				return
			}
		}
	}()

	// Return a special marker to indicate this is handled via streaming
	return "STREAMING", nil
}
