package workspaces

import (
	"bytes"
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

func TestEnterpriseWorkspaceManager_SetDecrypterEncryptsCommit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := objectstore.NewMemoryStore()
	ctx := context.Background()

	_, err := store.Put(ctx, "workspaces/users/123/projects/test-project/files/README.md", bytes.NewReader([]byte("# Test Project")), objectstore.PutOptions{})
	require.NoError(t, err)

	cfg := &config.Config{
		Workdir: tmpDir,
		Projects: config.ProjectsConfig{
			Backend: "s3",
			Workspace: config.WorkspaceConfig{
				Mode: "enterprise",
				Root: filepath.Join(tmpDir, "sandboxes"),
			},
			S3: config.S3Config{
				Prefix: "workspaces",
			},
		},
	}

	mgr := NewEnterpriseManager(cfg, store)
	ent, ok := mgr.(*EnterpriseWorkspaceManager)
	require.True(t, ok)

	cryptor := &fakeCryptor{}
	ent.SetDecrypter(cryptor)

	ws, err := ent.Checkout(ctx, 123, "test-project", "session-1")
	require.NoError(t, err)

	readmePath := filepath.Join(ws.BaseDir, "README.md")
	err = os.WriteFile(readmePath, []byte("# Updated Project"), 0o644)
	require.NoError(t, err)

	err = ent.Commit(ctx, ws)
	require.NoError(t, err)
	assert.NotZero(t, cryptor.encryptCalls)

	reader, attrs, err := store.Get(ctx, "workspaces/users/123/projects/test-project/files/README.md")
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "application/octet-stream", attrs.ContentType)
	assert.Equal(t, "enc:# Updated Project", string(data))
}
