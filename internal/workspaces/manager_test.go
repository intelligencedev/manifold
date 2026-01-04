package workspaces

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"manifold/internal/config"
	"manifold/internal/objectstore"
)

// mockObjectStore implements objectstore.ObjectStore for testing.
type mockObjectStore struct{}

func (m *mockObjectStore) Get(ctx context.Context, key string) (io.ReadCloser, objectstore.ObjectAttrs, error) {
	return nil, objectstore.ObjectAttrs{}, objectstore.ErrNotFound
}

func (m *mockObjectStore) Put(ctx context.Context, key string, r io.Reader, opts objectstore.PutOptions) (string, error) {
	return "", nil
}

func (m *mockObjectStore) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockObjectStore) List(ctx context.Context, opts objectstore.ListOptions) (objectstore.ListResult, error) {
	return objectstore.ListResult{}, nil
}

func (m *mockObjectStore) Head(ctx context.Context, key string) (objectstore.ObjectAttrs, error) {
	return objectstore.ObjectAttrs{}, objectstore.ErrNotFound
}

func (m *mockObjectStore) Copy(ctx context.Context, srcKey, dstKey string) error {
	return nil
}

func (m *mockObjectStore) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func TestNewManager_LegacyMode(t *testing.T) {
	cfg := &config.Config{
		Workdir: "/tmp/test-workdir",
		Projects: config.ProjectsConfig{
			Workspace: config.WorkspaceConfig{
				Mode: "legacy",
			},
		},
	}

	mgr := NewManager(cfg)
	assert.NotNil(t, mgr)
	assert.Equal(t, "legacy", mgr.Mode())
}

func TestNewManager_DefaultMode(t *testing.T) {
	cfg := &config.Config{
		Workdir: "/tmp/test-workdir",
		Projects: config.ProjectsConfig{
			Workspace: config.WorkspaceConfig{
				Mode: "", // empty defaults to legacy
			},
		},
	}

	mgr := NewManager(cfg)
	assert.NotNil(t, mgr)
	assert.Equal(t, "legacy", mgr.Mode())
}

func TestNewManager_EphemeralMode_FallsBackToLegacy(t *testing.T) {
	// Ephemeral mode is not implemented yet, should fall back to legacy
	cfg := &config.Config{
		Workdir: "/tmp/test-workdir",
		Projects: config.ProjectsConfig{
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
			},
		},
	}

	mgr := NewManager(cfg)
	assert.NotNil(t, mgr)
	assert.Equal(t, "legacy", mgr.Mode())
}

func TestNewManagerWithStore_NilStore_FallsBackToLegacy(t *testing.T) {
	cfg := &config.Config{
		Workdir: "/tmp/test-workdir",
		Projects: config.ProjectsConfig{
			Workspace: config.WorkspaceConfig{
				Mode: "ephemeral",
			},
		},
	}

	mgr := NewManagerWithStore(cfg, nil)
	assert.NotNil(t, mgr)
	assert.Equal(t, "legacy", mgr.Mode())
}

func TestNewManagerWithStore_WithStore_AlwaysEphemeral(t *testing.T) {
	// When an S3 store is provided, the workspace manager should always use
	// ephemeral mode regardless of the configured workspace mode. This is
	// because the legacy manager checks for local filesystem directories
	// that don't exist when projects are stored in S3.
	cfg := &config.Config{
		Workdir: t.TempDir(),
		Projects: config.ProjectsConfig{
			Workspace: config.WorkspaceConfig{
				Mode: "legacy", // explicitly set to legacy
			},
		},
	}

	// Use a mock store to simulate S3 being configured
	store := &mockObjectStore{}

	mgr := NewManagerWithStore(cfg, store)
	assert.NotNil(t, mgr)
	assert.Equal(t, "ephemeral", mgr.Mode(), "S3-backed workspace manager should always use ephemeral mode")
}
func TestLegacyWorkspaceManager_Checkout_EmptyProjectID(t *testing.T) {
	mgr := NewLegacyManager("/tmp/test-workdir")

	ws, err := mgr.Checkout(context.Background(), 123, "", "session-1")
	require.NoError(t, err)
	assert.Equal(t, int64(123), ws.UserID)
	assert.Equal(t, "", ws.ProjectID)
	assert.Equal(t, "session-1", ws.SessionID)
	assert.Equal(t, "", ws.BaseDir)
	assert.Equal(t, "legacy", ws.Mode)
}

func TestLegacyWorkspaceManager_Checkout_InvalidProjectID(t *testing.T) {
	mgr := NewLegacyManager("/tmp/test-workdir")

	tests := []struct {
		name      string
		projectID string
	}{
		{"path traversal with ..", "../escape"},
		{"path traversal in middle", "foo/../bar"},
		{"absolute path unix", "/etc/passwd"},
		{"double dots", ".."},
		{"hidden traversal", "foo/.."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mgr.Checkout(context.Background(), 123, tt.projectID, "session-1")
			assert.ErrorIs(t, err, ErrInvalidProjectID)
		})
	}
}

