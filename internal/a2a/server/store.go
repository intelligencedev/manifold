package server

import (
	"context"
	"sync"
)

// TaskStore defines the interface for storing and managing tasks
// This is just a stub interface for now
// The implementation will be provided later

type TaskStore interface {
	Create(ctx context.Context, initial Task) (*Task, error)
	Get(ctx context.Context, id string) (*Task, error)
	UpdateStatus(ctx context.Context, id string, status TaskStatus) error
	AppendArtifact(ctx context.Context, id string, art Artifact) error
	Cancel(ctx context.Context, id string) (*Task, error)
	SetPushConfig(ctx context.Context, id string, cfg *PushNotificationConfig) error
	GetPushConfig(ctx context.Context, id string) (*PushNotificationConfig, error)
}

// InMemoryStore is a simple in-memory implementation of TaskStore
// This will be used for testing and development

type InMemoryStore struct {
	tasks map[string]*Task
	mutex sync.Mutex
}

func NewInMemory() *InMemoryStore {
	return &InMemoryStore{
		tasks: make(map[string]*Task),
	}
}
