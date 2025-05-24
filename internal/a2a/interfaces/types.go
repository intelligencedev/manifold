// Package interfaces provides shared interfaces for A2A components
package interfaces

import (
	"context"
	"net/http"
	"time"
)

// Task represents an A2A task
type Task struct {
	ID          string     `json:"id"`
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CanceledAt  *time.Time `json:"canceledAt,omitempty"`
	Artifacts   []Artifact `json:"artifacts,omitempty"`
	Messages    []Message  `json:"messages,omitempty"`
}

// TaskStatus represents the status of an A2A task
type TaskStatus string

// Task status constants
const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCanceled  TaskStatus = "canceled"
)

// Message represents a message in an A2A task
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// Artifact represents an artifact produced or consumed by an A2A task
type Artifact struct {
	ID          string    `json:"id"`
	ContentType string    `json:"contentType"`
	Name        string    `json:"name,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Data        []byte    `json:"data,omitempty"`
}

// PushNotificationConfig represents a configuration for push notifications
type PushNotificationConfig struct {
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	SecretHash string            `json:"-"` // Not exposed in JSON
}

// TaskStore is the interface for storing and retrieving tasks
type TaskStore interface {
	Create(ctx context.Context, task Task) (*Task, error)
	Get(ctx context.Context, id string) (*Task, error)
	UpdateStatus(ctx context.Context, id string, status TaskStatus) error
	AppendArtifact(ctx context.Context, id string, artifact Artifact) error
	Cancel(ctx context.Context, id string) (*Task, error)
	SetPushConfig(ctx context.Context, id string, config *PushNotificationConfig) error
	GetPushConfig(ctx context.Context, id string) (*PushNotificationConfig, error)
}

// Authenticator is the interface for authenticating A2A requests
type Authenticator interface {
	Authenticate(r *http.Request) (bool, error)
}
