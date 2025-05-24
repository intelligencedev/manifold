// Package server provides the A2A server implementation
package server

import (
	"manifold/internal/a2a/interfaces"
)

// Export Task and TaskStatus from interfaces package
type (
	Task                   = interfaces.Task
	TaskStatus             = interfaces.TaskStatus
	PushNotificationConfig = interfaces.PushNotificationConfig
	Artifact               = interfaces.Artifact
	Message                = interfaces.Message
)

// Export task status constants for convenience
const (
	TaskStatusPending   = interfaces.TaskStatusPending
	TaskStatusRunning   = interfaces.TaskStatusRunning
	TaskStatusCompleted = interfaces.TaskStatusCompleted
	TaskStatusFailed    = interfaces.TaskStatusFailed
	TaskStatusCanceled  = interfaces.TaskStatusCanceled
)

// Interfaces needed by Server
type TaskStore = interfaces.TaskStore
type Authenticator = interfaces.Authenticator

// DBBackedTaskStore represents a TaskStore that has a database connection
type DBBackedTaskStore interface {
	TaskStore
	HasDBPool() bool
	GetDBPool() interface{}
}
