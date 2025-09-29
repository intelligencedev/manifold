package registry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Prompt represents a named prompt definition.
type Prompt struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tags        []string          `json:"tags"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"createdAt"`
}

// VariableSchema describes one templated variable for a prompt.
type VariableSchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// Guardrails embeds light-weight validations that can run before execution.
type Guardrails struct {
	MaxTokens  int      `json:"maxTokens"`
	Validators []string `json:"validators"`
}

// PromptVersion describes a concrete prompt template captured at a point in time.
type PromptVersion struct {
	ID          string                    `json:"id"`
	PromptID    string                    `json:"promptId"`
	Semver      string                    `json:"semver"`
	Template    string                    `json:"template"`
	Variables   map[string]VariableSchema `json:"variables"`
	Guardrails  Guardrails                `json:"guardrails"`
	ContentHash string                    `json:"contentHash"`
	CreatedBy   string                    `json:"createdBy"`
	CreatedAt   time.Time                 `json:"createdAt"`
}

// Manifest records a signed statement of prompt version integrity.
type Manifest struct {
	PromptVersionID string    `json:"promptVersionId"`
	ContentHash     string    `json:"contentHash"`
	Signed          bool      `json:"signed"`
	EnvDigest       string    `json:"envDigest"`
	CreatedAt       time.Time `json:"createdAt"`
}

// Store defines the persistence contract the registry relies on.
type Store interface {
	CreatePrompt(ctx context.Context, prompt Prompt) (Prompt, error)
	GetPrompt(ctx context.Context, id string) (Prompt, bool, error)
	ListPrompts(ctx context.Context, filter ListFilter) ([]Prompt, error)
	CreatePromptVersion(ctx context.Context, version PromptVersion) (PromptVersion, error)
	ListPromptVersions(ctx context.Context, promptID string) ([]PromptVersion, error)
	GetPromptVersion(ctx context.Context, id string) (PromptVersion, bool, error)
	DeletePrompt(ctx context.Context, id string) error
}

// ListFilter controls pagination and fuzzy matching behaviour.
type ListFilter struct {
	Query   string
	Tag     string
	Page    int
	PerPage int
}

// Registry coordinates prompt and version management.
type Registry struct {
	store Store
	clock Clock
}

// Clock abstracts timekeeping for testability.
type Clock interface {
	Now() time.Time
}

// SystemClock is the default clock implementation.
type SystemClock struct{}

// Now returns the current UTC timestamp.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// New constructs a registry using the provided store.
func New(store Store) *Registry {
	return &Registry{store: store, clock: SystemClock{}}
}

// WithClock injects a custom clock (mainly for tests).
func (r *Registry) WithClock(clock Clock) *Registry {
	r.clock = clock
	return r
}

var (
	// ErrPromptExists indicates a prompt conflict on creation.
	ErrPromptExists = errors.New("playground/registry: prompt already exists")
	// ErrPromptNotFound is returned when a prompt cannot be located.
	ErrPromptNotFound = errors.New("playground/registry: prompt not found")
)

// CreatePrompt persists a new prompt definition.
func (r *Registry) CreatePrompt(ctx context.Context, prompt Prompt) (Prompt, error) {
	prompt.CreatedAt = r.clock.Now()
	return r.store.CreatePrompt(ctx, prompt)
}

// CreatePromptVersion stores a prompt version after enriching with metadata.
func (r *Registry) CreatePromptVersion(ctx context.Context, promptID string, version PromptVersion) (PromptVersion, error) {
	if promptID == "" {
		return PromptVersion{}, fmt.Errorf("promptID must be provided")
	}
	if version.Template == "" {
		return PromptVersion{}, fmt.Errorf("template must be provided")
	}
	version.PromptID = promptID
	hash, err := ComputeContentHash(version.Template, version.Variables)
	if err != nil {
		return PromptVersion{}, err
	}
	version.ContentHash = hash
	version.CreatedAt = r.clock.Now()
	return r.store.CreatePromptVersion(ctx, version)
}

// ListPrompts returns prompts applying an optional filter.
func (r *Registry) ListPrompts(ctx context.Context, filter ListFilter) ([]Prompt, error) {
	return r.store.ListPrompts(ctx, filter)
}

// GetPrompt fetches a prompt by ID.
func (r *Registry) GetPrompt(ctx context.Context, id string) (Prompt, bool, error) {
	return r.store.GetPrompt(ctx, id)
}

// DeletePrompt removes a prompt and its versions.
func (r *Registry) DeletePrompt(ctx context.Context, id string) error {
	return r.store.DeletePrompt(ctx, id)
}

// ListPromptVersions retrieves versions for a prompt.
func (r *Registry) ListPromptVersions(ctx context.Context, promptID string) ([]PromptVersion, error) {
	return r.store.ListPromptVersions(ctx, promptID)
}

// GetPromptVersion fetches a prompt version by its identifier.
func (r *Registry) GetPromptVersion(ctx context.Context, id string) (PromptVersion, bool, error) {
	return r.store.GetPromptVersion(ctx, id)
}
