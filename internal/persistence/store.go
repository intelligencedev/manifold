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
	UserID          int64             `json:"userId"`
	Name            string            `json:"name"`
	Provider        string            `json:"provider"`
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
	List(ctx context.Context, userID int64) ([]Specialist, error)
	GetByName(ctx context.Context, userID int64, name string) (Specialist, bool, error)
	Upsert(ctx context.Context, userID int64, s Specialist) (Specialist, error)
	Delete(ctx context.Context, userID int64, name string) error
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
	// Optional, not persisted: used to hydrate tool calls for the UI.
	Title    string `json:"title,omitempty"`
	ToolArgs string `json:"toolArgs,omitempty"`
	ToolID   string `json:"toolId,omitempty"`
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
	UserID         int64            `json:"userId"`
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
	List(ctx context.Context, userID int64) ([]any, error) // deprecated; use ListWorkflows
	ListWorkflows(ctx context.Context, userID int64) ([]WarppWorkflow, error)
	GetByIntent(ctx context.Context, userID int64, intent string) (WarppWorkflow, bool, error)
	Upsert(ctx context.Context, userID int64, w WarppWorkflow) (WarppWorkflow, error)
	Delete(ctx context.Context, userID int64, intent string) error
}

// MCPServer represents a stored MCP server configuration.
type MCPServer struct {
	ID               int64             `json:"id"`
	UserID           int64             `json:"userId"`
	Name             string            `json:"name"`
	Command          string            `json:"command"`
	Args             []string          `json:"args"`
	Env              map[string]string `json:"env"`
	URL              string            `json:"url"`
	Headers          map[string]string `json:"headers"`
	BearerToken      string            `json:"bearerToken"`
	Origin           string            `json:"origin"`
	ProtocolVersion  string            `json:"protocolVersion"`
	KeepAliveSeconds int               `json:"keepAliveSeconds"`
	Disabled         bool              `json:"disabled"`

	// OAuth fields
	OAuthProvider     string    `json:"oauthProvider"`
	OAuthClientID     string    `json:"oauthClientId"`
	OAuthClientSecret string    `json:"oauthClientSecret"`
	OAuthAccessToken  string    `json:"-"`
	OAuthRefreshToken string    `json:"-"`
	OAuthExpiresAt    time.Time `json:"-"`
	OAuthScopes       []string  `json:"oauthScopes"`
}

// MCPStore defines CRUD over MCP servers.
type MCPStore interface {
	Init(ctx context.Context) error
	List(ctx context.Context, userID int64) ([]MCPServer, error)
	GetByName(ctx context.Context, userID int64, name string) (MCPServer, bool, error)
	Upsert(ctx context.Context, userID int64, s MCPServer) (MCPServer, error)
	Delete(ctx context.Context, userID int64, name string) error
}
