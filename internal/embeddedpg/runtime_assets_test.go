package embeddedpg

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	pgassets "manifold/internal/embeddedpg/assets"
)

func TestPrepareRuntimeAssetExtractsAndVerifiesZip(t *testing.T) {
	t.Parallel()

	archive, digest := testRuntimeZip(t, "bin/pg_ctl", "pg-ctl")
	asset := pgassets.RuntimeAsset{
		RuntimeID:   "postgres-17.5-test-linux-amd64",
		PGMajor:     17,
		ArchiveName: "postgres-runtime-test.zip",
		Archive:     archive,
		Manifest: pgassets.Manifest{
			RuntimeID: "postgres-17.5-test-linux-amd64",
			Postgres: pgassets.PostgresManifest{
				Major:   17,
				Version: "17.5",
			},
			Files: []pgassets.FileManifest{{
				Path:   "bin/pg_ctl",
				Mode:   "0755",
				SHA256: digest,
			}},
		},
	}

	prepared, err := prepareRuntimeAsset(asset, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, asset.RuntimeID, prepared.runtimeID)
	require.Equal(t, 17, prepared.pgMajor)
	require.Equal(t, "17.5", prepared.version)
	assertFileContains(t, filepath.Join(prepared.root, "bin", "pg_ctl"), "pg-ctl")
}

func TestVerifyRuntimeFilesRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bin", "postgres"), []byte("postgres"), 0755))

	err := verifyRuntimeFiles(root, pgassets.Manifest{
		Files: []pgassets.FileManifest{{
			Path:   "bin/postgres",
			Mode:   "0755",
			SHA256: "bad",
		}},
	})
	require.ErrorContains(t, err, "checksum mismatch")
}

func TestSafeRuntimePathRejectsTraversal(t *testing.T) {
	t.Parallel()

	_, err := safeRuntimePath("/tmp/runtime", "../escape")
	require.Error(t, err)

	_, err = safeRuntimePath("/tmp/runtime", "/absolute")
	require.Error(t, err)
}

func testRuntimeZip(t *testing.T, filePath, content string) ([]byte, string) {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{Name: filePath, Method: zip.Deflate}
	hdr.SetMode(0755)
	w, err := zw.CreateHeader(hdr)
	require.NoError(t, err)
	_, err = w.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	sum := sha256.Sum256([]byte(content))
	return buf.Bytes(), hex.EncodeToString(sum[:])
}
