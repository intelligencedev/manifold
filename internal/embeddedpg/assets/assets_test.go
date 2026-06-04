package assets

import (
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentNoRuntimeAsset(t *testing.T) {
	t.Parallel()

	asset, err := Current()
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoRuntimeAsset))
	require.Empty(t, asset.RuntimeID)
}

func TestValidateCurrentManifest(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		SchemaVersion: 1,
		RuntimeID:     "postgres-17.5-test",
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Postgres: PostgresManifest{
			Major:   17,
			Version: "17.5",
		},
		Files: []FileManifest{{
			Path:   "bin/postgres",
			Mode:   "0755",
			SHA256: "abc123",
		}},
	}
	require.NoError(t, validateCurrentManifest(manifest))

	manifest.OS = "not-this-os"
	require.Error(t, validateCurrentManifest(manifest))
}
