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

		desc := "just returns hello world"
		helloSkill := AgentSkill{
			Id:          "hello_world",
			Name:        "Returns hello world",
			Description: &desc,
			Tags:        []string{"hello world"},
			Examples:    []string{"hi", "hello world"},
		}

		card := AgentCard{
			Name:               "Hello World Agent",
			Description:        &desc,
			Url:                "http://localhost:8080/api/a2a", // Agent will run here
			Version:            "1.0.0",
			DefaultInputModes:  []string{"text"},
			DefaultOutputModes: []string{"text"},
			Capabilities:       AgentCapabilities{},                               // Basic capabilities
			Skills:             []AgentSkill{helloSkill},                          // Includes the skill defined above
			Authentication:     &AgentAuthentication{Schemes: []string{"public"}}, // No auth needed
		}

		// agentCard := map[string]interface{}{
		// 	"name":               "Hello World Agent",
		// 	"description":        "Just a hello world agent",
		// 	"url":                "http://localhost:8080/",
		// 	"version":            "1.0.0",
		// 	"defaultInputModes":  []string{"text"},
		// 	"defaultOutputModes": []string{"text"},
		// 	"capabilities":       map[string]interface{}{}, // Basic capabilities
		// 	"skills": []map[string]interface{}{
		// 		{
		// 			// Define your skill here, adjust as needed
		// 			"name":               "general_assistant",
		// 			"description":        "General purpose AI assistant",
		// 			"inputContentTypes":  []string{"text"},
		// 			"outputContentTypes": []string{"text"},
		// 		},
		// 	},
		// 	"authentication": map[string]interface{}{
		// 		"schemes": []string{"public"},
		// 	},
		// }

		return c.JSON(http.StatusOK, card)
	}
}
