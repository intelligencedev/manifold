// Package workspaces manages ephemeral and legacy workspace lifecycles for agent runs.
// It abstracts the mapping between project IDs and filesystem paths used by tools.
package workspaces

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"manifold/internal/config"
	"manifold/internal/objectstore"
	"manifold/internal/validation"
)

// ErrInvalidProjectID indicates the project_id value is malformed or attempts path traversal.
var ErrInvalidProjectID = validation.ErrInvalidProjectID

// ErrInvalidSessionID indicates the session_id value is malformed or attempts path traversal.
var ErrInvalidSessionID = validation.ErrInvalidSessionID

// ErrProjectNotFound indicates the requested project does not exist.
var ErrProjectNotFound = errors.New("project not found")

// Workspace represents a checked-out working directory for an agent run.
type Workspace struct {
	// UserID is the owning user's identifier.
	UserID int64
	// ProjectID is the logical project identifier.
	ProjectID string
	// SessionID is the chat session identifier (for per-session workspaces).
	SessionID string
	// BaseDir is the local filesystem path where tools operate.
	BaseDir string
	// Mode indicates whether this is a "legacy" or "ephemeral" workspace.
	Mode string
}

// WorkspaceManager abstracts workspace checkout, commit, and cleanup operations.
// Different implementations support legacy (direct project dir) and ephemeral
// (per-session copy) strategies.
type WorkspaceManager interface {
	// Checkout prepares a workspace for the given user, project, and session.
	// For legacy mode, this returns the existing project directory.
	// For ephemeral mode, this creates a working copy.
	Checkout(ctx context.Context, userID int64, projectID, sessionID string) (Workspace, error)

	// Commit persists any changes from the workspace back to durable storage.
	// For legacy mode, this is a no-op since changes are already on disk.
	// For ephemeral mode, this syncs changes to S3/object storage.
	Commit(ctx context.Context, ws Workspace) error

	// Cleanup removes ephemeral workspace resources.
	// For legacy mode, this is a no-op.
	// For ephemeral mode, this removes the temporary directory.
	Cleanup(ctx context.Context, ws Workspace) error

	// Mode returns the workspace mode ("legacy" or "ephemeral").
	Mode() string
}

// NewManager creates a WorkspaceManager based on configuration.
// Returns LegacyWorkspaceManager for "legacy" mode (default) and
// EphemeralWorkspaceManager for "ephemeral" mode when S3 is configured.
func NewManager(cfg *config.Config) WorkspaceManager {
	mode := cfg.Projects.Workspace.Mode
	if mode == "" {
		mode = "legacy"
	}

	switch mode {
	case "enterprise":
		return NewEnterpriseManager(cfg, nil)
	case "ephemeral":
		// Ephemeral mode requires S3 backend configuration
		// The actual S3 store is created separately and injected via NewManagerWithStore
		// Fall back to legacy for now; use NewManagerWithStore for full ephemeral support
		return &LegacyWorkspaceManager{
			workdir: cfg.Workdir,
			mode:    "legacy",
		}
	default:
		return &LegacyWorkspaceManager{
			workdir: cfg.Workdir,
			mode:    "legacy",
		}
	}
}

// NewManagerWithStore creates a WorkspaceManager with an injected object store.
// This is used for ephemeral workspaces backed by S3 storage.
// When an object store is provided, ephemeral mode is always used regardless of
// the configured workspace mode, since the legacy manager cannot verify projects
// that exist only in S3.
func NewManagerWithStore(cfg *config.Config, store objectstore.ObjectStore) WorkspaceManager {
	if store == nil {
		// Can't do ephemeral without a store, fall back to legacy
		return &LegacyWorkspaceManager{
			workdir: cfg.Workdir,
			mode:    "legacy",
		}
	}

	mode := cfg.Projects.Workspace.Mode
	if mode == "enterprise" {
		return NewEnterpriseManager(cfg, store)
	}
	// Default: ephemeral mode when S3 store is provided
	return NewEphemeralManager(store, cfg)
}

// LegacyWorkspaceManager implements WorkspaceManager using direct project directories.
// Checkout returns the existing project path; Commit and Cleanup are no-ops.
type LegacyWorkspaceManager struct {
	workdir string
	mode    string
}

// NewLegacyManager creates a LegacyWorkspaceManager with the given workdir.
func NewLegacyManager(workdir string) *LegacyWorkspaceManager {
	return &LegacyWorkspaceManager{
		workdir: workdir,
		mode:    "legacy",
	}
}

// Mode returns "legacy".
func (m *LegacyWorkspaceManager) Mode() string {
	return m.mode
}

// Checkout validates the project ID and returns the project directory as the workspace.
// It performs strict path traversal checks to prevent sandbox escapes.
func (m *LegacyWorkspaceManager) Checkout(ctx context.Context, userID int64, projectID, sessionID string) (Workspace, error) {
	ws := Workspace{
		UserID:    userID,
		ProjectID: projectID,
		SessionID: sessionID,
		Mode:      m.mode,
	}

	// Empty project ID means no workspace scoping
	if projectID == "" {
		return ws, nil
	}

	cleanPID, err := ValidateProjectID(projectID)
	if err != nil {
		return Workspace{}, err
	}

	// Build and validate the project path
	baseRoot := filepath.Join(m.workdir, "users", fmt.Sprint(userID), "projects")
	base := filepath.Join(baseRoot, cleanPID)

	// Get absolute paths for comparison
	absBaseRoot, err := filepath.Abs(baseRoot)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve base root: %w", err)
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve base: %w", err)
	}

	// Ensure the resolved path is within the projects directory
	relBase, err := filepath.Rel(absBaseRoot, absBase)
	if err != nil || relBase == "." || strings.HasPrefix(relBase, ".."+string(os.PathSeparator)) || relBase == ".." {
		return Workspace{}, ErrInvalidProjectID
	}

	// Verify the project directory exists
	st, err := os.Stat(absBase)
	if err != nil || !st.IsDir() {
		return Workspace{}, ErrProjectNotFound
	}

	ws.BaseDir = absBase
	return ws, nil
}

// Commit is a no-op for legacy workspaces since changes are written directly to disk.
func (m *LegacyWorkspaceManager) Commit(ctx context.Context, ws Workspace) error {
	return nil
}

// Cleanup is a no-op for legacy workspaces.
func (m *LegacyWorkspaceManager) Cleanup(ctx context.Context, ws Workspace) error {
	return nil
}

// ValidateProjectID checks if a project ID is safe for use in filesystem paths.
// Returns cleaned project ID and error if validation fails.
// Deprecated: Use validation.ProjectID directly for new code.
func ValidateProjectID(projectID string) (string, error) {
	return validation.ProjectID(projectID)
}

// ValidateSessionID checks if a session ID is safe for use as a single filesystem path segment.
// Deprecated: Use validation.SessionID directly for new code.
func ValidateSessionID(sessionID string) (string, error) {
	return validation.SessionID(sessionID)
}
