// Package server provides the A2A server implementation
package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// InMemoryStore is a simple in-memory implementation of TaskStore
type InMemoryStore struct {
	tasks       map[string]*Task
	pushConfigs map[string]*PushNotificationConfig
	mutex       sync.RWMutex
}

// NewInMemory creates a new in-memory task store
func NewInMemory() *InMemoryStore {
	return &InMemoryStore{
		tasks:       make(map[string]*Task),
		pushConfigs: make(map[string]*PushNotificationConfig),
	}
}

// Create implements TaskStore.Create
func (s *InMemoryStore) Create(ctx context.Context, initial Task) (*Task, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Generate a task ID if none is provided
	if initial.ID == "" {
		initial.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now().UTC()
	initial.CreatedAt = now
	initial.UpdatedAt = now

	// Create a copy of the task
	task := initial

	// Store the task
	s.tasks[task.ID] = &task

	return &task, nil
}

// Get implements TaskStore.Get
func (s *InMemoryStore) Get(ctx context.Context, id string) (*Task, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}

	return task, nil
}

// UpdateStatus implements TaskStore.UpdateStatus
func (s *InMemoryStore) UpdateStatus(ctx context.Context, id string, status TaskStatus) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return errors.New("task not found")
	}

	// Update status and timestamp
	task.Status = status
	task.UpdatedAt = time.Now().UTC()

	// Set completion or cancellation timestamp if applicable
	switch status {
	case TaskStatusCompleted, TaskStatusFailed:
		now := time.Now().UTC()
		task.CompletedAt = &now
	case TaskStatusCanceled:
		now := time.Now().UTC()
		task.CanceledAt = &now
	}

	return nil
}

// AppendArtifact implements TaskStore.AppendArtifact
func (s *InMemoryStore) AppendArtifact(ctx context.Context, id string, art Artifact) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return errors.New("task not found")
	}

	// Generate artifact ID if none is provided
	if art.ID == "" {
		art.ID = uuid.New().String()
	}

	// Set creation timestamp if not set
	if art.CreatedAt.IsZero() {
		art.CreatedAt = time.Now().UTC()
	}

	// Append the artifact to the task
	task.Artifacts = append(task.Artifacts, art)
	task.UpdatedAt = time.Now().UTC()

	return nil
}

// Cancel implements TaskStore.Cancel
func (s *InMemoryStore) Cancel(ctx context.Context, id string) (*Task, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}

	// Update status to canceled
	task.Status = TaskStatusCanceled
	task.UpdatedAt = time.Now().UTC()
	now := time.Now().UTC()
	task.CanceledAt = &now

	return task, nil
}

// SetPushConfig implements TaskStore.SetPushConfig
func (s *InMemoryStore) SetPushConfig(ctx context.Context, id string, cfg *PushNotificationConfig) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Verify task exists
	_, ok := s.tasks[id]
	if !ok {
		return errors.New("task not found")
	}

	// Store the push configuration
	s.pushConfigs[id] = cfg

	return nil
}

// GetPushConfig implements TaskStore.GetPushConfig
func (s *InMemoryStore) GetPushConfig(ctx context.Context, id string) (*PushNotificationConfig, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Verify task exists
	_, ok := s.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}

	// Get the push configuration
	cfg, ok := s.pushConfigs[id]
	if !ok {
		return nil, errors.New("no push configuration found for task")
	}

	return cfg, nil
}
