package embeddedpg

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"manifold/internal/config"
)

func TestStartDisabled(t *testing.T) {
	t.Parallel()

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()

		runtime, err := Start(nil)
		require.NoError(t, err)
		require.Nil(t, runtime)
	})

	t.Run("embedded disabled", func(t *testing.T) {
		t.Parallel()

		runtime, err := Start(&config.DBConfig{})
		require.NoError(t, err)
		require.Nil(t, runtime)
	})
}

func TestRuntimeStopNil(t *testing.T) {
	t.Parallel()

	var runtime *Runtime
	require.NoError(t, runtime.Stop())
}

// ---------------------------------------------------------------------------
// Extension unit tests
// ---------------------------------------------------------------------------

func TestPlatformIdentifiers(t *testing.T) {
	t.Parallel()
	osName, archName := platformIdentifiers()
	require.NotEmpty(t, osName)
	require.NotEmpty(t, archName)
}

func TestMapExtensionPath(t *testing.T) {
	t.Parallel()
	base := "/pg"

	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"lib/vector.dylib", "/pg/lib/postgresql/vector.dylib", true},
		{"deps/libgeos.dylib", "/pg/lib/libgeos.dylib", true},
		{"share/extension/vector.control", "/pg/share/postgresql/extension/vector.control", true},
		{"./lib/foo.so", "/pg/lib/postgresql/foo.so", true},
		{"unknown/file.txt", "", false},
		{"bin/postgres", "", false},
	}

	for _, tt := range tests {
		got, ok := mapExtensionPath(tt.input, base)
		require.Equal(t, tt.ok, ok, "path=%s", tt.input)
		if ok {
			require.Equal(t, tt.want, got, "path=%s", tt.input)
		}
	}
}

func TestExtractExtensionPackage(t *testing.T) {
	t.Parallel()

	// Create a test tar.gz with the expected layout.
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test-ext.tar.gz")
	binariesPath := filepath.Join(tmpDir, "binaries")

	// Pre-create the target directories (as they would exist after PG extraction).
	require.NoError(t, os.MkdirAll(filepath.Join(binariesPath, "lib", "postgresql"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(binariesPath, "share", "postgresql", "extension"), 0755))

	// Build test archive.
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	addFile := func(name, content string) {
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	addFile("lib/vector.dylib", "fake-vector-lib")
	addFile("share/extension/vector.control", "# vector control")
	addFile("share/extension/vector--0.8.0.sql", "CREATE FUNCTION vector();")
	addFile("deps/libhelper.dylib", "fake-dep-lib")

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	require.NoError(t, f.Close())

	// Extract.
	require.NoError(t, extractExtensionPackage(archivePath, binariesPath))

	// Verify files landed in the correct places.
	assertFileContains(t, filepath.Join(binariesPath, "lib", "postgresql", "vector.dylib"), "fake-vector-lib")
	assertFileContains(t, filepath.Join(binariesPath, "share", "postgresql", "extension", "vector.control"), "# vector control")
	assertFileContains(t, filepath.Join(binariesPath, "share", "postgresql", "extension", "vector--0.8.0.sql"), "CREATE FUNCTION vector();")
	assertFileContains(t, filepath.Join(binariesPath, "lib", "libhelper.dylib"), "fake-dep-lib")
}

func TestExtensionStampSkip(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	binariesPath := filepath.Join(tmpDir, "binaries")
	cachePath := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(binariesPath, 0755))

	// Pre-create stamp file for pgvector.
	version, ok := extensionVersion("pgvector", 17)
	require.True(t, ok)
	stamp := filepath.Join(binariesPath, ".ext-pgvector-"+version)
	require.NoError(t, os.WriteFile(stamp, []byte(version+"\n"), 0644))

	// installExtensions should detect the stamp and skip without any download/copy.
	installed := installExtensions(binariesPath, cachePath, 17, []string{"pgvector"}, "http://localhost:1/nope")
	require.True(t, installed["pgvector"])
}

func TestPgMajorFromVersion(t *testing.T) {
	t.Parallel()
	require.Equal(t, 18, pgMajorFromVersion("18.3.0"))
	require.Equal(t, 17, pgMajorFromVersion("17.5.0"))
	require.Equal(t, 0, pgMajorFromVersion(""))
}

func TestResolveVersionDefaultsToPostgres17(t *testing.T) {
	t.Parallel()

	version, err := resolveVersion("")
	require.NoError(t, err)
	require.Equal(t, 17, pgMajorFromVersion(version))
}

func TestStartEmbeddedRequiresRuntimeAssetOrDevFallback(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	runtime, err := Start(&config.DBConfig{
		Embedded:        true,
		EmbeddedDataDir: filepath.Join(tmpDir, "data"),
	})
	require.ErrorContains(t, err, "no embedded PostgreSQL runtime is bundled")
	require.Nil(t, runtime)
}

func assertFileContains(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	require.Equal(t, expected, string(data))
}

// ---------------------------------------------------------------------------
// Full integration test (env-gated)
// ---------------------------------------------------------------------------

func TestStartEmbeddedIntegration(t *testing.T) {
	if os.Getenv("MANIFOLD_TEST_EMBEDDED_POSTGRES") == "" {
		t.Skip("set MANIFOLD_TEST_EMBEDDED_POSTGRES=1 to run embedded postgres integration test")
	}

	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dbCfg := &config.DBConfig{
		Embedded:                               true,
		EmbeddedPort:                           15439,
		EmbeddedDataDir:                        dataDir,
		EmbeddedVersion:                        "17",
		EmbeddedExtensions:                     defaultExtensions,
		EmbeddedAllowExternalRuntimeResolution: true,
		Vector: config.VectorConfig{
			Backend: "postgres",
		},
	}

	runtime, err := Start(dbCfg)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	t.Cleanup(func() {
		require.NoError(t, runtime.Stop())
	})

	require.NotEmpty(t, dbCfg.DefaultDSN)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbCfg.DefaultDSN)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))

	_, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS embedded_pg_smoke (id INT PRIMARY KEY)`)
	require.NoError(t, err)
}
