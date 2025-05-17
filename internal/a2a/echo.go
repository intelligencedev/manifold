// Package a2a provides the Agent2Agent (A2A) Protocol implementation for Manifold
package a2a

import (
	"context"
	"net/http"
	"sync"

	"github.com/google/uuid"
	echo "github.com/labstack/echo/v4"

	"manifold/internal/a2a/auth"
	"manifold/internal/a2a/models"
	"manifold/internal/a2a/server"

	config "manifold/internal/config"
)

// Define our implementation of the TaskStore interface
type manifoldTaskStore struct {
	cfg   *config.Config
	tasks map[string]*models.Task
	mutex sync.Mutex
}

// HasDBPool implements server.DBBackedTaskStore
func (s *manifoldTaskStore) HasDBPool() bool {
	return s.cfg != nil && s.cfg.DBPool != nil
}

// GetDBPool implements server.DBBackedTaskStore
func (s *manifoldTaskStore) GetDBPool() interface{} {
	if s.cfg == nil {
		return nil
	}
	return s.cfg.DBPool
}

// NewTaskStore creates a TaskStore implementation configured for Manifold
func NewTaskStore(cfgParam interface{}) models.TaskStore { // Renamed parameter "config" to "cfgParam"
	var cfg *config.Config
	if c, ok := cfgParam.(*config.Config); ok {
		cfg = c
	}
	return &manifoldTaskStore{
		cfg:   cfg,
		tasks: make(map[string]*models.Task),
	}
}

// Create implements TaskStore.Create
func (s *manifoldTaskStore) Create(ctx context.Context, initial models.Task) (*models.Task, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Generate a task ID if none is provided
	if initial.ID == "" {
		initial.ID = uuid.New().String()
	}

	// Create a copy of the task
	task := initial

	// Store the task
	s.tasks[task.ID] = &task

	return &task, nil
}

// Get implements TaskStore.Get
func (s *manifoldTaskStore) Get(ctx context.Context, id string) (*models.Task, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}

	return task, nil
}

// UpdateStatus implements TaskStore.UpdateStatus
func (s *manifoldTaskStore) UpdateStatus(ctx context.Context, id string, status models.TaskStatus) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}

	task.Status = status
	return nil
}

// AppendArtifact implements TaskStore.AppendArtifact
func (s *manifoldTaskStore) AppendArtifact(ctx context.Context, id string, art models.Artifact) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, ok := s.tasks[id]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}

	// This is a simplification - you'll need to implement proper artifact handling
	// based on the A2A specification
	return nil
}

// Cancel implements TaskStore.Cancel
func (s *manifoldTaskStore) Cancel(ctx context.Context, id string) (*models.Task, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}

	task.Status = models.TaskStatusCanceled
	return task, nil
}

// SetPushConfig implements TaskStore.SetPushConfig
func (s *manifoldTaskStore) SetPushConfig(ctx context.Context, id string, cfg *models.PushNotificationConfig) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, ok := s.tasks[id]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}

	// Implement push notification config setting
	return nil
}

// GetPushConfig implements TaskStore.GetPushConfig
func (s *manifoldTaskStore) GetPushConfig(ctx context.Context, id string) (*models.PushNotificationConfig, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, ok := s.tasks[id]
	if !ok {
		return nil, echo.NewHTTPError(http.StatusNotFound, "Task not found")
	}

	// Return a placeholder config
	return &models.PushNotificationConfig{}, nil
}

// NewAuthenticator creates an Authenticator for A2A requests
func NewAuthenticator(cfgParam interface{}) models.Authenticator { // Renamed parameter "config" to "cfgParam"
	cfg, ok := cfgParam.(*config.Config) // Updated to use cfgParam
	if ok && cfg.A2A.Token != "" {
		return auth.NewTokenModels(cfg.A2A.Token)
	}
	return auth.NewNoopModels()
}

// NewEchoHandler creates an http.Handler that can be used with Echo's WrapHandler
func NewEchoHandler(store models.TaskStore, authenticator models.Authenticator) http.Handler {
	// Create adapters to convert between our types and server package types
	storeAdapter := NewServerTaskStoreAdapter(store)
	authAdapter := NewServerAuthenticatorAdapter(authenticator)

	// Create a new A2A server
	a2aServer := server.NewServer(storeAdapter, authAdapter)

	// Wrap it with the authentication middleware
	return server.Authenticate(a2aServer, authAdapter)
}

// AgentCardHandler returns an echo.HandlerFunc that serves the Agent Card JSON
func AgentCardHandler(cfgParam interface{}) echo.HandlerFunc { // Renamed parameter "config" to "cfgParam"
	// Create a sample Agent Card based on the A2A specification
	// This should be customized based on your Manifold capabilities
	return func(c echo.Context) error {
		agentCard := map[string]interface{}{
			"name":        "Manifold A2A Agent",
			"description": "Manifold A2A agent implementation based on the Agent2Agent Protocol.",
			"url":         "https://your-manifold-url.com/api/a2a", // Update this URL to match your deployment
			"provider": map[string]interface{}{
				"organization": "Manifold",
				"url":          "https://manifold.ai", // Update with your organization URL
			},
			"capabilities": map[string]interface{}{
				"streaming":              true,
				"pushNotifications":      false,
				"stateTransitionHistory": false,
			},
			"authentication": map[string]interface{}{
				"type": func() string {
					if cfg, ok := cfgParam.(*config.Config); ok && cfg.A2A.Token != "" { // Updated to use cfgParam
						return "bearer"
					}
					return "none"
				}(),
			},
			"defaultInputContentTypes": []string{
				"text/plain",
				"application/json",
			},
			"defaultOutputContentTypes": []string{
				"text/plain",
				"application/json",
			},
			"skills": []map[string]interface{}{
				{
					"name":               "general_assistant",
					"description":        "General purpose AI assistant",
					"inputContentTypes":  []string{"text/plain"},
					"outputContentTypes": []string{"text/plain"},
				},
			},
		}

		return c.JSON(http.StatusOK, agentCard)
	}
}
