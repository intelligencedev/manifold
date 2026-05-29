package embeddedpg

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/rs/zerolog/log"

	"manifold/internal/config"
)

const (
	defaultDatabase = "manifold"
	defaultPort     = 5433
	defaultUsername = "manifold"
)

func defaultEmbeddedPassword() string {
	return defaultUsername
}

type Runtime struct {
	database string
	db       *embeddedpostgres.EmbeddedPostgres
	dsn      string
	stopErr  error
	stopOnce sync.Once
}

func Start(dbCfg *config.DBConfig) (*Runtime, error) {
	if dbCfg == nil || !dbCfg.Embedded {
		return nil, nil
	}

	rc, err := newRuntimeConfig(*dbCfg)
	if err != nil {
		return nil, err
	}

	// Ensure PG binaries are present in the persistent binaries directory.
	// On the very first run this downloads + extracts via a bootstrap cycle.
	if err := ensureBinaries(rc); err != nil {
		return nil, err
	}

	// Install extensions (idempotent — uses per-extension stamp files).
	extensions := dbCfg.EmbeddedExtensions
	if len(extensions) == 0 {
		extensions = defaultExtensions
	}
	installed := installExtensions(
		rc.binariesPath, rc.cachePath, rc.pgMajor,
		extensions, dbCfg.EmbeddedExtensionURL,
	)

	rt := &Runtime{
		database: rc.database,
		db:       embeddedpostgres.NewDatabase(rc.embedded),
		dsn:      rc.connectionURL(),
	}
	if err := rt.db.Start(); err != nil {
		return nil, fmt.Errorf("start embedded postgres: %w", err)
	}

	dbCfg.DefaultDSN = rt.DSN()

	// Only fall back to memory vector if pgvector was not installed.
	if !installed["pgvector"] && needsEmbeddedVectorFallback(dbCfg.Vector.Backend) {
		log.Warn().Str("backend", dbCfg.Vector.Backend).
			Msg("pgvector extension not available; forcing vector backend to memory")
		dbCfg.Vector.Backend = "memory"
		dbCfg.Vector.DSN = ""
	}

	log.Info().Str("dsn", sanitizeDSN(rt.DSN())).Msg("embedded postgres started")
	return rt, nil
}

func (r *Runtime) DSN() string {
	if r == nil {
		return ""
	}
	return r.dsn
}

func (r *Runtime) Stop() error {
	if r == nil || r.db == nil {
		return nil
	}
	r.stopOnce.Do(func() {
		r.stopErr = r.db.Stop()
		if r.stopErr != nil {
			log.Warn().Err(r.stopErr).Str("database", r.database).Msg("embedded postgres stop failed")
			return
		}
		log.Info().Str("database", r.database).Msg("embedded postgres stopped")
	})
	return r.stopErr
}

type runtimeConfig struct {
	database     string
	host         string
	password     string
	port         uint32
	username     string
	pgMajor      int
	binariesPath string
	cachePath    string
	embedded     embeddedpostgres.Config
}

func newRuntimeConfig(dbCfg config.DBConfig) (runtimeConfig, error) {
	baseDir, err := resolveBaseDir(dbCfg.EmbeddedDataDir)
	if err != nil {
		return runtimeConfig{}, err
	}

	dataDir := strings.TrimSpace(dbCfg.EmbeddedDataDir)
	if dataDir == "" {
		dataDir = filepath.Join(baseDir, "data")
	}

	port := dbCfg.EmbeddedPort
	if port == 0 {
		port = defaultPort
	}

	version, err := resolveVersion(dbCfg.EmbeddedVersion)
	if err != nil {
		return runtimeConfig{}, err
	}

	host := "127.0.0.1"
	username := defaultUsername
	password := defaultEmbeddedPassword()
	database := defaultDatabase

	binariesPath := filepath.Join(baseDir, fmt.Sprintf("binaries-%s", string(version)))
	cachePath := filepath.Join(baseDir, "cache")

	return runtimeConfig{
		database:     database,
		host:         host,
		password:     password,
		port:         port,
		username:     username,
		pgMajor:      pgMajorFromVersion(version),
		binariesPath: binariesPath,
		cachePath:    cachePath,
		embedded: embeddedpostgres.DefaultConfig().
			Version(version).
			Port(port).
			Username(username).
			Password(password).
			Database(database).
			BinariesPath(binariesPath).
			RuntimePath(filepath.Join(baseDir, "runtime")).
			CachePath(cachePath).
			DataPath(dataDir).
			StartTimeout(45 * time.Second),
	}, nil
}

func (c runtimeConfig) connectionURL() string {
	return (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.username, c.password),
		Host:   net.JoinHostPort(c.host, fmt.Sprintf("%d", c.port)),
		Path:   c.database,
		RawQuery: url.Values{
			"sslmode": []string{"disable"},
		}.Encode(),
	}).String()
}

func resolveBaseDir(dataDir string) (string, error) {
	trimmed := strings.TrimSpace(dataDir)
	if trimmed != "" {
		return filepath.Dir(trimmed), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(homeDir, ".manifold", "embedded-postgres"), nil
}

func resolveVersion(raw string) (embeddedpostgres.PostgresVersion, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "18", "18.3", "18.3.0", "v18":
		return embeddedpostgres.V18, nil
	case "17", "17.5", "17.5.0", "v17":
		return embeddedpostgres.V17, nil
	case "16", "16.9", "16.9.0", "v16":
		return embeddedpostgres.V16, nil
	case "15", "15.13", "15.13.0", "v15":
		return embeddedpostgres.V15, nil
	default:
		return "", fmt.Errorf("unsupported embedded postgres version %q", raw)
	}
}

func needsEmbeddedVectorFallback(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "auto", "postgres", "pg", "pgvector":
		return true
	default:
		return false
	}
}

func sanitizeDSN(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword(parsed.User.Username(), "xxxxx")
	}
	return parsed.String()
}

// ensureBinaries guarantees that the persistent binaries directory is
// populated. On the very first run, it triggers a bootstrap Start()/Stop()
// cycle so the library downloads and extracts the PG tarball.
func ensureBinaries(rc runtimeConfig) error {
	pgCtl := filepath.Join(rc.binariesPath, "bin", "pg_ctl")
	if _, err := os.Stat(pgCtl); err == nil {
		return nil // already prepared
	}

	log.Info().Str("path", rc.binariesPath).
		Msg("first run: downloading and preparing PostgreSQL binaries...")

	bootstrap := embeddedpostgres.NewDatabase(rc.embedded)
	if err := bootstrap.Start(); err != nil {
		return fmt.Errorf("bootstrap embedded postgres: %w", err)
	}
	if err := bootstrap.Stop(); err != nil {
		log.Warn().Err(err).Msg("bootstrap shutdown warning (non-fatal)")
	}

	log.Info().Str("path", rc.binariesPath).Msg("PostgreSQL binaries prepared")
	return nil
}

// pgMajorFromVersion extracts the major version number from a PostgresVersion
// string like "18.3.0" → 18.
func pgMajorFromVersion(v embeddedpostgres.PostgresVersion) int {
	parts := strings.SplitN(string(v), ".", 2)
	if len(parts) > 0 {
		n, _ := strconv.Atoi(parts[0])
		return n
	}
	return 0
}
