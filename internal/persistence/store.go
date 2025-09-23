package persistence

import "context"

// Store is a placeholder for transcripts/state persistence.
type Store interface{}

// Specialist represents a stored specialist configuration for CRUD.
type Specialist struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	BaseURL         string            `json:"baseURL"`
	APIKey          string            `json:"apiKey"`
	Model           string            `json:"model"`
	EnableTools     bool              `json:"enableTools"`
	Paused          bool              `json:"paused"`
	AllowTools      []string          `json:"allowTools"`
	ReasoningEffort string            `json:"reasoningEffort"`
	System          string            `json:"system"`
	ExtraHeaders    map[string]string `json:"extraHeaders"`
	ExtraParams     map[string]any    `json:"extraParams"`
}

// SpecialistsStore defines CRUD over specialists.
type SpecialistsStore interface {
	Init(ctx context.Context) error
	List(ctx context.Context) ([]Specialist, error)
	GetByName(ctx context.Context, name string) (Specialist, bool, error)
	Upsert(ctx context.Context, s Specialist) (Specialist, error)
	Delete(ctx context.Context, name string) error
}
