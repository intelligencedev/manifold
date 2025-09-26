package persistence

import (
	"context"
	"time"
)

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

// ChatSession represents a persisted conversation with metadata for display.
type ChatSession struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	LastMessagePreview string    `json:"lastMessagePreview"`
	Model              string    `json:"model"`
	Summary            string    `json:"summary"`
	SummarizedCount    int       `json:"summarizedCount"`
}

// ChatMessage is a single turn within a chat session.
type ChatMessage struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// ChatStore persists chat sessions and messages.
type ChatStore interface {
	Init(ctx context.Context) error
	EnsureSession(ctx context.Context, id string, name string) (ChatSession, error)
	ListSessions(ctx context.Context) ([]ChatSession, error)
	GetSession(ctx context.Context, id string) (ChatSession, bool, error)
	CreateSession(ctx context.Context, name string) (ChatSession, error)
	RenameSession(ctx context.Context, id, name string) (ChatSession, error)
	DeleteSession(ctx context.Context, id string) error
	ListMessages(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
	AppendMessages(ctx context.Context, sessionID string, messages []ChatMessage, preview string, model string) error
	UpdateSummary(ctx context.Context, sessionID string, summary string, summarizedCount int) error
}
