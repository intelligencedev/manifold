package observability

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog/log"
)

// resetLoggerGlobals clears package-level logger state so each test starts clean.
func resetLoggerGlobals(t *testing.T, console io.Writer) {
	t.Helper()
	prevConsole := consoleWriter
	prevCurrent, prevSide, prevOtel := currentLogWriter, sideLogWriter, otelLogWriter
	prevOtelEnabled := otelWriterEnabled
	consoleWriter = console
	currentLogWriter, sideLogWriter, otelLogWriter = nil, nil, nil
	otelWriterEnabled = false
	t.Cleanup(func() {
		consoleWriter = prevConsole
		currentLogWriter, sideLogWriter, otelLogWriter = prevCurrent, prevSide, prevOtel
		otelWriterEnabled = prevOtelEnabled
	})
}

// The MANIFOLD_LOG_STDOUT console tee must keep working even after a component
// registers its own side-channel writer via AddLogWriter (as app init does).
func TestConsoleTeeSurvivesAddLogWriter(t *testing.T) {
	var console bytes.Buffer
	resetLoggerGlobals(t, &console)
	t.Setenv(EnvLogStdout, "1")

	logFile := filepath.Join(t.TempDir(), "manifold.log")
	InitLogger(logFile, "info")

	// A later subsystem attaches its own side writer (matches app_init_services).
	AddLogWriter(&bytes.Buffer{})

	log.Info().Msg("line_after_add_log_writer")

	if !bytes.Contains(console.Bytes(), []byte("line_after_add_log_writer")) {
		t.Fatalf("console tee lost after AddLogWriter; console got: %q", console.String())
	}
}
