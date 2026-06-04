package embeddedpg

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"manifold/internal/config"
)

// DoctorRequest configures an embedded PostgreSQL smoke test run.
type DoctorRequest struct {
	AllowExternalRuntimeResolution bool
	DiagnosticLogPath              string
}

// DoctorResult is the machine-readable result for the embedded PostgreSQL
// doctor command.
type DoctorResult struct {
	OK          bool                      `json:"ok"`
	OS          string                    `json:"os"`
	Arch        string                    `json:"arch"`
	RuntimeID   string                    `json:"runtimeID,omitempty"`
	PGMajor     int                       `json:"pgMajor,omitempty"`
	Port        uint32                    `json:"port,omitempty"`
	DataDir     string                    `json:"dataDir,omitempty"`
	Extensions  map[string]ExtensionProbe `json:"extensions,omitempty"`
	StartedAt   time.Time                 `json:"startedAt"`
	CompletedAt time.Time                 `json:"completedAt"`
	Error       string                    `json:"error,omitempty"`
}

// Doctor extracts/verifies the embedded runtime, starts PostgreSQL with a
// temporary data directory, verifies required extensions, and shuts down.
func Doctor(ctx context.Context, req DoctorRequest) (result DoctorResult) {
	result = DoctorResult{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		StartedAt: time.Now().UTC(),
	}
	defer func() {
		result.CompletedAt = time.Now().UTC()
	}()

	port, err := freeLoopbackPort()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Port = uint32(port)

	tempRoot, err := os.MkdirTemp("", "manifold-embedded-postgres-doctor-")
	if err != nil {
		result.Error = fmt.Sprintf("create temporary data directory: %v", err)
		return result
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()
	dataDir := filepath.Join(tempRoot, "data")
	result.DataDir = dataDir

	dbCfg := &config.DBConfig{
		Embedded:                               true,
		EmbeddedPort:                           uint32(port),
		EmbeddedDataDir:                        dataDir,
		EmbeddedVersion:                        defaultEmbeddedPostgresVersion,
		EmbeddedAllowExternalRuntimeResolution: req.AllowExternalRuntimeResolution,
		EmbeddedDiagnosticLogPath:              req.DiagnosticLogPath,
	}
	embeddedRuntime, err := Start(dbCfg)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.RuntimeID = embeddedRuntime.runtimeID
	result.PGMajor = embeddedRuntime.pgMajor
	result.Extensions = embeddedRuntime.extensionProbes
	if stopErr := embeddedRuntime.Stop(); stopErr != nil {
		result.Error = fmt.Sprintf("stop embedded postgres: %v", stopErr)
		return result
	}

	select {
	case <-ctx.Done():
		result.Error = ctx.Err().Error()
	default:
		result.OK = true
	}
	return result
}

func freeLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %T", listener.Addr())
	}
	return addr.Port, nil
}
