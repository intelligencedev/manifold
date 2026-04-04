package embeddedpg

import (
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

func TestStartEmbeddedIntegration(t *testing.T) {
	if os.Getenv("MANIFOLD_TEST_EMBEDDED_POSTGRES") == "" {
		t.Skip("set MANIFOLD_TEST_EMBEDDED_POSTGRES=1 to run embedded postgres integration test")
	}

	dataDir := filepath.Join(t.TempDir(), "data")
	dbCfg := &config.DBConfig{
		Embedded:        true,
		EmbeddedPort:    15439,
		EmbeddedDataDir: dataDir,
		EmbeddedVersion: "18",
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

	require.Equal(t, "memory", dbCfg.Vector.Backend)
	require.Empty(t, dbCfg.Vector.DSN)
	require.NotEmpty(t, dbCfg.DefaultDSN)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbCfg.DefaultDSN)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))

	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS embedded_pg_smoke (id INT PRIMARY KEY)`)
	require.NoError(t, err)
}