func TestLegacyWorkspaceManager_Checkout_ProjectNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewLegacyManager(tmpDir)

	// Create the users/123/projects directory but not the project itself
	projectsDir := filepath.Join(tmpDir, "users", "123", "projects")
	require.NoError(t, os.MkdirAll(projectsDir, 0755))

	_, err := mgr.Checkout(context.Background(), 123, "nonexistent-project", "session-1")
	assert.ErrorIs(t, err, ErrProjectNotFound)
}

func TestLegacyWorkspaceManager_Checkout_Success(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewLegacyManager(tmpDir)

	// Create the project directory structure
	projectDir := filepath.Join(tmpDir, "users", "42", "projects", "my-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	ws, err := mgr.Checkout(context.Background(), 42, "my-project", "session-xyz")
	require.NoError(t, err)

	assert.Equal(t, int64(42), ws.UserID)
	assert.Equal(t, "my-project", ws.ProjectID)
	assert.Equal(t, "session-xyz", ws.SessionID)
	assert.Equal(t, "legacy", ws.Mode)

	// BaseDir should be the absolute path to the project
	absProjectDir, _ := filepath.Abs(projectDir)
	assert.Equal(t, absProjectDir, ws.BaseDir)
}

func TestLegacyWorkspaceManager_Checkout_WithUserID0(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewLegacyManager(tmpDir)

	// Create project for system user (ID 0)
	projectDir := filepath.Join(tmpDir, "users", "0", "projects", "system-project")
	require.NoError(t, os.MkdirAll(projectDir, 0755))

	ws, err := mgr.Checkout(context.Background(), 0, "system-project", "")
	require.NoError(t, err)

	assert.Equal(t, int64(0), ws.UserID)
	assert.Equal(t, "system-project", ws.ProjectID)
	assert.NotEmpty(t, ws.BaseDir)
}

func TestLegacyWorkspaceManager_Commit_Noop(t *testing.T) {
	mgr := NewLegacyManager("/tmp/test-workdir")

	ws := Workspace{
		UserID:    123,
		ProjectID: "test-project",
		SessionID: "session-1",
		BaseDir:   "/tmp/test-workdir/users/123/projects/test-project",
		Mode:      "legacy",
	}

	// Commit should be a no-op and return nil
	err := mgr.Commit(context.Background(), ws)
	assert.NoError(t, err)
}

func TestLegacyWorkspaceManager_Cleanup_Noop(t *testing.T) {
	mgr := NewLegacyManager("/tmp/test-workdir")

	ws := Workspace{
		UserID:    123,
		ProjectID: "test-project",
		SessionID: "session-1",
		BaseDir:   "/tmp/test-workdir/users/123/projects/test-project",
		Mode:      "legacy",
	}

	// Cleanup should be a no-op and return nil
	err := mgr.Cleanup(context.Background(), ws)
	assert.NoError(t, err)
}

func TestValidateProjectID(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		want      string
		wantErr   error
	}{
		{"empty string", "", "", nil},
		{"simple id", "my-project", "my-project", nil},
		{"uuid style", "abc123-def456", "abc123-def456", nil},
		{"with numbers", "project123", "project123", nil},
		{"path separators", "a/b", "", ErrInvalidProjectID},
		{"windows separators", `a\\b`, "", ErrInvalidProjectID},
		{"path traversal", "../escape", "", ErrInvalidProjectID},
		{"absolute path", "/etc/passwd", "", ErrInvalidProjectID},
		{"double dots", "..", "", ErrInvalidProjectID},
		{"hidden traversal", "foo/..", "", ErrInvalidProjectID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateProjectID(tt.projectID)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestValidateSessionID(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		want      string
		wantErr   error
	}{
		{"empty string", "", "", nil},
		{"simple", "session-1", "session-1", nil},
		{"generated style", "ses-123", "ses-123", nil},
		{"path separators", "a/b", "", ErrInvalidSessionID},
		{"windows separators", `a\\b`, "", ErrInvalidSessionID},
		{"path traversal", "../escape", "", ErrInvalidSessionID},
		{"absolute path", "/etc/passwd", "", ErrInvalidSessionID},
		{"double dots", "..", "", ErrInvalidSessionID},
		{"dot", ".", "", ErrInvalidSessionID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateSessionID(tt.sessionID)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLegacyWorkspaceManager_Mode(t *testing.T) {
	mgr := NewLegacyManager("/tmp/workdir")
	assert.Equal(t, "legacy", mgr.Mode())
}
