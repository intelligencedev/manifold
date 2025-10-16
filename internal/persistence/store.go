package persistence

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound indicates the requested record does not exist.
	ErrNotFound = errors.New("persistence: not found")
	// ErrForbidden indicates the caller is not authorized to access the record.
	ErrForbidden = errors.New("persistence: forbidden")
)

// Store is a placeholder for transcripts/state persistence.
type Store interface{}

// Specialist represents a stored specialist configuration for CRUD.
type Specialist struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
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
	UserID             *int64    `json:"userId,omitempty"`
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
	EnsureSession(ctx context.Context, userID *int64, id string, name string) (ChatSession, error)
	ListSessions(ctx context.Context, userID *int64) ([]ChatSession, error)
	GetSession(ctx context.Context, userID *int64, id string) (ChatSession, error)
	CreateSession(ctx context.Context, userID *int64, name string) (ChatSession, error)
	RenameSession(ctx context.Context, userID *int64, id, name string) (ChatSession, error)
	DeleteSession(ctx context.Context, userID *int64, id string) error
	ListMessages(ctx context.Context, userID *int64, sessionID string, limit int) ([]ChatMessage, error)
	AppendMessages(ctx context.Context, userID *int64, sessionID string, messages []ChatMessage, preview string, model string) error
	UpdateSummary(ctx context.Context, userID *int64, sessionID string, summary string, summarizedCount int) error
}

// WarppWorkflow is a minimal persistence representation of a WARPP workflow.
// It mirrors internal/warpp.Workflow but uses flexible types for nested fields
// to avoid import cycles.
type WarppWorkflow struct {
	Intent         string           `json:"intent"`
	Description    string           `json:"description"`
	Keywords       []string         `json:"keywords"`
	Steps          []map[string]any `json:"steps"`
	UI             map[string]any   `json:"ui,omitempty"`
	MaxConcurrency int              `json:"max_concurrency,omitempty"`
	FailFast       bool             `json:"fail_fast,omitempty"`
}

// WarppWorkflowStore persists WARPP workflows by intent.
type WarppWorkflowStore interface {
	Init(ctx context.Context) error
	List(ctx context.Context) ([]any, error) // deprecated; use ListWorkflows
	ListWorkflows(ctx context.Context) ([]WarppWorkflow, error)
	Get(ctx context.Context, intent string) (WarppWorkflow, bool, error)
	Upsert(ctx context.Context, wf WarppWorkflow) (WarppWorkflow, error)
	Delete(ctx context.Context, intent string) error
}
