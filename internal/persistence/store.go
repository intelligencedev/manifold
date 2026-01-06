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
	// ErrRevisionConflict indicates optimistic concurrency failure (stale revision).
	ErrRevisionConflict = errors.New("persistence: revision conflict")
)

// Store is a placeholder for transcripts/state persistence.
type Store interface{}

// UserPreferences represents a user's persistent settings.
type UserPreferences struct {
	UserID          int64     `json:"userId"`
	ActiveProjectID string    `json:"activeProjectId,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// UserPreferencesStore persists user-specific preferences (e.g., active project).
type UserPreferencesStore interface {
	// Init creates the table if it doesn't exist.
	Init(ctx context.Context) error
	// Get retrieves preferences for a user. Returns zero-value if not found.
	Get(ctx context.Context, userID int64) (UserPreferences, error)
	// SetActiveProject updates the user's active project selection.
	SetActiveProject(ctx context.Context, userID int64, projectID string) error
}

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

// Project represents a project stored in the database.
// This is the authoritative metadata record for projects stored in S3 or filesystem.
type Project struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"userId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Revision is an optimistic concurrency control token.
	// It must be passed to Update operations and is checked for conflicts.
	Revision int64 `json:"revision"`
	// Bytes is the total size of all files in the project (cached).
	Bytes int64 `json:"bytes"`
	// FileCount is the number of files in the project (cached).
	FileCount int `json:"fileCount"`
	// StorageBackend indicates where files are stored: "filesystem" or "s3".
	StorageBackend string `json:"storageBackend,omitempty"`
}

// ProjectFile represents a file entry in the project_files index.
// This enables fast directory listing without expensive S3 LIST operations.
type ProjectFile struct {
	ProjectID string    `json:"projectId"`
	Path      string    `json:"path"`      // Full project-relative path (e.g., "src/main.go")
	Name      string    `json:"name"`      // Basename only (e.g., "main.go")
	IsDir     bool      `json:"isDir"`     // True if this is a directory entry
	Size      int64     `json:"size"`      // File size in bytes (0 for directories)
	ModTime   time.Time `json:"modTime"`   // Last modification time
	ETag      string    `json:"etag"`      // S3 ETag or content hash for change detection
	UpdatedAt time.Time `json:"updatedAt"` // When this index entry was last updated
}

// ProjectsStore persists project metadata and optional file index.
type ProjectsStore interface {
	// Init creates tables/indexes if they don't exist.
	Init(ctx context.Context) error

	// Create inserts a new project. Returns the created project with ID and revision set.
	Create(ctx context.Context, userID int64, name string) (Project, error)

	// Get retrieves a project by ID. Returns ErrNotFound if not found.
	// Returns ErrForbidden if userID doesn't match the project owner.
	Get(ctx context.Context, userID int64, projectID string) (Project, error)

	// List returns all projects for a user, sorted by UpdatedAt desc, then Name asc.
	List(ctx context.Context, userID int64) ([]Project, error)

	// Update modifies project metadata. The project's Revision must match the current
	// database revision; otherwise ErrRevisionConflict is returned.
	// On success, the returned project has an incremented Revision.
	Update(ctx context.Context, p Project) (Project, error)

	// UpdateStats updates cached file count and byte totals.
	// This is a partial update that doesn't require revision checking.
	UpdateStats(ctx context.Context, projectID string, bytes int64, fileCount int) error

	// Delete removes a project and all associated file index entries.
	Delete(ctx context.Context, userID int64, projectID string) error

	// --- File Index Operations (optional, for fast directory listing) ---

	// IndexFile upserts a file entry in the project file index.
	IndexFile(ctx context.Context, f ProjectFile) error

	// RemoveFileIndex removes a file entry from the index.
	RemoveFileIndex(ctx context.Context, projectID, path string) error

	// RemoveFileIndexPrefix removes all file entries under a path prefix (for directory deletes).
	RemoveFileIndexPrefix(ctx context.Context, projectID, pathPrefix string) error

	// ListFiles returns file entries directly under the given path (non-recursive).
	// If path is "." or "", returns root directory entries.
	// Results are sorted: directories first, then by name.
	ListFiles(ctx context.Context, projectID, path string) ([]ProjectFile, error)

	// GetFile retrieves a single file index entry by exact path.
	GetFile(ctx context.Context, projectID, path string) (ProjectFile, error)
}
