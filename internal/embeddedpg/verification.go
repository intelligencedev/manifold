package embeddedpg

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var requiredSQLExtensions = []string{"vector", "postgis", "pgrouting"}

type ExtensionProbe struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Detail  string `json:"detail,omitempty"`
}

type extensionVerificationContext struct {
	RuntimeID         string
	DiagnosticLogPath string
}

type extensionVerificationError struct {
	Extension         string
	RuntimeID         string
	DiagnosticLogPath string
	Err               error
}

func (e extensionVerificationError) Error() string {
	logPath := strings.TrimSpace(e.DiagnosticLogPath)
	if logPath == "" {
		logPath = "not configured"
	}
	runtimeID := strings.TrimSpace(e.RuntimeID)
	if runtimeID == "" {
		runtimeID = "unknown"
	}
	return fmt.Sprintf(
		"embedded PostgreSQL runtime verification failed for extension %q on %s/%s with runtime %q: %v. Diagnostic log: %s. Please report this issue with a Manifold support bundle.",
		e.Extension, runtime.GOOS, runtime.GOARCH, runtimeID, e.Err, logPath,
	)
}

func (e extensionVerificationError) Unwrap() error {
	return e.Err
}

func verifyRequiredExtensions(ctx context.Context, dsn string, req extensionVerificationContext) (map[string]ExtensionProbe, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("embedded postgres DSN is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to embedded postgres for extension verification: %w", err)
	}
	defer pool.Close()

	for _, name := range requiredSQLExtensions {
		if _, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+name); err != nil {
			return nil, extensionVerificationError{
				Extension:         name,
				RuntimeID:         req.RuntimeID,
				DiagnosticLogPath: req.DiagnosticLogPath,
				Err:               err,
			}
		}
	}

	probes := make(map[string]ExtensionProbe, len(requiredSQLExtensions))
	rows, err := pool.Query(ctx, `SELECT extname, extversion FROM pg_extension WHERE extname = ANY($1)`, requiredSQLExtensions)
	if err != nil {
		return nil, fmt.Errorf("query embedded postgres extensions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, version string
		if err := rows.Scan(&name, &version); err != nil {
			return nil, fmt.Errorf("scan embedded postgres extension: %w", err)
		}
		probes[name] = ExtensionProbe{Name: name, Version: version}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read embedded postgres extensions: %w", err)
	}
	for _, name := range requiredSQLExtensions {
		if _, ok := probes[name]; !ok {
			return nil, extensionVerificationError{
				Extension:         name,
				RuntimeID:         req.RuntimeID,
				DiagnosticLogPath: req.DiagnosticLogPath,
				Err:               fmt.Errorf("extension was not present after CREATE EXTENSION"),
			}
		}
	}

	var postgisDetail string
	if err := pool.QueryRow(ctx, `SELECT postgis_full_version()`).Scan(&postgisDetail); err != nil {
		return nil, extensionVerificationError{
			Extension:         "postgis",
			RuntimeID:         req.RuntimeID,
			DiagnosticLogPath: req.DiagnosticLogPath,
			Err:               err,
		}
	}
	postgisProbe := probes["postgis"]
	postgisProbe.Detail = postgisDetail
	probes["postgis"] = postgisProbe

	var pgroutingDetail string
	if err := pool.QueryRow(ctx, `SELECT pgr_version()`).Scan(&pgroutingDetail); err != nil {
		return nil, extensionVerificationError{
			Extension:         "pgrouting",
			RuntimeID:         req.RuntimeID,
			DiagnosticLogPath: req.DiagnosticLogPath,
			Err:               err,
		}
	}
	pgroutingProbe := probes["pgrouting"]
	pgroutingProbe.Detail = pgroutingDetail
	probes["pgrouting"] = pgroutingProbe

	return probes, nil
}
