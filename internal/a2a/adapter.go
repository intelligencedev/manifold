// Package a2a provides the Agent2Agent (A2A) Protocol implementation for Manifold
package a2a

import (
	"context"
	"net/http"

	"manifold/internal/a2a/models"
	"manifold/internal/a2a/server"
)

// serverTaskStoreAdapter adapts a models.TaskStore to a server.TaskStore
type serverTaskStoreAdapter struct {
	store models.TaskStore
}

// NewServerTaskStoreAdapter creates a new adapter from a models.TaskStore to a server.TaskStore
func NewServerTaskStoreAdapter(store models.TaskStore) server.TaskStore {
	return &serverTaskStoreAdapter{
		store: store,
	}
}

// Create implements server.TaskStore.Create
func (a *serverTaskStoreAdapter) Create(ctx context.Context, initial server.Task) (*server.Task, error) {
	modelTask := models.Task{
		ID:          initial.ID,
		Status:      models.TaskStatus(initial.Status),
		CreatedAt:   initial.CreatedAt,
		UpdatedAt:   initial.UpdatedAt,
		CompletedAt: initial.CompletedAt,
		CanceledAt:  initial.CanceledAt,
		// Would need to convert artifacts and messages as well
	}

	result, err := a.store.Create(ctx, modelTask)
	if err != nil {
		return nil, err
	}

	return &server.Task{
		ID:          result.ID,
		Status:      server.TaskStatus(result.Status),
		CreatedAt:   result.CreatedAt,
		UpdatedAt:   result.UpdatedAt,
		CompletedAt: result.CompletedAt,
		CanceledAt:  result.CanceledAt,
		// Would need to convert artifacts and messages as well
	}, nil
}

// Get implements server.TaskStore.Get
func (a *serverTaskStoreAdapter) Get(ctx context.Context, id string) (*server.Task, error) {
	result, err := a.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &server.Task{
		ID:          result.ID,
		Status:      server.TaskStatus(result.Status),
		CreatedAt:   result.CreatedAt,
		UpdatedAt:   result.UpdatedAt,
		CompletedAt: result.CompletedAt,
		CanceledAt:  result.CanceledAt,
		// Would need to convert artifacts and messages as well
	}, nil
}

// UpdateStatus implements server.TaskStore.UpdateStatus
func (a *serverTaskStoreAdapter) UpdateStatus(ctx context.Context, id string, status server.TaskStatus) error {
	return a.store.UpdateStatus(ctx, id, models.TaskStatus(status))
}

// AppendArtifact implements server.TaskStore.AppendArtifact
func (a *serverTaskStoreAdapter) AppendArtifact(ctx context.Context, id string, art server.Artifact) error {
	modelArtifact := models.Artifact{
		ID:          art.ID,
		ContentType: art.ContentType,
		Name:        art.Name,
		CreatedAt:   art.CreatedAt,
		Data:        art.Data,
	}

	return a.store.AppendArtifact(ctx, id, modelArtifact)
}

// Cancel implements server.TaskStore.Cancel
func (a *serverTaskStoreAdapter) Cancel(ctx context.Context, id string) (*server.Task, error) {
	result, err := a.store.Cancel(ctx, id)
	if err != nil {
		return nil, err
	}

	return &server.Task{
		ID:          result.ID,
		Status:      server.TaskStatus(result.Status),
		CreatedAt:   result.CreatedAt,
		UpdatedAt:   result.UpdatedAt,
		CompletedAt: result.CompletedAt,
		CanceledAt:  result.CanceledAt,
		// Would need to convert artifacts and messages as well
	}, nil
}

// SetPushConfig implements server.TaskStore.SetPushConfig
func (a *serverTaskStoreAdapter) SetPushConfig(ctx context.Context, id string, cfg *server.PushNotificationConfig) error {
	modelConfig := &models.PushNotificationConfig{
		URL:        cfg.URL,
		Headers:    cfg.Headers,
		SecretHash: cfg.SecretHash,
	}

	return a.store.SetPushConfig(ctx, id, modelConfig)
}

// GetPushConfig implements server.TaskStore.GetPushConfig
func (a *serverTaskStoreAdapter) GetPushConfig(ctx context.Context, id string) (*server.PushNotificationConfig, error) {
	result, err := a.store.GetPushConfig(ctx, id)
	if err != nil {
		return nil, err
	}

	return &server.PushNotificationConfig{
		URL:        result.URL,
		Headers:    result.Headers,
		SecretHash: result.SecretHash,
	}, nil
}

// serverAuthenticatorAdapter adapts a models.Authenticator to a server.Authenticator
type serverAuthenticatorAdapter struct {
	auth models.Authenticator
}

// NewServerAuthenticatorAdapter creates a new adapter from a models.Authenticator to a server.Authenticator
func NewServerAuthenticatorAdapter(auth models.Authenticator) server.Authenticator {
	return &serverAuthenticatorAdapter{
		auth: auth,
	}
}

// Authenticate implements server.Authenticator.Authenticate
func (a *serverAuthenticatorAdapter) Authenticate(r *http.Request) (bool, error) {
	return a.auth.Authenticate(r)
}
