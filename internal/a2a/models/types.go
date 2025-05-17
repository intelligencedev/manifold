// Package models provides shared data models for the A2A protocol
package models

import (
	"manifold/internal/a2a/interfaces"
)

// We're just re-exporting the types from the interfaces package
type (
	Task                   = interfaces.Task
	TaskStatus             = interfaces.TaskStatus
	Message                = interfaces.Message
	Artifact               = interfaces.Artifact
	PushNotificationConfig = interfaces.PushNotificationConfig
	TaskStore              = interfaces.TaskStore
	Authenticator          = interfaces.Authenticator
)

// Task status constants
const (
	TaskStatusPending   = interfaces.TaskStatusPending
	TaskStatusRunning   = interfaces.TaskStatusRunning
	TaskStatusCompleted = interfaces.TaskStatusCompleted
	TaskStatusFailed    = interfaces.TaskStatusFailed
	TaskStatusCanceled  = interfaces.TaskStatusCanceled
)
